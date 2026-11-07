package main

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// 불꽃놀이: Space 로 쏘아 올리면 올라가다 터져서 불티가 중력에 떨어지며 사라짐. a 는 자동 발사.
// 세계 좌표는 fwW x fwH (점 단위와 비슷한 비율). 그릴 때 캔버스 크기에 맞게 늘이거나 줄임.

const (
	fwW         = 160.0
	fwH         = 60.0
	fwSparks    = 40
	fwMaxRocket = 12 // 한꺼번에 떠 있는 불꽃 상한 (키 반복으로 너무 많아지지 않게)
)

var fwColors = []lipgloss.Color{colorAccent, colorOk, colorWarn, colorText}

type fwParticle struct {
	x, y, vx, vy float64
	life, full   int // 남은 프레임, 처음 수명 (0 이면 아직 터지지 않은 로켓)
	col          lipgloss.Color
}

type fireworksMode struct {
	ps       []fwParticle
	auto     bool
	autoWait int
	shots    int
	rng      *rand.Rand
}

func newFireworksMode() fidgetMode {
	return &fireworksMode{rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

func (m *fireworksMode) Title() string { return "불꽃놀이" }

func (m *fireworksMode) Keys() [][2]string {
	return [][2]string{{"Space", "쏘기"}, {"a", "자동"}, {"r", "지우기"}}
}

func (m *fireworksMode) Status() string {
	s := fmt.Sprintf("쏘아 올림 %d발", m.shots)
	if m.auto {
		s += " · 자동 발사 중"
	}
	return s
}

func (m *fireworksMode) rockets() int {
	n := 0
	for _, p := range m.ps {
		if p.full == 0 {
			n++
		}
	}
	return n
}

func (m *fireworksMode) launch() bool {
	if m.rockets() >= fwMaxRocket {
		return false
	}
	m.ps = append(m.ps, fwParticle{
		x:   15 + m.rng.Float64()*(fwW-30),
		y:   fwH,
		vx:  (m.rng.Float64() - 0.5) * 0.3,
		vy:  -(1.6 + m.rng.Float64()*0.45),
		col: fwColors[m.rng.Intn(len(fwColors))],
	})
	m.shots++
	return true
}

func (m *fireworksMode) Key(k string) bool {
	switch k {
	case " ", "space", "enter":
		return m.launch()
	case "a", "A":
		m.auto = !m.auto
		m.autoWait = 0
	case "r", "R":
		m.ps = nil
	default:
		return false
	}
	return true
}

func (m *fireworksMode) Step() bool {
	if m.auto {
		if m.autoWait--; m.autoWait <= 0 {
			m.launch()
			m.autoWait = 12 + m.rng.Intn(25)
		}
	}
	next := m.ps[:0]
	var born []fwParticle
	for _, p := range m.ps {
		if p.full == 0 { // 로켓
			p.x += p.vx
			p.y += p.vy
			p.vy += 0.035
			if p.vy > -0.35 {
				for i := 0; i < fwSparks; i++ {
					a := m.rng.Float64() * 2 * math.Pi
					s := 0.4 + m.rng.Float64()*1.0
					life := 25 + m.rng.Intn(20)
					born = append(born, fwParticle{x: p.x, y: p.y, vx: math.Cos(a) * s, vy: math.Sin(a) * s * 0.8, life: life, full: life, col: p.col})
				}
				continue
			}
			next = append(next, p)
			continue
		}
		p.x += p.vx
		p.y += p.vy
		p.vx *= 0.97
		p.vy = p.vy*0.97 + 0.025
		if p.life--; p.life > 0 && p.y < fwH {
			next = append(next, p)
		}
	}
	m.ps = append(next, born...)
	return len(m.ps) > 0 || m.auto
}

func (m *fireworksMode) Rows(cw, ch int) [][]seg {
	cv := newCanvas(cw, ch)
	cells := make([]lipgloss.Color, cv.cw*cv.ch)
	W, H := float64(cv.W()), float64(cv.H())
	plot := func(x, y float64, col lipgloss.Color) {
		px, py := int(x*W/fwW), int(y*H/fwH)
		if px < 0 || py < 0 || px >= cv.W() || py >= cv.H() {
			return
		}
		cv.Set(px, py)
		cells[py/4*cv.cw+px/2] = col
	}
	for _, p := range m.ps {
		col := p.col
		if p.full > 0 && p.life*3 < p.full { // 수명이 1/3 아래면 흐려짐
			col = colorTextMuted
		}
		plot(p.x, p.y, col)
		if p.full == 0 { // 로켓 꼬리
			plot(p.x-p.vx, p.y-p.vy*1.5, colorTextMuted)
		}
	}
	rows := make([][]seg, cv.ch)
	for cy := range rows {
		rows[cy] = cv.RowFunc(cy, func(cx int) lipgloss.Color {
			if c := cells[cy*cv.cw+cx]; c != "" {
				return c
			}
			return colorText
		})
	}
	return rows
}
