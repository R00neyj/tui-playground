@echo off
set "SELF=%~f0"
powershell -NoProfile -Command "$s=[IO.File]::ReadAllText('%SELF:'=''%',[Text.Encoding]::UTF8); iex $s.Substring($s.IndexOf('#'+'PS#'))"
if errorlevel 1 pause
exit /b
#PS#
$script:oldEnc=[Console]::OutputEncoding
[Console]::OutputEncoding=[Text.Encoding]::UTF8
$Host.UI.RawUI.WindowTitle='Shutdown Timer'
$state="$env:LOCALAPPDATA\shutdown_timer.state"
$script:skip=$false
$script:cur=$null
$script:boot=''
$script:msg=''
$script:msgColor='Gray'

function W([string]$s){
  $n=0
  foreach($c in $s.ToCharArray()){
    $o=[int]$c
    if(($o -ge 0x1100 -and $o -le 0x11FF) -or ($o -ge 0x3130 -and $o -le 0x318F) -or ($o -ge 0xAC00 -and $o -le 0xD7A3) -or ($o -ge 0xFF00 -and $o -le 0xFF60)){$n+=2}else{$n++}
  }
  $n
}
function PadW([string]$s,[int]$w){ $s+(' '*[Math]::Max(0,$w-(W $s))) }
function T([string]$t,$fg='Gray',$bg=$null){
  if($bg){[Console]::BackgroundColor=$bg}
  [Console]::ForegroundColor=$fg
  [Console]::Write($t)
  [Console]::ResetColor()
}
function EOL {
  $pad=[Console]::WindowWidth-1-[Console]::CursorLeft
  if($pad -gt 0){[Console]::Write(' '*$pad)}
  [Console]::WriteLine()
}
function Fmt([timespan]$t){
  if($t.TotalSeconds -lt 0){$t=[timespan]::Zero}
  '{0:00}:{1:00}:{2:00}' -f [int][Math]::Floor($t.TotalHours),$t.Minutes,$t.Seconds
}
function Wait-Ms([int]$ms){
  if($script:skip){return}
  if([Console]::KeyAvailable){[void][Console]::ReadKey($true);$script:skip=$true}else{Start-Sleep -Milliseconds $ms}
}
function Say([string]$m,[string]$c){ $script:msg=$m; $script:msgColor=$c }

function Get-Timer {
  if(-not (Test-Path $state)){return $null}
  $l=@(Get-Content $state -Encoding UTF8)
  $inv=[Globalization.CultureInfo]::InvariantCulture
  try{ $s=[datetime]::ParseExact($l[0],'o',$inv); $t=[datetime]::ParseExact($l[1],'o',$inv) }catch{ $t=$null }
  $stale=($script:boot -and $l[2] -ne $script:boot)
  if(-not $t -or $stale -or $t -le (Get-Date)){ Remove-Item $state -Force; return $null }
  @{start=$s;target=$t}
}
function Set-Timer([long]$sec){
  $old=$script:cur
  & shutdown.exe /s /t $sec *> $null
  $code=$LASTEXITCODE
  $had=$false
  if($code -eq 1190){
    # 이미 예약된 종료가 있음 -> 취소 후 재시도
    $had=$true
    & shutdown.exe /a *> $null
    & shutdown.exe /s /t $sec *> $null
    $code=$LASTEXITCODE
  }
  if($code -ne 0){
    if($had){
      # 기존 예약이 이 프로그램이 건 것이면 남은 시간만큼 복구 시도
      $left=if($old){ [long][Math]::Floor(($old.target-(Get-Date)).TotalSeconds) }else{ 0 }
      if($left -gt 0){
        & shutdown.exe /s /t $left *> $null
        if($LASTEXITCODE -eq 0){ Say ('예약 실패 (오류 '+$code+') / 기존 타이머 유지됨') 'Red'; return }
      }
      Remove-Item $state -Force -ErrorAction SilentlyContinue
      $script:cur=$null
      Say ('예약 실패 (오류 '+$code+') / 기존 예약은 취소된 상태') 'Red'
    }else{
      Say ('예약 실패 (shutdown 오류 '+$code+')') 'Red'
    }
    return
  }
  $n=Get-Date
  $t=$n.AddSeconds($sec)
  $script:cur=@{start=$n;target=$t}
  Remove-Item $state -Force -ErrorAction SilentlyContinue
  $col='Green'; $warn=''
  try{ Set-Content $state @($n.ToString('o'),$t.ToString('o'),$script:boot) -Encoding UTF8 -ErrorAction Stop }
  catch{ $script:cur.nofile=$true; $col='Yellow'; $warn=' / 상태 파일 저장 실패' }
  $txt=(Fmt ([timespan]::FromSeconds($sec)))+' 후 종료 예약 ('+$t.ToString('HH:mm:ss')+')'
  if($had){ $txt+=' / 기존 예약 취소됨' }
  Say ($txt+$warn) $col
}
function Sync-Timer {
  # 다른 창에서 취소/변경했거나 만료됐으면 화면 상태를 상태 파일에 맞춤
  $old=$script:cur
  if($old -and $old.nofile){
    if($old.target -le (Get-Date)){ $script:cur=$null; Say '' 'Gray' }
    return
  }
  $new=Get-Timer
  if($old -and -not $new){
    if($old.target -le (Get-Date)){ Say '' 'Gray' }else{ Say '예약이 다른 곳에서 취소됨' 'Yellow' }
  }elseif($old -and $new -and $new.target -ne $old.target){
    Say '예약이 다른 곳에서 변경됨' 'Yellow'
  }
  $script:cur=$new
}
function Stop-Timer {
  & shutdown.exe /a *> $null
  $ok=($LASTEXITCODE -eq 0)
  Remove-Item $state -Force -ErrorAction SilentlyContinue
  $script:cur=$null
  if($ok){ Say '종료 예약 취소 완료.' 'Green' }else{ Say '예약된 종료 없음.' 'Yellow' }
}
function Parse-Min([string]$s){
  $s=$s.Trim().ToLower()
  if($s -eq ''){return 30}
  if($s -notmatch '^(?:(\d{1,6})h)?(?:(\d{1,9})m?)?$'){return -1}
  $h=if($matches[1]){[long]$matches[1]}else{0}
  $m=if($matches[2]){[long]$matches[2]}else{0}
  $tot=$h*60+$m
  if($tot -gt 5256000){return -1}
  $tot
}

