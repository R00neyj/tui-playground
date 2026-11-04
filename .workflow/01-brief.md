# 11_shutdown_timer — Brief

[← Hub](README.md) · Next: [2. Feature spec](02-spec.md)

Created: 2026-11-05

<!-- wf:brief
status: done
-->

> Done when: every section is filled in and the user has confirmed → `status: done`

## At a glance

- 터미널 플레이그라운드 = 여러 TUI 미니앱을 홈 메뉴에서 골라 쓰는 단일 실행 파일 (취미·학습용, 지인에게 공유)
- 첫 번째 앱은 이미 있는 종료 타이머. 이번 목표는 홈 메뉴 + 두 번째 미니앱 + 앱 추가 구조
- 성공 기준: 새 앱 추가가 파일 1개 + 등록 1줄, 홈에서 앱 실행 후 복귀

## 1. One-line definition

지인에게 exe 하나로 나눠줄 수 있는, Go + Bubble Tea 기반 TUI 미니앱 모음 (종료 타이머가 첫 앱).

## 2. Problem

- What is painful today: 종료 타이머 하나만 있는 단일 목적 프로그램이라 다른 TUI 도구를 얹을 틀이 없음
- How it is solved today: 앱마다 별도 프로그램을 따로 만드는 방식뿐임 (PowerShell 배치 → Go 단일 앱으로 이미 한 번 이식함)

## 3. Target users

| User type | Situation | What they want |
| --- | --- | --- |
| 본인 (학습·취미) | TUI 개발을 연습하고 싶음 | 새 아이디어를 가볍게 얹어볼 수 있는 틀 |
| 지인·동료 | exe를 받아서 씀 | 설치 없이 실행, 영상·다운로드·렌더링 끝나면 PC 종료 |

## 4. Reference research

| Subject | Strengths | Weaknesses | What to borrow |
| --- | --- | --- | --- |
| [Basci0/shutdowner](https://pkg.go.dev/github.com/Basci0/shutdowner) | Go, Windows 종료 전용, 단순함 | pterm 기반이고 Bubble Tea는 to-do 상태임 | 단순한 흐름 |
| [tea-timer](https://pkg.go.dev/github.com/Cod-e-Codes/tea-timer) · [tuimer](https://github.com/rverchere/tuimer) | Bubble Tea 카운트다운, 크로스플랫폼, 7세그먼트 표시 | 종료 기능은 없음 | 큰 숫자 표시 방식 |
| [rothgar/awesome-tuis](https://github.com/rothgar/awesome-tuis) | 카테고리별 TUI 앱 목록 | 목록일 뿐 실행 틀이 아님 | 앱 분류·아이디어 소스 |
| [Wise Auto Shutdown](https://www.wisecleaner.com/wise-auto-shutdown.html) | 종료/재시작/절전 등 여러 동작, 5분 전 알림과 연기 | GUI·트레이 방식 (범위 밖) | 종료 직전 알림 아이디어 |

## 5. Target outcome

- 실행하면 홈 메뉴에 앱 목록이 보이고, 고른 앱을 실행한 뒤 홈으로 돌아옴
- 두 번째 미니앱이 1개 이상 있음 (종류는 미정)
- exe 하나로 다른 PC에서 실행됨 (설치 없음)
- 기존 종료 타이머는 홈 메뉴의 앱 하나로 유지함

## 6. Success criteria

- 새 앱 추가가 "파일 1개 + 등록 1줄"로 가능함
- 홈에서 앱을 실행하고 끝내면 홈으로 복귀함

## 7. Out of scope

- GUI / 트레이 아이콘 (터미널 화면만 제공)

## 8. Constraints

- Go + Bubble Tea (기존 코드 유지), 빌드 결과물은 단일 실행 파일
- 플랫폼: Windows + macOS/Linux
- 일정: 정해진 기한 없음. 취미·학습용이라 방식에 큰 제약 없음

## 9. Assumptions

- 두 번째 미니앱의 종류는 정하지 않았음 → 기능 명세 단계에서 정함
- 종료 타이머를 목표 결과 목록에서 따로 고르진 않았지만, 이미 있는 기능이라 유지하는 걸로 봄
- macOS/Linux 지원 시 종료 예약 명령과 부팅 시각 조회가 OS마다 달라 별도 구현이 필요할 것으로 봄 (확인 필요)
- "exe 하나"는 OS별로 빌드한 실행 파일 하나씩을 뜻하는 걸로 봄
- 지인 공유용 서명 없는 exe는 Windows SmartScreen 경고가 뜰 수 있음 (안내 방식은 나중에 정함)
