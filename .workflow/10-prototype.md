# 11_shutdown_timer — Prototype

[← Hub](README.md) · Prev: [6. Mid-fi wireframe](06-wireframe/index.html) · Next: [11. Usability test](11-usability.md)

Created: 2026-11-05

<!-- wf:prototype
status: done
-->

> Basis: [feature spec](02-spec.md), approved [wireframe](06-wireframe/index.html), [DESIGN.md](../DESIGN.md). Done when: Must features work and run instructions are written → `status: done`

## At a glance

- Go + Bubble Tea 단일 exe. 앱 8개(종료 타이머, 포모도로, 스톱워치, 시스템 모니터, 디스크 사용량, 웹 브라우저, 점심 룰렛, 멍때리기)와 홈 꾸밈(배경·전환·조작부·설정)을 구현함
- 기준 브랜치: `main` (`ca9d4c3`, 2026-11-07 에 feature 브랜치 2개를 fast-forward 로 병합함)
- 실제 Windows Terminal 캡처 5장으로 디자인 점검함. DESIGN.md 와 어긋난 곳 없음

## Tech choices

lite 모드라 기술 설계 문서가 없어서 여기에 기록함

| Item | Choice | Reason |
| --- | --- | --- |
| 언어·UI | Go 1.27, Bubble Tea 1.3, lipgloss 1.1 | 실행 파일 하나로 배포, Go 미설치 PC에서도 실행 |
| 홈 꾸밈 | 화면을 칸 격자로 풀어 합성 (grid.go) | 배경·전환·조작부를 한 경로에서 처리, 트루컬러→256→16색 자동 대체 |
| 웹 글 읽기 | golang.org/x/net/html + x/text | 글 본문 추출과 EUC-KR 등 문자 코드 변환 |
| 자바스크립트 페이지·라이브 모드 | 설치된 Chrome/Edge/Brave 를 headless 로 실행, DevTools 프로토콜로 제어 (의존 라이브러리 없음) | 별도 렌더링 엔진을 넣지 않고 exe 크기를 유지 |
| 멍때리기 그림 | 점자 문자(⠿) 캔버스 (fidget_canvas.go) | 한 칸에 2×4 점을 찍어 터미널에서 부드러운 그림 |

## How to run

```
go vet ./...
go test ./...
go build -ldflags "-s -w" -o TUI-playground.exe .
.\TUI-playground.exe
```

색이 안 나오면 환경 변수 `NO_COLOR` 가 켜져 있는지 확인할 것 (켜져 있으면 배경·전환이 꺼짐)

## Implementation scope

| Feature | Status | Notes |
| --- | --- | --- |
| F-01 홈 메뉴 | implemented | 앱 8개, 설정 키 `s`, 아케이드 조작부 선택 |
| F-02 종료 타이머 | implemented | Windows 는 실제 shutdown.exe, macOS/Linux 는 컴파일만 확인 |
| F-03 앱 등록 구조 | implemented | `apps.go` 의 `registry` 에 한 줄 추가 |
| F-04 포모도로 | implemented | 벨 소리는 실제 터미널에서 미확인 |
| F-05 exe 배포 | implemented | 빌드 명령 한 줄로 단일 exe |
| F-06 macOS/Linux | 컴파일만 | linux/darwin `go vet` 만 확인, 실제 실행은 안 해봄 |
| F-07 시스템 모니터 · F-10 스톱워치 | implemented | |
| F-08 · F-09 · F-11 | 보류(코드 제거) | 2026-11-05 결정, [기능 명세](02-spec.md) 참고 |
| F-12 디스크 사용량 | implemented | 큰 드라이브 전체 스캔 속도 미확인 |
| F-13 웹 브라우저 | implemented | 자바스크립트 렌더링·라이브 모드(가상 커서, 그림 칸)는 Chrome/Edge 필요 |
| F-14 홈 꾸밈 | implemented | 실제 터미널 끊김 여부 미확인 |
| F-15 점심 룰렛 | implemented | 메뉴는 `lunch.json` 에 저장 |
| F-16 멍때리기 | implemented | 스피너·말랑이·뽁뽁이 3개 모드 |

## Differences from the wireframe

| Screen | Change | Reason |
| --- | --- | --- |
| 앱 화면 전체 | 배경·조작부가 홈뿐 아니라 앱 안쪽에도 이어짐 | 사용자 요청 |
| 홈 | 설정 키 `s`, 조작부 패널 추가 | 사용자 요청 (재미 목적) |
| 트리맵·웹 브라우저·점심 룰렛·멍때리기 | 와이어프레임 없이 구현 | 나중에 추가한 앱 (F-12, F-13, F-15, F-16). 화면은 DESIGN.md 컴포넌트로 작성하고 새 컴포넌트는 DESIGN.md에 기록 |

## Design check

Captures of each core screen compared against [DESIGN.md](../DESIGN.md) and the wireframe.

| Screen | Capture | Tokens/components match | Off from DESIGN.md / slop |
| --- | --- | --- | --- |
캡처 환경: Windows Terminal 110×34, 디더링 행성 배경, 아케이드 조작부 켬 (2026-11-07)

| Screen | Capture | Tokens/components match | Off from DESIGN.md / slop |
| --- | --- | --- | --- |
| home | [home.png](10-prototype/home.png) | yes (Frame, Header, List item, 키 안내) | 없음. 배경·조작부 색은 'Deliberate exceptions' 에 있음 |
| 종료 타이머 | [timer.png](10-prototype/timer.png) | yes (`대기` 배지는 회색 7 배경 + 검정 글자, Big digits, Progress bar, Menu row) | 없음 |
| 포모도로 | [pomodoro.png](10-prototype/pomodoro.png) | yes | 없음 |
| 디스크 사용량 | [disk.png](10-prototype/disk.png) (스캔 중 화면) | yes | 없음. 트리맵 결과 화면은 스캔이 끝나기 전이라 못 찍음 |
| 웹 브라우저 | [browser.png](10-prototype/browser.png) (시작 화면) | yes (입력 줄은 Patterns 대로 박스 아래 한 줄) | 없음. 라이브 모드 화면은 안 찍음 |

- 모호폭 문자(`━`, `▸`)는 이 터미널에서 1칸으로 그려져 오른쪽 테두리가 맞음
- 캡처 오른쪽 끝에 다른 창 일부가 보이는 것은 창 테두리 영역까지 찍힌 캡처 문제이고 앱 화면과는 무관함

## Check results

- 2026-11-05: `go vet ./...`, `go test ./...`, linux/darwin `go vet` 통과
- 2026-11-07: 병합 뒤 `main` 에서 `go vet ./...`, `go test ./...` 통과
