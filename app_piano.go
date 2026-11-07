package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// 키보드 피아노. 가운데 줄 키(a s d f …)가 흰 건반, 윗줄(w e t y u o p)이 검은 건반.
// 소리는 Windows PlaySound 로 냄 (piano_sound_*.go). 아직 울리는 음과 섞어서 내므로 거의 동시에 누르면 화음이 됨 (piano_wav.go).

var pianoInfo = AppInfo{
	ID:   "piano",
	Name: "키보드 피아노",
	Desc: "키보드로 건반 누르기",
	New:  newPianoApp,
}

// pianoTone 은 테스트에서 가짜로 바꿔 끼움 (실제로 소리를 내지 않게)
var pianoTone = platformTone

const (
	pianoNoteLen = 400 * time.Millisecond
	pianoLit     = 200 * time.Millisecond // 누른 건반이 밝게 보이는 시간
	pianoKeyW    = 4                      // 흰 건반 하나의 폭 (3칸 + 틈 1칸)
	pianoMinOct  = 2
	pianoMaxOct  = 6
)

// 흰 건반: 키, 옥타브 첫 C 기준 반음 수. 도레미파솔라시도레미
var pianoWhite = []struct {
	key  string
	semi int
	name string
}{
	{"a", 0, "도"}, {"s", 2, "레"}, {"d", 4, "미"}, {"f", 5, "파"}, {"g", 7, "솔"},
	{"h", 9, "라"}, {"j", 11, "시"}, {"k", 12, "도"}, {"l", 14, "레"}, {";", 16, "미"},
}

// 검은 건반: 키, 반음 수, 왼쪽 흰 건반 번호 (그 오른쪽 틈 위에 그림)
var pianoBlack = []struct {
	key   string
	semi  int
	after int
}{
	{"w", 1, 0}, {"e", 3, 1}, {"t", 6, 3}, {"y", 8, 4}, {"u", 10, 5}, {"o", 13, 7}, {"p", 15, 8},
}

var pianoNames = []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

var pianoNow = time.Now

type pianoApp struct {
	gen    int
	oct    int
	lit    map[int]time.Time // 반음 수 -> 누른 시각
	last   string
	played int
	hangul bool // 한글 입력 상태로 들어온 키가 있었음
}

func newPianoApp(env *Env) App {
	return &pianoApp{gen: newGen(), oct: 4, lit: map[int]time.Time{}}
}

func (a *pianoApp) Init() tea.Cmd { return nil }

// pianoFreq 는 옥타브 oct 의 C 에서 semi 반음 위 음의 주파수 (A4 = 440Hz, 12평균율)
func pianoFreq(oct, semi int) float64 {
	midi := (oct+1)*12 + semi
	return 440 * math.Pow(2, float64(midi-69)/12)
}

func (a *pianoApp) play(semi int) tea.Cmd {
	freq := pianoFreq(a.oct, semi)
	pianoTone(freq, pianoNoteLen)
	a.lit[semi] = pianoNow()
	a.played++
	a.last = fmt.Sprintf("%s%d  %.1fHz", pianoNames[semi%12], a.oct+semi/12, freq)
	return tickCmd(pianoLit+10*time.Millisecond, a.gen, "piano") // 불빛을 끄려고 한 번 더 그림
}

func (a *pianoApp) Update(msg tea.Msg) (App, tea.Cmd) {
	if cmd, ok := forwardHangul(msg, func(m tea.Msg) tea.Cmd { _, c := a.Update(m); return c }); ok { // 한글 입력 상태에서도 건반이 듣게 함
		a.hangul = true
		return a, cmd
	}
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return a, nil
	}
	s := strings.ToLower(k.String())
	switch s {
	case "esc":
		return a, goHome()
	case "z", "left":
		a.oct = max(pianoMinOct, a.oct-1)
		return a, nil
	case "x", "right":
		a.oct = min(pianoMaxOct, a.oct+1)
		return a, nil
	}
	for _, w := range pianoWhite {
		if s == w.key {
			return a, a.play(w.semi)
		}
	}
	for _, b := range pianoBlack {
		if s == b.key {
			return a, a.play(b.semi)
		}
	}
	return a, nil
}

