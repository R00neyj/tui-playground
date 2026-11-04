package main

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// 포모도로 (F-04). PC 종료와 무관한 집중/휴식 카운트다운.
// 남은 시간은 매번 deadline(종료 시각)에서 계산하므로 틱이 늦어져도 시간이 밀리지 않음.

var pomodoroInfo = AppInfo{
	ID:   "pomodoro",
	Name: "포모도로",
	Desc: "집중·휴식 반복 카운트다운",
	New:  newPomodoroApp,
}

// 테스트에서 바꿔 끼우는 주입점
var (
	pomoNow  = time.Now
	pomoBell = func() { _, _ = os.Stdout.WriteString("\a") }
)

const (
	pomoDefaultFocus = 25
	pomoDefaultBreak = 5
	pomoMaxMin       = 999 // 큰 숫자 폭과 시간 계산 넘침을 막는 상한
	pomoTick         = 250 * time.Millisecond

	pomoErrMin = "1 이상의 숫자만 입력할 수 있음."
	pomoErrMax = "1~999 사이의 숫자만 입력할 수 있음."
)

type pomoPhase int

const (
	pomoIdle  pomoPhase = iota // 대기 (아직 시작 전, 리셋 직후)
	pomoFocus                  // 집중
	pomoBreak                  // 휴식
)

type pomoScreen int

const (
	pomoScrMain pomoScreen = iota
	pomoScrSetup
)

type pomodoroApp struct {
	gen int    // 앱 인스턴스 번호 (옛 틱 무시용)
	tag string // 현재 틱 줄기 이름. 시작·재개마다 바뀌어서 옛 틱이 겹쳐 돌지 않게 함
	seq int

	focusMin, breakMin int

	phase    pomoPhase
	paused   bool
	deadline time.Time     // 진행 중일 때 현재 구간의 종료 시각
	left     time.Duration // 일시정지 중일 때 남은 시간
	total    time.Duration // 현재 구간의 전체 길이 (진행률 계산용)
	done     int           // 완료한 집중 횟수

	msg     string
	msgTone Tone
	confirm bool // 나가기 확인 중

	screen   pomoScreen
	inputs   [2]textinput.Model
	field    int
	fresh    bool // 입력칸에 처음 닿은 상태: 첫 글자를 치면 기존 값을 지움
	inputErr string
}

func newPomodoroApp(env *Env) App {
	a := &pomodoroApp{gen: newGen(), focusMin: pomoDefaultFocus, breakMin: pomoDefaultBreak}
	for i, p := range []string{"집중(분)  > ", "휴식(분)  > "} {
		ti := textinput.New()
		ti.Prompt = p
		ti.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent)
		ti.CharLimit = 6
		ti.Width = 8
		a.inputs[i] = ti
	}
	return a
}

func (a *pomodoroApp) Init() tea.Cmd { return nil }

func pomoDur(min int) time.Duration { return time.Duration(min) * time.Minute }

func (a *pomodoroApp) say(msg string, tone Tone) { a.msg, a.msgTone = msg, tone }

func (a *pomodoroApp) running() bool { return a.phase != pomoIdle && !a.paused }

// remaining 은 now 시점의 남은 시간 (0 ~ total)
func (a *pomodoroApp) remaining(now time.Time) time.Duration {
	var r time.Duration
	switch {
	case a.phase == pomoIdle:
		return pomoDur(a.focusMin)
	case a.paused:
		r = a.left
	default:
		r = a.deadline.Sub(now)
	}
	return max(0, min(r, a.total))
}

func (a *pomodoroApp) nextTick() tea.Cmd {
	a.seq++
	a.tag = "pomo" + strconv.Itoa(a.seq)
	return tickCmd(pomoTick, a.gen, a.tag)
}

// ---- 구간 진행 ----

func (a *pomodoroApp) start(now time.Time) tea.Cmd {
	a.phase, a.paused = pomoFocus, false
	a.total = pomoDur(a.focusMin)
	a.deadline = now.Add(a.total)
	a.say("", ToneInfo)
	return a.nextTick()
}

func (a *pomodoroApp) pause(now time.Time) {
	a.advance(now)
	a.left = max(0, a.deadline.Sub(now))
	a.paused = true
	a.say("일시정지됨. Space 로 다시 시작함", ToneInfo)
}

func (a *pomodoroApp) resume(now time.Time) tea.Cmd {
	a.paused = false
	a.deadline = now.Add(a.left)
	a.say("", ToneInfo)
	return a.nextTick()
}

func (a *pomodoroApp) reset() {
	a.phase, a.paused, a.confirm = pomoIdle, false, false
	a.done = 0
	a.total, a.left = 0, 0
	a.deadline = time.Time{}
	a.say("", ToneInfo)
}

