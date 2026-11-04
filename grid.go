package main

import (
	"math"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// 칸 격자 (홈 배경, 화면 전환, 아케이드 조작부용).
// 앱 화면은 lipgloss 가 만든 색 입힌 문자열이라, 한 번 칸 단위로 풀어서 다룬 뒤 다시 문자열로 되돌림.
// DESIGN.md "Deliberate exceptions > 움직임" 참고: 이 파일의 RGB 색은 배경·전환 효과 전용임.

// ---- 색 ----

// vec 은 0~255 범위의 RGB (계산 중에는 실수로 들고 있다가 출력 때 반올림함)
type vec [3]float64

func (v vec) mul(k float64) vec { return vec{v[0] * k, v[1] * k, v[2] * k} }
func (v vec) add(n float64) vec { return vec{v[0] + n, v[1] + n, v[2] + n} }

func mixV(a, b vec, t float64) vec {
	return vec{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t, a[2] + (b[2]-a[2])*t}
}

func gradV(stops []vec, v float64) vec {
	v = clampF(v, 0, 1) * float64(len(stops)-1)
	i := int(math.Min(float64(len(stops)-2), math.Floor(v)))
	return mixV(stops[i], stops[i+1], v-float64(i))
}

func clampF(v, a, b float64) float64 { return math.Max(a, math.Min(b, v)) }
func frac(v float64) float64         { return v - math.Floor(v) }

// hash 는 (x,y,z) → 0..1 의사 난수 (화면마다 같은 값이 나와서 테스트가 쉬움)
func hash(x, y, z float64) float64 {
	return frac(math.Sin(x*127.1+y*311.7+z*74.7) * 43758.5453)
}

// easeIO 는 느리게 시작해 빨라졌다가 느리게 끝나는 보간
func easeIO(u float64) float64 {
	if u < 0.5 {
		return 4 * u * u * u
	}
	return 1 - math.Pow(-2*u+2, 3)/2
}

var pal16 = [16]vec{
	{12, 12, 12}, {197, 15, 31}, {19, 161, 14}, {193, 156, 0}, {0, 55, 218}, {136, 23, 152}, {58, 150, 221}, {204, 204, 204},
	{118, 118, 118}, {231, 72, 86}, {22, 198, 12}, {249, 241, 165}, {59, 120, 255}, {180, 0, 158}, {97, 214, 214}, {242, 242, 242},
}

// 색 표현 방식. 터미널이 트루컬러를 지원하지 않으면 256색 → 16색 순으로 가까운 색으로 바꿈.
// 테스트에서 바꿔 끼울 수 있게 변수로 둠.
var (
	profOnce sync.Once
	profile  termenv.Profile
)

func termProfile() termenv.Profile {
	profOnce.Do(func() { profile = lipgloss.ColorProfile() })
	return profile
}

func setTermProfile(p termenv.Profile) { termProfile(); profile = p }

// animEnabled 는 배경·전환 효과를 켤 수 있는지 (색이 안 나오는 터미널이나 TUI_ANIM=off 면 끔)
func animEnabled() bool {
	if strings.EqualFold(os.Getenv("TUI_ANIM"), "off") {
		return false
	}
	return termProfile() != termenv.Ascii
}

func colSeq(bg bool, c vec) string {
	r, g, b := clamp255(c[0]), clamp255(c[1]), clamp255(c[2])
	switch termProfile() {
	case termenv.TrueColor:
		if bg {
			return "48;2;" + itoa(r) + ";" + itoa(g) + ";" + itoa(b)
		}
		return "38;2;" + itoa(r) + ";" + itoa(g) + ";" + itoa(b)
	case termenv.ANSI256:
		n := nearest256(r, g, b)
		if bg {
			return "48;5;" + itoa(n)
		}
		return "38;5;" + itoa(n)
	case termenv.ANSI:
		i := nearest16(vec{float64(r), float64(g), float64(b)})
		base := 30
		if bg {
			base = 40
		}
		if i >= 8 {
			base += 60
			i -= 8
		}
		return itoa(base + i)
	}
	return ""
}

func itoa(n int) string { return strconv.Itoa(n) }

func clamp255(v float64) int { return int(math.Round(clampF(v, 0, 255))) }

func nearest16(c vec) int {
	best, bd := 0, math.MaxFloat64
	for i, p := range pal16 {
		d := (p[0]-c[0])*(p[0]-c[0]) + (p[1]-c[1])*(p[1]-c[1]) + (p[2]-c[2])*(p[2]-c[2])
		if d < bd {
			best, bd = i, d
		}
	}
	return best
}

func nearest256(r, g, b int) int {
	levels := [6]int{0, 95, 135, 175, 215, 255}
	idx := func(v int) int {
		bi, bd := 0, 1<<30
		for i, l := range levels {
			if d := (v - l) * (v - l); d < bd {
				bi, bd = i, d
			}
		}
		return bi
	}
	ri, gi, bi := idx(r), idx(g), idx(b)
	cube := 16 + 36*ri + 6*gi + bi
	cr, cg, cb := levels[ri], levels[gi], levels[bi]
	cd := (r-cr)*(r-cr) + (g-cg)*(g-cg) + (b-cb)*(b-cb)
	avg := (r + g + b) / 3
	gi2 := (avg - 8 + 5) / 10
	if gi2 < 0 {
		gi2 = 0
	}
	if gi2 > 23 {
		gi2 = 23
	}
	gv := 8 + 10*gi2
	gd := (r-gv)*(r-gv) + (g-gv)*(g-gv) + (b-gv)*(b-gv)
	if gd < cd {
		return 232 + gi2
	}
	return cube
}

// ---- 칸 ----

const (
	attrBold uint8 = 1 << iota
	attrFaint
	attrItalic
	attrUnderline
	attrReverse
	attrStrike
)

type style struct {
	fg, bg string // SGR 조각 ("38;5;14", "92", "48;2;1;2;3" ...). 비어 있으면 터미널 기본색
	attr   uint8
}

// cell 의 w 는 1(보통), 2(한글 등 넓은 글자의 앞칸), 0(넓은 글자의 뒷칸)
type cell struct {
	r  rune
	w  int8
	st style
}

type grid struct {
	w, h int
	c    []cell
}

func newGrid(w, h int) *grid {
	g := &grid{w: w, h: h, c: make([]cell, max(0, w*h))}
	for i := range g.c {
		g.c[i] = cell{r: ' ', w: 1}
	}
	return g
}

func (g *grid) at(x, y int) *cell { return &g.c[y*g.w+x] }

func (g *grid) in(x, y int) bool { return x >= 0 && y >= 0 && x < g.w && y < g.h }

var runeW sync.Map

func runeWidth(r rune) int8 {
	if r < 0x80 {
		return 1
	}
	if v, ok := runeW.Load(r); ok {
		return v.(int8)
	}
	w := int8(ansi.StringWidth(string(r)))
	runeW.Store(r, w)
	return w
}

// put 은 (x,y)에 글자를 놓음. 넓은 글자는 두 칸을 씀. 격자 밖이면 무시함.
func (g *grid) put(x, y int, r rune, st style) {
	w := runeWidth(r)
	if w == 0 || !g.in(x, y) {
		return
	}
	if w == 2 {
		if !g.in(x+1, y) {
			g.setCell(x, y, cell{r: ' ', w: 1, st: st})
			return
		}
		g.setCell(x, y, cell{r: r, w: 2, st: st})
		g.setCell(x+1, y, cell{r: 0, w: 0, st: st})
		return
	}
	g.setCell(x, y, cell{r: r, w: 1, st: st})
}

func (g *grid) setCell(x, y int, c cell) {
	// 넓은 글자의 반쪽만 덮어쓰면 남은 반쪽이 깨지므로 같이 지움
	cur := g.at(x, y)
	if cur.w == 2 && g.in(x+1, y) {
		g.c[y*g.w+x+1] = cell{r: ' ', w: 1, st: cur.st}
	}
	if cur.w == 0 && x > 0 {
		g.c[y*g.w+x-1] = cell{r: ' ', w: 1, st: g.c[y*g.w+x-1].st}
	}
	*cur = c
}

// text 는 문자열을 한 줄로 놓음
func (g *grid) text(x, y int, s string, st style) {
	for _, r := range s {
		w := int(runeWidth(r))
		if w == 0 {
			continue
		}
		g.put(x, y, r, st)
		x += w
	}
}

// sanitize 는 넓은 글자의 앞칸·뒷칸이 짝이 안 맞는 곳을 공백으로 고침 (효과로 칸을 섞은 뒤에 부름)
func (g *grid) sanitize() {
	for y := 0; y < g.h; y++ {
		for x := 0; x < g.w; x++ {
			c := g.at(x, y)
			switch c.w {
			case 2:
				if x+1 >= g.w || g.at(x+1, y).w != 0 {
					*c = cell{r: ' ', w: 1, st: c.st}
				}
			case 0:
				if x == 0 || g.at(x-1, y).w != 2 {
					*c = cell{r: ' ', w: 1, st: c.st}
				}
			}
		}
	}
}

// ---- 문자열 ↔ 격자 ----

// parseGrid 는 lipgloss 가 만든 색 입힌 여러 줄 문자열을 w×h 격자로 풂 (모자라는 곳은 빈칸, 넘치는 곳은 버림)
func parseGrid(s string, w, h int) *grid {
	g := newGrid(w, h)
	for y, line := range strings.Split(s, "\n") {
		if y >= h {
			break
		}
		parseLine(g, y, strings.TrimSuffix(line, "\r"))
	}
	return g
}

func parseLine(g *grid, y int, line string) {
	var st style
	x := 0
	rs := []rune(line)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == 0x1b && i+1 < len(rs) && rs[i+1] == '[' { // CSI
			j := i + 2
			for j < len(rs) && (rs[j] < 0x40 || rs[j] > 0x7e) {
				j++
			}
			if j < len(rs) && rs[j] == 'm' {
				applySGR(&st, string(rs[i+2:j]))
			}
			i = j
			continue
		}
		if r == 0x1b || r < 0x20 {
			continue
		}
		w := int(runeWidth(r))
		if w == 0 {
			continue
		}
		if x >= g.w {
			break
		}
		g.put(x, y, r, st)
		x += w
	}
}

