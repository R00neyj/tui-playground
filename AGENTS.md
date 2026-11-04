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

- 박스는 직접 그림: 안쪽 폭 `in` = 창 폭-6 (최소 50, 최대 72). 줄 폭 계산은 반드시 `lipgloss.Width` 를 쓸 것 (한글은 2칸)
- 안내 문구가 가장 긴 줄(메뉴 1번 줄)이 약 48칸이라 `boxW` 최소값을 50 밑으로 낮추면 오른쪽 테두리가 깨짐
- 큰 숫자 모드는 폭 60 이상, 높이 21 이상일 때만 씀
- 화면 정렬 테스트(`TestViewsAligned`)가 박스 줄 폭이 모두 같은지 확인함. 화면을 바꾸면 이 테스트를 같이 돌릴 것
- 인트로 로고는 영문 약칭 `TUI` 블록 글꼴이고, 한글 앱 이름(`appName`)은 부제목과 헤더에 씀

## 작업 규칙

- vibe-workflow 단계(`.workflow/`)가 끝날 때마다 커밋함. 그 외에는 사용자가 요청할 때만 커밋함. 기본 브랜치는 `main`
- UI work: read DESIGN.md first; use only its tokens and components, and log new ones there.
- 응답과 문서는 한국어로 작성함
- 홈 배경·전환은 터미널이 색을 지원할 때만 켜짐(`TUI_ANIM=off` 로 끌 수 있음). 테스트는 `forceProfile` 로 색 모드를 바꿔 끼우고, `TestMain` 이 설정 파일 경로를 임시 폴더로 돌려놓음
