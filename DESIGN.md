# 11_shutdown_timer — Design system

[← Hub](.workflow/README.md) · Prev: [3. UX flowchart](.workflow/03-flow.md) · Next: [6. Mid-fi wireframe](.workflow/06-wireframe/index.html)

Created: 2026-11-05

<!-- wf:design
status: done
-->

> The single source of truth for UI. Read it before any UI work or UI fix. Add a token or component here before using it.
> Source: 현재 종료 타이머 화면(코드)을 기준으로 정리함. 레퍼런스 단계는 lite 모드라서 건너뜀.
> Done when: tokens and core components are defined and the user approved → `status: done`. It keeps changing after that; log every change.

## At a glance

- 터미널(TUI) 앱이라 값의 단위는 px 가 아니라 "칸(cell)"이고, 색은 ANSI 16색 번호를 씀 (글꼴은 사용자 터미널이 정함)
- 강조색 = 밝은 cyan, 시간 단계 상태색 = 초록/노랑/빨강, 보조 글자 = 회색 두 단계
- 둥근 모서리 박스 하나 안에 모든 화면을 담음 (넓은 여백)

## Principles

- 색마다 역할이 하나: 강조(조작·제목) / 상태(PC 종료까지 남은 시간) / 회색(보조)
- 상태는 색에만 의존하지 않고 글자(배지)로도 표시함
- 박스는 한 겹만 쓰고, 도움말 키는 항상 하단 한 곳에 모음
- 빨강은 오류와 "PC가 곧 꺼짐"에만 씀. 모니터의 높은 사용률은 노랑(warn)까지만 쓰고 포모도로에는 쓰지 않음

## Tokens

Code uses these names, never raw values. 값은 ANSI 16색 번호이고 실제 색은 터미널 테마를 따름.

### Color

대비 값은 Windows Terminal 기본 테마(Campbell, 배경 `#0C0C0C`) 기준 계산값임. 다른 테마에서는 달라짐.

| Token | ANSI | Use | 대비(배경 기준) |
| --- | --- | --- | --- |
| color.text | 15 | 본문, 값(시각·남은 시간) | 17.5 |
| color.text-muted | 7 | 도움말, 보조 설명, 비활성 메뉴 | 12.2 |
| color.text-faint | 8 | 테두리, 구분선, 진행률 막대의 빈 칸 (글자에는 쓰지 않음) | 4.3 |
| color.accent | 14 | 제목, 키 번호, 입력 프롬프트 | 11.3 |
| color.ok | 10 | 남은 시간 10분 초과, 성공 메시지, 포모도로 휴식 | 8.5 |
| color.warn | 11 | 남은 시간 10분 이하, 경고 메시지 | 16.9 |
| color.danger | 9 | 남은 시간 1분 이하, 오류 메시지 | 5.1 |
| color.on-state | 0 | 상태색 배경 위 글자 (배지) | 5.1 이상 |

### Typography

| Token | Value | Use |
| --- | --- | --- |
| font.family | 사용자 터미널 글꼴 (고정폭) | 앱이 바꿀 수 없음 |
| text.title | color.accent | 화면 제목 |
| text.body | color.text | 값, 메뉴 이름 |
| text.caption | color.text-muted | 힌트, 보조 설명 |
| text.digits | `█` 2칸 × 3×5 격자, 글자 사이 2칸 | 남은 시간 큰 숫자 (폭 ≥ 60, 높이 ≥ 21 일 때만) |

한글은 2칸 폭으로 계산함. 폭 계산은 항상 `lipgloss.Width` 를 씀.

### Spacing · border · motion

