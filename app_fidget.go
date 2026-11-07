package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// 멍때리기 (F-16). 스피너·말랑이·뽁뽁이·모래·생명 게임·불꽃놀이 모드를 Tab 으로 바꿔가며 가지고 놂 (모드는 fidget_*.go).

var fidgetInfo = AppInfo{
	ID:   "fidget",
	Name: "멍때리기",
	Desc: "스피너, 뽁뽁이, 모래, 불꽃놀이 등",
	New:  newFidgetApp,
}

const (
	fidgetFrame   = 33 * time.Millisecond // 틱 간격. Step 한 번이 이 시간만큼의 움직임임
	fidgetMaxRows = 14                    // 본문 최대 줄 수 (넓은 창에서 너무 커지지 않게)
	fidgetMinRows = 3
	fidgetChrome  = 9 // 본문 줄을 뺀 박스 줄 수: 윗선, 머리, 구분선, 탭, 구분선, 키 안내, 구분선, 메시지, 아랫선
)

// fidgetMode 는 멍때리기 안의 모드 하나. 상태는 창 크기와 무관해야 하고 Rows 가 매번 받은 크기에 맞춰 그림.
type fidgetMode interface {
	// Title 은 탭에 보이는 이름 (한글 3글자 안팎)
	Title() string
	// Key 는 호스트가 처리하지 않은 키(Tab·Esc 제외)를 받음. 반응했으면 true
	Key(k string) bool
	// Step 은 한 프레임(fidgetFrame)만큼 진행시키고, 계속 움직이는 중이면 true. false 를 돌려주면 틱을 멈추고 다음 키 입력 때 다시 시작함
	Step() bool
	// Rows 는 본문을 정확히 ch 줄로 돌려줌. 줄마다 폭의 합은 cw 이하여야 함 (넘으면 박스 테두리가 깨짐)
	Rows(cw, ch int) [][]seg
	// Status 는 맨 아랫줄에 보일 짧은 문구 (예: "12.4바퀴"). 없으면 빈 문자열
	Status() string
	// Keys 는 이 모드의 키 안내 ({키, 설명}). 호스트가 Tab 과 Esc 안내를 뒤에 붙임
	Keys() [][2]string
}

type fidgetApp struct {
	gen     int
	modes   []fidgetMode
	cur     int
	ticking bool
}

func newFidgetApp(env *Env) App {
	return &fidgetApp{
		gen:   newGen(),
		modes: []fidgetMode{newSpinnerMode(), newSquishyMode(), newBubbleMode(), newSandMode(), newLifeMode(), newFireworksMode()},
	}
}

func (a *fidgetApp) Init() tea.Cmd { return nil }

func (a *fidgetApp) tag() string { return "fidget" }

// wake 는 틱이 멈춰 있으면 다시 시작함 (이미 돌고 있으면 틱을 겹쳐 걸지 않음)
func (a *fidgetApp) wake() tea.Cmd {
	if a.ticking {
		return nil
	}
	a.ticking = true
	return tickCmd(fidgetFrame, a.gen, a.tag())
}

func (a *fidgetApp) Update(msg tea.Msg) (App, tea.Cmd) {
	if cmd, ok := forwardHangul(msg, func(m tea.Msg) tea.Cmd { _, c := a.Update(m); return c }); ok { // 한글 입력 상태에서도 단축키가 듣게 함
		return a, cmd
	}
	switch m := msg.(type) {
	case appTickMsg:
		if m.Gen != a.gen || m.Tag != a.tag() || !a.ticking {
			return a, nil
		}
		if a.modes[a.cur].Step() {
			return a, tickCmd(fidgetFrame, a.gen, a.tag())
		}
		a.ticking = false
		return a, nil
	case tea.KeyMsg:
		switch k := m.String(); k {
		case "esc", "q":
			return a, goHome()
		case "tab":
			a.cur = (a.cur + 1) % len(a.modes)
		case "shift+tab":
			a.cur = (a.cur + len(a.modes) - 1) % len(a.modes)
		default:
			a.modes[a.cur].Key(k)
		}
		return a, a.wake()
	}
	return a, nil
}

// bodyRows 는 창 높이에서 박스 장식을 뺀 본문 줄 수
func (a *fidgetApp) bodyRows(c Ctx) int {
	return max(fidgetMinRows, min(fidgetMaxRows, c.H-fidgetChrome))
}

func (a *fidgetApp) View(c Ctx) string {
	m := a.modes[a.cur]
	cw, ch := c.In()-4, a.bodyRows(c)

	titles := make([]string, len(a.modes))
	for i, md := range a.modes {
		titles[i] = md.Title()
	}
	f := NewFrame(c)
	fidgetTabs(f, titles, a.cur)

	rows := m.Rows(cw, ch)
	for i := 0; i < ch; i++ {
		if i < len(rows) {
			f.Row(rows[i]...)
		} else {
			f.Blank()
		}
	}
	f.Rule()
	f.MenuInline(append(append([][2]string{}, m.Keys()...), [2]string{"Tab", "모드"}, [2]string{"Esc", "홈으로"})...)
	f.Rule().Message(m.Status(), ToneInfo)
	return f.String()
}

// fidgetTabs 는 모드 탭 줄. 모드가 많아 한 줄에 다 안 들어가면 현재 모드 주변만 보이고 잘린 쪽에 … 를 붙임
func fidgetTabs(f *Frame, titles []string, cur int) {
	item := func(i int) seg {
		if i == cur {
			return sgBold("[ "+titles[i]+" ]", colorAccent)
		}
		return sg("  "+titles[i]+"  ", colorTextMuted)
	}
	width := func(lo, hi int) int {
		w := 0
		for i := lo; i <= hi; i++ {
			w += item(i).width()
		}
		if lo > 0 {
			w += 1
		}
		if hi < len(titles)-1 {
			w += 1
		}
		return w
	}
	avail := f.In() - 4
	lo, hi := cur, cur
	for grew := true; grew; {
		grew = false
		if hi+1 < len(titles) && width(lo, hi+1) <= avail {
			hi++
			grew = true
		}
		if lo > 0 && width(lo-1, hi) <= avail {
			lo--
			grew = true
		}
	}
	// 다 들어가면 Frame.Tabs 와 같은 앞 여백(6칸)을 둬서 다른 앱의 탭 줄과 위치를 맞춤
	var segs []seg
	if lo == 0 && hi == len(titles)-1 && width(lo, hi)+6 <= avail {
		segs = append(segs, sg(sp(6), colorTextMuted))
	}
	if lo > 0 {
		segs = append(segs, sg("…", colorTextMuted))
	}
	for i := lo; i <= hi; i++ {
		segs = append(segs, item(i))
	}
	if hi < len(titles)-1 {
		segs = append(segs, sg("…", colorTextMuted))
	}
	f.Row(segs...)
}
