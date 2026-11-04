package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type phase int

const (
	phIntro phase = iota
	phMain
	phReserve
)

type tickMsg time.Time

type model struct {
	ph         phase
	w, h       int
	introStart time.Time
	boot       int64
	cur        *Timer
	msg        string
	msgCol     lipgloss.Color
	input      textinput.Model
	inputErr   string
	lastSync   int64
}

func itoa(n int) string { return strconv.Itoa(n) }

func newModel() model {
	ti := textinput.New()
	ti.Prompt = "분  > "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(cCyan)
	ti.CharLimit = 16
	ti.Width = 20
	return model{ph: phIntro, introStart: time.Now(), boot: bootUnix(), input: ti, msgCol: cGray}
}

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tea.SetWindowTitle(appName), tick(33*time.Millisecond))
}

func (m model) finishIntro() model {
	m.ph = phMain
	m.cur = loadTimer(m.boot)
	m.lastSync = time.Now().Unix()
	return m
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		if m.ph == phIntro {
			if time.Since(m.introStart) >= introTotal {
				return m.finishIntro(), tick(250 * time.Millisecond)
			}
			return m, tick(33 * time.Millisecond)
		}
		if now := time.Now().Unix(); now != m.lastSync {
			m.lastSync = now
			m.sync()
		}
		return m, tick(250 * time.Millisecond)

	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.ph {
		case phIntro:
			return m.finishIntro(), tick(250 * time.Millisecond)

		case phMain:
			switch key {
			case "1":
				m.ph = phReserve
				m.input.SetValue("")
				m.inputErr = ""
				return m, m.input.Focus()
			case "2":
				m.stopTimer()
			case "3", "esc":
				return m, tea.Quit
			}
			return m, nil

		case phReserve:
			switch key {
			case "esc":
				m.ph = phMain
				return m, nil
			case "enter":
				mins := parseMin(m.input.Value())
				switch {
				case mins < 0:
					m.inputErr = "잘못된 입력."
					m.input.SetValue("")
				case mins == 0:
					m.ph = phMain
				default:
					m.setTimer(mins * 60)
					m.ph = phMain
				}
				return m, nil
			}
			m.inputErr = ""
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}

	// 깜박임 등 textinput 내부 메시지
	if m.ph == phReserve {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) View() string {
	switch m.ph {
	case phIntro:
		return m.viewIntro()
	case phReserve:
		return m.viewReserve()
	}
	return m.viewMain()
}

func main() {
	if _, err := tea.NewProgram(newModel(), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "오류:", strings.TrimSpace(err.Error()))
		os.Exit(1)
	}
}
