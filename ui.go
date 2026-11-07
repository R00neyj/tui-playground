package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// ---- 그리기 환경 ----

// Ctx 는 한 번 그릴 때 호스트가 앱에 넘겨주는 환경 (창 크기와 현재 시각)
type Ctx struct {
	W, H int
	Now  time.Time
}

// In 은 박스 안쪽 폭 (DESIGN.md box.width: 창 폭 − 6, 최소 50, 최대 100)
// maxIn 은 전체화면에서도 박스가 너무 퍼지지 않게 하는 안쪽 폭 상한
const maxIn = 100

func (c Ctx) In() int { return max(50, min(maxIn, c.W-6)) }

// Big 은 큰 숫자를 쓸 수 있는지 (DESIGN.md breakpoint.big: 폭 ≥ 60 그리고 높이 ≥ 21)
func (c Ctx) Big() bool { return c.In() >= 60 && c.H >= 21 }

// ---- 글자 조각 ----

type seg struct {
	t      string
	fg, bg lipgloss.Color
	bold   bool
}

func sg(t string, fg lipgloss.Color) seg      { return seg{t: t, fg: fg} }
func sgb(t string, fg, bg lipgloss.Color) seg { return seg{t: t, fg: fg, bg: bg} }
func sgBold(t string, fg lipgloss.Color) seg  { return seg{t: t, fg: fg, bold: true} }
func (s seg) width() int                      { return lipgloss.Width(s.t) }
func fg(t string, c lipgloss.Color) string    { return lipgloss.NewStyle().Foreground(c).Render(t) }
func sp(n int) string                         { return strings.Repeat(" ", max(0, n)) }
func padR(s string, w int) string             { return s + sp(w-lipgloss.Width(s)) }
func padL(s string, w int) string             { return sp(w-lipgloss.Width(s)) + s }
func segsWidth(segs []seg) (n int) {
	for _, s := range segs {
		n += s.width()
	}
	return
}
func hr(l, r string, in int) string { return fg(l+strings.Repeat("─", in)+r, colorTextFaint) }

func (s seg) render() string {
	st := lipgloss.NewStyle().Foreground(s.fg)
	if s.bg != "" {
		st = st.Background(s.bg)
	}
	if s.bold {
		st = st.Bold(true)
	}
	return st.Render(s.t)
}

// truncW 는 표시 폭이 w 를 넘으면 잘라내고 끝에 … 를 붙임
func truncW(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	var b strings.Builder
	n := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if n+rw > w-1 {
			break
		}
		b.WriteRune(r)
		n += rw
	}
	return b.String() + "…"
}

// ---- 프레임 (DESIGN.md: Frame, Header) ----

// Frame 은 둥근 박스 한 겹. 앱은 NewFrame 으로 시작해 Row/Rule 등을 이어 붙이고 String 으로 마무리함.
type Frame struct {
	c     Ctx
	in    int
	lines []string
	below []string
}

// NewFrame 은 윗 테두리, 머리줄(앱 이름 + 시각), 구분선까지 그린 프레임을 돌려줌
func NewFrame(c Ctx) *Frame {
	f := &Frame{c: c, in: c.In()}
	f.lines = append(f.lines, hr("╭", "╮", f.in))
	clk := c.Now.Format("2006-01-02 15:04:05")
	f.RowLR([]seg{sgBold(appName, colorAccent)}, []seg{sg(clk, colorTextMuted)})
	f.Rule()
	return f
}

func (f *Frame) In() int { return f.in }

// Row 는 한 줄 (왼쪽 테두리 + 앞 공백 2칸 + 조각들 + 패딩 + 오른쪽 테두리)
func (f *Frame) Row(segs ...seg) *Frame {
	var b strings.Builder
	b.WriteString(fg("│", colorTextFaint))
	b.WriteString("  ")
	u := 2
	for _, s := range segs {
		b.WriteString(s.render())
		u += s.width()
	}
	if f.in-u > 0 {
		b.WriteString(sp(f.in - u))
	}
	b.WriteString(fg("│", colorTextFaint))
	f.lines = append(f.lines, b.String())
	return f
}