| Token | Value |
| --- | --- |
| space.1 / 2 | 1칸 / 2칸 (박스 안쪽 왼쪽 여백 2칸, 오른쪽 여백 2칸) |
| space.row | 섹션 사이에 빈 줄 1개 |
| box.width | 안쪽 폭 = 창 폭 − 6, 최소 50 ~ 최대 100 (전체화면에서 넓게 씀). 가운데 정렬 |
| border.style | 둥근: `╭ ╮ ╰ ╯`, 가로 `─`, 세로 `│`, 구분선 `├ ┤` |
| progress.glyph | 채움 `━`(상태색), 빈 칸 `━`(color.text-faint), 끝에 ` NNN%` |
| breakpoint.big | 폭 ≥ 60 그리고 높이 ≥ 21 이면 큰 숫자, 아니면 한 줄 숫자 |
| motion.tick | 250ms (화면 갱신, 시계는 1초 단위) |
| motion.intro | 2350ms, 아무 키나 누르면 건너뜀 |

## Components

| Component | Purpose | Variants | States | Tokens | Screens |
| --- | --- | --- | --- | --- | --- |
| Frame | 모든 화면의 박스 (머리줄 / 본문 / 키 안내 / 메시지) | 메인, 입력 | 기본 | border.style, color.text-faint | 전체 |
| Header | 앱 이름(왼쪽)과 현재 시각(오른쪽) | - | 기본 | text.title, color.text-faint | home, timer-main, pomo-main |
| Badge | 현재 상태 글자 | 예약됨, 대기, 집중, 휴식, 일시정지 | 상태색 배경 + color.on-state 글자 (대기·일시정지는 color.text-muted 배경) | color.ok/warn/danger/accent, color.on-state | timer-main, pomo-main |
| Big digits | 남은 시간 | 큰 숫자, 한 줄 | 시간 단계별 색 | text.digits, 상태색 | timer-main, pomo-main |
| Progress bar | 경과 비율 | - | 0~100% | progress.glyph, 상태색 | timer-main, pomo-main |
| Menu row | `[키] 이름  힌트` 한 줄 | 활성, 비활성 | 비활성이면 이름이 color.text-muted | color.accent(키), text.body, text.caption | timer-main, pomo-main |
| List item | 홈의 앱 한 줄 `[번호] 이름  한 줄 설명  상태 접미` (번호는 1~9 까지만) | 선택, 일반 | 선택 시 `▸` 표시와 color.accent 이름, 설명은 color.text-muted. 종료 타이머 예약 중이면 접미에 상태색으로 남은 시간 | color.accent, text.body, text.caption | home |
| Input | 한 줄 입력 | - | 기본 / 오류(아래 줄에 color.danger 문구) | color.accent(프롬프트), color.danger | timer-new, pomo-setup |
| Message line | 하단 한 줄 안내 | 정보, 성공, 경고, 오류 | 비어 있으면 빈 줄 | color.text-muted, ok, warn, danger | home, timer-main, pomo-main |
| Confirm | 되돌릴 수 없는 동작 확인 | - | `y` / `n` | color.warn(문구), color.accent(키) | pomo-leave |
| Meter | 사용률 한 줄 (라벨, 막대, %, 값) | CPU / 메모리 / 디스크 | 70% 미만 color.accent, 70% 이상 color.warn, 읽기 실패는 `-` (color.text-muted) | progress.glyph, color.accent, color.warn | sysmon |
| Sparkline | 최근 추이 한 줄 | 60칸 | 일시정지 중에는 고정 | `▁▂▃▄▅▆▇█`, color.accent | sysmon |
| Tab row | 기간 선택 | 선택, 일반 | 선택은 `[ 이름 ]` 굵게 + color.accent, 나머지는 color.text-muted | color.accent, text.caption | claude-usage, codex-usage |
| Table | 숫자 표 (헤더, 행, 합계 행) | 기간별, 모델별 | 숫자는 오른쪽 정렬·천 단위 쉼표, 비면 안내 문구 | text.body, text.caption | claude-usage, codex-usage |
| Bar row | 일별 막대 한 줄 (날짜, `█` 막대, 값) | - | 오늘은 color.accent, 나머지는 color.text-muted | color.accent | claude-usage, codex-usage |
| Check row | 할 일 한 줄 `[ ]` / `[x]` | 선택, 일반, 완료 | 완료는 color.text-muted + 끝에 `완료` 글자 (취소선은 터미널마다 달라 쓰지 않음) | color.accent, color.text-muted | todo |
| Lap row | 랩 번호, 구간, 누적 시간 | 최근 3개 | - | text.body | stopwatch |
| Intro | 시작 화면 | - | 로고 → 부제목 → 로딩 막대 → 준비 완료 | color.accent, color.ok, color.text-faint | intro |

