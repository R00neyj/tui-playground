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
)

// model 은 호스트: 인트로 → 홈 → 앱 사이를 오가고, 앱 하나에 메시지를 전달함
type model struct {
	ph         phase
	w, h       int
	introStart time.Time
	env        *Env
	sel        int // 홈에서 선택된 줄
	app        App // 실행 중인 앱 (phApp 일 때만)
}

func newModel() model {
	return model{ph: phIntro, introStart: time.Now(), env: &Env{Boot: bootUnix()}}
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
	m.sel = i
	m.app = registry[i].New(m.env)
	m.ph = phApp
	return m, m.app.Init()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
		m.ph, m.app = phHome, nil
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.ph {
		case phIntro:
			m.ph = phHome
			return m, tick(250 * time.Millisecond)
		case phHome:
			return m.homeKey(msg)
		}
	}

	if m.ph == phApp && m.app != nil {
		var cmd tea.Cmd
		m.app, cmd = m.app.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) homeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
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
	switch m.ph {
	case phIntro:
		return m.viewIntro()
	case phApp:
		if m.app != nil {
			return c.Place(m.app.View(c))
		}
	}
	return c.Place(m.viewHome(c))
}

func main() {
	if _, err := tea.NewProgram(newModel(), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "오류:", strings.TrimSpace(err.Error()))
		os.Exit(1)
	}
}