// RowLR 은 왼쪽 조각과 오른쪽 조각을 양 끝에 붙인 한 줄 (오른쪽 여백 2칸)
func (f *Frame) RowLR(left, right []seg) *Frame {
	gap := max(1, f.in-4-segsWidth(left)-segsWidth(right))
	all := append(append(append([]seg{}, left...), sg(sp(gap), colorText)), right...)
	return f.Row(all...)
}

func (f *Frame) Blank() *Frame { return f.Row() }

func (f *Frame) Rule() *Frame {
	f.lines = append(f.lines, hr("├", "┤", f.in))
	return f
}

// Below 는 박스 아래(바깥)에 붙는 줄들 (입력 프롬프트 등). 앞 공백 4칸을 붙여줌
func (f *Frame) Below(lines ...string) *Frame {
	for _, l := range lines {
		f.below = append(f.below, sp(4)+l)
	}
	return f
}

// String 은 아랫 테두리를 붙이고, 가운데 정렬 때 흔들리지 않게 모든 줄을 같은 폭으로 맞춰 돌려줌
func (f *Frame) String() string {
	lines := append(append([]string{}, f.lines...), hr("╰", "╯", f.in))
	lines = append(lines, f.below...)
	w := f.in + 2
	for i, l := range lines {
		lines[i] = l + sp(w-lipgloss.Width(l))
	}
	return strings.Join(lines, "\n")
}

// Place 는 블록을 창 가운데에 놓음
func (c Ctx) Place(block string) string {
	if c.W <= 0 || c.H <= 0 {
		return block
	}
	return lipgloss.Place(c.W, c.H, lipgloss.Center, lipgloss.Center, block)
}

// ---- 배지 (DESIGN.md: Badge) ----

// Badge 는 상태색 배경 + color.on-state 글자. 대기·일시정지는 ToneInfo(muted 배경)로 그림
func Badge(text string, tone Tone) seg {
	return sgb(" "+text+" ", colorOnState, toneColor(tone))
}

// ---- 메시지 줄 (DESIGN.md: Message line) ----

func (f *Frame) Message(text string, tone Tone) *Frame {
	if text == "" {
		return f.Blank()
	}
	return f.Row(sg(text, toneColor(tone)))
}

// ---- 메뉴 줄 (DESIGN.md: Menu row) ----

// Menu 는 `[키] 이름  힌트` 한 줄. dim 이면 이름을 color.text-muted 로 그림
func (f *Frame) Menu(key, label, hint string, dim bool) *Frame {
	lc := colorText
	if dim {
		lc = colorTextMuted
	}
	segs := []seg{sg("[", colorTextMuted), sg(key, colorAccent), sg("]", colorTextMuted), sg("  "+label, lc)}
	if hint != "" {
		segs = append(segs, sg("    "+hint, colorTextMuted))
	}
	return f.Row(segs...)
}

// MenuInline 은 한 줄에 여러 키를 늘어놓음: `[↑↓] 선택    [Enter] 실행`.
// 폭이 모자라면 간격을 2칸으로 줄이고, 그래도 넘치면 두 번째 항목부터 빼서 첫 항목과 마지막 항목(보통 종료)을 지킴.
func (f *Frame) MenuInline(items ...[2]string) *Frame {
	build := func(items [][2]string, gap int) []seg {
		var segs []seg
		for i, it := range items {
			if i > 0 {
				segs = append(segs, sg(sp(gap), colorText))
			}
			segs = append(segs, sg("[", colorTextMuted), sg(it[0], colorAccent), sg("]", colorTextMuted), sg(" "+it[1], colorText))
		}
		return segs
	}
	cur := append([][2]string{}, items...)
	segs := build(cur, 4)
	if segsWidth(segs) > f.in-4 {
		segs = build(cur, 2)
	}
	for segsWidth(segs) > f.in-4 && len(cur) > 2 {
		cur = append(cur[:1], cur[2:]...)
		segs = build(cur, 2)
	}
	return f.Row(segs...)
}

