package main

import (
	"math"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// 점자 캔버스: 터미널 1칸을 가로 2 x 세로 4 점으로 쓰는 그림판. 범위 밖 점은 조용히 무시함.

// fidgetBlockGlyphs 가 true 면 점자 대신 블록 문자(▀▄█)로 그림 (점자 폭이 어긋나는 글꼴용, TUI_FIDGET=block)
var fidgetBlockGlyphs = os.Getenv("TUI_FIDGET") == "block"

type canvas struct {
	cw, ch int
	px     []bool
}

func newCanvas(cw, ch int) *canvas {
	cw, ch = max(1, cw), max(1, ch)
	return &canvas{cw: cw, ch: ch, px: make([]bool, 2*cw*4*ch)}
}

func (c *canvas) W() int { return 2 * c.cw }
func (c *canvas) H() int { return 4 * c.ch }

func (c *canvas) Clear() { clear(c.px) }

func (c *canvas) Set(x, y int) {
	if x < 0 || y < 0 || x >= c.W() || y >= c.H() {
		return
	}
	c.px[y*c.W()+x] = true
}

func (c *canvas) at(x, y int) bool {
	if x < 0 || y < 0 || x >= c.W() || y >= c.H() {
		return false
	}
	return c.px[y*c.W()+x]
}

// Line 은 브레젠햄 직선
func (c *canvas) Line(x0, y0, x1, y1 int) {
	dx, dy := abs(x1-x0), -abs(y1-y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		c.Set(x0, y0)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// FillEllipse 는 중심 (cx,cy), 반지름 (rx,ry) 인 꽉 찬 타원
func (c *canvas) FillEllipse(cx, cy, rx, ry float64) {
	if rx <= 0 || ry <= 0 {
		return
	}
	y0, y1 := int(math.Floor(cy-ry)), int(math.Ceil(cy+ry))
	x0, x1 := int(math.Floor(cx-rx)), int(math.Ceil(cx+rx))
	for y := max(0, y0); y <= min(c.H()-1, y1); y++ {
		for x := max(0, x0); x <= min(c.W()-1, x1); x++ {
			dx, dy := (float64(x)+0.5-cx)/rx, (float64(y)+0.5-cy)/ry
			if dx*dx+dy*dy <= 1 {
				c.px[y*c.W()+x] = true
			}
		}
	}
}

func (c *canvas) FillCircle(cx, cy, r float64) { c.FillEllipse(cx, cy, r, r) }

// FillTriangle 은 꼭짓점 세 개로 이루어진 꽉 찬 삼각형
func (c *canvas) FillTriangle(x0, y0, x1, y1, x2, y2 float64) {
	minX, maxX := math.Min(x0, math.Min(x1, x2)), math.Max(x0, math.Max(x1, x2))
	minY, maxY := math.Min(y0, math.Min(y1, y2)), math.Max(y0, math.Max(y1, y2))
	edge := func(ax, ay, bx, by, px, py float64) float64 { return (px-ax)*(by-ay) - (py-ay)*(bx-ax) }
	for y := max(0, int(math.Floor(minY))); y <= min(c.H()-1, int(math.Ceil(maxY))); y++ {
		for x := max(0, int(math.Floor(minX))); x <= min(c.W()-1, int(math.Ceil(maxX))); x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			a, b, d := edge(x0, y0, x1, y1, px, py), edge(x1, y1, x2, y2, px, py), edge(x2, y2, x0, y0, px, py)
			if (a >= 0 && b >= 0 && d >= 0) || (a <= 0 && b <= 0 && d <= 0) {
				c.px[y*c.W()+x] = true
			}
		}
	}
}

// 점자 점 번호 비트: 왼쪽 열 위에서부터 0x01 0x02 0x04, 오른쪽 열 0x08 0x10 0x20, 맨 아래 줄 왼쪽 0x40 오른쪽 0x80
var brailleBit = [4][2]rune{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}

// cell 은 칸 (cx, cy) 를 글자 하나로 바꿈. 비어 있으면 공백(폭이 확실한 문자)
func (c *canvas) cell(cx, cy int) string {
	x, y := cx*2, cy*4
	if fidgetBlockGlyphs {
		n, top, bot := 0, 0, 0
		for j := 0; j < 4; j++ {
			for i := 0; i < 2; i++ {
				if c.at(x+i, y+j) {
					n++
					if j < 2 {
						top++
					} else {
						bot++
					}
				}
			}
		}
		switch {
		case n == 0:
			return " "
		case top >= 2 && bot >= 2:
			return "█"
		case top >= bot:
			return "▀"
		default:
			return "▄"
		}
	}
	var bits rune
	for j := 0; j < 4; j++ {
		for i := 0; i < 2; i++ {
			if c.at(x+i, y+j) {
				bits |= brailleBit[j][i]
			}
		}
	}
	if bits == 0 {
		return " "
	}
	return string(rune(0x2800) + bits)
}

// Row 는 칸 줄 하나(cy)를 한 가지 색의 seg 로 만듦. 폭은 항상 cw
func (c *canvas) Row(cy int, col lipgloss.Color) []seg {
	return c.RowFunc(cy, func(int) lipgloss.Color { return col })
}

// RowFunc 는 칸마다 색을 정함. 같은 색이 이어지면 seg 하나로 합침. 폭은 항상 cw
func (c *canvas) RowFunc(cy int, colorAt func(cx int) lipgloss.Color) []seg {
	var segs []seg
	var b strings.Builder
	var cur lipgloss.Color
	flush := func() {
		if b.Len() > 0 {
			segs = append(segs, sg(b.String(), cur))
			b.Reset()
		}
	}
	for cx := 0; cx < c.cw; cx++ {
		col := colorAt(cx)
		if b.Len() > 0 && col != cur {
			flush()
		}
		cur = col
		b.WriteString(c.cell(cx, cy))
	}
	flush()
	return segs
}

// Rows 는 모든 칸 줄을 한 가지 색으로 만듦
func (c *canvas) Rows(col lipgloss.Color) [][]seg {
	rows := make([][]seg, c.ch)
	for cy := range rows {
		rows[cy] = c.Row(cy, col)
	}
	return rows
}
