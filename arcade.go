package main

import (
	"github.com/charmbracelet/lipgloss"
	"math"
	"time"
)

// 아케이드 조작부 (홈 화면 아래 패널). 방향키를 누르면 조이스틱이 기울고, Enter/Esc/숫자키를 누르면 버튼이 눌림.
// 설정에서 켜고 끌 수 있고 기본값은 끔. 색은 이 패널 전용 RGB (DESIGN.md "Deliberate exceptions > 움직임").

const arcadeH = 7 // 패널 높이(줄)

// 눌림 종류. 방향 4개, 버튼 A/B, 숫자 1~9
const (
	pUp = iota
	pDown
	pLeft
	pRight
	pA
	pB
	pNum   // pNum+0 = 숫자 1
	pCount = pNum + 9
)

// pulses 는 키를 누른 시각 기록. 값 복사가 되는 고정 배열이라 모델에 그대로 넣어도 안전함.
type pulses struct {
	start, until [pCount]time.Time
}

func (p *pulses) press(kind int, now time.Time, d time.Duration) {
	p.start[kind], p.until[kind] = now, now.Add(d)
}

func (p *pulses) active(now time.Time) bool {
	for i := range p.until {
		if now.Before(p.until[i].Add(160 * time.Millisecond)) {
			return true
		}
	}
	return false
}

// held 는 눌린 정도 0..1. 누르는 동안 빠르게 올라가고, 떼면 조금 천천히 내려감
func (p *pulses) held(kind int, now time.Time) float64 {
	if p.until[kind].IsZero() {
		return 0
	}
	if now.Before(p.until[kind]) {
		return clampF(now.Sub(p.start[kind]).Seconds()/0.07, 0, 1)
	}
	return clampF(1-now.Sub(p.until[kind]).Seconds()/0.12, 0, 1)
}

func (p *pulses) down(kind int, now time.Time) bool { return now.Before(p.until[kind]) }

func vst(fg, bg vec) style { return style{fg: colSeq(false, fg), bg: colSeq(true, bg)} }

var (
	arcPanel  = vec{28, 30, 42}
	arcEdge   = vec{48, 52, 70}
	arcBase   = vec{70, 74, 96}
	arcBaseHi = vec{120, 126, 150}
	arcRed    = vec{236, 64, 72}
	arcRedHi  = vec{255, 96, 104}
	arcBlue   = vec{66, 140, 255}
	arcYellow = vec{250, 204, 60}
	arcMuted  = vec{204, 204, 204}
	arcLabel  = vec{150, 154, 170}
)

// arcadeGrid 는 폭 w, 높이 arcadeH 의 조작부. n 은 앱 개수(숫자 버튼 수), bLabel 은 B 버튼 설명
func arcadeGrid(w, n int, p pulses, now time.Time, bLabel string) *grid {
	g := newGrid(w, arcadeH)
	for y := 0; y < g.h; y++ {
		for x := 0; x < g.w; x++ {
			g.c[y*g.w+x] = cell{r: ' ', w: 1, st: vst(arcPanel, arcPanel)}
		}
	}
	for x := 0; x < w; x++ {
		g.put(x, 0, '▀', vst(arcEdge, arcPanel))
	}
	joystick(g, 9, 1, p, now)

	// 큰 버튼 A, B
	// 버튼 위의 글자는 실제로 누르는 키 이름임 (A/B 같은 없는 키를 쓰면 잘못 알려주게 됨)
	bigButton(g, 21, 2, "Enter", arcRed, p.down(pA, now))
	bigButton(g, 31, 2, "Esc", arcBlue, p.down(pB, now))
	g.text(21+(7-lipgloss.Width("실행"))/2, 5, "실행", vst(arcLabel, arcPanel))
	g.text(31+(7-lipgloss.Width(bLabel))/2, 5, bLabel, vst(arcLabel, arcPanel))

	// 숫자 버튼 (앱 개수만큼)
	step := 4
	if n > 7 {
		step = 3
	}
	for i := 0; i < n && i < 9; i++ {
		x := 20 + i*step
		if x+3 > w {
			break
		}
		lit := p.down(pNum+i, now)
		c := vec{130, 104, 30}
		fg := vec{30, 26, 10}
		if lit {
			c, fg = arcYellow, vec{20, 20, 20}
		}
		g.put(x, 1, '(', vst(c.mul(0.8), arcPanel))
		g.put(x+1, 1, rune('1'+i), vst(fg, c))
		g.put(x+2, 1, ')', vst(c.mul(0.8), arcPanel))
	}

	// 코인 투입구 (깜박임)
	if w >= 50 {
		blink := int(now.UnixMilli()/500)%2 == 0
		c := vec{130, 104, 30}
		if blink {
			c = arcYellow
		}
		cx := w - 9
		g.text(cx, 2, "▛▀▀▀▜", vst(arcBaseHi, arcPanel))
		g.put(cx, 3, '▌', vst(arcBaseHi, arcPanel))
		g.put(cx+2, 3, '▮', vst(c, arcPanel))
		g.put(cx+4, 3, '▐', vst(arcBaseHi, arcPanel))
		g.text(cx, 4, "▙▄▄▄▟", vst(arcBaseHi, arcPanel))
		g.text(cx, 5, "COIN", vst(c, arcPanel))
	}
	return g
}

