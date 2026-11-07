package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// 지뢰 찾기. 칸을 열어 숫자(주변 지뢰 수)를 보고 지뢰가 없는 칸을 모두 열면 이김.
// 첫 칸과 그 주변에는 지뢰를 두지 않음. 최고 기록(초)은 scores.json 의 "mines" (작을수록 좋음).

var minesInfo = AppInfo{
	ID:   "mines",
	Name: "지뢰 찾기",
	Desc: "숫자를 보고 지뢰 피해 열기",
	New:  newMinesApp,
}

const (
	minesW     = 15
	minesH     = 9
	minesCount = 18
	minesCellW = 3
	minesScore = "mines"
)

var (
	minesNow  = time.Now
	minesRand = rand.Intn
)

type minesPhase int

const (
	minesFresh minesPhase = iota // 아직 지뢰를 놓지 않음 (첫 칸을 열 때 놓음)
	minesPlay
	minesWon
	minesLost
)

type minesApp struct {
	ph      minesPhase
	mine    [minesW * minesH]bool
	open    [minesW * minesH]bool
	flag    [minesW * minesH]bool
	cx, cy  int
	opened  int
	flags   int
	start   time.Time
	end     time.Time
	best    int
	newBest bool
	boomAt  int
}

func newMinesApp(env *Env) App {
	a := &minesApp{cx: minesW / 2, cy: minesH / 2, boomAt: -1}
	if v, ok := loadScore(minesScore); ok && v > 0 {
		a.best = v
	}
	return a
}

func (a *minesApp) Init() tea.Cmd { return nil }

func (a *minesApp) neighbors(i int, fn func(j int)) {
	x, y := i%minesW, i/minesW
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			nx, ny := x+dx, y+dy
			if (dx != 0 || dy != 0) && nx >= 0 && ny >= 0 && nx < minesW && ny < minesH {
				fn(ny*minesW + nx)
			}
		}
	}
}

func (a *minesApp) around(i int) (n int) {
	a.neighbors(i, func(j int) {
		if a.mine[j] {
			n++
		}
	})
	return
}

// plant 는 safe 칸과 그 이웃을 뺀 곳에 지뢰를 무작위로 놓음
func (a *minesApp) plant(safe int) {
	ban := map[int]bool{safe: true}
	a.neighbors(safe, func(j int) { ban[j] = true })
	var spots []int
	for i := 0; i < minesW*minesH; i++ {
		if !ban[i] {
			spots = append(spots, i)
		}
	}
	for k := 0; k < minesCount && len(spots) > 0; k++ {
		j := minesRand(len(spots))
		a.mine[spots[j]] = true
		spots = append(spots[:j], spots[j+1:]...)
	}
	a.ph = minesPlay
	a.start = minesNow()
}

// reveal 은 칸 하나를 엶. 0 이면 주변을 이어서 엶 (반복문이라 판이 커도 스택이 넘치지 않음)
func (a *minesApp) reveal(i int) {
	if a.open[i] || a.flag[i] {
		return
	}
	if a.mine[i] {
		a.boomAt = i
		a.finish(false)
		return
	}
	stack := []int{i}
	for len(stack) > 0 {
		j := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if a.open[j] || a.flag[j] || a.mine[j] {
			continue
		}
		a.open[j] = true
		a.opened++
		if a.around(j) == 0 {
			a.neighbors(j, func(k int) { stack = append(stack, k) })
		}
	}
	if a.opened == minesW*minesH-minesCount {
		a.finish(true)
	}
}

func (a *minesApp) finish(won bool) {
	a.end = minesNow()
	if !won {
		a.ph = minesLost
		return
	}
	a.ph = minesWon
	if s := a.secs(); a.best == 0 || s < a.best {
		a.best, a.newBest = s, true
		_ = saveScore(minesScore, s)
	}
}

func (a *minesApp) secs() int {
	switch a.ph {
	case minesFresh:
		return 0
	case minesPlay:
		return int(minesNow().Sub(a.start).Seconds())
	}
	return max(1, int(a.end.Sub(a.start).Seconds()))
}

