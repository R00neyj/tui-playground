# 11_shutdown_timer — Feature spec

[← Hub](README.md) · Prev: [1. Research & brief](01-brief.md) · Next: [3. UX flowchart](03-flow.md)

Created: 2026-11-05

<!-- wf:spec
status: done
-->

> Basis: [brief](01-brief.md). Done when: every feature has acceptance criteria and at least one feature is marked core → `status: done`
> Features marked **Core** become the tasks of the stage 10 usability test.

## At a glance

- 기능 6개: Must 5개(홈 메뉴, 종료 타이머, 앱 등록 구조, 포모도로 앱, exe 배포) + Should 1개(macOS/Linux)
- 핵심(Core) 3개 = 사용성 테스트 과제: 홈에서 앱 고르고 복귀(F-01), 종료 타이머 예약/취소(F-02), 포모도로 사용(F-04)
- 두 번째 앱은 포모도로/카운트다운으로 정했고, 종료 타이머의 큰 숫자·진행률 화면을 재사용함

## Feature list

| ID | Feature | Core | Priority | Source (brief section) |
| --- | --- | --- | --- | --- |
| [F-01](#f-01) | 홈 메뉴 (앱 선택·실행·복귀) | Y | Must | [Brief section 5](01-brief.md#5-target-outcome) |
| [F-02](#f-02) | 종료 타이머 앱 (기존 기능 이식) | Y | Must | [Brief section 5](01-brief.md#5-target-outcome) |
| [F-03](#f-03) | 앱 등록 구조 (파일 1개 + 등록 1줄) | N | Must | [Brief section 6](01-brief.md#6-success-criteria) |
| [F-04](#f-04) | 포모도로/카운트다운 앱 | Y | Must | [Brief section 5](01-brief.md#5-target-outcome) |
| [F-05](#f-05) | 단일 실행 파일 배포 (Windows) | N | Must | [Brief section 5](01-brief.md#5-target-outcome) |
| [F-06](#f-06) | macOS/Linux 지원 | N | Should | [Brief section 8](01-brief.md#8-constraints) |

Priority: Must (required for release) / Should / Could / Won't (excluded this time)

## Feature details

<a id="f-01"></a>

### F-01 홈 메뉴

- User: 모든 사용자
- Purpose: 미니앱을 골라 쓰고 다시 돌아옴
- Input: ↑/↓ 또는 숫자키로 선택, Enter 실행, Esc/q 종료
- Behavior: 등록된 앱을 이름 + 한 줄 설명으로 나열. 앱 실행 중 Esc 를 누르면 홈으로 복귀. 인트로는 프로그램 시작 때 한 번만 보여줌. 종료 타이머 예약이 있으면 목록에 예약 상태와 남은 시간을 함께 표시함
- Output/result: 홈 ↔ 앱 화면 전환
- Exceptions: 등록된 앱이 없으면 "등록된 앱 없음" 표시, 창이 작으면 목록만 표시
- Acceptance criteria:
  - [ ] 목록에서 앱을 골라 실행할 수 있음
  - [ ] 앱에서 Esc 를 누르면 홈으로 돌아옴
  - [ ] 홈에서 Esc/q 로 프로그램이 종료됨

<a id="f-02"></a>

### F-02 종료 타이머 앱

- User: 영상·다운로드·렌더링 뒤 PC를 끄고 싶은 사람
- Purpose: 지정한 시간 뒤 PC 종료를 예약하고 상태를 확인·취소함
- Input: `30` / `90` / `1h30m` / `2h` (빈 입력 = 30분, `0` = 뒤로)
- Behavior: 현재 동작(예약, 취소, 남은 시간, 진행률, 다른 창과 동기화, 실패 시 기존 예약 복구)을 유지. `3`/Esc 는 종료가 아니라 홈 복귀로 바뀜
- Output/result: 예약 상태 배지, 큰 숫자 남은 시간, 진행률 막대
- Exceptions: 잘못된 입력 안내, 예약 실패 메시지, 상태 파일 저장 실패 시 경고
- Acceptance criteria:
  - [ ] 입력 형식대로 예약되고 남은 시간·진행률이 표시됨
  - [ ] 취소하면 예약이 해제됨
  - [ ] 홈으로 나갔다 다시 들어와도 예약 상태가 유지됨 (예약은 OS 에 걸려 있음)
  - [ ] 새 예약이 실패해도 기존 예약이 사라진 채로 끝나지 않음

<a id="f-03"></a>

### F-03 앱 등록 구조

- User: 개발자(본인)
- Purpose: 새 미니앱을 기존 코드를 거의 건드리지 않고 추가함
- Input: 앱이 구현하는 공통 인터페이스(이름, 설명, 화면 모델)
- Behavior: 앱 파일 1개를 만들고 등록 목록에 1줄을 추가하면 홈 메뉴에 나타남
- Output/result: 홈 목록에 새 앱이 표시되고 실행됨
- Exceptions: 이름이 겹치면 등록 시점에 오류로 알림
- Acceptance criteria:
  - [ ] 샘플 앱 추가가 "파일 1개 + 등록 1줄" 변경만으로 끝남
  - [ ] 기존 앱의 코드는 수정하지 않아도 됨

<a id="f-04"></a>

### F-04 포모도로/카운트다운 앱

- User: 시간을 나눠 집중하고 싶은 사람
- Purpose: 집중/휴식 구간을 번갈아 카운트다운함 (PC 종료와 무관)
- Input: 집중 분(기본 25), 휴식 분(기본 5). 시작/일시정지/리셋 키
- Behavior: 종료 타이머와 같은 큰 숫자·진행률 화면을 사용. 구간이 끝나면 터미널 벨과 함께 다음 구간으로 넘어감. 진입하면 기본값(25/5)으로 대기 화면을 보여주고, 진행 중에 Esc 를 누르면 확인 후 나가며 타이머는 멈춤
- Output/result: 남은 시간, 현재 구간(집중/휴식), 완료한 집중 횟수
- Exceptions: 0 이하 또는 잘못된 값은 입력 거부
- Acceptance criteria:
  - [ ] 시작, 일시정지, 리셋이 동작함
  - [ ] 구간이 끝나면 알림 후 다음 구간으로 전환됨
  - [ ] 완료 횟수가 화면에 표시됨

<a id="f-05"></a>

### F-05 단일 실행 파일 배포 (Windows)

- User: exe 를 받는 지인·동료
- Purpose: 설치 없이 파일 하나로 실행
- Input: 없음 (더블클릭 또는 터미널 실행)
- Behavior: `go build` 결과물 하나로 동작. Go 설치 불필요
- Output/result: 홈 메뉴 화면 도달
- Exceptions: 서명 없는 exe 는 Windows SmartScreen 경고가 뜰 수 있음 (안내 문구로 대응)
- Acceptance criteria:
  - [ ] Go 가 없는 Windows PC 에서 exe 하나로 홈 화면까지 도달함

<a id="f-06"></a>

### F-06 macOS/Linux 지원

- User: macOS/Linux 를 쓰는 지인
- Purpose: 같은 코드로 다른 OS 에서도 실행
- Input: 없음
- Behavior: 종료 예약 명령과 부팅 시각 조회를 OS별로 분리 (빌드 태그). 명령 형태와 필요한 권한은 구현 시 확인
- Output/result: OS별 실행 파일, 종료 타이머 동작
- Exceptions: 권한 부족으로 종료 예약이 거부되면 이유를 화면에 알림
- Acceptance criteria:
  - [ ] macOS 와 Linux 에서 빌드되고 홈 화면까지 실행됨
  - [ ] 각 OS에서 종료 타이머 예약/취소가 동작함 (또는 불가 시 안내)

## Non-functional requirements

| Item | Criterion |
| --- | --- |
| Supported environments | Windows 10/11 (Must), macOS/Linux (Should) |
| Performance | 키 입력 뒤 화면 전환에 체감 지연이 없음 |
| Accessibility | 키보드만으로 모든 조작이 가능함. 상태를 색으로만 구분하지 않고 텍스트도 함께 표시함 |