function Intro {
  $ps=[powershell]::Create().AddScript('(Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToString("o")')
  $h=$ps.BeginInvoke()
  [Console]::CursorVisible=$false
  Clear-Host
  $win=[Console]::WindowWidth
  $rows=@(
    '███████╗██╗  ██╗██╗   ██╗████████╗██████╗  ██████╗ ██╗    ██╗███╗   ██╗',
    '██╔════╝██║  ██║██║   ██║╚══██╔══╝██╔══██╗██╔═══██╗██║    ██║████╗  ██║',
    '███████╗███████║██║   ██║   ██║   ██║  ██║██║   ██║██║ █╗ ██║██╔██╗ ██║',
    '╚════██║██╔══██║██║   ██║   ██║   ██║  ██║██║   ██║██║███╗██║██║╚██╗██║',
    '███████║██║  ██║╚██████╔╝   ██║   ██████╔╝╚██████╔╝╚███╔███╔╝██║ ╚████║',
    '╚══════╝╚═╝  ╚═╝ ╚═════╝    ╚═╝   ╚═════╝  ╚═════╝  ╚══╝╚══╝ ╚═╝  ╚═══╝'
  )
  if($win -lt 80){ $rows=@('S H U T D O W N') }
  $len=($rows | ForEach-Object { $_.Length } | Measure-Object -Maximum).Maximum
  $rows=@($rows | ForEach-Object { $_.PadRight($len) })
  $grad='Blue','Blue','Blue','DarkCyan','DarkCyan','DarkCyan','Cyan','Cyan','Cyan','Cyan','Cyan','Cyan','White','White','White','Gray','Gray','Gray'
  $n=$grad.Count
  $cw=[int][Math]::Ceiling($len/$n)
  $left=[Math]::Max(0,[int](($win-$len)/2))
  $top=3
  function Paint($ci,$col){
    $x=$ci*$cw
    if($ci -lt 0 -or $x -ge $len){return}
    $l=[Math]::Min($cw,$len-$x)
    [Console]::ForegroundColor=$col
    for($r=0;$r -lt $rows.Count;$r++){
      [Console]::SetCursorPosition($left+$x,$top+$r)
      [Console]::Write($rows[$r].Substring($x,$l))
    }
  }
  for($i=0;$i -lt $n;$i++){ Paint $i 'White'; Paint ($i-1) $grad[[Math]::Max(0,$i-1)]; Wait-Ms 22 }
  Paint ($n-1) $grad[$n-1]
  for($i=0;$i -lt $n+3;$i++){ Paint $i 'White'; Paint ($i-1) 'White'; Paint ($i-2) 'White'; Paint ($i-3) $grad[[Math]::Max(0,$i-3)]; Wait-Ms 14 }

  $sub='P O W E R   O F F   S C H E D U L E R'
  $sy=$top+$rows.Count+1
  [Console]::SetCursorPosition([Math]::Max(0,[int](($win-$sub.Length)/2)),$sy)
  [Console]::ForegroundColor='DarkGray'
  foreach($ch in $sub.ToCharArray()){ [Console]::Write($ch); Wait-Ms 12 }

  $bw=40
  $bl=[Math]::Max(0,[int](($win-$bw-6)/2))
  $by=$sy+3
  $steps='시스템 정보 수집','예약 상태 확인','화면 구성'
  for($p=0;$p -le 100;$p+=2){
    $f=[int]($bw*$p/100)
    [Console]::SetCursorPosition($bl,$by)
    [Console]::ForegroundColor='Cyan';     [Console]::Write('█'*$f)
    [Console]::ForegroundColor='DarkGray'; [Console]::Write('░'*($bw-$f))
    [Console]::ForegroundColor='White';    [Console]::Write(' {0,3}%' -f $p)
    if($p -lt 100){
      [Console]::SetCursorPosition($bl,$by+1)
      [Console]::ForegroundColor='DarkGray'
      [Console]::Write((PadW $steps[[Math]::Min(2,[int][Math]::Floor($p/34))] 30))
    }
    Wait-Ms 14
  }
  $script:boot=[string](@($ps.EndInvoke($h))[0])
  $ps.Dispose()
  [Console]::SetCursorPosition($bl,$by+1)
  [Console]::ForegroundColor='Green'
  [Console]::Write((PadW '준비 완료' 30))
  [Console]::ResetColor()
  if(-not $script:skip){ Start-Sleep -Milliseconds 450 }
  Clear-Host
}

