package main

import (
	"fmt"
	"math"

	"github.com/charmbracelet/lipgloss"
)

// 뽁뽁이: 고정 10 x 4 방울 격자에서 Space 로 하나씩 터뜨림. 방울 크기는 그릴 때 창에 맞춤.

const (
	bubbleCols       = 10
	bubbleRows       = 4
	bubbleCount      = bubbleCols * bubbleRows
	bubbleAnimFrames = 6 // 터지는 애니메이션 길이 (프레임). 33ms 틱에서 약 0.2초
)

type bubbleMode struct {
	popped [bubbleCount]bool
	anim   [bubbleCount]int // 0 이면 애니메이션 없음, 1..bubbleAnimFrames 는 터지는 중인 프레임
	cursor int
	count  int
}

func newBubbleMode() fidgetMode { return &bubbleMode{} }

func (m *bubbleMode) Title() string { return "뽁뽁이" }

func (m *bubbleMode) Keys() [][2]string {
	return [][2]string{{"Space", "뽁"}, {"←→↑↓", "이동"}, {"r", "새 판"}}
}

func (m *bubbleMode) Status() string {
	s := fmt.Sprintf("터뜨림 %d/%d", m.count, bubbleCount)
	if m.count == bubbleCount {
		s += "  다 터뜨림! r 로 새 판"
	}
	return s
}

func (m *bubbleMode) Key(k string) bool {
	col, row := m.cursor%bubbleCols, m.cursor/bubbleCols
	switch k {
	case " ", "space", "enter":
		return m.pop()
	case "r", "R":
		*m = bubbleMode{}
		return true
	case "left":
		return m.moveTo(col > 0, m.cursor-1)
	case "right":
		return m.moveTo(col < bubbleCols-1, m.cursor+1)
	case "up":
		return m.moveTo(row > 0, m.cursor-bubbleCols)
	case "down":
		return m.moveTo(row < bubbleRows-1, m.cursor+bubbleCols)
	}
	return false
}

func (m *bubbleMode) moveTo(ok bool, to int) bool {
	if ok {
		m.cursor = to
	}
	return ok
}

func (m *bubbleMode) pop() bool {
	if m.popped[m.cursor] {
		return false
	}
	m.popped[m.cursor] = true
	m.anim[m.cursor] = 1
	m.count++
	for i := 1; i < bubbleCount; i++ {
		if next := (m.cursor + i) % bubbleCount; !m.popped[next] {
			m.cursor = next
			break
		}
	}
	return true
}

func (m *bubbleMode) Step() bool {
	live := false
	for i, f := range m.anim {
		if f == 0 {
			continue
		}
		if f++; f > bubbleAnimFrames {
			f = 0
		} else {
			live = true
		}
		m.anim[i] = f
	}
	return live
}

// bubbleGeom 은 캔버스 점 크기에 맞춘 격자 배치. 방울 하나가 차지하는 정사각형 한 변이 p 점 (4 이상이면 4의 배수라 방울이 칸 경계에 맞음)
type bubbleGeom struct {
	p, ox, oy int
	r, gap    float64
}

func newBubbleGeom(w, h int) bubbleGeom {
	p := max(1, min(w/bubbleCols, h/bubbleRows))
	ox, oy := max(0, (w-p*bubbleCols)/2), max(0, (h-p*bubbleRows)/2)
	if p >= 4 {
		p = p / 4 * 4
		ox, oy = max(0, (w/2-p/2*bubbleCols)/2)*2, max(0, (h/4-p/4*bubbleRows)/2)*4
	}
	gap := 0.0
	switch {
	case p >= 12:
		gap = float64(p / 4)
	case p >= 6:
		gap = 2
	case p >= 3:
		gap = 1
	}
	return bubbleGeom{p: p, ox: ox, oy: oy, r: math.Max(0.5, (float64(p)-gap)/2), gap: gap}
}

func (g bubbleGeom) center(i int) (float64, float64) {
	half := float64(g.p) / 2
	return float64(g.ox+i%bubbleCols*g.p) + half, float64(g.oy+i/bubbleCols*g.p) + half
}

type bubblePlot func(x, y int)

