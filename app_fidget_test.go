package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// fakeFidget 은 호스트 동작만 검사하려고 쓰는 가짜 모드
type fakeFidget struct {
	title  string
	steps  int
	live   int // Step 이 true 를 돌려줄 남은 횟수
	keys   []string
	status string
}

func (f *fakeFidget) Title() string     { return f.title }
func (f *fakeFidget) Key(k string) bool { f.keys = append(f.keys, k); f.live = 3; return true }
func (f *fakeFidget) Step() bool {
	f.steps++
	if f.live > 0 {
		f.live--
	}
	return f.live > 0
}
func (f *fakeFidget) Status() string    { return f.status }
func (f *fakeFidget) Keys() [][2]string { return [][2]string{{"Space", "튕기기"}} }
func (f *fakeFidget) Rows(cw, ch int) [][]seg {
	rows := make([][]seg, ch)
	for i := range rows {
		rows[i] = []seg{sg(strings.Repeat("x", cw), colorText)}
	}
	return rows
}

func fakeApp() (*fidgetApp, *fakeFidget, *fakeFidget) {
	a := newFidgetApp(&Env{}).(*fidgetApp)
	m0, m1 := &fakeFidget{title: "가짜1"}, &fakeFidget{title: "가짜2"}
	a.modes = []fidgetMode{m0, m1}
	return a, m0, m1
}

func TestFidgetTabCyclesAndForwardsKeys(t *testing.T) {
	a, m0, m1 := fakeApp()
	a.Update(key(" "))
	if len(m0.keys) != 1 || m0.keys[0] != " " {
		t.Errorf("현재 모드가 키를 받지 못함: %v", m0.keys)
	}
	a.Update(key("tab"))
	if a.cur != 1 {
		t.Fatalf("Tab 으로 모드가 바뀌지 않음")
	}
	a.Update(key("z"))
	if len(m1.keys) != 1 || len(m0.keys) != 1 {
		t.Errorf("키가 현재 모드에만 가야 함: m0=%v m1=%v", m0.keys, m1.keys)
	}
	a.Update(key("tab"))
	if a.cur != 0 {
		t.Errorf("Tab 이 처음으로 돌아오지 않음")
	}
	if len(m0.keys) != 1 {
		t.Errorf("Tab 키가 모드로 전달됨")
	}
}

func TestFidgetEscAndFreshEsc(t *testing.T) {
	a, _, _ := fakeApp()
	if _, cmd := a.Update(key("esc")); cmd == nil {
		t.Errorf("Esc 가 홈으로 가지 않음")
	}
}

func TestFidgetTickLifecycle(t *testing.T) {
	a, m0, _ := fakeApp()
	_, cmd := a.Update(key(" "))
	if cmd == nil || !a.ticking {
		t.Fatalf("키 입력 뒤 틱이 시작되지 않음")
	}
	if _, cmd := a.Update(key(" ")); cmd != nil {
		t.Errorf("이미 틱이 도는데 틱을 또 걸었음")
	}
	tick := appTickMsg{Gen: a.gen, Tag: a.tag()}
	for i := 0; i < 2; i++ {
		if _, cmd := a.Update(tick); cmd == nil {
			t.Fatalf("움직이는 중인데 틱이 끊김 (i=%d)", i)
		}
	}
	if _, cmd := a.Update(tick); cmd != nil || a.ticking {
		t.Errorf("Step 이 false 를 돌려줬는데 틱이 계속됨")
	}
	steps := m0.steps
	a.Update(tick)
	if m0.steps != steps {
		t.Errorf("멈춘 뒤 도착한 틱이 Step 을 불렀음")
	}
	if _, cmd := a.Update(key(" ")); cmd == nil {
		t.Errorf("멈춘 뒤 키를 누르면 틱이 다시 시작돼야 함")
	}
	a.Update(appTickMsg{Gen: a.gen + 1, Tag: a.tag()})
	if m0.steps != steps+0 && m0.steps != steps {
		t.Errorf("다른 인스턴스의 틱이 Step 을 불렀음")
	}
}