## Patterns

- Layout grid: 박스 하나를 화면 가운데에 두고, 위에서부터 머리줄 → 상태 → 값(큰 숫자) → 진행률 → 키 안내 → 메시지 순서
- Empty / loading / error states: 비어 있음은 한 줄 문구(예: "등록된 앱 없음"), 오류는 해당 위치 아래 줄에 color.danger 로 표시
- Forms: 입력은 박스 아래에 프롬프트 한 줄. 잘못된 입력은 같은 화면에 남고 안내만 바뀜. `0`/Esc 는 뒤로

## Avoid

- 글자에 color.text-faint(8) 쓰기: 대비가 4.3 이라 4.5 에 못 미침. 현재 코드의 일부 힌트 문구가 이 색을 쓰고 있어 9단계에서 text-muted 로 바꿈
- `대기` 배지를 흰 글자 + 회색(8) 배경으로 쓰기: 대비 4.1. 회색(7) 배경 + 검정 글자로 바꿈
- `▁▂▃…█`, `━`, `▸` 는 일부 한글 글꼴 설정에서 2칸으로 그려질 수 있는 문자(모호폭)임 → 9단계에서 실제 터미널로 확인하고, 어긋나면 ASCII 대체 문자를 씀
- 이모지, 장식용 아이콘, 상태를 색으로만 구분하기
- 박스 안에 박스를 중첩하기, 장식용 선
- 빨강을 포모도로 같은 "PC 종료와 무관한" 상태에 쓰기

### Deliberate exceptions

| Pattern | Why it fits this product |
| --- | --- |
| 인트로 로고의 파랑→cyan→흰색 그라데이션 | 시작 화면 2초에만 나오고 건너뛸 수 있음. 터미널 장난감 느낌을 주는 첫인상용이고 ANSI 색만 씀 |
| 인트로 부제목의 글자 간격 | 인트로에서만 쓰는 타이핑 효과. 본 화면에는 쓰지 않음 |
| 테두리 박스와 구분선 | 터미널에서 영역을 나누는 거의 유일한 수단임. 한 겹만 쓰고 구분선은 머리줄·키 안내·메시지 구획에만 씀 |
| 키 번호 `[1] [2] [3]` | 장식 번호가 아니라 실제로 누르는 키임 |
| 홈 배경 애니메이션 5종 (오로라·별빛·디지털 비·레트로 격자·디더링 행성) | 이 프로젝트의 목적이 재미이고, 홈 박스 바깥 여백에만 깔림(박스 안은 불투명). 설정에서 끌 수 있음(기본 켬: H). RGB 직접 색은 `grid.go`·`bg.go`·`transition.go`·`arcade.go` 안에서만 쓰고, 터미널 색 지원에 따라 256색·16색으로 자동 대체함. `TUI_ANIM=off` 또는 색이 없는 터미널이면 꺼짐 |
| 화면 전환 4종 (와이프·원형·디졸브·밀어내기, 0.7초) | 앱 진입·복귀 때만. 키를 누르면 즉시 끝남. 설정에서 끌 수 있음(기본: 3 디졸브) |
| 아케이드 조작부 (패널형) | 홈 박스 아래에 조이스틱·버튼을 그려 키 입력에 반응시키는 순수 장식. 설정에서 켬(기본 끔). 창 세로가 모자라면 자동으로 숨김. 숫자 버튼은 실제 숫자키와 일치 |

