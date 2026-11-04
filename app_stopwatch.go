package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// 스톱워치 (F-10). 시작 시각 기준으로 경과를 계산해서 틱이 늦어져도 어긋나지 않음.
// 측정 기록은 메모리에만 있고, 홈으로 나가면 사라짐.

var stopwatchInfo = AppInfo{
	ID:   "stopwatch",
	Name: "스톱워치",
	Desc: "시간 측정과 랩 기록",
	New:  newStopwatchApp,
}

// swNow 는 현재 시각. 테스트에서 가짜 시계로 바꿔 끼움 (time.Now 는 단조 시계라 시각 조정에 영향받지 않음)
var swNow = time.Now

const (
	swTickEvery = 100 * time.Millisecond // 0.1초 단위 표시라서 화면 갱신도 100ms
	swLapShown  = 3                      // 화면에 보이는 랩 개수
)

type swPhase int

const (
	swIdle    swPhase = iota // 대기: 아직 시작하지 않음 (또는 리셋됨)
	swRunning                // 측정 중
	swStopped                // 정지: 경과 시간과 랩을 그대로 보여줌
)

type swLap struct {
	Split time.Duration // 직전 랩부터의 구간 시간
	Total time.Duration // 시작부터의 누적 시간
}

type stopwatchApp struct {
	gen     int
	epoch   int // 시작할 때마다 올림. 정지 후 곧바로 다시 시작해도 옛 틱이 섞이지 않게 함
	ph      swPhase
	startAt time.Time     // 이번 측정 구간을 시작한 시각
	base    time.Duration // 이전 구간들에서 쌓인 경과 시간
	laps    []swLap
	confirm bool // 나가기 확인 중
}

func newStopwatchApp(env *Env) App { return &stopwatchApp{gen: newGen()} }

func (a *stopwatchApp) Init() tea.Cmd { return nil }

// elapsed 는 지금까지 잰 시간
func (a *stopwatchApp) elapsed() time.Duration {
	d := a.base
	if a.ph == swRunning {
		d += swNow().Sub(a.startAt)
	}
	return max(0, d)
}

func (a *stopwatchApp) hasRecord() bool {
	return a.ph == swRunning || (a.ph == swStopped && (len(a.laps) > 0 || a.base > 0))
}

func (a *stopwatchApp) tag() string { return "sw" + strconv.Itoa(a.epoch) }

func (a *stopwatchApp) Update(msg tea.Msg) (App, tea.Cmd) {
	switch msg := msg.(type) {
	case appTickMsg:
		if msg.Gen != a.gen || msg.Tag != a.tag() || a.ph != swRunning {
			return a, nil
		}
		return a, tickCmd(swTickEvery, a.gen, a.tag())

	case tea.KeyMsg:
		return a.key(msg.String())
	}
	return a, nil
}

func (a *stopwatchApp) key(k string) (App, tea.Cmd) {
	if a.confirm {
		switch k {
		case "y", "Y":
			return a, goHome()
		case "n", "N", "esc":
			a.confirm = false
		}
		return a, nil
	}
	switch k {
	case " ", "space":
		if a.ph == swRunning {
			a.base = a.elapsed()
			a.ph = swStopped
			return a, nil
		}
		a.epoch++
		a.startAt = swNow()
		a.ph = swRunning
		return a, tickCmd(swTickEvery, a.gen, a.tag())
	case "l", "L":
		if a.ph != swRunning { // 정지·대기 중 랩은 무시
			return a, nil
		}
		total := a.elapsed()
		prev := time.Duration(0)
		if n := len(a.laps); n > 0 {
			prev = a.laps[n-1].Total
		}
		a.laps = append(a.laps, swLap{Split: max(0, total-prev), Total: total})
	case "r", "R":
		a.ph, a.base, a.laps = swIdle, 0, nil
	case "esc":
		if a.hasRecord() {
			a.confirm = true
			return a, nil
		}
		return a, goHome()
	}
	return a, nil
}

