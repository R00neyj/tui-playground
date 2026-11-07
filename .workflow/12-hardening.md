# 11_tui_playground — Hardening & verification

[← Hub](README.md) · Prev: [11. Usability test](11-usability.md) · Next: [15. Release](15-release.md)

Created: 2026-11-07

<!-- wf:hardening
status: done
-->

> Source: [feature spec](02-spec.md) acceptance criteria and exceptions, [DESIGN.md](../DESIGN.md). Done when: every Must acceptance criterion is verified with evidence, no open `high` item, `wfctl check` passes → `status: done`

## At a glance

- Must 인수 조건 13개 모두 pass (테스트 12개 실행 + 다른 PC 에서 exe 실행 1개는 사용자 확인)
- 고친 것 2개: 시스템 모니터 추이 줄 라벨의 흐린 글자색(H-01), 홈 Esc 종료 테스트 누락(H-02). 남은 high 없음
- `wfctl check` 통과 (2026-11-07)

## Acceptance criteria

테스트는 `go test -count=1 -run '<이름>' -v .` 로 실행함 (2026-11-07)

| Feature | Criterion | How verified | Result |
| --- | --- | --- | --- |
| [F-01](02-spec.md#f-01) | 목록에서 앱을 골라 실행 | `TestHostFlow`(↑↓·숫자키로 열기), `TestProgramEndToEnd`(실제 프로그램에 키 입력, 앱 8개 모두 열기) | pass |
| [F-01](02-spec.md#f-01) | 앱에서 Esc 로 홈 복귀 | `TestFreshAppEscGoesHome`(앱 8개), `TestProgramEndToEnd` | pass |
| [F-01](02-spec.md#f-01) | 홈에서 Esc/q 로 종료 | `TestHostFlow`(q·Esc 모두 `tea.QuitMsg` 확인, Esc 는 이번에 추가), `TestProgramEndToEnd`(q) | pass |
| [F-02](02-spec.md#f-02) | 입력 형식대로 예약, 남은 시간·진행률 표시 | `TestParseMin`, `TestTimerSetNew`, `TestTimerProgressMoves` (가짜 `runShutdown`) | pass |
| [F-02](02-spec.md#f-02) | 취소하면 예약 해제 | `TestTimerStop` + 사용성 테스트 T2 에서 실제 취소 | pass |
| [F-02](02-spec.md#f-02) | 홈을 오가도 예약 유지 | `TestTimerStatusForHome` (상태 파일에서 다시 읽음) | pass |
| [F-02](02-spec.md#f-02) | 새 예약이 실패해도 기존 예약이 사라지지 않음 | `TestTimerFailKeepsOld`, `TestTimerFailRestoreFails`(복구 실패 시 안내), `TestTimerCancelFailKeepsExistingOnReplace` | pass |
| [F-03](02-spec.md#f-03) | 샘플 앱 추가가 파일 1개 + 등록 1줄 | 커밋 `87ad089`: 점심 룰렛은 `app_lunch.go` + `apps.go` 1줄(테스트 파일 제외) | pass |
| [F-03](02-spec.md#f-03) | 기존 앱 코드는 수정 안 함 | 같은 커밋에서 기존 앱 파일(`app_timer.go` 등) 변경 없음 | pass |
| [F-04](02-spec.md#f-04) | 시작·일시정지·리셋 | `TestPomoStartPauseResumeReset` + 사용성 테스트 T3 | pass |
| [F-04](02-spec.md#f-04) | 구간이 끝나면 알림 후 다음 구간 | `TestPomoSegmentEndSwitchesWithBell` | pass |
| [F-04](02-spec.md#f-04) | 완료 횟수 표시 | `TestPomoSegmentEndSwitchesWithBell`, 캡처 [pomodoro.png](10-prototype/pomodoro.png) ("오늘 완료 0회") | pass |
| [F-05](02-spec.md#f-05) | Go 없는 PC 에서 exe 하나로 홈 도달 | `go version -m`: `CGO_ENABLED=0`, `GOOS=windows` 단일 exe(약 10MB). 사용자가 Go 없는 다른 Windows PC 에서 실행해 홈 화면 확인 | pass |

## Hardening checklist

| Area | Check | Result | Note |
| --- | --- | --- | --- |
| Errors | 네트워크·OS 호출 실패 시 메시지 | ok | 브라우저 `TestBrTimeout`·`TestBrStatusErrors`·`TestBrFetchErrorShownAndKeepsPage`, 종료 예약 실패 사유 표시 `TestTimerScheduleFailShowsReason` |
| Edge cases | 빈 값, 잘못된 입력, 깨진 파일 | ok | `TestParseMin`, `TestPomoSetupInvalid`, `TestLunchEmptyList`, `TestLunchLoadBroken`, 읽을 수 없는 폴더 건너뛰기 `TestScanSkipsUnreadable` |
| Loading | 오래 걸리는 작업에 진행 표시 | ok | 디스크 스캔 진행 화면(캡처 [disk.png](10-prototype/disk.png)), 스캔 취소 `TestCancelKeyStopsScan` |
| Data | 상태 유지, 데이터 손실 없음 | ok | 상태 파일 임시 파일 교체 저장, 저장 실패 시 `NoFile` 표시 `TestTimerStateWriteFailure`, 재부팅 뒤 버림 `TestTimerStaleBootDropped` |
| Security | 비밀 값이 저장소에 없음 | ok | 키·토큰 문자열 검색 결과 없음(테스트용 HTML 1건 제외), `.env`·`.pem` 파일 없음. 서버 없는 로컬 앱 |
| Access | 키보드만으로 조작, 대비 | ok (H-01 수정 후) | 모든 조작이 키보드. 상태는 배지 글자로도 표시 |
| Performance | 첫 화면까지 시간 | ok | 인트로 2.35초, 아무 키로 건너뜀. 큰 드라이브 전체 스캔 속도는 미측정 |
| Design | 토큰 밖 색 직접 사용 없음 | ok | `TestNoRawColors`, RGB 직접 색은 DESIGN.md 예외 파일(grid·bg·transition·arcade·theme)에만 있음 |
| Anti-slop | 탐지 검색 | ok | 이모지·홍보 문구 없음. 배경 애니메이션·그라데이션 로고는 DESIGN.md 'Deliberate exceptions' 에 있음, `TUI_ANIM=off` 로 끌 수 있음 |

## Issues

| ID | Area | Problem | Severity | Fix |
| --- | --- | --- | --- | --- |
| H-01 | Access | 시스템 모니터 추이 줄 라벨이 `color.text-faint`(대비 4.3) 글자 | medium | `ui.go` 라벨을 `color.text-muted` 로 바꿈 (fixed) |
| H-02 | 검증 | 홈 Esc 종료가 테스트되지 않음 (q 만 확인) | low | `main_test.go` `TestHostFlow` 에 Esc 추가, 종료 메시지까지 확인 (fixed) |
| H-03 | 미확인 | 포모도로 벨 소리, macOS/Linux 실제 실행, 큰 드라이브 스캔 속도 | low | Must 밖이거나 환경 의존. 릴리스 노트에 알려진 한계로 적음 |

## Checks

`wfctl check` summary: test=pass build=pass lint=pass (2026-11-07, H-01·H-02 수정 후 다시 실행)