// advance 는 구간이 끝났으면 벨을 울리고 다음 구간으로 넘어감 (집중이 끝나면 완료 횟수 +1)
func (a *pomodoroApp) advance(now time.Time) {
	if !a.running() || now.Before(a.deadline) {
		return
	}
	var dur time.Duration
	if a.phase == pomoFocus {
		a.done++
		a.phase = pomoBreak
		dur = pomoDur(a.breakMin)
		a.say(fmt.Sprintf("집중 구간 끝. 휴식 %d분을 시작함 (벨)", a.breakMin), ToneInfo)
	} else {
		a.phase = pomoFocus
		dur = pomoDur(a.focusMin)
		a.say(fmt.Sprintf("휴식 구간 끝. 집중 %d분을 시작함 (벨)", a.focusMin), ToneInfo)
	}
	// 이전 종료 시각에서 이어 붙여 오차가 쌓이지 않게 함. 절전 등으로 한참 늦었으면 지금부터 셈
	next := a.deadline.Add(dur)
	if !next.After(now) {
		next = now.Add(dur)
	}
	a.deadline, a.total = next, dur
	pomoBell()
}

// ---- 입력 처리 ----

func (a *pomodoroApp) Update(msg tea.Msg) (App, tea.Cmd) {
	switch m := msg.(type) {
	case appTickMsg:
		if m.Gen != a.gen || m.Tag != a.tag || !a.running() {
			return a, nil
		}
		a.advance(pomoNow())
		return a, tickCmd(pomoTick, a.gen, a.tag)
	case tea.KeyMsg:
		if a.screen == pomoScrSetup {
			return a.updateSetup(m)
		}
		return a.updateMain(m)
	}
	if a.screen == pomoScrSetup { // 커서 깜박임 등 입력창 내부 메시지
		var cmd tea.Cmd
		a.inputs[a.field], cmd = a.inputs[a.field].Update(msg)
		return a, cmd
	}
	return a, nil
}

func (a *pomodoroApp) updateMain(k tea.KeyMsg) (App, tea.Cmd) {
	if a.confirm {
		switch k.String() {
		case "y":
			a.reset() // 나가면 타이머는 멈춤
			return a, goHome()
		case "n", "esc":
			a.confirm = false
		}
		return a, nil
	}
	now := pomoNow()
	switch k.String() {
	case " ", "space":
		switch {
		case a.phase == pomoIdle:
			return a, a.start(now)
		case a.paused:
			return a, a.resume(now)
		default:
			a.pause(now)
		}
	case "r":
		a.reset()
	case "e":
		return a, a.openSetup()
	case "esc":
		if a.phase == pomoIdle {
			return a, goHome()
		}
		a.confirm = true
	}
	return a, nil
}

// ---- 설정 ----

func (a *pomodoroApp) openSetup() tea.Cmd {
	a.screen = pomoScrSetup
	a.inputs[0].SetValue(strconv.Itoa(a.focusMin))
	a.inputs[1].SetValue(strconv.Itoa(a.breakMin))
	a.inputErr = ""
	return a.focusField(0)
}

func (a *pomodoroApp) focusField(i int) tea.Cmd {
	a.field, a.fresh = i, true
	a.inputs[1-i].Blur()
	return a.inputs[i].Focus()
}

// pomoParseMin 은 1 이상의 정수만 받음. 거부하면 오류 문구를 돌려줌
func pomoParseMin(s string) (int, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, pomoErrMin
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, pomoErrMin
		}
	}
	n, err := strconv.Atoi(s)
	switch {
	case err != nil || n < 1:
		return 0, pomoErrMin
	case n > pomoMaxMin:
		return 0, pomoErrMax
	}
	return n, ""
}

func (a *pomodoroApp) updateSetup(k tea.KeyMsg) (App, tea.Cmd) {
	switch k.String() {
	case "esc":
		a.screen = pomoScrMain
		return a, nil
	case "enter":
		n, e := pomoParseMin(a.inputs[a.field].Value())
		if e != "" {
			a.inputErr = e
			return a, nil
		}
		a.inputErr = ""
		if a.field == 0 {
			return a, a.focusField(1)
		}
		f, e := pomoParseMin(a.inputs[0].Value())
		if e != "" { // 첫 칸은 이미 통과했지만 방어적으로 되돌아감
			a.inputErr = e
			return a, a.focusField(0)
		}
		a.focusMin, a.breakMin = f, n
		a.screen = pomoScrMain
		if a.phase == pomoIdle {
			a.say("설정을 저장함", ToneOk)
		} else {
			a.say("설정을 저장함. 다음 구간부터 적용됨", ToneOk)
		}
		return a, nil
	}
	if a.fresh {
		a.fresh = false
		if k.Type == tea.KeyRunes {
			a.inputs[a.field].SetValue("")
		}
	}
	a.inputErr = ""
	var cmd tea.Cmd
	a.inputs[a.field], cmd = a.inputs[a.field].Update(k)
	return a, cmd
}