// chord 는 열린 숫자 칸에서 둘레 깃발 수가 숫자와 같으면 깃발 없는 둘레 칸을 한꺼번에 엶
func (a *minesApp) chord(i int) {
	f := 0
	a.neighbors(i, func(j int) {
		if a.flag[j] {
			f++
		}
	})
	if f != a.around(i) {
		return
	}
	a.neighbors(i, func(j int) {
		if a.ph == minesPlay {
			a.reveal(j)
		}
	})
}

func (a *minesApp) Update(msg tea.Msg) (App, tea.Cmd) {
	if cmd, ok := forwardHangul(msg, func(m tea.Msg) tea.Cmd { _, c := a.Update(m); return c }); ok { // 한글 입력 상태에서도 단축키가 듣게 함
		return a, cmd
	}
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return a, nil
	}
	i := a.cy*minesW + a.cx
	switch k.String() {
	case "esc", "q":
		return a, goHome()
	case "left":
		a.cx = max(0, a.cx-1)
	case "right":
		a.cx = min(minesW-1, a.cx+1)
	case "up":
		a.cy = max(0, a.cy-1)
	case "down":
		a.cy = min(minesH-1, a.cy+1)
	case "r", "R":
		best := a.best
		*a = minesApp{cx: a.cx, cy: a.cy, best: best, boomAt: -1}
	case " ", "space", "enter":
		switch a.ph {
		case minesFresh:
			a.plant(i)
			a.reveal(i)
		case minesPlay:
			if a.open[i] {
				a.chord(i)
			} else {
				a.reveal(i)
			}
		}
	case "f", "F":
		if (a.ph == minesPlay || a.ph == minesFresh) && !a.open[i] {
			a.flag[i] = !a.flag[i]
			if a.flag[i] {
				a.flags++
			} else {
				a.flags--
			}
		}
	}
	return a, nil
}

var minesNumColor = map[int]lipgloss.Color{1: colorAccent, 2: colorOk, 3: colorWarn}

// cell 은 칸 하나의 글자(3칸 폭)와 색
func (a *minesApp) cell(i int) seg {
	done := a.ph == minesWon || a.ph == minesLost
	switch {
	case done && a.mine[i] && i == a.boomAt:
		return seg{t: " * ", fg: colorOnState, bg: colorWarn, bold: true}
	case done && a.mine[i] && !a.flag[i]:
		return sgBold(" * ", colorWarn)
	case done && a.flag[i] && !a.mine[i]:
		return sg(" x ", colorTextMuted)
	case a.flag[i]:
		return sgBold(" F ", colorWarn)
	case !a.open[i]:
		return sg(" . ", colorTextMuted)
	}
	n := a.around(i)
	if n == 0 {
		return sg("   ", colorText)
	}
	col, ok := minesNumColor[n]
	if !ok {
		col = colorText
	}
	return sgBold(" "+strconv.Itoa(n)+" ", col)
}

func (a *minesApp) View(c Ctx) string {
	f := NewFrame(c)
	lead := sg(sp((f.In()-4-minesW*minesCellW)/2), colorText)
	for y := 0; y < minesH; y++ {
		segs := []seg{lead}
		for x := 0; x < minesW; x++ {
			i := y*minesW + x
			s := a.cell(i)
			if x == a.cx && y == a.cy && a.ph != minesWon && a.ph != minesLost {
				s.bg, s.bold = colorSelect, true
				if s.fg == colorTextMuted {
					s.fg = colorText
				}
			}
			segs = append(segs, s)
		}
		f.Row(segs...)
	}
	f.Rule()
	f.MenuInline([2]string{"←→↑↓", "이동"}, [2]string{"Space", "열기"}, [2]string{"f", "깃발"}, [2]string{"r", "새 판"}, [2]string{"Esc", "홈으로"})
	f.Rule()
	best := ""
	if a.best > 0 {
		best = fmt.Sprintf(" · 최고 %d초", a.best)
	}
	switch a.ph {
	case minesWon:
		msg := fmt.Sprintf("다 찾음! %d초", a.secs())
		if a.newBest {
			msg += " 새 최고 기록!"
		}
		f.Message(msg+"  r 로 새 판", ToneOk)
	case minesLost:
		f.Message("펑! 지뢰를 밟았음. r 로 새 판", ToneWarn)
	default:
		f.Message(fmt.Sprintf("지뢰 %d · 깃발 %d · %d초%s", minesCount, a.flags, a.secs(), best), ToneInfo)
	}
	return f.String()
}