$G=@{
  '0'='###','#.#','#.#','#.#','###'
  '1'='.#.','##.','.#.','.#.','###'
  '2'='###','..#','###','#..','###'
  '3'='###','..#','###','..#','###'
  '4'='#.#','#.#','###','..#','..#'
  '5'='###','#..','###','..#','###'
  '6'='###','#..','###','#.#','###'
  '7'='###','..#','..#','..#','..#'
  '8'='###','#.#','###','#.#','###'
  '9'='###','#.#','###','..#','###'
  '-'='...','...','###','...','...'
  ':'='.','#','.','#','.'
}
function S($t,$fg='Gray',$bg=$null){ ,@($t,$fg,$bg) }
function BoxW { [Math]::Max(50,[Math]::Min(72,[Console]::WindowWidth-6)) }
function Hr($l,$r,$in){ T ((' '*$script:ox)+$l+('─'*$in)+$r) 'DarkGray'; EOL }
function Ln($segs,$in){
  T ((' '*$script:ox)+'│  ') 'DarkGray'
  $u=2
  # 세그먼트가 1개면 @((S ...)) 가 배열로 풀려서 문자열이 먼저 나옴 -> 다시 감싸줌
  if($segs.Count -and $segs[0] -is [string]){ $segs=@(,$segs) }
  foreach($s in $segs){ T $s[0] $s[1] $s[2]; $u+=W $s[0] }
  if($in-$u -gt 0){ T (' '*($in-$u)) }
  T '│' 'DarkGray'; EOL
}
function BigRows([string]$s){
  foreach($r in 0..4){
    $parts=foreach($ch in $s.ToCharArray()){ (($G[[string]$ch][$r].ToCharArray() | ForEach-Object { if($_ -eq '#'){'██'}else{'  '} }) -join '') }
    $parts -join '  '
  }
}