// fmtSW 는 `MM:SS.t` (0.1초 단위, 버림). 100분이 넘으면 분 자리가 늘어남
func fmtSW(d time.Duration) string {
	tenths := max(0, d.Milliseconds()) / 100
	return fmt.Sprintf("%02d:%02d.%d", tenths/600, tenths/10%60, tenths%10)
}

// ---- 화면 ----

func (a *stopwatchApp) View(c Ctx) string {
	// 큰 숫자 + 여백 -> 큰 숫자 + 빽빽 -> 한 줄 숫자 + 여백 -> 한 줄 숫자 + 빽빽 순으로, 창 높이에 들어가는 첫 모양을 씀
	type variant struct{ big, tight bool }
	var last string
	for _, v := range []variant{{true, false}, {true, true}, {false, false}, {false, true}} {
		if v.big && !c.Big() {
			continue
		}
		cc := c
		if !v.big {
			cc.H = 0 // Frame.Digits 가 한 줄 숫자를 쓰게 함
		}
		last = a.render(cc, v.tight)
		if strings.Count(last, "\n")+1 <= c.H {
			break
		}
	}
	return last
}

func (a *stopwatchApp) render(c Ctx, tight bool) string {
	var badge seg
	col := colorTextMuted
	switch a.ph {
	case swRunning:
		badge, col = Badge("측정 중", ToneAccent), colorAccent
	case swStopped:
		badge, col = Badge("정지", ToneInfo), colorText
	default:
		badge = Badge("대기", ToneInfo)
	}
	gap := func(f *Frame) {
		if !tight {
			f.Blank()
		}
	}

	f := NewFrame(c)
	gap(f)
	f.Row(badge)
	gap(f)
	f.Digits("경과시간", fmtSW(a.elapsed()), col)
	gap(f)

	// 랩 목록은 항상 3줄을 차지함 (랩이 늘어도 화면이 위아래로 흔들리지 않게)
	shown := 0
	for i := len(a.laps) - 1; i >= 0 && shown < swLapShown; i-- {
		l := a.laps[i]
		f.Row(sg(padR("랩 "+strconv.Itoa(i+1), 7), colorTextMuted), sg(fmtSW(l.Split), colorText),
			sg("   누적 ", colorTextMuted), sg(fmtSW(l.Total), colorText))
		shown++
	}
	if shown == 0 {
		f.Row(sg("랩 기록 없음", colorTextMuted))
		shown++
	}
	for ; shown < swLapShown; shown++ {
		f.Blank()
	}
	f.Rule()

	running := a.ph == swRunning
	if running {
		f.Menu("Space", "정지", "", false)
		f.Menu("l", "랩", "", false)
	} else {
		f.Menu("Space", "시작", "", false)
		f.Menu("l", "랩", "측정 중에만 기록됨", true)
	}
	f.Menu("r", "리셋", "", false)
	esc := ""
	switch a.ph {
	case swRunning:
		esc = "측정 중이면 확인함"
	case swStopped:
		esc = "측정 기록이 있으면 확인함"
	}
	f.Menu("Esc", "홈으로", esc, false)
	f.Rule()

	if a.confirm {
		a.confirmRow(f)
	} else {
		f.Blank()
	}
	return f.String()
}

// confirmRow 는 나가기 확인 한 줄: `문구   [y] 나감   [n] 계속`. 폭이 모자라면 짧은 문구와 좁은 간격을 씀
func (a *stopwatchApp) confirmRow(f *Frame) {
	long := "측정 중인 시간이 사라집니다. 홈으로 나갈까요?"
	if a.ph == swStopped {
		long = "측정 기록이 사라집니다. 홈으로 나갈까요?"
	}
	build := func(text string, gap int) []seg {
		g := sg(sp(gap), colorText)
		return []seg{
			sg(text, toneColor(ToneWarn)), g,
			sg("[", colorTextMuted), sg("y", colorAccent), sg("]", colorTextMuted), sg(" 나감", colorText), g,
			sg("[", colorTextMuted), sg("n", colorAccent), sg("]", colorTextMuted), sg(" 계속", colorText),
		}
	}
	segs := build(long, 3)
	if segsWidth(segs) > f.In()-4 {
		segs = build("기록이 사라짐. 나갈까요?", 2)
	}
	f.Row(segs...)
}