// ---- 화면 ----

func (a *pomodoroApp) View(c Ctx) string {
	if a.screen == pomoScrSetup {
		return a.viewSetup(c)
	}
	return a.viewMain(c)
}

// tone 은 구간별 색 의미: 집중=accent, 휴식=ok, 대기·일시정지=info
func (a *pomodoroApp) tone() Tone {
	switch {
	case a.phase == pomoIdle || a.paused:
		return ToneInfo
	case a.phase == pomoBreak:
		return ToneOk
	}
	return ToneAccent
}

func (a *pomodoroApp) viewMain(c Ctx) string {
	rem := a.remaining(c.Now)
	tone := a.tone()
	col := toneColor(tone)

	var badge, label string
	switch {
	case a.paused:
		badge = "일시정지"
	case a.phase == pomoFocus:
		badge = "집중"
	case a.phase == pomoBreak:
		badge = "휴식"
	default:
		badge = "대기"
	}
	label = fmt.Sprintf("오늘 완료  %d회", a.done)
	if a.paused { // 일시정지 중에는 어느 구간인지 함께 보여줌
		name := "집중"
		if a.phase == pomoBreak {
			name = "휴식"
		}
		label = name + " · " + label
	}

	var p float64
	if a.phase != pomoIdle && a.total > 0 {
		p = 1 - float64(rem)/float64(a.total)
	}
	secs := int(math.Ceil(rem.Seconds()))

	f := NewFrame(c).Blank()
	f.Row(Badge(badge, tone), sg("   ", colorText), sg(label, colorTextMuted)).Blank()
	f.Digits("남은시간", fmt.Sprintf("%02d:%02d", secs/60, secs%60), col).Blank()
	f.Progress(p, col).Rule()

	spaceLabel := "일시정지"
	if a.phase == pomoIdle || a.paused {
		spaceLabel = "시작"
	}
	escHint := ""
	if a.running() {
		escHint = "진행 중이면 확인함"
	}
	f.Menu("Space", spaceLabel, "", false)
	f.Menu("r", "리셋", "", false)
	f.Menu("e", "설정", fmt.Sprintf("집중 %d분 · 휴식 %d분", a.focusMin, a.breakMin), false)
	f.Menu("Esc", "홈으로", escHint, false)
	f.Rule()
	if a.confirm {
		a.confirmRow(f)
	} else {
		f.Message(a.msg, a.msgTone)
	}
	return f.String()
}

// confirmRow 는 하단 메시지 줄을 나가기 확인(y/n)으로 바꿈. 좁으면 짧은 문구를 씀
func (a *pomodoroApp) confirmRow(f *Frame) {
	build := func(text string, gap int) []seg {
		return []seg{
			sg(text, toneColor(ToneWarn)), sg(sp(gap), colorText),
			sg("[", colorTextMuted), sg("y", colorAccent), sg("]", colorTextMuted), sg(" 나감", colorText), sg(sp(gap), colorText),
			sg("[", colorTextMuted), sg("n", colorAccent), sg("]", colorTextMuted), sg(" 계속", colorText),
		}
	}
	segs := build("진행 중인 타이머가 멈춥니다. 홈으로 나갈까요?", 3)
	if segsWidth(segs) > f.In()-4 {
		segs = build("멈추고 나갈까요?", 2)
	}
	f.Row(segs...)
}

func (a *pomodoroApp) viewSetup(c Ctx) string {
	f := NewFrame(c)
	f.Row(sgBold("포모도로 설정", colorAccent)).Rule()
	f.Row(sg("집중과 휴식 시간을 분 단위로 입력함", colorTextMuted))
	f.Row(sg("현재 값  ", colorTextMuted), sg(fmt.Sprintf("집중 %d분  ", a.focusMin), colorText), sg("/", colorTextMuted), sg(fmt.Sprintf("  휴식 %d분", a.breakMin), colorText))
	f.Blank()
	f.Row(sg("입력 예  ", colorTextMuted), sg("25", colorText), sg("  /  ", colorTextMuted), sg("50", colorText), sg("  /  ", colorTextMuted), sg("5", colorText))
	f.Row(sg("Enter", colorAccent), sg(" = 저장    ", colorTextMuted), sg("Esc", colorAccent), sg(" = 취소", colorTextMuted))
	f.Below("", a.inputs[0].View(), a.inputs[1].View())
	if a.inputErr != "" {
		f.Below("", fg(a.inputErr, colorDanger))
	}
	return f.String()
}
