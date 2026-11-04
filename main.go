package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type phase int

const (
	phIntro phase = iota
	phHome
	phApp
	phSettings
)

// model 은 호스트: 인트로 → 홈 → 앱 사이를 오가고, 앱 하나에 메시지를 전달함
type model struct {
	ph         phase
	w, h       int
	introStart time.Time
	env        *Env
	sel        int // 홈에서 선택된 줄
	app        App // 실행 중인 앱 (phApp 일 때만)

	// 홈 꾸밈: 배경·전환·아케이드 조작부 (설정에서 바꿈)
	set     Settings
	setRow  int    // 설정 화면에서 고른 항목
	saveErr string // 마지막 설정 저장 실패 사유
	start   time.Time
	trans   *transition
	animOn  bool // animTick 이 이미 돌고 있는지
	pulses  pulses
}

// transition 은 진행 중인 화면 전환. a=옛 화면, b=새 화면 (시작 순간의 그림)
type transition struct {
	kind    int
	a, b    *grid
	started time.Time
	reverse bool
}

// animTickMsg 는 배경·전환을 그리기 위한 짧은 틱 (필요할 때만 돎)
type animTickMsg time.Time

func animTick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return animTickMsg(t) })
}

func newModel() model {
	return model{ph: phIntro, introStart: time.Now(), env: &Env{Boot: bootUnix()}, set: loadSettings(), start: time.Now()}
}

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tea.SetWindowTitle(appName), tick(33*time.Millisecond))
}

func (m model) ctx() Ctx { return Ctx{W: m.w, H: m.h, Now: time.Now()} }

func (m model) openApp(i int) (model, tea.Cmd) {
	if i < 0 || i >= len(registry) {
		return m, nil
	}
	next := m
	next.sel = i
	next.app = registry[i].New(m.env)
	next.ph = phApp
	return m.withTransition(next, false), next.app.Init()
}

// withTransition 은 m(옛 화면)에서 next(새 화면)로 바뀔 때 전환 효과를 붙인 새 모델을 돌려줌. 효과를 못 쓰면 next 그대로임.
func (m model) withTransition(next model, reverse bool) model {
	if m.set.Trans == 0 || !animEnabled() || m.w <= 0 || m.h <= 0 {
		return next
	}
	next.trans = &transition{kind: m.set.Trans, a: m.screenGrid(), b: next.screenGrid(), started: time.Now(), reverse: reverse}
	return next
}

// wantsAnim 은 지금 짧은 틱(배경·전환·조작부 움직임)이 필요한지
func (m model) wantsAnim(now time.Time) bool {
	if !animEnabled() || m.w <= 0 || m.h <= 0 {
		return false
	}
	if m.trans != nil {
		return true
	}
	if m.ph == phHome || m.ph == phSettings || (m.ph == phApp && m.app != nil) {
		return m.set.Bg != "" || m.pulses.active(now)
	}
	return false
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nm, cmd := m.update(msg)
	if !nm.animOn && nm.wantsAnim(time.Now()) {
		nm.animOn = true
		d := 66 * time.Millisecond
		if nm.trans != nil {
			d = 33 * time.Millisecond
		}
		cmd = tea.Batch(cmd, animTick(d))
	}
	return nm, cmd
}

func (m model) update(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case animTickMsg:
		m.animOn = false // 아직 필요하면 Update 가 다시 걺
		if m.trans != nil && time.Since(m.trans.started) >= transDuration(m.trans.kind) {
			m.trans = nil
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		if m.ph == phIntro {
			if time.Since(m.introStart) >= introTotal {
				m.ph = phHome
				return m, tick(250 * time.Millisecond)
			}
			return m, tick(33 * time.Millisecond)
		}
		next := tick(250 * time.Millisecond)
		if m.ph == phApp {
			var cmd tea.Cmd
			m.app, cmd = m.app.Update(msg)
			return m, tea.Batch(next, cmd)
		}
		return m, next

	case goHomeMsg:
		next := m
		next.ph, next.app = phHome, nil
		return m.withTransition(next, true), nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.trans != nil { // 아무 키나 누르면 전환을 바로 끝냄
			m.trans = nil
		}
		if m.ph == phApp { // 앱 안에서도 조작부가 키 입력에 반응함 (키는 그대로 앱에 전달됨)
			m = m.notePress(msg.String())
		}
		switch m.ph {
		case phIntro:
			m.ph = phHome
			return m, tick(250 * time.Millisecond)
		case phHome:
			return m.homeKey(msg)
		case phSettings:
			return m.settingsKey(msg)
		}
	}

	if m.ph == phApp && m.app != nil {
		var cmd tea.Cmd
		m.app, cmd = m.app.Update(msg)
		return m, cmd
	}
	return m, nil
}

