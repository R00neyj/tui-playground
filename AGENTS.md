# 터미널 플레이그라운드 (TUI-playground)

Windows 콘솔용 TUI 앱. 현재 기능은 종료 타이머 하나임 (지정한 시간 뒤에 PC 종료를 예약하고, 남은 시간과 진행률을 보여줌).
Go + Bubble Tea 로 작성했고, 결과물은 `TUI-playground.exe` 하나임. 실행하는 PC 에 Go 는 필요 없음.

## 빌드와 테스트

```
go vet ./...
go test ./...
go build -ldflags "-s -w" -o TUI-playground.exe .
```

- Go 를 방금 설치한 셸에서는 PATH 가 갱신되지 않아 `go` 를 못 찾을 수 있음. 새 셸을 열거나 PATH 를 다시 읽어야 함
- 테스트는 실제 `shutdown.exe` 를 호출하지 않음 (`runShutdown` 변수를 가짜로 바꿔 끼움). 테스트에서 진짜 종료 예약을 걸지 말 것
- `.exe`, `.state` 는 `.gitignore` 대상임

## 파일 구성

| 파일 | 역할 |
| --- | --- |
| `main.go` | 모델, 화면 전환(인트로/메인/입력), 키 처리 |
| `timer.go` | 예약·취소(`shutdown.exe` 호출), 상태 파일, 부팅 시각, 입력 파싱, 다른 창과 동기화(`sync`) |
| `view.go` | 메인 화면, 새 타이머 입력 화면, 큰 숫자, 박스 그리기 |
| `intro.go` | 인트로 애니메이션 (로고, 부제목 타이핑, 로딩 막대) |
| `grid.go` `screen.go` | 화면을 칸 격자로 다루는 도구(색 입힌 문자열 ↔ 칸), 홈 합성(배경 + 박스 + 조작부) |
| `bg.go` `transition.go` `arcade.go` | 홈 배경 5종, 화면 전환 4종, 아케이드 조작부 |
| `app_diskusage.go` | 디스크 사용량 트리맵 앱(읽기 전용, 폴더 스캔 + squarified 배치) |
| `app_lunch.go` | 점심 룰렛 앱(메뉴가 돌다 느려지며 멈춤, 추가·삭제, `lunch.json` 저장) |
| `app_fidget.go` `fidget_canvas.go` `fidget_spinner.go` `fidget_squishy.go` `fidget_bubble.go` | 멍때리기 앱. 호스트(탭·틱·`fidgetMode` 인터페이스) + 점자 캔버스 + 모드 3개. 새 모드는 `fidgetMode` 를 구현하고 `app_fidget.go` 의 `modes` 에 한 줄 추가 |
| `app_browser.go` `app_browser_live.go` `browser_cdp.go` `browser_snap.go` `browser_parse.go` `browser_render.go` `browser_marks.go` `browser_kill_*.go` | 웹 브라우저 앱. 라이브 모드(설치된 Chrome/Edge 를 DevTools 프로토콜로 조종, 페이지를 칸 격자에 그리고 방향키 가상 커서 + Enter 클릭, 입력창에 글자 입력)와 리더 모드(내려받기, 문자 코드 변환, 본문 추출, 줄바꿈, 제어문자 제거, 빠른 이동 목록·검색). 의존: golang.org/x/net(html, websocket), x/text |
| `settings.go` | 홈 설정(배경·전환·조작부) 저장과 설정 화면. 저장 위치는 사용자 설정 폴더의 `TUI-playground/settings.json` |
| `main_test.go` | 파싱, 예약 로직, 화면 정렬, 동기화 테스트 |
| `shutdown.bat` | 예전 PowerShell 버전. 더 이상 개발하지 않고 그대로 보관함 |

## 동작 규칙

