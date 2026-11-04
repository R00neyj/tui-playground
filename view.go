package main

import (
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	cBlack    = lipgloss.Color("0")
	cRed      = lipgloss.Color("9")
	cGreen    = lipgloss.Color("10")
	cYellow   = lipgloss.Color("11")
	cBlue     = lipgloss.Color("12")
	cCyan     = lipgloss.Color("14")
	cWhite    = lipgloss.Color("15")
	cGray     = lipgloss.Color("7")
	cDarkGray = lipgloss.Color("8")
	cDarkCyan = lipgloss.Color("6")
)

type seg struct {
	t      string
	fg, bg lipgloss.Color
}

func sg(t string, fg lipgloss.Color) seg { return seg{t: t, fg: fg} }

func (s seg) render() string {
	st := lipgloss.NewStyle().Foreground(s.fg)
	if s.bg != "" {
		st = st.Background(s.bg)
	}
	return st.Render(s.t)
}

func fg(t string, c lipgloss.Color) string { return lipgloss.NewStyle().Foreground(c).Render(t) }

func boxW(w int) int { return max(50, min(72, w-6)) }

func hr(l, r string, in int) string { return fg(l+strings.Repeat("─", in)+r, cDarkGray) }

// ln: 테두리 안쪽 한 줄 (앞 공백 2칸 + 세그먼트들 + 오른쪽 패딩)
func ln(in int, segs ...seg) string {
	var b strings.Builder
	b.WriteString(fg("│", cDarkGray))
	b.WriteString("  ")
	u := 2
	for _, s := range segs {
		b.WriteString(s.render())
		u += lipgloss.Width(s.t)
	}
	if in-u > 0 {
		b.WriteString(strings.Repeat(" ", in-u))
	}
	b.WriteString(fg("│", cDarkGray))
	return b.String()
}

// ---- 큰 숫자 ----

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
}

func bigRows(s string) []string {
	rows := make([]string, 5)
	for r := 0; r < 5; r++ {
		parts := make([]string, 0, len(s))
		for _, ch := range s {
			g := glyph[ch][r]
			var sb strings.Builder
			for _, c := range g {
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

// ---- 메인 화면 ----

func (m model) center(block string) string {
	if m.w <= 0 || m.h <= 0 {
		return block
	}
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, block)
}

func (m model) viewMain() string {
	now := time.Now()
	c := m.cur
	in := boxW(m.w)
	clk := now.Format("2006-01-02 15:04:05")

	var col lipgloss.Color
	var p float64
	var badge seg
	var tg, rm string
	if c != nil {
		rem := c.Target.Sub(now)
		if rem < 0 {
			rem = 0
		}
		switch {
		case rem <= time.Minute:
			col = cRed
		case rem <= 10*time.Minute:
			col = cYellow
		default:
			col = cGreen
		}
		if total := c.Target.Sub(c.Start).Seconds(); total > 0 {
			p = math.Min(1, math.Max(0, now.Sub(c.Start).Seconds()/total))
		}
		badge = seg{" 예약됨 ", cBlack, col}
		tg, rm = c.Target.Format("2006-01-02 15:04:05"), fmtDur(rem)
	} else {
		col = cDarkGray
		badge = seg{" 대기 ", cWhite, cDarkGray}
		tg, rm = "-", "--:--:--"
	}

	var big []string
	if in >= 60 && m.h >= 21 {
		big = bigRows(rm)
		if lipgloss.Width(big[0]) > in-6 {
			big = nil
		}
	}

	bwid := in - 4 - 6
	f := int(math.Floor(p * float64(bwid)))
	pct := int(math.Round(p * 100))

	menu2 := cDarkGray
	if c != nil {
		menu2 = cWhite
	}

	lines := []string{
		hr("╭", "╮", in),
		ln(in, sg(appName, cCyan), sg(strings.Repeat(" ", max(1, in-4-lipgloss.Width(appName)-len(clk))), cGray), sg(clk, cDarkGray)),
		hr("├", "┤", in),
		ln(in),
		ln(in, badge, sg("   종료시각  ", cDarkGray), sg(tg, cWhite)),
		ln(in),
	}
	if len(big) > 0 {
		lead := max(0, (in-lipgloss.Width(big[0]))/2-2)
		for _, row := range big {
			lines = append(lines, ln(in, sg(strings.Repeat(" ", lead), cGray), sg(row, col)))
		}
	} else {
		lines = append(lines, ln(in, sg("남은시간  ", cDarkGray), sg(rm, col)))
	}
	lines = append(lines,
		ln(in),
		ln(in, sg(strings.Repeat("━", f), col), sg(strings.Repeat("━", bwid-f), cDarkGray), sg(pad3(pct)+"%", col)),
		hr("├", "┤", in),
		ln(in, sg("[", cDarkGray), sg("1", cCyan), sg("]", cDarkGray), sg("  새 타이머 설정", cWhite), sg("    기존 타이머는 자동 취소", cDarkGray)),
		ln(in, sg("[", cDarkGray), sg("2", cCyan), sg("]", cDarkGray), sg("  타이머 취소", menu2)),
		ln(in, sg("[", cDarkGray), sg("3", cCyan), sg("]", cDarkGray), sg("  종료", cWhite), sg("    Esc 도 가능", cDarkGray)),
		hr("├", "┤", in),
		ln(in, sg(m.msg, m.msgCol)),
		hr("╰", "╯", in),
	)
	return m.center(strings.Join(lines, "\n"))
}

func pad3(n int) string {
	s := itoa(n)
	return strings.Repeat(" ", max(0, 4-len(s))) + s
}

// ---- 새 타이머 입력 화면 ----

func (m model) viewReserve() string {
	in := boxW(m.w)
	lines := []string{
		hr("╭", "╮", in),
		ln(in, sg("NEW TIMER", cCyan)),
		hr("├", "┤", in),
	}
	if m.cur != nil {
		lines = append(lines,
			ln(in, sg("현재 예약  ", cDarkGray), sg(m.cur.Target.Format("15:04:05")+"  (남은 "+fmtDur(time.Until(m.cur.Target))+")", cYellow)),
			ln(in, sg("새로 설정하면 기존 타이머는 자동 취소됨", cDarkGray)),
		)
	} else {
		lines = append(lines, ln(in, sg("현재 예약된 타이머 없음", cDarkGray)))
	}
	lines = append(lines,
		ln(in),
		ln(in, sg("입력 예  ", cDarkGray), sg("30", cWhite), sg("  /  ", cDarkGray), sg("90", cWhite), sg("  /  ", cDarkGray), sg("1h30m", cWhite), sg("  /  ", cDarkGray), sg("2h", cWhite)),
		ln(in, sg("Enter = 30분    0 / Esc = 뒤로", cDarkGray)),
		hr("╰", "╯", in),
		"",
		"    "+m.input.View(),
	)
	if m.inputErr != "" {
		lines = append(lines, "    "+fg(m.inputErr, cRed))
	} else {
		lines = append(lines, "")
	}
	// 입력 줄이 가운데 정렬로 흔들리지 않도록 박스 폭에 맞춰 왼쪽 정렬한 블록으로 배치
	block := lipgloss.NewStyle().Width(in + 2).Render(strings.Join(lines, "\n"))
	return m.center(block)
}