func (a *pianoApp) isLit(semi int) bool {
	t, ok := a.lit[semi]
	return ok && pianoNow().Sub(t) < pianoLit
}

func (a *pianoApp) View(c Ctx) string {
	f := NewFrame(c)
	n := len(pianoWhite)
	kw := n*pianoKeyW - 1
	lead := (f.In() - 4 - kw) / 2

	type cell struct {
		t   string
		col lipgloss.Color
	}
	newLine := func() []cell {
		cs := make([]cell, kw)
		for i := range cs {
			cs[i] = cell{" ", colorText}
		}
		return cs
	}
	render := func(cs []cell) []seg {
		out := []seg{sg(sp(lead), colorText)}
		for _, c := range cs {
			if m := len(out); m > 1 && out[m-1].fg == c.col {
				out[m-1].t += c.t
				continue
			}
			out = append(out, sg(c.t, c.col))
		}
		return out
	}
	whiteRow := func() []cell {
		cs := newLine()
		for i, w := range pianoWhite {
			col := colorText
			if a.isLit(w.semi) {
				col = colorAccent
			}
			for x := 0; x < pianoKeyW-1; x++ {
				cs[i*pianoKeyW+x] = cell{"█", col}
			}
		}
		return cs
	}

	// 윗줄: 검은 건반 키 이름
	labels := newLine()
	for _, b := range pianoBlack {
		x := b.after*pianoKeyW + pianoKeyW - 1
		labels[x] = cell{b.key, colorTextMuted}
	}
	f.Row(render(labels)...)
	// 검은 건반이 걸친 두 줄
	for r := 0; r < 2; r++ {
		cs := whiteRow()
		for _, b := range pianoBlack {
			col := colorTextFaint
			if a.isLit(b.semi) {
				col = colorAccent
			}
			x := b.after*pianoKeyW + pianoKeyW - 1
			for dx := -1; dx <= 1; dx++ {
				cs[x+dx] = cell{"█", col}
			}
		}
		f.Row(render(cs)...)
	}
	// 흰 건반 아랫부분 두 줄
	f.Row(render(whiteRow())...)
	f.Row(render(whiteRow())...)
	// 흰 건반 키 이름
	keys := newLine()
	names := newLine()
	for i, w := range pianoWhite {
		keys[i*pianoKeyW+1] = cell{w.key, colorTextMuted}
		x := i * pianoKeyW
		col := colorTextMuted
		if a.isLit(w.semi) {
			col = colorAccent
		}
		names[x] = cell{w.name, col}
		names[x+1] = cell{"", col}
	}
	f.Row(render(keys)...)
	f.Row(render(names)...)
	f.Blank()
	f.RowLR([]seg{sg("옥타브 "+strconv.Itoa(a.oct), colorText)}, []seg{sg(fmt.Sprintf("누른 횟수 %d", a.played), colorTextMuted)})
	f.Rule()
	f.MenuInline([2]string{"a~;", "흰 건반"}, [2]string{"w e t…", "검은 건반"}, [2]string{"z x", "옥타브"}, [2]string{"Esc", "홈으로"})
	f.Rule()
	switch {
	case !pianoHasSound:
		f.Message("이 OS 에서는 소리가 나지 않고 건반만 빛남", ToneInfo)
	case a.hangul:
		f.Message("한글 입력 상태임. 한/영 키로 영문으로 바꾸면 바로 반응함", ToneWarn)
	case a.last != "":
		f.Message(a.last, ToneAccent)
	default:
		f.Message("여러 키를 동시에 누르면 화음이 남", ToneInfo)
	}
	return f.String()
}
