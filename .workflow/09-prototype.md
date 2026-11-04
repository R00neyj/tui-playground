# 11_shutdown_timer — Prototype

[← Hub](README.md) · Prev: [6. Mid-fi wireframe](06-wireframe/index.html) · Next: [10. Usability test](10-usability.md)

Created: 2026-11-05

<!-- wf:prototype
status: draft
-->

> Basis: [feature spec](02-spec.md), approved [wireframe](06-wireframe/index.html), [DESIGN.md](../DESIGN.md). Done when: Must features work and run instructions are written → `status: done`

## At a glance

- Go + Bubble Tea 단일 exe. 앱 9개(종료 타이머, 포모도로, 스톱워치, 시스템 모니터, Claude·Codex 사용량, 할 일, 디스크 사용량, 웹 브라우저)와 홈 꾸밈(배경·전환·조작부·설정)을 구현함
- 사용성 평가용 기준 브랜치: `feature/arcade-home` (평가 대상으로 사용자가 선택함)
- 이 문서는 아직 `draft`: 실제 터미널에서의 디자인 점검과 사용자 확인이 남아 있음

## Tech choices

Decided in [tech design](08-tech.md); in lite mode record them here.

| Item | Choice | Reason |
| --- | --- | --- |
| 언어·UI | Go 1.27, Bubble Tea 1.3, lipgloss 1.1 | 실행 파일 하나로 배포, Go 미설치 PC에서도 실행 |
| 홈 꾸밈 | 화면을 칸 격자로 풀어 합성 (grid.go) | 배경·전환·조작부를 한 경로에서 처리, 트루컬러→256→16색 자동 대체 |
| 웹 글 읽기 | golang.org/x/net/html + x/text | 글 본문 추출과 EUC-KR 등 문자 코드 변환 |
| 자바스크립트 페이지 | 설치된 Chrome/Edge/Brave 를 headless 로 실행 (의존 라이브러리 없음) | 별도 렌더링 엔진을 넣지 않고 exe 크기를 유지 |

## How to run

```
go vet ./...
go test ./...
go build -ldflags "-s -w" -o TUI-playground.exe .
.\TUI-playground.exe
```

색이 안 나오면 환경 변수 `NO_COLOR` 가 켜져 있는지 확인할 것 (켜져 있으면 배경·전환이 꺼짐).

## Implementation scope

| Feature | Status | Notes |
| --- | --- | --- |
| F-01 홈 메뉴 | implemented | 앱 9개, 설정 키 `s`, 아케이드 조작부 선택 |
| F-02 종료 타이머 | implemented | Windows 는 실제 shutdown.exe, macOS/Linux 는 컴파일만 확인 |
| F-04 포모도로 | implemented | 벨 소리는 실제 터미널에서 미확인 |
| F-07~F-11 | implemented | 시스템 모니터, Claude·Codex 사용량(Codex 기록 형식 미확인), 스톱워치, 할 일 |
| F-12 디스크 사용량 | implemented | 큰 드라이브 전체 스캔 속도 미확인 |
| F-13 웹 브라우저 | implemented | 자바스크립트 렌더링은 Chrome/Edge 필요 |
| F-14 홈 꾸밈 | implemented | 실제 터미널 끊김 여부 미확인 |

## Differences from the wireframe

| Screen | Change | Reason |
| --- | --- | --- |
| 앱 화면 전체 | 배경·조작부가 홈뿐 아니라 앱 안쪽에도 이어짐 | 사용자 요청 |
| 홈 | 설정 키 `s`, 조작부 패널 추가 | 사용자 요청 (재미 목적) |
| 트리맵·웹 브라우저 | 와이어프레임 없이 구현 | 급하게 추가한 앱 (F-12, F-13). 화면은 DESIGN.md 컴포넌트로 작성하고 새 컴포넌트는 DESIGN.md에 기록 |

## Design check

Captures of each core screen compared against [DESIGN.md](../DESIGN.md) and the wireframe.

| Screen | Capture | Tokens/components match | Off from DESIGN.md / slop |
| --- | --- | --- | --- |
| home | 미확인 (텍스트·HTML 변환으로만 확인) | 대체로 yes | 배경·조작부 색은 DESIGN.md 'Deliberate exceptions' 로 기록함. 실제 터미널 캡처는 아직 없음 |
| 디스크 트리맵 | 미확인 | yes | 범주색 viz-1..6 은 ok/warn/accent 와 색이 겹쳐 예외로 기록함 |
| 웹 브라우저 | 미확인 | yes | 새 색 토큰 없음 |

## Check results

Summary of `wfctl check` results.

- `go vet ./...`, `go test ./...`(앱 9개 + 홈 꾸밈 + 브라우저), linux/darwin `go vet` 통과 (2026-11-05)
- 실제 Windows Terminal 에서의 눈으로 확인은 사용자가 일부 함 (배경·전환·조작부·디스크·브라우저를 직접 보고 피드백 반영)
