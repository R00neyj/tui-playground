# 11_tui_playground — Release

[← Hub](README.md) · Prev: [12. Hardening & verification](12-hardening.md)

Created: 2026-11-07

<!-- wf:release
status: draft
version: 1.0.0
url:
check_on: 2026-11-21
-->

> The user decides whether to run the release. Done when: deployment confirmed → `status: released`

## At a glance

- v1.0.0: 앱 8개 + 홈 꾸밈, Windows 64비트 exe 를 GitHub Release 로 배포
- 근거 점검 모두 충족 (내부 테스트는 lite 모드라 해당 없음). 미뤄 둔 문제는 low 1건(H-03)
- 성공 기준 재확인 날짜: 2026-11-21

## Release decision basis

- [x] Brief success criteria are measurable: 앱 추가 = 파일 1개 + 등록 1줄, 홈에서 실행 후 복귀 ([브리프](01-brief.md#6-success-criteria))
- [x] All Must features implemented: F-01~F-05 ([프로토타입](10-prototype.md))
- [x] All core usability test tasks succeeded: T1~T3 성공, T4 는 사용자 판단으로 생략 ([사용성 테스트](11-usability.md))
- [x] [Hardening](12-hardening.md): Must 인수 조건 13/13 pass, high 0개
- [ ] 0 blocker issues in the internal test (full mode): lite 모드라 해당 없음
- [x] All `wfctl check` items pass: test·build·lint (2026-11-07)

## Deferred issues

| ID | Content | Reason for deferral |
| --- | --- | --- |
| [H-03](12-hardening.md) | 포모도로 벨 소리, macOS/Linux 실제 실행, 큰 드라이브 스캔 속도 미확인 | Must 밖이거나 환경에 따라 다름. 릴리스 노트에 한계로 적음 |

## Release notes

### v1.0.0

- 홈 메뉴에서 미니앱 8개 실행: 종료 타이머, 포모도로, 스톱워치, 시스템 모니터, 디스크 사용량, 웹 브라우저, 점심 룰렛, 멍때리기
- 홈 꾸밈: 배경 5종, 화면 전환 4종, 아케이드 조작부 (설정에서 바꾸거나 끔)
- 설치 없이 exe 하나로 실행 (Windows 64비트)
- 종료 타이머는 실제로 PC 종료를 예약함. 취소는 앱 또는 `shutdown /a`
- 알려진 한계: 벨 소리, macOS/Linux 실제 실행, 큰 드라이브 스캔 속도는 미확인

## Success check

On `check_on` (default: release + 14 days), measure the brief's success criteria. Misses feed the next feature-add cycle.

| Success criterion ([brief](01-brief.md#6-success-criteria)) | How measured | Result | Next action |
| --- | --- | --- | --- |
| 새 앱 추가가 "파일 1개 + 등록 1줄"로 가능함 | 릴리스 뒤 추가한 앱 커밋의 변경 파일 확인 | | |
| 홈에서 앱을 실행하고 끝내면 홈으로 복귀함 | `TestProgramEndToEnd` 실행 + 실제 사용 중 문제 있었는지 사용자에게 확인 | | |

## Deploy record

| Item | Value |
| --- | --- |
| Version | v1.0.0 |
| Date/time | |
| URL | |
| Rollback method | GitHub Release `v1.0.0` 삭제 또는 pre-release 로 바꾸고, 이전 빌드 `eval-1` 을 안내함. 코드는 태그 이전 커밋으로 되돌릴 필요 없음(배포물은 exe 뿐) |
