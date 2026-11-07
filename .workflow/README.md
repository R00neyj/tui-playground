# 11_shutdown_timer — Overall flow

Started: 2026-11-05

This document is the project hub. Follow the links in the table below to each stage's detail document.
The inside of the `<!-- wf:index -->` block is redrawn by `wfctl sync` from the stage documents' markers — do not edit it by hand.

## Overview

- One-line definition: 터미널 플레이그라운드(TUI-playground) — Windows 콘솔용 Go + Bubble Tea 앱. 현재 기능은 종료 타이머 하나임 (자세한 구조는 `AGENTS.md`)
- Target users: 본인(학습·취미)과 exe를 받아 쓰는 지인·동료
- Current goal: 홈 메뉴 + 두 번째 미니앱 + "새 앱 추가 = 파일 1개 + 등록 1줄" 구조 ([브리프](01-brief.md))

## Progress

<!-- wf:index:begin -->
Mode: **lite**

| Phase | # | Doc | Status | Skill |
| --- | --- | --- | --- | --- |
| Planning | 1 | [Research & brief](01-brief.md) | done | `/wf-brief` |
| Planning | 2 | [Feature spec](02-spec.md) | done | `/wf-spec` |
| UX | 3 | [UX flowchart](03-flow.md) | done | `/wf-flow` |
| UI | 4 | [UI references](04-references.md) | skipped (lite) | `/wf-references` |
| UI | 5 | [Design system (DESIGN.md)](../DESIGN.md) | done | `/wf-design` |
| UI | 6 | [Mid-fi wireframe](06-wireframe/index.html) | done | `/wf-wireframe` |
| UI | 7 | [Wireframe review](07-wireframe-review.md) | skipped (lite) | `/wf-wireframe-review` |
| UI | 8 | [Styled key screens](08-ui-sample.md) | skipped (lite) | `/wf-ui-sample` |
| Development | 9 | [Tech design](09-tech.md) | skipped (lite) | `/wf-tech` |
| Development | 10 | [Prototype](10-prototype.md) | done | `/wf-prototype` |
| Development | 11 | [Usability test](11-usability.md) | done | `/wf-usability` |
| Development | 12 | [Hardening & verification](12-hardening.md) | done | `/wf-hardening` |
| Release | 13 | [Pre-release](13-prerelease.md) | skipped (lite) | `/wf-prerelease` |
| Release | 14 | [Internal test](14-internal-test.md) | skipped (lite) | `/wf-internal-test` |
| Release | 15 | [Release](15-release.md) | not started ◀ now | `/wf-release` |

Next up: **15. Release** → `/wf-release`
<!-- wf:index:end -->

## Decision log

Add one line each time a stage ends or the direction changes. Link the supporting document.

| Date | Stage | Decision | Basis |
| --- | --- | --- | --- |
| 2026-11-05 | - | Workflow started | - |
| 2026-11-05 | 1 | 제품 방향을 "TUI 미니앱 모음(+학습용 실험장)"으로 확정, 플랫폼은 Windows + macOS/Linux | [브리프](01-brief.md) |
| 2026-11-05 | 2 | 기능 6개 확정(Must 5, Should 1). 두 번째 앱은 포모도로, 핵심은 F-01·F-02·F-04, macOS/Linux는 Should | [기능 명세](02-spec.md) |
| 2026-11-05 | 3 | 화면 7개 확정. 포모도로는 25/5 기본값 대기 화면으로 진입, 진행 중 Esc는 확인, 홈 목록에 종료 타이머 예약 상태 표시(F-01·F-04 명세에 한 줄씩 반영) | [UX 플로차트](03-flow.md) |
| 2026-11-05 | - | 작업 규칙: 각 단계가 끝나면 커밋함. 기본 브랜치는 main | - |
| 2026-11-05 | 5 | 디자인 시스템 확정: 현재 화면 유지(강조 cyan, 둥근 박스, 넓은 여백). 대비 4.5 미달인 힌트 글자·`대기` 배지는 9단계에서 수정 | [DESIGN.md](../DESIGN.md) |
| 2026-11-05 | 6 | 화면 7개를 80×24 터미널 기준 와이어프레임으로 작성(상태별 변형 포함, 링크로 흐름 확인). 사용자 확인 완료 | [와이어프레임](06-wireframe/index.html) |
| 2026-11-05 | 2·3·6 | Feature-add cycle: F-07 시스템 모니터, F-08 Claude 사용량, F-09 Codex 사용량, F-10 스톱워치, F-11 할 일 추가. 기능 11개가 되어 명세를 02-spec/F-xx.md 로 분리, 화면 12개, 홈은 한 줄형 목록. DESIGN.md 에 컴포넌트 7개 추가 | [기능 명세](02-spec.md), [UX 플로차트](03-flow.md), [와이어프레임](06-wireframe/index.html), [DESIGN.md](../DESIGN.md) |
| 2026-11-07 | 10 | 프로토타입 완료: 앱 8개 + 홈 꾸밈. feature 브랜치 2개를 main 에 병합, 문서를 새 번호(10-prototype.md)로 옮김, Windows Terminal 캡처 5장으로 디자인 점검(어긋남 없음) | [프로토타입](10-prototype.md) |
| 2026-11-07 | 11 | 사용성 테스트 완료: T1~T3(종료 예약·취소, 포모도로 50/10) 모두 성공, 막힌 곳 없음. T4(프로그램 종료)는 업무 로직이 단순해 사용자 판단으로 생략 | [사용성 테스트](11-usability.md) |
| 2026-11-07 | 12 | 마감 점검 완료: Must 인수 조건 13개 pass, 시스템 모니터 라벨 글자색 수정(H-01)·홈 Esc 종료 테스트 추가(H-02). 벨 소리·macOS/Linux 실행·큰 드라이브 스캔 속도는 릴리스 노트 한계로 | [마감 점검](12-hardening.md) |

## Open questions

Once answered, move to the decision log and delete here.

-
