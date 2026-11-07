package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// 생명 게임: 콘웨이의 규칙(살아 있는 이웃 2~3이면 생존, 빈 칸은 이웃 3이면 탄생). 가장자리는 반대편과 이어짐.

const (
	lifeW     = 80
	lifeH     = 40
	lifeEvery = 3 // 몇 프레임마다 한 세대 (33ms x 3 = 약 0.1초)
)

type lifeMode struct {
	cells   [lifeW * lifeH]bool
	cx, cy  int
	playing bool
	frame   int
	gen     int
	rng     *rand.Rand
}

func newLifeMode() fidgetMode {
	m := &lifeMode{cx: lifeW / 2, cy: lifeH / 2, rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
	m.randomize()
	return m
}

func (m *lifeMode) Title() string { return "생명 게임" }

func (m *lifeMode) Keys() [][2]string {
	return [][2]string{{"Enter", "재생"}, {"g", "글라이더"}, {"n", "한 세대"}, {"c", "비우기"}, {"r", "무작위"}, {"Space", "칸 찍기"}, {"←→↑↓", "이동"}}
}

func (m *lifeMode) alive() int {
	n := 0
	for _, c := range m.cells {
		if c {
			n++
		}
	}
	return n
}

func (m *lifeMode) Status() string {
	st := "멈춤 (Enter 로 재생)"
	if m.playing {
		st = "재생 중"
	}
	return fmt.Sprintf("세대 %d · 살아 있음 %d · %s", m.gen, m.alive(), st)
}

func (m *lifeMode) randomize() {
	for i := range m.cells {
		m.cells[i] = m.rng.Intn(4) == 0
	}
	m.gen = 0
}

func (m *lifeMode) idx(x, y int) int {
	return ((y%lifeH+lifeH)%lifeH)*lifeW + (x%lifeW+lifeW)%lifeW
}

func (m *lifeMode) Key(k string) bool {
	switch k {
	case "enter":
		m.playing = !m.playing
	case " ", "space":
		i := m.idx(m.cx, m.cy)
		m.cells[i] = !m.cells[i]
	case "left":
		m.cx = (m.cx + lifeW - 1) % lifeW
	case "right":
		m.cx = (m.cx + 1) % lifeW
	case "up":
		m.cy = (m.cy + lifeH - 1) % lifeH
	case "down":
		m.cy = (m.cy + 1) % lifeH
	case "n", "N":
		m.playing = false
		m.advance()
	case "g", "G":
		// 오른쪽 아래로 날아가는 글라이더
		for _, p := range [][2]int{{1, 0}, {2, 1}, {0, 2}, {1, 2}, {2, 2}} {
			m.cells[m.idx(m.cx+p[0], m.cy+p[1])] = true
		}
	case "r", "R":
		m.randomize()
	case "c", "C":
		m.cells = [lifeW * lifeH]bool{}
		m.gen, m.playing = 0, false
	default:
		return false
	}
	return true
}

func (m *lifeMode) advance() {
	var next [lifeW * lifeH]bool
	for y := 0; y < lifeH; y++ {
		for x := 0; x < lifeW; x++ {
			n := 0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if (dx != 0 || dy != 0) && m.cells[m.idx(x+dx, y+dy)] {
						n++
					}
				}
			}
			i := y*lifeW + x
			next[i] = n == 3 || (n == 2 && m.cells[i])
		}
	}
	m.cells = next
	m.gen++
}

func (m *lifeMode) Step() bool {
	if !m.playing {
		return false
	}
	if m.frame++; m.frame >= lifeEvery {
		m.frame = 0
		m.advance()
	}
	return true
}

func (m *lifeMode) Rows(cw, ch int) [][]seg {
	return worldRows(cw, ch, lifeW, lifeH, func(x, y int) lipgloss.Color {
		if x == m.cx && y == m.cy && !m.playing {
			return colorAccent
		}
		if m.cells[y*lifeW+x] {
			return colorText
		}
		return ""
	})
}