func applySGR(st *style, params string) {
	if params == "" {
		*st = style{}
		return
	}
	p := strings.Split(params, ";")
	for i := 0; i < len(p); i++ {
		n, err := strconv.Atoi(p[i])
		if err != nil {
			continue
		}
		switch {
		case n == 0:
			*st = style{}
		case n == 1:
			st.attr |= attrBold
		case n == 2:
			st.attr |= attrFaint
		case n == 3:
			st.attr |= attrItalic
		case n == 4:
			st.attr |= attrUnderline
		case n == 7:
			st.attr |= attrReverse
		case n == 9:
			st.attr |= attrStrike
		case n == 22:
			st.attr &^= attrBold | attrFaint
		case n == 23:
			st.attr &^= attrItalic
		case n == 24:
			st.attr &^= attrUnderline
		case n == 27:
			st.attr &^= attrReverse
		case n == 29:
			st.attr &^= attrStrike
		case n == 39:
			st.fg = ""
		case n == 49:
			st.bg = ""
		case (n >= 30 && n <= 37) || (n >= 90 && n <= 97):
			st.fg = p[i]
		case (n >= 40 && n <= 47) || (n >= 100 && n <= 107):
			st.bg = p[i]
		case n == 38 || n == 48:
			var frag string
			if i+1 < len(p) && p[i+1] == "5" && i+2 < len(p) {
				frag = p[i] + ";5;" + p[i+2]
				i += 2
			} else if i+1 < len(p) && p[i+1] == "2" && i+4 < len(p) {
				frag = p[i] + ";2;" + p[i+2] + ";" + p[i+3] + ";" + p[i+4]
				i += 4
			} else {
				continue
			}
			if n == 38 {
				st.fg = frag
			} else {
				st.bg = frag
			}
		}
	}
}