// ---- 큰 숫자 (DESIGN.md: Big digits) ----

var glyph = map[rune][5]string{
	'0': {"###", "#.#", "#.#", "#.#", "###"},
	'1': {".#.", "##.", ".#.", ".#.", "###"},
	'2': {"###", "..#", "###", "#..", "###"},
	'3': {"###", "..#", "###", "..#", "###"},
	'4': {"#.#", "#.#", "###", "..#", "..#"},
	'5': {"###", "#..", "###", "..#", "###"},
	'6': {"###", "#..", "###", "#.#", "###"},
	'7': {"###", "..#", "..#", "..#", "..#"},
	'8': {"###", "#.#", "###", "#.#", "###"},
	'9': {"###", "#.#", "###", "..#", "###"},
	'-': {"...", "...", "###", "...", "..."},
	':': {".", "#", ".", "#", "."},
	'.': {".", ".", ".", ".", "#"},
}

func bigRows(s string) []string {
	rows := make([]string, 5)
	for r := 0; r < 5; r++ {
		parts := make([]string, 0, len(s))
		for _, ch := range s {
			g, ok := glyph[ch]
			if !ok {
				g = glyph['-']
			}
			var sb strings.Builder
			for _, c := range g[r] {
				if c == '#' {
					sb.WriteString("██")
				} else {
					sb.WriteString("  ")
				}
			}
			parts = append(parts, sb.String())
		}
		rows[r] = strings.Join(parts, "  ")
	}
	return rows
}

// Digits 는 큰 숫자(5줄)를 그림. 창이 작거나 폭이 모자라면 `라벨  값` 한 줄로 바꿈.
func (f *Frame) Digits(label, s string, col lipgloss.Color) *Frame {
	if f.c.Big() {
		rows := bigRows(s)
		if lipgloss.Width(rows[0]) <= f.in-6 {
			lead := max(0, (f.in-lipgloss.Width(rows[0]))/2-2)
			for _, r := range rows {
				f.Row(sg(sp(lead), colorText), sg(r, col))
			}
			return f
		}
	}
	return f.Row(sg(label+"  ", colorTextMuted), sg(s, col))
}

// ---- 진행률 막대 (DESIGN.md: Progress bar) ----

func clamp01(p float64) float64 { return math.Min(1, math.Max(0, p)) }

func (f *Frame) Progress(p float64, col lipgloss.Color) *Frame {
	p = clamp01(p)
	bw := f.in - 4 - 6
	n := int(math.Floor(p * float64(bw)))
	return f.Row(
		sg(strings.Repeat("━", n), col),
		sg(strings.Repeat("━", bw-n), colorTextFaint),
		sg(fmt.Sprintf(" %3d%%", int(math.Round(p*100))), col),
	)
}

// ---- 사용률 한 줄 (DESIGN.md: Meter) ----

const meterValW = 16

// Meter 는 `라벨(10칸) 막대 NN% 값` 한 줄. p < 0 이면 읽지 못한 것으로 보고 `-` 를 표시함.
func (f *Frame) Meter(label string, p float64, val string, col lipgloss.Color) *Frame {
	bw := max(8, f.in-4-10-1-1-4-2-meterValW)
	pct, valTxt := "   -", val
	n := 0
	if p >= 0 {
		p = clamp01(p)
		n = int(math.Floor(p * float64(bw)))
		pct = fmt.Sprintf("%3d%%", int(math.Round(p*100)))
	} else {
		col = colorTextMuted
		if valTxt == "" {
			valTxt = "-"
		}
	}
	return f.Row(
		sg(padR(label, 10)+" ", colorText),
		sg(strings.Repeat("━", n), col),
		sg(strings.Repeat("━", bw-n), colorTextFaint),
		sg(" "+pct+"  ", colorText),
		sg(truncW(valTxt, meterValW), colorTextMuted),
	)
}

