package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// 뱀. 방향키로 뱀을 움직여 먹이를 먹으면 길어지고 빨라짐. 벽이나 자기 몸에 부딪히면 끝.
// 판은 snakeW x snakeH 칸이고, 위아래 반 칸 글자(▀▄█)로 한 줄에 두 칸씩 그려서 칸이 거의 정사각형으로 보임.
// 최고 점수는 scores.json 의 "snake".

var snakeInfo = AppInfo{
	ID:   "snake",
	Name: "뱀 게임",
	Desc: "먹이를 먹고 길어지기",
	New:  newSnakeApp,
}

const (
	snakeW       = 40
	snakeH       = 16 // 짝수 (두 칸이 한 줄)
	snakeStart   = 130 * time.Millisecond
	snakeFastest = 60 * time.Millisecond
	snakeSpeedUp = 4 * time.Millisecond // 먹이 하나마다 빨라지는 정도
	snakeScore   = "snake"
)

var snakeRand = rand.Intn

type snakePhase int

const (
	snakeReady snakePhase = iota
	snakeRun
	snakePause
	snakeOver
)

type pt struct{ x, y int }

type snakeApp struct {
	gen, epoch int
	ph         snakePhase
	body       []pt // [0] 이 머리
	dir        pt
	queue      []pt // 다음 칸들에서 쓸 방향 (빠르게 두 번 꺾는 입력을 놓치지 않게)
	food       pt
	score      int
	best       int
	newBest    bool
	won        bool
}

func newSnakeApp(env *Env) App {
	a := &snakeApp{gen: newGen()}
	if v, ok := loadScore(snakeScore); ok {
		a.best = v
	}
	a.reset()
	return a
}

func (a *snakeApp) reset() {
	y := snakeH / 2
	a.body = []pt{{snakeW/2 + 1, y}, {snakeW / 2, y}, {snakeW/2 - 1, y}, {snakeW/2 - 2, y}}
	a.dir, a.queue = pt{1, 0}, nil
	a.score, a.newBest, a.won = 0, false, false
	a.ph = snakeReady
	a.placeFood()
}

func (a *snakeApp) occupied(p pt) bool {
	for _, b := range a.body {
		if b == p {
			return true
		}
	}
	return false
}

func (a *snakeApp) placeFood() bool {
	free := snakeW*snakeH - len(a.body)
	if free <= 0 {
		return false
	}
	n := snakeRand(free)
	for y := 0; y < snakeH; y++ {
		for x := 0; x < snakeW; x++ {
			if p := (pt{x, y}); !a.occupied(p) {
				if n == 0 {
					a.food = p
					return true
				}
				n--
			}
		}
	}
	return false
}

func (a *snakeApp) Init() tea.Cmd { return nil }

func (a *snakeApp) tag() string { return "snake" + strconv.Itoa(a.epoch) }

func (a *snakeApp) speed() time.Duration {
	return max(snakeFastest, snakeStart-time.Duration(a.score)*snakeSpeedUp)
}

func (a *snakeApp) run() tea.Cmd {
	a.epoch++
	a.ph = snakeRun
	return tickCmd(a.speed(), a.gen, a.tag())
}

func (a *snakeApp) turn(d pt) tea.Cmd {
	last := a.dir
	if n := len(a.queue); n > 0 {
		last = a.queue[n-1]
	}
	if d != last && (d.x != -last.x || d.y != -last.y) && len(a.queue) < 2 {
		a.queue = append(a.queue, d)
	}
	if a.ph == snakeReady || a.ph == snakePause {
		return a.run()
	}
	return nil
}

func (a *snakeApp) Update(msg tea.Msg) (App, tea.Cmd) {
	if cmd, ok := forwardHangul(msg, func(m tea.Msg) tea.Cmd { _, c := a.Update(m); return c }); ok { // 한글 입력 상태에서도 단축키가 듣게 함
		return a, cmd
	}
	switch m := msg.(type) {
	case appTickMsg:
		if m.Gen != a.gen || m.Tag != a.tag() || a.ph != snakeRun {
			return a, nil
		}
		a.step()
		if a.ph == snakeRun {
			return a, tickCmd(a.speed(), a.gen, a.tag())
		}
	case tea.KeyMsg:
		switch m.String() {
		case "esc", "q":
			return a, goHome()
		case "up", "w":
			return a, a.turn(pt{0, -1})
		case "down", "s":
			return a, a.turn(pt{0, 1})
		case "left", "a":
			return a, a.turn(pt{-1, 0})
		case "right", "d":
			return a, a.turn(pt{1, 0})
		case " ", "space", "enter":
			switch a.ph {
			case snakeRun:
				a.ph = snakePause
				a.epoch++
			case snakeOver:
				a.reset()
				return a, a.run()
			default:
				return a, a.run()
			}
		case "r", "R":
			a.epoch++
			a.reset()
		}
	}
	return a, nil
}