func TestFidgetViewAligned(t *testing.T) {
	a, _, m1 := fakeApp()
	m1.status = "상태 문구"
	for _, tabbed := range []bool{false, true} {
		if tabbed {
			a.Update(key("tab"))
		}
		for _, sz := range testSizes {
			c := testCtx(sz[0], sz[1])
			v := a.View(c)
			assertFrame(t, "fidget", c, v)
			if n := strings.Count(v, "\n") + 1; n > sz[1] {
				t.Errorf("%dx%d 창에서 %d줄로 창 높이를 넘음", sz[0], sz[1], n)
			}
		}
	}
	if !strings.Contains(plain(a.View(testCtx(80, 24))), "상태 문구") {
		t.Errorf("상태 문구가 보이지 않음")
	}
}

func TestFidgetRealModesRender(t *testing.T) {
	a := newFidgetApp(&Env{}).(*fidgetApp)
	if len(a.modes) != 3 {
		t.Fatalf("모드가 3개여야 함: %d", len(a.modes))
	}
	for i := range a.modes {
		a.cur = i
		for _, sz := range testSizes {
			c := testCtx(sz[0], sz[1])
			v := a.View(c)
			assertFrame(t, a.modes[i].Title(), c, v)
			if n := strings.Count(v, "\n") + 1; n > sz[1] {
				t.Errorf("%s: %dx%d 창에서 %d줄로 창 높이를 넘음", a.modes[i].Title(), sz[0], sz[1], n)
			}
		}
	}
}

func TestCanvasPrimitives(t *testing.T) {
	c := newCanvas(4, 2) // 8 x 8 점
	if c.W() != 8 || c.H() != 8 {
		t.Fatalf("캔버스 크기: %dx%d", c.W(), c.H())
	}
	c.Set(-1, 0)
	c.Set(0, 99)
	c.Set(100, 100) // 범위 밖은 무시
	for _, r := range c.Rows(colorText) {
		if got := lipgloss.Width(r[0].t); got != 4 || strings.TrimSpace(r[0].t) != "" {
			t.Fatalf("빈 캔버스 줄이 이상함: %q", r[0].t)
		}
	}
	c.Set(0, 0)
	if got := c.cell(0, 0); got != "⠁" {
		t.Errorf("왼쪽 위 점은 U+2801 이어야 함: %U", []rune(got)[0])
	}
	c.Set(1, 3)
	if got := []rune(c.cell(0, 0))[0]; got != 0x2800+0x01+0x80 {
		t.Errorf("오른쪽 아래 점 비트가 다름: %U", got)
	}
	c.Clear()
	c.Line(0, 0, 7, 7)
	if !c.at(0, 0) || !c.at(7, 7) || !c.at(3, 3) {
		t.Errorf("직선이 끝점이나 중간을 지나지 않음")
	}
	c.Clear()
	c.FillCircle(4, 4, 3)
	if !c.at(4, 4) || c.at(0, 0) || c.at(7, 0) {
		t.Errorf("원이 중심만 채우고 모서리는 비워야 함")
	}
	c.Clear()
	c.FillTriangle(0, 0, 7, 0, 0, 7)
	if !c.at(1, 1) || c.at(7, 7) {
		t.Errorf("삼각형 안팎 판정이 다름")
	}
}

func TestCanvasBlockGlyphsWidth(t *testing.T) {
	old := fidgetBlockGlyphs
	fidgetBlockGlyphs = true
	defer func() { fidgetBlockGlyphs = old }()
	c := newCanvas(6, 3)
	c.FillCircle(6, 6, 5)
	for y := 0; y < c.ch; y++ {
		for _, s := range c.Row(y, colorText) {
			if strings.ContainsAny(s.t, "⠀⣿") {
				t.Errorf("블록 모드인데 점자가 섞임")
			}
		}
		w := 0
		for _, s := range c.Row(y, colorText) {
			w += lipgloss.Width(s.t)
		}
		if w != 6 {
			t.Errorf("줄 폭이 %d (기대 6)", w)
		}
	}
}