// ---- 추이 (DESIGN.md: Sparkline) ----

const sparkGlyphs = "▁▂▃▄▅▆▇█"

// Spark 는 최근 값들을 8단계 막대 글자로 그린 한 줄. max 는 맨 위 단계에 해당하는 값
func (f *Frame) Spark(label string, vals []float64, max float64, col lipgloss.Color) *Frame {
	rs := []rune(sparkGlyphs)
	width := f.in - 4 - 10 - 1
	if len(vals) > width {
		vals = vals[len(vals)-width:]
	}
	var b strings.Builder
	for _, v := range vals {
		i := 0
		if max > 0 {
			i = int(math.Round(clamp01(v/max) * float64(len(rs)-1)))
		}
		b.WriteRune(rs[i])
	}
	return f.Row(sg(padR(label, 10)+" ", colorTextMuted), sg(b.String(), col))
}

// ---- 탭 줄 (DESIGN.md: Tab row) ----

func (f *Frame) Tabs(label string, items []string, sel int) *Frame {
	segs := []seg{sg(padR(label, 6), colorTextMuted)}
	for i, it := range items {
		if i == sel {
			segs = append(segs, sgBold("[ "+it+" ]", colorAccent))
		} else {
			segs = append(segs, sg("  "+it+"  ", colorTextMuted))
		}
	}
	return f.Row(segs...)
}

// ---- 표 (DESIGN.md: Table) ----

// Cells 는 표의 한 줄. 첫 칸은 왼쪽 정렬, 나머지는 오른쪽 정렬함.
func (f *Frame) Cells(widths []int, cells []string, col lipgloss.Color, bold bool) *Frame {
	var segs []seg
	for i, c := range cells {
		w := widths[i]
		txt := truncW(c, w)
		if i == 0 {
			txt = padR(txt, w)
		} else {
			txt = padL(txt, w)
		}
		s := sg(txt, col)
		s.bold = bold
		segs = append(segs, s)
	}
	return f.Row(segs...)
}

// ---- 막대 줄 (DESIGN.md: Bar row) ----

const barRowMax = 28

// BarRow 는 `라벨 █████ 값` 한 줄. accent 가 true 면 막대를 color.accent, 아니면 color.text-muted 로 그림
func (f *Frame) BarRow(label string, v, max float64, text string, accent bool) *Frame {
	n := 0
	if max > 0 {
		n = int(math.Round(clamp01(v/max) * barRowMax))
	}
	col := colorTextMuted
	if accent {
		col = colorAccent
	}
	segs := []seg{sg(padR(label, 12), colorTextMuted), sg(strings.Repeat("█", n), col)}
	if n > 0 {
		segs = append(segs, sg(" ", colorText))
	}
	return f.Row(append(segs, sg(text, colorText))...)
}

// ---- 체크 줄 (DESIGN.md: Check row) ----

// Check 는 할 일 한 줄 `▸ [x] 내용 ... 완료`. 완료 항목은 color.text-muted 와 끝의 `완료` 글자로 구분함
func (f *Frame) Check(text string, done, selected bool) *Frame {
	mark, box, tc := " ", "[ ]", colorText
	if selected {
		mark = "▸"
	}
	if done {
		box, tc = "[x]", colorTextMuted
	}
	left := []seg{sg(mark+" ", colorAccent), sg(box, colorAccent), sg(" ", colorText), sg(truncW(text, f.in-4-10), tc)}
	if selected {
		left[3].bold = true
	}
	var right []seg
	if done {
		right = []seg{sg("완료", colorTextMuted)}
	}
	return f.RowLR(left, right)
}

// ---- 입력 줄 (DESIGN.md: Input) ----

// PromptLine 은 박스 아래에 놓는 `프롬프트 > 값` 한 줄의 문자열
func PromptLine(prompt, value string) string {
	return fg(prompt, colorAccent) + value
}