func (a *snakeApp) step() {
	if len(a.queue) > 0 {
		a.dir, a.queue = a.queue[0], a.queue[1:]
	}
	h := a.body[0]
	n := pt{h.x + a.dir.x, h.y + a.dir.y}
	eat := n == a.food
	tail := a.body[:len(a.body)-1] // 먹지 않으면 꼬리 칸은 이번에 비워지므로 부딪힌 것으로 보지 않음
	if eat {
		tail = a.body
	}
	hit := n.x < 0 || n.y < 0 || n.x >= snakeW || n.y >= snakeH
	for _, b := range tail {
		if b == n {
			hit = true
		}
	}
	if hit {
		a.over()
		return
	}
	if eat {
		a.body = append([]pt{n}, a.body...)
		a.score++
		if !a.placeFood() {
			a.won = true
			a.over()
		}
		return
	}
	copy(a.body[1:], a.body[:len(a.body)-1])
	a.body[0] = n
}

func (a *snakeApp) over() {
	a.ph = snakeOver
	if a.score > a.best {
		a.best, a.newBest = a.score, true
		_ = saveScore(snakeScore, a.score)
	}
}

// boardRows 는 판을 snakeH/2 줄로 그림. 빈 칸 두 개가 겹친 자리는 바닥 점(.)으로 판 범위를 보여줌
func (a *snakeApp) boardRows() [][]seg {
	at := make(map[pt]lipgloss.Color, len(a.body)+1)
	at[a.food] = colorWarn
	for i, b := range a.body {
		if i == 0 {
			at[b] = colorText
		} else {
			at[b] = colorAccent
		}
	}
	if a.ph == snakeOver && !a.won {
		at[a.body[0]] = colorWarn
	}
	rows := make([][]seg, snakeH/2)
	for r := range rows {
		var segs []seg
		var b strings.Builder
		var cur seg
		flush := func() {
			if b.Len() > 0 {
				cur.t = b.String()
				segs = append(segs, cur)
				b.Reset()
			}
		}
		for x := 0; x < snakeW; x++ {
			top, bot := at[pt{x, r * 2}], at[pt{x, r*2 + 1}]
			var s seg
			switch {
			case top == "" && bot == "":
				s = seg{t: ".", fg: colorTextFaint}
			case bot == "":
				s = seg{t: "▀", fg: top}
			case top == "":
				s = seg{t: "▄", fg: bot}
			case top == bot:
				s = seg{t: "█", fg: top}
			default:
				s = seg{t: "▀", fg: top, bg: bot}
			}
			if b.Len() > 0 && (s.fg != cur.fg || s.bg != cur.bg) {
				flush()
			}
			cur = s
			b.WriteString(s.t)
		}
		flush()
		rows[r] = segs
	}
	return rows
}

func (a *snakeApp) View(c Ctx) string {
	f := NewFrame(c)
	lead := sg(sp((f.In()-4-snakeW)/2), colorText)
	for _, r := range a.boardRows() {
		f.Row(append([]seg{lead}, r...)...)
	}
	f.RowLR([]seg{sg(fmt.Sprintf("점수 %d", a.score), colorText)}, []seg{sg(fmt.Sprintf("최고 %d", a.best), colorAccent)})
	f.Rule()
	f.MenuInline([2]string{"←→↑↓", "방향"}, [2]string{"Space", "시작·멈춤"}, [2]string{"r", "새 판"}, [2]string{"Esc", "홈으로"})
	f.Rule()
	switch a.ph {
	case snakeReady:
		f.Message("방향키나 Space 를 누르면 출발함", ToneInfo)
	case snakePause:
		f.Message("멈춤. Space 로 계속", ToneInfo)
	case snakeOver:
		msg, tone := fmt.Sprintf("부딪힘! 점수 %d. Space 로 새 판", a.score), ToneWarn
		if a.won {
			msg, tone = "판을 꽉 채움!", ToneOk
		} else if a.newBest {
			msg, tone = fmt.Sprintf("새 최고 기록 %d! Space 로 새 판", a.score), ToneOk
		}
		f.Message(msg, tone)
	default:
		f.Message("", ToneInfo)
	}
	return f.String()
}
