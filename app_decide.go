package main

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// 결정 도구. 주사위·동전·사다리 타기를 Tab 으로 바꿔 씀. 결과는 메모리에만 있음.

var decideInfo = AppInfo{
	ID:   "decide",
	Name: "결정 도구",
	Desc: "주사위, 동전, 사다리 타기",
	New:  newDecideApp,
}

var decideRand = rand.Intn

const (
	decideBody  = 7 // 탭 아래 본문 줄 수 (창 높이 17 에서도 들어가게)
	decideFrame = 60 * time.Millisecond
	diceMax     = 5
	diceRoll    = 14 // 굴리는 프레임 수
	coinSpin    = 24
	ladderMin   = 2
	ladderMax   = 6
	ladderRows  = 5
)

type decideApp struct {
	gen, epoch int
	tab        int
	busy       int // 남은 애니메이션 프레임 (0 이면 멈춤)

	dice   []int
	nDice  int
	rolled bool

	coinFace  bool // true 앞면
	coinAngle float64
	heads     int
	tails     int
	flipped   bool

	ladN    int
	rungs   [ladderRows][ladderMax - 1]bool
	prizes  []string
	ladSel  int
	traced  []bool // 출발점마다 결과를 열었는지
	showAll bool
}

func newDecideApp(env *Env) App {
	a := &decideApp{gen: newGen(), nDice: 2, ladN: 4}
	a.dice = []int{1, 1}
	a.newLadder()
	return a
}

func (a *decideApp) Init() tea.Cmd { return nil }

func (a *decideApp) tag() string { return "decide" + strconv.Itoa(a.epoch) }

func (a *decideApp) animate(frames int) tea.Cmd {
	a.epoch++
	a.busy = frames
	return tickCmd(decideFrame, a.gen, a.tag())
}

func (a *decideApp) newLadder() {
	// 모든 틈에 가로줄이 하나 이상 있을 때까지 다시 뽑음 (한 번에 될 확률이 높아서 보통 몇 번 안에 끝남)
	for try := 0; ; try++ {
		a.rungs = [ladderRows][ladderMax - 1]bool{}
		if try >= 200 { // 거의 일어나지 않지만 무한 반복을 막는 대체 배치: 이웃 틈끼리 다른 줄을 씀
			for g := 0; g < a.ladN-1; g++ {
				a.rungs[(g%2)*2][g] = true
			}
			break
		}
		for g := 0; g < a.ladN-1; g++ {
			for r := 0; r < ladderRows; r++ {
				left := g > 0 && a.rungs[r][g-1]
				a.rungs[r][g] = !left && decideRand(5) < 2 // 40%
			}
		}
		if a.everyGapHasRung() {
			break
		}
	}
	a.prizes = make([]string, a.ladN)
	for i := range a.prizes {
		a.prizes[i] = "꽝"
	}
	a.prizes[decideRand(a.ladN)] = "당첨"
	a.traced = make([]bool, a.ladN)
	a.showAll = false
	a.ladSel = min(a.ladSel, a.ladN-1)
}

func (a *decideApp) everyGapHasRung() bool {
	for g := 0; g < a.ladN-1; g++ {
		has := false
		for r := 0; r < ladderRows; r++ {
			has = has || a.rungs[r][g]
		}
		if !has {
			return false
		}
	}
	return true
}

// ladderPath 는 출발점 s 에서 내려가며 지나는 (줄, 기둥) 과 가로줄 (줄, 틈), 도착 기둥을 돌려줌
func (a *decideApp) ladderPath(s int) (cols [ladderRows]int, rungs map[[2]int]bool, end int) {
	rungs = map[[2]int]bool{}
	c := s
	for r := 0; r < ladderRows; r++ {
		cols[r] = c
		switch {
		case c < a.ladN-1 && a.rungs[r][c]:
			rungs[[2]int{r, c}] = true
			c++
		case c > 0 && a.rungs[r][c-1]:
			rungs[[2]int{r, c - 1}] = true
			c--
		}
	}
	return cols, rungs, c
}

