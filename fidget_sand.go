package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// 모래: 위에서 모래를 뿌리면 떨어져 쌓이고 옆으로 흘러내림. 세계는 고정 크기 칸 격자(fidget_world.go 로 그림).

const (
	sandW     = 96
	sandH     = 48
	sandBurst = 10 // Space 한 번에 뿌리는 프레임 수 (누르고 있으면 키 반복으로 계속 뿌려짐)
	sandStep  = 2  // ←→ 한 번에 움직이는 칸
)

// 모래 색 (1부터. 0 은 빈 칸)
var sandColors = []lipgloss.Color{colorWarn, colorOk, colorAccent, colorText}
var sandColorNames = []string{"노랑", "초록", "하늘", "흰색"}

type sandMode struct {
	grid   [sandW * sandH]uint8
	cursor int // 뿌리는 위치 (x)
	pour   int // 남은 뿌리기 프레임
	color  int // sandColors 번호
	count  int
	flip   bool // 줄마다 훑는 방향을 번갈아 바꿔서 한쪽으로 쏠리지 않게 함
	rng    *rand.Rand
}

func newSandMode() fidgetMode {
	return &sandMode{cursor: sandW / 2, rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

func (m *sandMode) Title() string { return "모래" }

func (m *sandMode) Keys() [][2]string {
	return [][2]string{{"Space", "뿌리기"}, {"r", "비우기"}, {"c", "색"}, {"←→", "이동"}}
}

func (m *sandMode) Status() string {
	s := fmt.Sprintf("모래 %d알 · %s", m.count, sandColorNames[m.color])
	if m.count >= sandW*sandH*9/10 {
		s += "  거의 가득 참! r 로 비우기"
	}
	return s
}

func (m *sandMode) Key(k string) bool {
	switch k {
	case " ", "space", "enter":
		m.pour = sandBurst
	case "left":
		m.cursor = max(0, m.cursor-sandStep)
	case "right":
		m.cursor = min(sandW-1, m.cursor+sandStep)
	case "c", "C":
		m.color = (m.color + 1) % len(sandColors)
	case "r", "R":
		m.grid = [sandW * sandH]uint8{}
		m.count, m.pour = 0, 0
	default:
		return false
	}
	return true
}

func (m *sandMode) at(x, y int) uint8 { return m.grid[y*sandW+x] }

func (m *sandMode) empty(x, y int) bool {
	return x >= 0 && x < sandW && y >= 0 && y < sandH && m.grid[y*sandW+x] == 0
}

func (m *sandMode) Step() bool {
	if m.pour > 0 {
		m.pour--
		for i := 0; i < 2; i++ {
			x := m.cursor + m.rng.Intn(3) - 1
			if m.empty(x, 0) {
				m.grid[x] = uint8(m.color + 1)
				m.count++
			}
		}
	}
	moved := false
	m.flip = !m.flip
	for y := sandH - 2; y >= 0; y-- {
		for i := 0; i < sandW; i++ {
			x := i
			if m.flip {
				x = sandW - 1 - i
			}
			g := m.at(x, y)
			if g == 0 {
				continue
			}
			nx := -1
			switch {
			case m.empty(x, y+1):
				nx = x
			default:
				d := 1
				if m.rng.Intn(2) == 0 {
					d = -1
				}
				if m.empty(x+d, y+1) {
					nx = x + d
				} else if m.empty(x-d, y+1) {
					nx = x - d
				}
			}
			if nx >= 0 {
				m.grid[y*sandW+x] = 0
				m.grid[(y+1)*sandW+nx] = g
				moved = true
			}
		}
	}
	return moved || m.pour > 0
}

func (m *sandMode) Rows(cw, ch int) [][]seg {
	return worldRows(cw, ch, sandW, sandH, func(x, y int) lipgloss.Color {
		if y < 2 && x == m.cursor && m.at(x, y) == 0 {
			return colorAccent
		}
		if g := m.at(x, y); g > 0 {
			return sandColors[g-1]
		}
		return ""
	})
}
