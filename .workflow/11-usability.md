# 11_tui_playground — Usability test

[← Hub](README.md) · Prev: [10. Prototype](10-prototype.md) · Next: [12. Hardening & verification](12-hardening.md)

Created: 2026-11-07

<!-- wf:usability
status: done
-->

> Done when: every core task is `success` in the latest round → `status: done`

> This is a self-test by the person who built it: it catches obvious problems only. Real usability findings come from [internal test](14-internal-test.md) with other people.

## At a glance

- 핵심 기능(F-01·F-02·F-04)에서 과제 4개를 뽑아 만든 사람이 직접 해 보는 자체 테스트
- lite 모드라 내부 테스트가 없음. 같은 과제를 다른 사람 1명 이상에게도 해 보게 하길 권함
- 1회차: T1~T3 모두 성공(난이도 4, 막힌 곳 없음). T4 는 사용자 판단으로 생략함(복잡한 업무 로직이 없는 앱이라 테스트 효과가 적음)

## Facilitator only — fill in after the round ends

| Task | Source feature | Success criterion | Expected path |
| --- | --- | --- | --- |
| T1 | [F-02](02-spec.md#f-02), [F-01](02-spec.md#f-01) | 1시간 30분 뒤 종료 예약이 걸림 | 홈 → 종료 타이머 → 새 타이머 설정 → `1h30m` 또는 `90` 입력 |
| T2 | [F-02](02-spec.md#f-02) | 예약이 해제됨 | 종료 타이머 → 타이머 취소 |
| T3 | [F-04](02-spec.md#f-04) | 집중 50분·휴식 10분으로 바꾸고 시작함 | 홈 → 포모도로 → 설정(`e`) → 50/10 입력 → Space 로 시작 |
| T4 | [F-01](02-spec.md#f-01) | 프로그램이 종료됨 | 앱에서 Esc 로 홈 → Esc/q 로 종료 |

## Task prompts shown to the user

State the goal only. Do not include menu names, button names, or steps.

- T1: 이제 1시간 30분짜리 영화를 볼 생각임. 영화가 끝날 즈음 PC 가 알아서 꺼지게 해 두고 싶음
- T2: 마음이 바뀌어서 오늘은 PC 를 계속 켜 둬야 함. 방금 해 둔 것을 없던 일로 하고 싶음
- T3: 50분 집중하고 10분 쉬는 방식으로 공부를 시작하고 싶음
- T4: 다 썼으니 이 프로그램을 완전히 닫고 싶음

## Round 1 — 2026-11-07

| Task | Result | Difficulty (1 very hard – 4 very easy) | Where they got stuck (user's own words) | Observations |
| --- | --- | --- | --- | --- |
| T1 | success | 4 | 없음 | |
| T2 | success | 4 | 없음 | |
| T3 | success | 4 | 없음 | |
| T4 | 생략 | - | - | 사용자 판단으로 진행하지 않음 |

### Issues and fixes

| ID | Task | Issue | Severity | Fix | Fixed file |
| --- | --- | --- | --- | --- | --- |
| - | - | 발견한 문제 없음 | - | - | - |