// bubbleEllipse 는 속이 찬 타원에서 안쪽 비율(inner, 0 이면 꽉 참) 만큼 파낸 모양을 찍음
func bubbleEllipse(plot bubblePlot, cx, cy, rx, ry, inner float64) {
	for y := int(math.Floor(cy - ry)); y <= int(math.Ceil(cy+ry)); y++ {
		for x := int(math.Floor(cx - rx)); x <= int(math.Ceil(cx+rx)); x++ {
			dx, dy := (float64(x)+0.5-cx)/rx, (float64(y)+0.5-cy)/ry
			if d := dx*dx + dy*dy; d <= 1 && d >= inner*inner {
				plot(x, y)
			}
		}
	}
}

func bubbleDrawIdle(plot bubblePlot, cx, cy, r float64) {
	if r < 2.5 {
		bubbleEllipse(plot, cx, cy, r, r, 0)
		return
	}
	hx, hy := int(math.Floor(cx-0.4*r)), int(math.Floor(cy-0.45*r))
	bubbleEllipse(func(x, y int) {
		if x != hx || y != hy {
			plot(x, y)
		}
	}, cx, cy, r, r, 0)
}

func bubbleDrawDent(plot bubblePlot, cx, cy, r float64) {
	bubbleEllipse(plot, cx, cy, math.Max(1, r*0.6), math.Max(0.5, r*0.2), 0)
}

// bubbleDrawPop 은 터지는 프레임 f(1..bubbleAnimFrames) 하나를 찍음: 찌그러짐 → 링 → 더 큰 얇은 링 → 불꽃 → 점 → 납작해짐
func bubbleDrawPop(plot bubblePlot, cx, cy, r, gap float64, f int) {
	reach := r + gap/2
	switch f {
	case 1:
		bubbleEllipse(plot, cx, cy, r*1.15, math.Max(0.5, r*0.75), 0)
	case 2:
		bubbleEllipse(plot, cx, cy, r, r, math.Max(0.4, 1-1.2/r))
	case 3:
		bubbleEllipse(plot, cx, cy, reach, reach, math.Max(0.5, 1-1.0/reach))
	case 4:
		d := reach * 0.7
		for _, s := range [][2]float64{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
			bubbleEllipse(plot, cx+s[0]*d, cy+s[1]*d, 0.5, 0.5, 0)
		}
	case 5:
		bubbleEllipse(plot, cx, cy, math.Max(0.5, r*0.3), math.Max(0.5, r*0.3), 0)
	default:
		bubbleDrawDent(plot, cx, cy, r)
	}
}

func (m *bubbleMode) Rows(cw, ch int) [][]seg {
	cv := newCanvas(cw, ch)
	cells := make([]lipgloss.Color, cv.cw*cv.ch)
	for i := range cells {
		cells[i] = colorTextMuted
	}
	plotAs := func(col lipgloss.Color) bubblePlot {
		return func(x, y int) {
			if x < 0 || y < 0 || x >= cv.W() || y >= cv.H() {
				return
			}
			cv.Set(x, y)
			cells[y/4*cv.cw+x/2] = col
		}
	}

	g := newBubbleGeom(cv.W(), cv.H())
	draw := func(i int) {
		cx, cy := g.center(i)
		switch f := m.anim[i]; {
		case i == m.cursor && f == 0 && !m.popped[i]:
			bubbleDrawIdle(plotAs(colorAccent), cx, cy, g.r)
		case i == m.cursor && f == 0:
			bubbleDrawDent(plotAs(colorAccent), cx, cy, g.r)
		case f == 1:
			bubbleDrawPop(plotAs(colorText), cx, cy, g.r, g.gap, f)
		case f > 1:
			bubbleDrawPop(plotAs(colorTextMuted), cx, cy, g.r, g.gap, f)
		case m.popped[i]:
			bubbleDrawDent(plotAs(colorTextFaint), cx, cy, g.r)
		default:
			bubbleDrawIdle(plotAs(colorText), cx, cy, g.r)
		}
	}
	for i := 0; i < bubbleCount; i++ {
		if i != m.cursor {
			draw(i)
		}
	}
	draw(m.cursor)

	rows := make([][]seg, max(ch, 0))
	for cy := range rows {
		rows[cy] = cv.RowFunc(cy, func(cx int) lipgloss.Color { return cells[cy*cv.cw+cx] })
	}
	return rows
}
