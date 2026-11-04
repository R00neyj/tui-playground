package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---- 앱 등록 구조 (F-03) ----
//
// 새 앱을 추가하는 방법: ① app_<이름>.go 에 App 을 구현하고 AppInfo 변수를 만든다
// ② apps.go 의 registry 에 그 변수를 한 줄 넣는다. 그 외 파일은 건드리지 않는다.

// App 은 홈에서 고르는 미니앱 하나. Bubble Tea 모델과 거의 같지만 View 가 창 크기와 시각(Ctx)을 받고,
// 홈으로 돌아가려면 goHome() 명령을 돌려준다. Ctrl+C 는 호스트가 처리함.
type App interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (App, tea.Cmd)
	// View 는 NewFrame(c)...String() 으로 만든 블록을 돌려준다 (가운데 정렬은 호스트가 함)
	View(c Ctx) string
}

// Status 는 홈 목록 줄 끝에 보이는 앱 상태 (예: 종료 타이머의 예약 상태)
type Status struct {
	Badge string // 있으면 상태 배지 (Tone 색 배경)
	Text  string // 배지 뒤 글자 (남은 시간 등)
	Tone  Tone
	Hint  string // 있으면 홈 하단 메시지 줄에 보임
}

// AppInfo 는 registry 에 넣는 앱 설명
type AppInfo struct {
	ID, Name, Desc string
	New            func(env *Env) App
	// Status 는 선택 사항. 앱이 실행 중이 아니어도 홈이 매초 불러서 목록 줄 끝에 표시함
	Status func(env *Env, now time.Time) Status
}

// Env 는 모든 앱이 공유하는 실행 환경
type Env struct {
	Boot int64 // 마지막 부팅 시각 (유닉스 초)
}

// ---- 호스트와 주고받는 메시지 ----

// tickMsg 는 호스트가 250ms 마다 보내는 공용 틱. 앱에도 전달되므로 시계 갱신이나 초 단위 동기화에 쓸 수 있음.
type tickMsg time.Time

// goHomeMsg 를 받으면 호스트가 앱을 버리고 홈으로 돌아감
type goHomeMsg struct{}

func goHome() tea.Cmd { return func() tea.Msg { return goHomeMsg{} } }

// ---- 앱 전용 틱 ----
//
// 앱을 나갔다 다시 들어오면 이전 인스턴스가 걸어둔 틱이 새 인스턴스에 도착할 수 있음.
// 그래서 앱은 생성할 때 newGen() 으로 번호를 받고, 자기 번호가 아닌 appTickMsg 는 무시해야 함.

type appTickMsg struct {
	Gen int
	Tag string
}

var genCounter int

func newGen() int { genCounter++; return genCounter }

func tickCmd(d time.Duration, gen int, tag string) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return appTickMsg{Gen: gen, Tag: tag} })
}

// ---- 구현 전 자리표시 앱 ----

// stubApp 은 아직 만들지 않은 앱 자리에 들어가는 임시 화면. Esc 로 홈에 돌아갈 수 있음.
type stubApp struct{ name string }

func newStub(name string) App    { return &stubApp{name: name} }
func (s *stubApp) Init() tea.Cmd { return nil }
func (s *stubApp) View(c Ctx) string {
	f := NewFrame(c).Blank()
	f.Row(sg(s.name+" — 아직 구현 전", colorTextMuted)).Blank().Rule()
	f.MenuInline([2]string{"Esc", "홈으로"})
	return f.Rule().Message("", ToneInfo).String()
}
func (s *stubApp) Update(msg tea.Msg) (App, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && (k.String() == "esc" || k.String() == "q") {
		return s, goHome()
	}
	return s, nil
}