func (a *decideApp) Update(msg tea.Msg) (App, tea.Cmd) {
	if cmd, ok := forwardHangul(msg, func(m tea.Msg) tea.Cmd { _, c := a.Update(m); return c }); ok { // 한글 입력 상태에서도 단축키가 듣게 함
		return a, cmd
	}
	switch m := msg.(type) {
	case appTickMsg:
		if m.Gen != a.gen || m.Tag != a.tag() || a.busy <= 0 {
			return a, nil
		}
		a.busy--
		a.frame()
		if a.busy > 0 {
			// 끝으로 갈수록 느려지게 간격을 늘림
			return a, tickCmd(decideFrame+time.Duration(max(0, 8-a.busy))*15*time.Millisecond, a.gen, a.tag())
		}
		a.finish()
		return a, nil
	case tea.KeyMsg:
		k := m.String()
		switch k {
		case "esc", "q":
			return a, goHome()
		case "tab":
			a.tab, a.busy = (a.tab+1)%3, 0
			a.epoch++
			return a, nil
		case "shift+tab":
			a.tab, a.busy = (a.tab+2)%3, 0
			a.epoch++
			return a, nil
		}
		if a.busy > 0 {
			return a, nil
		}
		switch a.tab {
		case 0:
			return a, a.diceKey(k)
		case 1:
			return a, a.coinKey(k)
		default:
			a.ladderKey(k)
		}
	}
	return a, nil
}

func (a *decideApp) frame() {
	switch a.tab {
	case 0:
		for i := range a.dice {
			a.dice[i] = 1 + decideRand(6)
		}
	case 1:
		a.coinAngle += 0.35 + float64(a.busy)*0.04
	}
}

func (a *decideApp) finish() {
	if a.tab == 0 {
		a.rolled = true
	}
	if a.tab == 1 {
		a.coinFace = decideRand(2) == 0
		a.coinAngle = 0
		a.flipped = true
		if a.coinFace {
			a.heads++
		} else {
			a.tails++
		}
	}
}

func (a *decideApp) diceKey(k string) tea.Cmd {
	switch k {
	case " ", "space", "enter":
		return a.animate(diceRoll)
	case "up", "+", "=":
		a.nDice = min(diceMax, a.nDice+1)
	case "down", "-":
		a.nDice = max(1, a.nDice-1)
	default:
		return nil
	}
	for len(a.dice) < a.nDice {
		a.dice = append(a.dice, 1)
	}
	a.dice = a.dice[:a.nDice]
	return nil
}

func (a *decideApp) coinKey(k string) tea.Cmd {
	switch k {
	case " ", "space", "enter":
		return a.animate(coinSpin)
	case "r", "R":
		a.heads, a.tails, a.flipped = 0, 0, false
	}
	return nil
}

func (a *decideApp) ladderKey(k string) {
	switch k {
	case "left":
		a.ladSel = max(0, a.ladSel-1)
	case "right":
		a.ladSel = min(a.ladN-1, a.ladSel+1)
	case " ", "space", "enter":
		a.traced[a.ladSel] = true
	case "a", "A":
		a.showAll = true
	case "up", "+", "=":
		a.ladN = min(ladderMax, a.ladN+1)
		a.newLadder()
	case "down", "-":
		a.ladN = max(ladderMin, a.ladN-1)
		a.newLadder()
	case "r", "R":
		a.newLadder()
	}
}

// ---- 화면 ----

func (a *decideApp) View(c Ctx) string {
	f := NewFrame(c).Tabs("", []string{"주사위", "동전", "사다리"}, a.tab)
	var body [][]seg
	var keys [][2]string
	var msg string
	switch a.tab {
	case 0:
		body, msg = a.diceView(f.In() - 4)
		keys = [][2]string{{"Space", "굴리기"}, {"↑↓", "개수"}}
	case 1:
		body, msg = a.coinView(f.In() - 4)
		keys = [][2]string{{"Space", "던지기"}, {"r", "횟수 지우기"}}
	default:
		body, msg = a.ladderView(f.In() - 4)
		keys = [][2]string{{"←→", "출발점"}, {"r", "새 사다리"}, {"↑↓", "인원"}, {"a", "모두 열기"}, {"Space", "따라가기"}}
	}
	for i := 0; i < decideBody; i++ {
		if i < len(body) {
			f.Row(body[i]...)
		} else {
			f.Blank()
		}
	}
	f.Rule()
	f.MenuInline(append(keys, [2]string{"Tab", "도구"}, [2]string{"Esc", "홈으로"})...)
	f.Rule()
	tone := ToneInfo
	if a.busy == 0 && msg != "" && !strings.HasPrefix(msg, "Space") {
		tone = ToneAccent
	}
	f.Message(msg, tone)
	return f.String()
}

