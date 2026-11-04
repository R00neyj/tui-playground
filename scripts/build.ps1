# 터미널 플레이그라운드 교차 빌드 스크립트 (F-05, F-06)
# windows/amd64, linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 실행 파일을 dist/ 아래에 만듦.
# 사용: powershell -File scripts/build.ps1   (프로젝트 루트 어디서 불러도 됨)
# 필요한 것: PATH 에 go. 실행 파일은 Go 설치 없이 돌아감 (CGO 끔, 정적 빌드).
# 서명하지 않은 exe 는 Windows SmartScreen 경고가 뜰 수 있음.
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $root 'dist'

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Error 'go 를 찾을 수 없음. Go 를 설치했거나 PATH 에 추가했는지 확인할 것.'
    exit 1
}

$targets = @(
    @{ OS = 'windows'; Arch = 'amd64' },
    @{ OS = 'linux';   Arch = 'amd64' },
    @{ OS = 'linux';   Arch = 'arm64' },
    @{ OS = 'darwin';  Arch = 'amd64' },
    @{ OS = 'darwin';  Arch = 'arm64' }
)

New-Item -ItemType Directory -Force -Path $dist | Out-Null

$saved = @{ GOOS = $env:GOOS; GOARCH = $env:GOARCH; CGO_ENABLED = $env:CGO_ENABLED }
$failed = @()
Push-Location $root
try {
    foreach ($t in $targets) {
        $name = "TUI-playground-$($t.OS)-$($t.Arch)"
        if ($t.OS -eq 'windows') { $name += '.exe' }
        $out = Join-Path $dist $name
        if (Test-Path $out) { Remove-Item -Force $out }

        $env:GOOS = $t.OS
        $env:GOARCH = $t.Arch
        $env:CGO_ENABLED = '0'
        Write-Host "빌드: $($t.OS)/$($t.Arch) -> dist/$name"
        & go build -ldflags '-s -w' -o $out .
        if ($LASTEXITCODE -ne 0 -or -not (Test-Path $out)) {
            Write-Warning "실패: $($t.OS)/$($t.Arch)"
            $failed += "$($t.OS)/$($t.Arch)"
        }
    }
}
finally {
    # 호출한 셸의 환경 변수를 원래대로 돌려놓음
    foreach ($k in $saved.Keys) {
        if ($null -eq $saved[$k]) { Remove-Item "Env:$k" -ErrorAction SilentlyContinue }
        else { Set-Item "Env:$k" $saved[$k] }
    }
    Pop-Location
}

Write-Host ''
Get-ChildItem $dist -File | Sort-Object Name | ForEach-Object {
    Write-Host ('{0,-40} {1,8:N1} MB' -f $_.Name, ($_.Length / 1MB))
}

if ($failed.Count -gt 0) {
    Write-Error ('빌드 실패: ' + ($failed -join ', '))
    exit 1
}
Write-Host '완료: 5개 실행 파일을 dist/ 에 만들었음'
