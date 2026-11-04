# 11_shutdown_timer — UX flow

[← Hub](README.md) · Prev: [2. Feature spec](02-spec.md) · Next: [5. Design system (DESIGN.md)](../DESIGN.md)

Created: 2026-11-05

<!-- wf:flow
status: done
-->

> Basis: [feature spec](02-spec.md). Done when: every Must feature appears in the flow and the user has confirmed → `status: done`

## At a glance

- 화면 7개: intro → home → (종료 타이머 | 포모도로) 두 갈래. 어느 앱에서든 Esc 로 home 에 복귀
- 종료 타이머는 예약이 OS 에 걸려 있어서 나가도 유지됨 → home 목록에 예약 상태를 표시함
- 포모도로는 기본값(25/5)으로 바로 대기 화면. 진행 중에 Esc 를 누르면 한 번 확인함

## Screen list

These become the stage 6 wireframe file names. A slug is lowercase letters, digits, and `-`.

| slug | Screen name | Related feature |
| --- | --- | --- |
| intro | 인트로 (로고, 부제목, 로딩 막대) | [F-01](02-spec.md#f-01) |
| home | 홈 메뉴 (앱 목록, 종료 타이머 예약 상태 표시) | [F-01](02-spec.md#f-01), [F-03](02-spec.md#f-03) |
| timer-main | 종료 타이머 (대기/예약됨, 큰 숫자, 진행률) | [F-02](02-spec.md#f-02) |
| timer-new | 종료 타이머 새 예약 입력 | [F-02](02-spec.md#f-02) |
| pomo-main | 포모도로 (대기/진행/일시정지, 구간·횟수 표시) | [F-04](02-spec.md#f-04) |
| pomo-setup | 포모도로 집중/휴식 분 설정 | [F-04](02-spec.md#f-04) |
| pomo-leave | 포모도로 나가기 확인 (진행 중일 때만) | [F-04](02-spec.md#f-04) |

## Overall flow

```mermaid
flowchart TD
  start([실행]) --> intro[intro]
  intro -->|끝 또는 아무 키| home[home]
  home -->|종료 타이머 선택| tmain[timer-main]
  home -->|포모도로 선택| pmain[pomo-main]
  home -->|Esc/q| exit([프로그램 종료])
  tmain -->|새 타이머 키| tnew[timer-new]
  tnew -->|유효한 입력| tmain
  tnew -->|0 또는 Esc| tmain
  tnew -->|잘못된 입력| tnew
  tmain -->|취소 키| tmain
  tmain -->|Esc| home
  pmain -->|설정 키| psetup[pomo-setup]
  psetup -->|Enter 또는 Esc| pmain
  pmain -->|시작·일시정지·리셋| pmain
  pmain -->|구간 종료, 벨| pmain
  pmain -->|Esc, 진행 중| pleave[pomo-leave]
  pmain -->|Esc, 대기 중| home
  pleave -->|예, 타이머 중단| home
  pleave -->|아니오| pmain
```

## Core task paths

| Feature | Start screen | Path | Exit/exception branches |
| --- | --- | --- | --- |
| [F-01](02-spec.md#f-01) | [home](06-wireframe/home.html) | home → 앱 선택 → 앱 화면 → Esc → home | 앱 0개면 "등록된 앱 없음" 표시, home 에서 Esc/q 로 종료 |
| [F-02](02-spec.md#f-02) | [home](06-wireframe/home.html) | home → timer-main → timer-new → timer-main(예약됨) → 취소 → timer-main(대기) | 잘못된 입력은 timer-new 에 남아 안내, 예약 실패는 메시지 표시, 나가도 예약 유지 |
| [F-04](02-spec.md#f-04) | [home](06-wireframe/home.html) | home → pomo-main(25/5 대기) → 시작 → 구간 종료·전환 → Esc → home | 설정 값 오류는 pomo-setup 에서 거부, 진행 중 Esc 는 pomo-leave 확인 |

## Check

- Must 기능 중 F-03(앱 등록 구조), F-05(exe 배포)는 개발·배포 기능이라 사용자 화면이 없음 → 화면 흐름 대상 아님
- Should 기능 F-06(macOS/Linux)도 화면 변화가 없음
- 도달할 수 없는 화면과 막다른 화면은 없음 (모든 화면에서 home 으로 돌아올 수 있음)
