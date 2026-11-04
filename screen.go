package main

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// 홈·설정 화면을 칸 격자로 합성함: 배경 + 가운데 박스 + (켜져 있으면) 아래 아케이드 조작부

var boxPanel = vec{12, 12, 12} // 박스 안쪽 바탕 (뒤의 배경 애니메이션이 비치지 않게)

// boxSize 는 박스 문자열의 폭과 줄 수
func boxSize(box string) (w, h int) {
	lines := strings.Split(box, "\n")
	for _, l := range lines {
		w = max(w, lipgloss.Width(l))
	}
	return w, len(lines)
}

func (m model) homeBoxLines(c Ctx) int {
	_, h := boxSize(m.viewHome(c))
	return h
}

// arcadeFits 는 조작부를 켰고 창에 들어가는지 (창이 작으면 자동으로 숨김)
func (m model) arcadeFits(c Ctx) bool {
	if !m.set.Arcade || !animEnabled() || c.W <= 0 || c.H <= 0 {
		return false
	}
	// 홈 박스 기준으로 판단함 (설정 화면은 더 낮아서 같은 기준이면 항상 들어감. 설정 화면의 안내 문구가 이 함수를 부르므로 boxView 를 쓰면 무한 재귀가 됨)
	bw, bh := boxSize(m.viewHome(c))
	return bh+arcadeH <= c.H && bw <= c.W
}

// fxHome 은 칸 격자 합성 경로를 쓸지
func (m model) fxHome(c Ctx) bool {
	if !animEnabled() || c.W <= 0 || c.H <= 0 {
		return false
	}
	return m.set.Bg != "" || m.arcadeFits(c)
}

// 앱 화면에도 배경과 조작부가 이어짐. 조작부가 켜져 있고 창에 들어가면 앱에는 조작부 높이를 뺀 창 크기를 알려줘서 앱 박스와 조작부가 함께 들어가게 함
const appMinH = 17 // 앱이 제대로 그려지는 최소 높이 (DESIGN.md 의 가장 작은 창)

func (m model) appCtx(c Ctx) (Ctx, bool) {
	if m.set.Arcade && animEnabled() && c.H-arcadeH >= appMinH {
		c.H -= arcadeH
		return c, true
	}
	return c, false
}

func (m model) fxApp(c Ctx) bool {
	if !animEnabled() || c.W <= 0 || c.H <= 0 || m.app == nil {
		return false
	}
	_, arc := m.appCtx(c)
	return m.set.Bg != "" || arc
}

func (m model) homeGrid(c Ctx) *grid {
	label := "B 종료"
	if m.ph == phSettings {
		label = "B 뒤로"
	}
	return m.composeGrid(c, m.boxView(c), m.arcadeFits(c), label)
}

func (m model) appGrid(c Ctx) *grid {
	c2, arc := m.appCtx(c)
	return m.composeGrid(c, m.app.View(c2), arc, "B 뒤로")
}

// composeGrid 는 배경 위에 박스를 가운데 놓고, arcade 면 박스 바로 아래에 조작부를 붙임
func (m model) composeGrid(c Ctx, box string, arcade bool, label string) *grid {
	g := newGrid(c.W, c.H)
	panel := ""
	if m.set.Bg != "" && animEnabled() {
		bgFill(g, m.set.Bg, m.clock())
		panel = colSeq(true, boxPanel)
	}
	bw, bh := boxSize(box)
	bg := parseGrid(box, bw, bh)
	total := bh
	if arcade {
		total += arcadeH
	}
	x0, y0 := max(0, (c.W-bw)/2), max(0, (c.H-total)/2)
	g.blit(bg, x0, y0, panel)
	if arcade {
		g.blit(arcadeGrid(max(bw, 52), len(registry), m.pulses, c.Now, label), x0, y0+bh, "")
	}
	return g
}

// screenGrid 는 지금 화면 전체를 w×h 격자로 돌려줌 (전환 효과의 입력)
func (m model) screenGrid() *grid {
	c := m.ctx()
	switch {
	case m.ph == phIntro:
		return parseGrid(m.viewIntro(), c.W, c.H)
	case m.ph == phApp && m.app != nil:
		if m.fxApp(c) {
			return m.appGrid(c)
		}
		return parseGrid(c.Place(m.app.View(c)), c.W, c.H)
	case m.fxHome(c):
		return m.homeGrid(c)
	}
	return parseGrid(c.Place(m.boxView(c)), c.W, c.H)
}

// clock 은 프로그램 시작 후 흐른 초 (배경 애니메이션 시간)
func (m model) clock() float64 { return time.Since(m.start).Seconds() }