// pips 는 주사위 눈 위치 (3x3 격자 칸 번호 0..8)
var dicePips = [7][]int{{}, {4}, {0, 8}, {0, 4, 8}, {0, 2, 6, 8}, {0, 2, 4, 6, 8}, {0, 2, 3, 5, 6, 8}}

func (a *decideApp) diceView(w int) ([][]seg, string) {
	const dw, dh, gap = 8, 4, 1 // 주사위 하나의 칸 크기
	n := len(a.dice)
	total := n*dw + (n-1)*gap
	ox := max(0, (w-total)/2)
	cv := newCanvas(w, dh)
	for i, v := range a.dice {
		x0 := (ox + i*(dw+gap)) * 2
		x1 := x0 + dw*2 - 1
		y0, y1 := 0, dh*4-1
		cv.Line(x0, y0, x1, y0)
		cv.Line(x0, y1, x1, y1)
		cv.Line(x0, y0, x0, y1)
		cv.Line(x1, y0, x1, y1)
		for _, p := range dicePips[v] {
			px := float64(x0) + float64(dw*2)*(0.25+0.25*float64(p%3))
			py := float64(y0) + float64(dh*4)*(0.25+0.25*float64(p/3))
			cv.FillCircle(px, py, 1.3)
		}
	}
	col := colorText
	if a.busy > 0 {
		col = colorTextMuted
	}
	body := [][]seg{nil}
	body = append(body, cv.Rows(col)...)
	body = append(body, nil)
	sum := 0
	parts := make([]string, n)
	for i, v := range a.dice {
		sum += v
		parts[i] = strconv.Itoa(v)
	}
	msg := "Space 로 굴림"
	if a.busy == 0 && a.rolled {
		msg = fmt.Sprintf("나온 눈 %s", strings.Join(parts, " "))
		if n > 1 {
			msg += fmt.Sprintf(" · 합 %d", sum)
		}
	}
	return body, msg
}

func (a *decideApp) coinView(w int) ([][]seg, string) {
	const ch = 5
	cv := newCanvas(w, ch)
	cx, cy := float64(cv.W())/2, float64(cv.H())/2
	r := float64(cv.H())/2 - 1
	front := a.coinFace
	rx := r
	if a.busy > 0 {
		cs := math.Cos(a.coinAngle)
		rx = math.Max(0.6, r*math.Abs(cs))
		front = cs > 0
	}
	plot := func(x, y int) { cv.Set(x, y) }
	if front {
		bubbleEllipse(plot, cx, cy, rx, r, 0)
	} else {
		bubbleEllipse(plot, cx, cy, rx, r, math.Max(0.4, 1-2.2/math.Max(rx, 1)))
	}
	col := colorWarn
	if a.busy > 0 {
		col = colorTextMuted
	}
	body := [][]seg{nil}
	body = append(body, cv.Rows(col)...)
	msg := "Space 로 던짐 (꽉 찬 원 = 앞면, 고리 = 뒷면)"
	if a.busy == 0 && a.flipped {
		face := "뒷면"
		if a.coinFace {
			face = "앞면"
		}
		msg = fmt.Sprintf("%s! · 지금까지 앞 %d · 뒤 %d", face, a.heads, a.tails)
	}
	return body, msg
}

