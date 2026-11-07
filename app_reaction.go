package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// 반응속도. 화면이 초록으로 바뀌는 순간 Space 를 누르면 걸린 시간(ms)을 잼.
// 바뀌기 전에 누르면 "너무 빠름". 최고 기록은 scores.json 의 "reaction_ms" (작을수록 좋음).

var reactionInfo = AppInfo{
	ID:   "reaction",
	Name: "반응속도",
	Desc: "초록색이 되면 바로 누르기",
	New:  newReactionApp,
}

// 테스트에서 바꿔 끼우는 주입점
var (
	reactNow  = time.Now
	reactRand = rand.Intn
)

const (
	reactMinWait = 1500 // 기다리는 시간 하한 (ms)
	reactJitter  = 2500 // 하한 위에 더하는 무작위 시간 (ms)
	reactKeep    = 5    // 화면에 보이는 최근 기록 수
	reactScore   = "reaction_ms"
)

type reactPhase int

const (
	reactIdle   reactPhase = iota
	reactWait              // 빨리 누르면 안 되는 대기
	reactGo                // 지금 눌러야 함
	reactResult            // 방금 잰 결과
	reactEarly             // 너무 빨리 누름
)

type reactionApp struct {
	gen, epoch int
	ph         reactPhase
	goAt       time.Time
	last       int
	recent     []int
	best       int // 0 이면 기록 없음
	newBest    bool
	saveErr    bool
}

func newReactionApp(env *Env) App {
	a := &reactionApp{gen: newGen()}
	if v, ok := loadScore(reactScore); ok && v > 0 {
		a.best = v
	}
	return a
}

func (a *reactionApp) Init() tea.Cmd { return nil }

func (a *reactionApp) tag() string { return "react" + strconv.Itoa(a.epoch) }

func (a *reactionApp) arm() tea.Cmd {
	a.epoch++
	a.ph = reactWait
	a.newBest = false
	d := time.Duration(reactMinWait+reactRand(reactJitter)) * time.Millisecond
	return tickCmd(d, a.gen, a.tag())
}

func (a *reactionApp) Update(msg tea.Msg) (App, tea.Cmd) {
	if cmd, ok := forwardHangul(msg, func(m tea.Msg) tea.Cmd { _, c := a.Update(m); return c }); ok { // 한글 입력 상태에서도 단축키가 듣게 함
		return a, cmd
	}
	switch m := msg.(type) {
	case appTickMsg:
		if m.Gen == a.gen && m.Tag == a.tag() && a.ph == reactWait {
			a.ph = reactGo
			a.goAt = reactNow()
		}
	case tea.KeyMsg:
		switch m.String() {
		case "esc", "q":
			return a, goHome()
		case " ", "space", "enter":
			switch a.ph {
			case reactWait:
				a.epoch++ // 걸려 있는 틱을 무효로 만듦
				a.ph = reactEarly
			case reactGo:
				a.record(int(reactNow().Sub(a.goAt).Milliseconds()))
			default:
				return a, a.arm()
			}
		case "r", "R":
			if a.ph != reactWait && a.ph != reactGo {
				a.recent, a.last, a.ph = nil, 0, reactIdle
			}
		}
	}
	return a, nil
}

func (a *reactionApp) record(ms int) {
	ms = max(0, ms)
	a.ph, a.last = reactResult, ms
	a.recent = append(a.recent, ms)
	if len(a.recent) > reactKeep {
		a.recent = a.recent[len(a.recent)-reactKeep:]
	}
	if a.best == 0 || ms < a.best {
		a.best, a.newBest = ms, true
		a.saveErr = saveScore(reactScore, ms) != nil
	}
}

func (a *reactionApp) View(c Ctx) string {
	f := NewFrame(c).Blank()
	var badge seg
	switch a.ph {
	case reactWait:
		badge = Badge("기다리는 중", ToneInfo)
	case reactGo:
		badge = Badge("지금!", ToneOk)
	case reactResult:
		badge = Badge("결과", ToneAccent)
	case reactEarly:
		badge = Badge("너무 빠름", ToneWarn)
	default:
		badge = Badge("대기", ToneInfo)
	}
	f.Row(badge).Blank()

	rows := 1
	if c.Big() {
		rows = 5
	}
	switch a.ph {
	case reactGo:
		for i := 0; i < rows; i++ {
			f.Row(sg(strings.Repeat("█", f.In()-4), colorOk))
		}
	case reactResult:
		f.Digits("반응 시간", strconv.Itoa(a.last), colorAccent)
	default:
		text := map[reactPhase]string{
			reactIdle:  "Space 를 누르면 시작함. 초록색으로 바뀌면 바로 Space!",
			reactWait:  "기다리세요… 초록색이 되면 Space",
			reactEarly: "초록색이 되기 전에 눌렀음. Space 로 다시",
		}[a.ph]
		col := colorTextMuted
		if a.ph == reactEarly {
			col = colorWarn
		}
		for i := 0; i < rows; i++ {
			if i == rows/2 {
				f.Row(sg(truncW(text, f.In()-4), col))
			} else {
				f.Blank()
			}
		}
	}
	f.Blank()

	parts := make([]string, len(a.recent))
	sum := 0
	for i, v := range a.recent {
		parts[i] = strconv.Itoa(v)
		sum += v
	}
	recent := "최근 기록 없음"
	if len(a.recent) > 0 {
		recent = fmt.Sprintf("최근 %s  평균 %dms", strings.Join(parts, " "), sum/len(a.recent))
	}
	best := "최고 -"
	if a.best > 0 {
		best = fmt.Sprintf("최고 %dms", a.best)
	}
	f.RowLR([]seg{sg(truncW(recent, f.In()-4-lenW(best)-2), colorText)}, []seg{sg(best, colorAccent)})
	f.Rule()
	f.MenuInline([2]string{"Space", "시작·누르기"}, [2]string{"r", "최근 기록 지우기"}, [2]string{"Esc", "홈으로"})
	f.Rule()
	switch {
	case a.ph == reactResult && a.newBest && a.saveErr:
		f.Message("새 최고 기록! (기록 파일 저장은 실패함)", ToneWarn)
	case a.ph == reactResult && a.newBest:
		f.Message("새 최고 기록!", ToneOk)
	case a.ph == reactResult:
		f.Message("Space 로 한 번 더", ToneInfo)
	default:
		f.Message("", ToneInfo)
	}
	return f.String()
}

func lenW(s string) int { return sg(s, colorText).width() }