### 추가 토큰 (F-12 디스크 트리맵)

| Token | Value | 용도 |
| --- | --- | --- |
| color.viz-1..6 | ANSI 12, 13, 14, 10, 11, 6 | 트리맵 사각형 바탕. 글자는 color.on-state. 사각형마다 이름·크기 글자가 있어 색이 유일한 단서가 아님 |
| color.select | ANSI 4 (짙은 파랑) | 선택된 칸 바탕 + color.text 굵은 글자 + `▸`. 흰 바탕은 눈부시고 검정은 선택 안 된 것처럼 보여서 고름 |

10/11/14 는 ok/warn/accent 와 같은 색이라 의미가 겹침. 트리맵 안에서만 범주색으로 쓰는 예외임.

### 추가 컴포넌트

| Component | 설명 | 화면 |
| --- | --- | --- |
| Treemap | 폴더 크기를 면적으로 보여주는 사각형 배치(squarified). 배경색 칸, 크기순, 작은 항목은 `기타 (N개)` 로 묶음. 창이 작으면 Bar row 목록으로 대체 | 디스크 사용량 |
| Detail line | `▸ 이름   크기  부모의 N%` 한 줄 + 전체 경로 한 줄 | 디스크 사용량 |
| Reader | 웹 페이지 본문을 박스 폭에 맞춰 줄바꿈해 스크롤로 읽음. 제목은 굵은 accent, 링크는 accent + `[n]`(muted), 인용 `│`·코드는 muted, 목록 `•`/`◦`. 페이지에서 온 글자는 제어문자를 모두 걸러서 출력함 | 웹 브라우저 |

### Motion tokens (홈 꾸밈 전용)

| Token | Value | 용도 |
| --- | --- | --- |
| motion.bg-frame | 66ms (약 15fps) | 홈 배경 갱신 주기 |
| motion.trans-frame | 33ms (약 30fps) | 전환 갱신 주기 |
| motion.trans-duration | 700ms | 화면 전환 길이 |
| motion.press | 방향 220ms · A/B 180ms · 숫자 300ms | 조작부 눌림 표시 시간 |
| color.panel-fx | rgb(12,12,12) | 배경 위에 얹은 박스 안쪽 바탕 |

움직임은 상태를 설명하는 경우(전환=화면이 바뀜, 조작부=키 입력)에만 쓰고, 배경은 읽는 글자 뒤 여백에만 둠.

## Change log

| Date | Change | Why |
| --- | --- | --- |
| 2026-11-05 | created | 현재 코드의 색·박스·여백 값을 토큰으로 정리하고 홈 목록·확인 문구 컴포넌트를 추가 |
| 2026-11-05 | Meter, Sparkline, Tab row, Table, Bar row, Check row, Lap row 추가, List item 을 한 줄형으로 변경, 빨강 사용 원칙 문구 정정 | 앱 5개(F-07~F-11) 추가로 필요해짐. 기존 원칙이 오류에도 빨강을 쓰는 것과 어긋나 있었음 |
| 2026-11-05 | 홈 배경 5종·화면 전환 4종·아케이드 조작부 예외와 Motion tokens 추가 | 재미 목적의 홈 꾸밈 기능(설정에서 선택·끔). 지침의 "움직임은 상태 설명용" 원칙에 맞게 위치와 끄는 방법을 제한함 |
| 2026-11-05 | color.viz-1..6, color.select, Treemap·Detail line·Reader 추가. 조작부 버튼 글자를 실제 키(Enter/Esc)로, 전환의 흰색을 연한 하늘색으로 낮춤 | 앱 2개(F-12, F-13) 추가와 사용자 피드백(A/B는 없는 키, 흰색이 눈부심, 검정 선택은 선택 안 된 느낌) |