- 예약은 `shutdown.exe /s /t <초>` 로 건다. 이미 예약이 있으면(오류 1190) `/a` 로 취소하고 다시 건다
- 새 예약이 실패하면 이 프로그램이 걸어둔 기존 예약을 남은 시간만큼 복구함. 복구 못 하면 "기존 예약은 취소된 상태"라고 알림
- 상태 파일: `%LOCALAPPDATA%\shutdown_timer_go.state` (3줄: 시작 시각, 목표 시각, 부팅 시각). 임시 파일에 쓴 뒤 교체하는 방식으로 저장함
- 부팅 시각이 다르면(재부팅 후) 상태 파일을 버림. 부팅 시각은 `NtQuerySystemInformation` 으로 구함
- 상태 파일 저장에 실패해도 예약은 유지하고 `Timer.NoFile` 로 표시함 (이 경우 파일 동기화를 건너뜀)
- 매초 `sync` 가 상태 파일을 다시 읽어서 다른 창에서 취소/변경한 것을 반영함. 외부에서 직접 `shutdown /a` 를 실행한 것은 감지하지 못함
- 입력 형식은 `30`, `90`, `1h30m`, `2h`. 빈 입력은 30분, `0` 은 뒤로. 최대 5256000분(10년)

## 화면 작성 시 주의

- 박스는 직접 그림: 안쪽 폭 `in` = 창 폭-6 (최소 50, 최대 100). 줄 폭 계산은 반드시 `lipgloss.Width` 를 쓸 것 (한글은 2칸)
- 안내 문구가 가장 긴 줄(메뉴 1번 줄)이 약 48칸이라 `boxW` 최소값을 50 밑으로 낮추면 오른쪽 테두리가 깨짐
- 큰 숫자 모드는 폭 60 이상, 높이 21 이상일 때만 씀
- 화면 정렬 테스트(`TestViewsAligned`)가 박스 줄 폭이 모두 같은지 확인함. 화면을 바꾸면 이 테스트를 같이 돌릴 것
- 인트로 로고는 영문 약칭 `TUI` 블록 글꼴이고, 한글 앱 이름(`appName`)은 부제목과 헤더에 씀

## 작업 규칙

- vibe-workflow 단계(`.workflow/`)가 끝날 때마다 커밋함. 그 외에는 사용자가 요청할 때만 커밋함. 기본 브랜치는 `main`
- UI work: read DESIGN.md first; use only its tokens and components, and log new ones there.
- 응답과 문서는 한국어로 작성함
- 홈 배경·전환은 터미널이 색을 지원할 때만 켜짐(`TUI_ANIM=off` 로 끌 수 있음). 테스트는 `forceProfile` 로 색 모드를 바꿔 끼우고, `TestMain` 이 설정 파일 경로를 임시 폴더로 돌려놓음
- 앱 테스트는 실제 네트워크와 실제 드라이브를 건드리지 않음 (`brWithFetch`, `scanTree` 를 가짜로 바꿔 끼움)
- 이 세션 같은 환경에서 `NO_COLOR` 가 켜져 있으면 배경·전환이 꺼짐. 눈으로 확인할 때는 `set NO_COLOR=` 로 지우고 실행할 것
- 웹 브라우저 테스트는 실제 브라우저(Chrome/Edge)를 띄우지 않음 (`TestMain` 이 `brFindEngine` 을 비워 둠). 실제 렌더링을 눈으로 확인할 때만 `brFindEngineDefault` 를 직접 끼워서 돌릴 것
- 웹 브라우저 라이브 모드: Esc 는 입력 중이면 입력만 끝내고, 아니면 브라우저 세션을 닫고 홈으로 감 (Esc 는 언제나 홈이라는 규칙의 유일한 예외)
- 라이브 모드 테스트는 가짜 `brOpenLive`/`fakeLive` 를 끼워 씀. 실제 Chrome 으로 확인할 때만 `TUI_LIVE_SMOKE=1 go test -run TestLiveSmoke -v` (로컬 서버 사용, `TUI_LIVE_URL` 로 주소 지정 가능)
- 라이브 모드는 봇 탐지 우회(UA 위장, 자동화 표식 숨김 등)를 하지 않음. 자동화 접속을 막는 사이트는 열리지 않을 수 있고, 그때는 리더 모드(v)나 다른 주소로 안내함
