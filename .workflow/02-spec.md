# 11_shutdown_timer — Feature spec

[← Hub](README.md) · Prev: [1. Research & brief](01-brief.md) · Next: [3. UX flowchart](03-flow.md)

Created: 2026-11-05

<!-- wf:spec
status: done
-->

> Basis: [brief](01-brief.md). Done when: every feature has acceptance criteria and at least one feature is marked core → `status: done`
> Features marked **Core** become the tasks of the stage 10 usability test. 기능이 8개를 넘어 세부 내용은 `02-spec/F-xx.md` 로 분리함.

## At a glance

- 기능 11개: Must 5개(홈 메뉴, 종료 타이머, 앱 등록 구조, 포모도로, exe 배포) · Should 3개(macOS/Linux, 시스템 모니터, Claude 사용량) · Could 3개(Codex 사용량, 스톱워치, 할 일)
- 핵심(Core) 3개 = 사용성 테스트 과제: 홈에서 앱 고르고 복귀(F-01), 종료 타이머 예약/취소(F-02), 포모도로 사용(F-04)
- F-07~F-11 은 2026-11-05 에 추가한 미니앱. 사용량 모니터는 로컬 기록 파일을 읽기만 함 (Codex 기록 형식은 미확인)

## Feature list

| ID | Feature | Core | Priority | Source |
| --- | --- | --- | --- | --- |
| <a id="f-01"></a>[F-01](02-spec/F-01.md) | 홈 메뉴 | Y | Must | [Brief section 5](01-brief.md#5-target-outcome) |
| <a id="f-02"></a>[F-02](02-spec/F-02.md) | 종료 타이머 앱 | Y | Must | [Brief section 5](01-brief.md#5-target-outcome) |
| <a id="f-03"></a>[F-03](02-spec/F-03.md) | 앱 등록 구조 | N | Must | [Brief section 6](01-brief.md#6-success-criteria) |
| <a id="f-04"></a>[F-04](02-spec/F-04.md) | 포모도로/카운트다운 앱 | Y | Must | [Brief section 5](01-brief.md#5-target-outcome) |
| <a id="f-05"></a>[F-05](02-spec/F-05.md) | 단일 실행 파일 배포 (Windows) | N | Must | [Brief section 5](01-brief.md#5-target-outcome) |
| <a id="f-06"></a>[F-06](02-spec/F-06.md) | macOS/Linux 지원 | N | Should | [Brief section 8](01-brief.md#8-constraints) |
| <a id="f-07"></a>[F-07](02-spec/F-07.md) | 시스템 모니터 | N | Should | 사용자 요청 (2026-11-05) |
| <a id="f-08"></a>[F-08](02-spec/F-08.md) | Claude Code 사용량 모니터 | N | Should | 사용자 요청 (2026-11-05) |
| <a id="f-09"></a>[F-09](02-spec/F-09.md) | Codex 사용량 모니터 | N | Could | 사용자 요청 (2026-11-05) |
| <a id="f-10"></a>[F-10](02-spec/F-10.md) | 스톱워치 | N | Could | 제안 (2026-11-05) |
| <a id="f-11"></a>[F-11](02-spec/F-11.md) | 할 일 목록 | N | Could | 제안 (2026-11-05) |
| <a id="f-12"></a>[F-12](02-spec/F-12.md) | 디스크 사용량 트리맵 | N | Could | 구현함 (2026-11-05) |
| <a id="f-13"></a>[F-13](02-spec/F-13.md) | 웹 브라우저(텍스트) | N | Could | 구현함 (2026-11-05) |
| <a id="f-14"></a>[F-14](02-spec/F-14.md) | 홈 꾸밈(배경·전환·아케이드 조작부·설정) | N | Could | 구현함 (2026-11-05) |

Priority: Must (required for release) / Should / Could / Won't (excluded this time)

## Non-functional requirements

| Item | Criterion |
| --- | --- |
| Supported environments | Windows 10/11 (Must), macOS/Linux (Should) |
| Performance | 키 입력 뒤 화면 전환에 체감 지연이 없음. 기록 파일을 읽는 앱은 읽는 동안 진행 상태를 표시함 |
| Accessibility | 키보드만으로 모든 조작이 가능함. 상태를 색으로만 구분하지 않고 텍스트도 함께 표시함 |
| Privacy | 사용량 모니터는 이 PC 의 기록 파일을 읽기만 하고 외부로 보내지 않음 |