function Draw-Main {
  $now=Get-Date
  $c=$script:cur
  $in=BoxW
  $script:ox=[Math]::Max(0,[int](([Console]::WindowWidth-$in-2)/2))
  $clk=$now.ToString('yyyy-MM-dd HH:mm:ss')
  if($c){
    $rem=$c.target-$now
    if($rem.TotalSeconds -lt 0){$rem=[timespan]::Zero}
    $col=if($rem.TotalSeconds -le 60){'Red'}elseif($rem.TotalMinutes -le 10){'Yellow'}else{'Green'}
    $p=[Math]::Min(1.0,[Math]::Max(0.0,[double](($now-$c.start).TotalSeconds/($c.target-$c.start).TotalSeconds)))
    $badge=' 예약됨 '; $bf='Black'; $bb=$col
    $tg=$c.target.ToString('yyyy-MM-dd HH:mm:ss'); $rm=Fmt $rem
  }else{
    $col='DarkGray'; $p=0
    $badge=' 대기 '; $bf='White'; $bb='DarkGray'
    $tg='-'; $rm='--:--:--'
  }
  $big=@()
  if($in -ge 60 -and [Console]::WindowHeight -ge 21){ $big=@(BigRows $rm); if((W $big[0]) -gt ($in-6)){ $big=@() } }
  $bwid=$in-4-6
  $f=[int][Math]::Floor($p*$bwid)
  $oy=[Math]::Max(0,[int](([Console]::WindowHeight-$(if($big.Count){21}else{17}))/2))
  [Console]::SetCursorPosition(0,$oy)
  Hr '╭' '╮' $in
  Ln @((S 'SHUTDOWN TIMER' 'Cyan'),(S (' '*[Math]::Max(1,$in-4-14-$clk.Length)) 'Gray'),(S $clk 'DarkGray')) $in
  Hr '├' '┤' $in
  Ln @() $in
  Ln @((S $badge $bf $bb),(S '   종료시각  ' 'DarkGray'),(S $tg 'White')) $in
  Ln @() $in
  if($big.Count){
    $lead=[Math]::Max(0,[int](($in-(W $big[0]))/2)-2)
    foreach($row in $big){ Ln @((S (' '*$lead) 'Gray'),(S $row $col)) $in }
  }else{
    Ln @((S '남은시간  ' 'DarkGray'),(S $rm $col)) $in
  }
  Ln @() $in
  Ln @((S ('━'*$f) $col),(S ('━'*($bwid-$f)) 'DarkGray'),(S (' {0,3}%' -f [int]($p*100)) $col)) $in
  Hr '├' '┤' $in
  Ln @((S '[' 'DarkGray'),(S '1' 'Cyan'),(S ']' 'DarkGray'),(S '  새 타이머 설정' 'White'),(S '    기존 타이머는 자동 취소' 'DarkGray')) $in
  Ln @((S '[' 'DarkGray'),(S '2' 'Cyan'),(S ']' 'DarkGray'),(S '  타이머 취소' $(if($c){'White'}else{'DarkGray'}))) $in
  Ln @((S '[' 'DarkGray'),(S '3' 'Cyan'),(S ']' 'DarkGray'),(S '  종료' 'White'),(S '    Esc 도 가능' 'DarkGray')) $in
  Hr '├' '┤' $in
  Ln @((S $script:msg $script:msgColor)) $in
  Hr '╰' '╯' $in
}

function Reserve {
  Clear-Host
  $in=BoxW
  $script:ox=[Math]::Max(0,[int](([Console]::WindowWidth-$in-2)/2))
  $oy=[Math]::Max(0,[int](([Console]::WindowHeight-15)/2))
  for($i=0;$i -lt $oy;$i++){ [Console]::WriteLine() }
  Hr '╭' '╮' $in
  Ln @((S 'NEW TIMER' 'Cyan')) $in
  Hr '├' '┤' $in
  if($script:cur){
    Ln @((S '현재 예약  ' 'DarkGray'),(S ($script:cur.target.ToString('HH:mm:ss')+'  (남은 '+(Fmt ($script:cur.target-(Get-Date)))+')') 'Yellow')) $in
    Ln @((S '새로 설정하면 기존 타이머는 자동 취소됨' 'DarkGray')) $in
  }else{
    Ln @((S '현재 예약된 타이머 없음' 'DarkGray')) $in
  }
  Ln @() $in
  Ln @((S '입력 예  ' 'DarkGray'),(S '30' 'White'),(S '  /  ' 'DarkGray'),(S '90' 'White'),(S '  /  ' 'DarkGray'),(S '1h30m' 'White'),(S '  /  ' 'DarkGray'),(S '2h' 'White')) $in
  Ln @((S 'Enter = 30분    0 = 뒤로' 'DarkGray')) $in
  Hr '╰' '╯' $in
  [Console]::WriteLine()
  while($true){
    T ((' '*($script:ox+4))+'분  > ') 'Cyan'
    [Console]::CursorVisible=$true
    $line=[Console]::ReadLine()
    [Console]::CursorVisible=$false
    if($null -eq $line){ return }
    $m=Parse-Min $line
    if($m -lt 0){ T ((' '*($script:ox+4))+'잘못된 입력.') 'Red'; [Console]::WriteLine(); continue }
    if($m -eq 0){ return }
    Set-Timer ($m*60)
    return
  }
}

try{
  Intro
  $script:cur=Get-Timer
  [Console]::CursorVisible=$false
  $last=''; $lw=0; $lh=0
  while($true){
    if([Console]::WindowWidth -ne $lw -or [Console]::WindowHeight -ne $lh){ $lw=[Console]::WindowWidth; $lh=[Console]::WindowHeight; Clear-Host; $last='' }
    $k=(Get-Date).ToString('HHmmss')
    if($k -ne $last){ $last=$k; Sync-Timer; Draw-Main }
    if([Console]::KeyAvailable){
      $key=[Console]::ReadKey($true)
      if($key.KeyChar -eq '1'){ Reserve; Clear-Host; $last='' }
      elseif($key.KeyChar -eq '2'){ Stop-Timer; $last='' }
      elseif($key.KeyChar -eq '3' -or $key.Key -eq 'Escape'){ break }
    }else{ Start-Sleep -Milliseconds 50 }
  }
}finally{
  [Console]::CursorVisible=$true
  [Console]::ResetColor()
  [Console]::OutputEncoding=$script:oldEnc
}
Clear-Host