// notePress 는 아케이드 조작부 애니메이션용으로 키 입력 시각을 기록함
func (m model) notePress(key string) model {
	now := time.Now()
	switch key {
	case "up", "k":
		m.pulses.press(pUp, now, 220*time.Millisecond)
	case "down", "j":
		m.pulses.press(pDown, now, 220*time.Millisecond)
	case "left", "h":
		m.pulses.press(pLeft, now, 220*time.Millisecond)
	case "right", "l":
		m.pulses.press(pRight, now, 220*time.Millisecond)
	case "enter":
		m.pulses.press(pA, now, 180*time.Millisecond)
	case "esc", "q":
		m.pulses.press(pB, now, 180*time.Millisecond)
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			m.pulses.press(pNum+int(key[0]-'1'), now, 300*time.Millisecond)
			m.pulses.press(pA, now, 120*time.Millisecond)
		}
	}
	return m
}

func (m model) settingsKey(msg tea.KeyMsg) (model, tea.Cmd) {
	key := msg.String()
	m = m.notePress(key)
	switch key {
	case "up", "k":
		m.setRow = max(0, m.setRow-1)
	case "down", "j":
		m.setRow = min(setCount-1, m.setRow+1)
	case "left", "h", "right", "l", "enter", " ":
		dir := 1
		if key == "left" || key == "h" {
			dir = -1
		}
		m.set = m.set.cycle(m.setRow, dir)
		m.saveErr = ""
		if err := m.set.save(); err != nil {
			m.saveErr = err.Error()
		}
	case "esc", "q", "s":
		m.ph = phHome
	}
	return m, nil
}

func (m model) homeKey(msg tea.KeyMsg) (model, tea.Cmd) {
	key := msg.String()
	m = m.notePress(key)
	switch key {
	case "s":
		m.ph, m.setRow = phSettings, 0
	case "up", "k":
		if m.sel > 0 {
			m.sel--
		}
	case "down", "j":
		if m.sel < len(registry)-1 {
			m.sel++
		}
	case "enter":
		return m.openApp(m.sel)
	case "esc", "q":
		return m, tea.Quit
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			return m.openApp(int(key[0] - '1'))
		}
	}
	return m, nil
}

func (m model) View() string {
	c := m.ctx()
	if m.ph == phIntro {
		return m.viewIntro()
	}
	if m.trans != nil && c.W > 0 && c.H > 0 {
		u := clampF(float64(time.Since(m.trans.started))/float64(transDuration(m.trans.kind)), 0, 1)
		return blend(m.trans.kind, m.trans.a, m.trans.b, u, m.trans.reverse).String()
	}
	if m.ph == phApp {
		if m.app != nil {
			if m.fxApp(c) {
				return m.appGrid(c).String()
			}
			return c.Place(m.app.View(c))
		}
	}
	if m.fxHome(c) {
		return m.homeGrid(c).String()
	}
	return c.Place(m.boxView(c))
}

// boxView 는 홈 또는 설정 화면의 박스
func (m model) boxView(c Ctx) string {
	if m.ph == phSettings {
		return m.viewSettings(c)
	}
	return m.viewHome(c)
}

func main() {
	if _, err := tea.NewProgram(newModel(), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "오류:", strings.TrimSpace(err.Error()))
		os.Exit(1)
	}
}