func (a *decideApp) ladderView(w int) ([][]seg, string) {
	n := a.ladN
	gapW := min(10, (w-1)/max(1, n-1))
	total := (n-1)*gapW + 1
	ox := (w - total) / 2
	colX := func(i int) int { return ox + i*gapW }

	// 강조할 칸: 선택한 출발점의 길 + 열어 본 출발점들의 길
	hlCol := map[[2]int]lipgloss.Color{}
	hlRung := map[[2]int]lipgloss.Color{}
	revealed := map[int]int{} // 도착 기둥 -> 출발점
	for s := 0; s < n; s++ {
		if !(a.traced[s] || a.showAll) {
			continue
		}
		cols, rungs, end := a.ladderPath(s)
		revealed[end] = s
		if s != a.ladSel {
			continue
		}
		for r, c := range cols {
			hlCol[[2]int{r, c}] = colorAccent
		}
		for k := range rungs {
			hlRung[k] = colorAccent
			hlCol[[2]int{k[0], k[1]}] = colorAccent
			hlCol[[2]int{k[0], k[1] + 1}] = colorAccent
		}
	}

	type cell struct {
		t   string
		col lipgloss.Color
	}
	line := func() []cell {
		cs := make([]cell, w)
		for i := range cs {
			cs[i] = cell{" ", colorText}
		}
		return cs
	}
	toSegs := func(cs []cell) []seg {
		var out []seg
		for _, c := range cs {
			if c.t == "" {
				continue // 넓은 글자의 둘째 칸
			}
			if n := len(out); n > 0 && out[n-1].fg == c.col && !out[n-1].bold {
				out[n-1].t += c.t
				continue
			}
			out = append(out, sg(c.t, c.col))
		}
		return out
	}
	put := func(cs []cell, x int, s string, col lipgloss.Color) {
		for _, r := range s {
			rw := lipgloss.Width(string(r))
			if x < 0 || x+rw > len(cs) {
				return
			}
			cs[x] = cell{string(r), col}
			for k := 1; k < rw; k++ {
				cs[x+k] = cell{"", col}
			}
			x += rw
		}
	}

	var body [][]seg
	top := line()
	for i := 0; i < n; i++ {
		col := colorTextMuted
		if i == a.ladSel {
			col = colorAccent
		}
		put(top, colX(i), strconv.Itoa(i+1), col)
	}
	body = append(body, toSegs(top))
	for r := 0; r < ladderRows; r++ {
		cs := line()
		for i := 0; i < n; i++ {
			col := colorTextMuted
			if c, ok := hlCol[[2]int{r, i}]; ok {
				col = c
			}
			glyph := "│"
			right := i < n-1 && a.rungs[r][i]
			left := i > 0 && a.rungs[r][i-1]
			switch {
			case right && left:
				glyph = "┼"
			case right:
				glyph = "├"
			case left:
				glyph = "┤"
			}
			put(cs, colX(i), glyph, col)
			if right {
				rc := colorTextMuted
				if c, ok := hlRung[[2]int{r, i}]; ok {
					rc = c
				}
				put(cs, colX(i)+1, strings.Repeat("─", gapW-1), rc)
			}
		}
		body = append(body, toSegs(cs))
	}
	bot := line()
	for i := 0; i < n; i++ {
		label, col := "?", colorTextMuted
		if s, ok := revealed[i]; ok {
			label, col = a.prizes[i], colorText
			if label == "당첨" {
				col = colorOk
			}
			if s == a.ladSel {
				col = colorAccent
				if label == "당첨" {
					col = colorOk
				}
			}
		}
		lw := lipgloss.Width(label)
		put(bot, min(max(0, colX(i)-(lw-1)/2), w-lw), label, col) // 가장자리 기둥의 글자가 판 밖으로 잘리지 않게
	}
	body = append(body, toSegs(bot))

	msg := fmt.Sprintf("Space 로 %d번 길 따라가기 · %d명 중 당첨 1", a.ladSel+1, n)
	if a.traced[a.ladSel] || a.showAll {
		_, _, end := a.ladderPath(a.ladSel)
		msg = fmt.Sprintf("%d번 → %s", a.ladSel+1, a.prizes[end])
	}
	return body, msg
}