func (s style) seq() string {
	parts := make([]string, 0, 8)
	parts = append(parts, "0")
	if s.attr&attrBold != 0 {
		parts = append(parts, "1")
	}
	if s.attr&attrFaint != 0 {
		parts = append(parts, "2")
	}
	if s.attr&attrItalic != 0 {
		parts = append(parts, "3")
	}
	if s.attr&attrUnderline != 0 {
		parts = append(parts, "4")
	}
	if s.attr&attrReverse != 0 {
		parts = append(parts, "7")
	}
	if s.attr&attrStrike != 0 {
		parts = append(parts, "9")
	}
	if s.fg != "" {
		parts = append(parts, s.fg)
	}
	if s.bg != "" {
		parts = append(parts, s.bg)
	}
	return "\x1b[" + strings.Join(parts, ";") + "m"
}

// String 은 격자를 색 입힌 여러 줄 문자열로 되돌림 (줄마다 폭이 정확히 w 칸)
func (g *grid) String() string {
	var b strings.Builder
	b.Grow(g.w * g.h * 8)
	for y := 0; y < g.h; y++ {
		var cur style
		for x := 0; x < g.w; x++ {
			c := g.at(x, y)
			if c.w == 0 {
				continue
			}
			if c.st != cur {
				b.WriteString(c.st.seq())
				cur = c.st
			}
			b.WriteRune(c.r)
		}
		if cur != (style{}) {
			b.WriteString("\x1b[0m")
		}
		if y < g.h-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// blit 은 src 를 (x0,y0)에 덮어씀. 배경색이 없는 칸은 panel 색으로 채워서 뒤의 배경 애니메이션이 비치지 않게 함.
func (g *grid) blit(src *grid, x0, y0 int, panel string) {
	for y := 0; y < src.h; y++ {
		for x := 0; x < src.w; x++ {
			if !g.in(x0+x, y0+y) {
				continue
			}
			c := *src.at(x, y)
			if c.st.bg == "" && panel != "" {
				c.st.bg = panel
			}
			g.setCell(x0+x, y0+y, c)
		}
	}
}