// joystick: top..top+5 줄을 씀 (위로 기울면 top-1 줄까지)
func joystick(g *grid, cx, top int, p pulses, now time.Time) {
	dx := p.held(pRight, now) - p.held(pLeft, now)
	dy := p.held(pDown, now) - p.held(pUp, now)
	jx, jy := int(math.Round(dx*3)), int(math.Round(dy))
	baseY, ballY, ballX := top+5, top+1+jy, cx+jx
	g.text(cx-5, baseY, "▄▄▄▄▄▄▄▄▄▄▄", vst(arcBase, arcPanel))
	g.text(cx-2, baseY, "▄▄▄▄▄", vst(arcBaseHi, arcPanel))
	botY := ballY + 2
	span := float64(baseY - botY + 1)
	for y := baseY - 1; y >= botY; y-- {
		x := int(math.Round(float64(cx) + float64(ballX-cx)*float64(baseY-y)/span))
		x2 := int(math.Round(float64(cx) + float64(ballX-cx)*float64(baseY-y+1)/span))
		ch := '█'
		if x2 != x {
			ch = '▚'
			if ballX > cx {
				ch = '▞'
			}
		}
		g.put(x, y, ch, vst(vec{196, 200, 212}, arcPanel))
	}
	c := arcRed
	if jx != 0 || jy != 0 {
		c = arcRedHi
	}
	g.text(ballX-2, ballY-1, "▗▄▄▄▖", vst(c, arcPanel))
	g.text(ballX-2, ballY, "▐███▌", vst(c, arcPanel))
	g.text(ballX-2, ballY+1, "▝▀▀▀▘", vst(c.mul(0.7), arcPanel))
}

// bigButton: 폭 7, 높이 3 (눌리면 한 줄 내려가고 어두워짐). label 은 이 버튼에 대응하는 실제 키 이름
func bigButton(g *grid, x, top int, label string, c vec, down bool) {
	y := top
	col := c
	if down {
		y++
		col = c.mul(0.5)
	}
	g.text(x, y, "▄▄▄▄▄▄▄", vst(col.mul(0.8), arcPanel))
	for i := 0; i < 7; i++ {
		g.put(x+i, y+1, ' ', vst(vec{20, 20, 24}, col))
	}
	fg := vec{20, 20, 24}
	if down {
		fg = vec{30, 30, 30}
	}
	g.text(x+(7-lipgloss.Width(label))/2, y+1, label, vst(fg, col))
	if !down {
		g.text(x, y+2, "▀▀▀▀▀▀▀", vst(col.mul(0.35), arcPanel))
	}
}
