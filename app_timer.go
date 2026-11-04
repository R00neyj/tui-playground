package main

import (
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// 종료 타이머 (F-02). 예약은 OS 에 걸려 있어서 앱을 나가도 유지됨.

var timerInfo = AppInfo{
	ID:     "timer",
	Name:   "종료 타이머",
	Desc:   "지정 시간 뒤 PC 종료 예약",
	New:    newTimerApp,
	Status: timerStatus,
}

func timerTone(rem time.Duration) Tone {
	switch {
	case rem <= time.Minute:
		return ToneDanger
	case rem <= 10*time.Minute:
		return ToneWarn
	}
	return ToneOk
}

// timerStatus 는 홈 목록 줄 끝에 보일 예약 상태 (상태 파일을 읽음)
func timerStatus(env *Env, now time.Time) Status {
	t := loadTimer(env.Boot)
	if t == nil {
		return Status{Text: "예약 없음", Tone: ToneInfo}
	}
	rem := t.Target.Sub(now)
	return Status{Badge: "예약됨", Text: fmtDur(rem), Tone: timerTone(rem), Hint: "종료 타이머 예약은 홈으로 나와도 유지됨"}
}

type timerPhase int

const (
	tpMain timerPhase = iota
	tpNew
)

type timerApp struct {
	env      *Env
	ph       timerPhase
	cur      *Timer
	msg      string
	msgTone  Tone
	input    textinput.Model
	inputErr string
	lastSync int64
}

func newTimerApp(env *Env) App {
	ti := textinput.New()
	ti.Prompt = "분  > "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent)
	ti.CharLimit = 16
	ti.Width = 20
	return &timerApp{env: env, cur: loadTimer(env.Boot), input: ti, lastSync: time.Now().Unix()}
}

func (a *timerApp) Init() tea.Cmd { return nil }

func (a *timerApp) say(msg string, tone Tone) { a.msg, a.msgTone = msg, tone }

func (a *timerApp) Update(msg tea.Msg) (App, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		if now := time.Now().Unix(); now != a.lastSync {
			a.lastSync = now
			a.sync()
		}
		return a, nil

	case tea.KeyMsg:
		key := msg.String()
		if a.ph == tpMain {
			switch key {
			case "1":
				a.ph = tpNew
				a.input.SetValue("")
				a.inputErr = ""
				return a, a.input.Focus()
			case "2":
				a.stopTimer()
			case "3", "esc":
				return a, goHome()
			}
			return a, nil
		}
		// 새 타이머 입력
		switch key {
		case "esc":
			a.ph = tpMain
			return a, nil
		case "enter":
			mins := parseMin(a.input.Value())
			switch {
			case mins < 0:
				a.inputErr = "잘못된 입력."
				a.input.SetValue("")
			case mins == 0:
				a.ph = tpMain
			default:
				a.setTimer(mins * 60)
				a.ph = tpMain
			}
			return a, nil
		}
		a.inputErr = ""
		var cmd tea.Cmd
		a.input, cmd = a.input.Update(msg)
		return a, cmd
	}
	if a.ph == tpNew { // 커서 깜박임 등 입력창 내부 메시지
		var cmd tea.Cmd
		a.input, cmd = a.input.Update(msg)
		return a, cmd
	}
	return a, nil
}

// ---- 예약 / 취소 / 동기화 ----

func (a *timerApp) setTimer(sec int64) {
	old := a.cur
	code := shutdownSchedule(sec)
	had := false
	if code == codeAlreadyScheduled { // 이미 예약된 종료가 있음 -> 취소 후 재시도
		had = true
		shutdownCancel()
		code = shutdownSchedule(sec)
	}
	if code != 0 {
		if had {
			// 기존 예약이 이 프로그램이 건 것이면 남은 시간만큼 복구 시도
			var left int64
			if old != nil {
				left = int64(time.Until(old.Target).Seconds())
			}
			if left > 0 && shutdownSchedule(left) == 0 {
				a.say(fmt.Sprintf("예약 실패 (오류 %d) / 기존 타이머 유지됨", code), ToneDanger)
				return
			}
			os.Remove(statePath())
			a.cur = nil
			a.say(fmt.Sprintf("예약 실패 (오류 %d) / 기존 예약은 취소된 상태", code), ToneDanger)
		} else {
			a.say(fmt.Sprintf("예약 실패 (shutdown 오류 %d)", code), ToneDanger)
		}
		return
	}
	n := time.Now()
	t := n.Add(time.Duration(sec) * time.Second)
	a.cur = &Timer{Start: n, Target: t}
	os.Remove(statePath())
	tone, warn := ToneOk, ""
	if err := writeState(a.cur, a.env.Boot); err != nil {
		a.cur.NoFile = true
		tone, warn = ToneWarn, " / 상태 파일 저장 실패"
	}
	txt := fmtDur(time.Duration(sec)*time.Second) + " 후 종료 예약 (" + t.Format("15:04:05") + ")"
	if had {
		txt += " / 기존 예약 취소됨"
	}
	a.say(txt+warn, tone)
}

func (a *timerApp) stopTimer() {
	ok := shutdownCancel()
	os.Remove(statePath())
	a.cur = nil
	if ok {
		a.say("종료 예약 취소 완료.", ToneOk)
	} else {
		a.say("예약된 종료 없음.", ToneWarn)
	}
}

// sync: 다른 창에서 취소/변경했거나 만료됐으면 화면 상태를 상태 파일에 맞춤
func (a *timerApp) sync() {
	old := a.cur
	if old != nil && old.NoFile {
		if !old.Target.After(time.Now()) {
			a.cur = nil
			a.say("", ToneInfo)
		}
		return
	}
	nw := loadTimer(a.env.Boot)
	switch {
	case old != nil && nw == nil:
		if !old.Target.After(time.Now()) {
			a.say("", ToneInfo)
		} else {
			a.say("예약이 다른 곳에서 취소됨", ToneWarn)
		}
	case old != nil && nw != nil && !nw.Target.Equal(old.Target):
		a.say("예약이 다른 곳에서 변경됨", ToneWarn)
	}
	a.cur = nw
}

// ---- 화면 ----

func (a *timerApp) View(c Ctx) string {
	if a.ph == tpNew {
		return a.viewNew(c)
	}
	return a.viewMain(c)
}

func (a *timerApp) viewMain(c Ctx) string {
	var col lipgloss.Color
	var p float64
	var badge seg
	var tg, rm string
	if t := a.cur; t != nil {
		rem := t.Target.Sub(c.Now)
		if rem < 0 {
			rem = 0
		}
		tone := timerTone(rem)
		col = toneColor(tone)
		if total := t.Target.Sub(t.Start).Seconds(); total > 0 {
			p = clamp01(c.Now.Sub(t.Start).Seconds() / total)
		}
		badge = Badge("예약됨", tone)
		tg, rm = t.Target.Format("2006-01-02 15:04:05"), fmtDur(rem)
	} else {
		col = colorTextMuted
		badge = Badge("대기", ToneInfo)
		tg, rm = "-", "--:--:--"
	}
	f := NewFrame(c).Blank()
	f.Row(badge, sg("   종료시각  ", colorTextMuted), sg(tg, colorText)).Blank()
	f.Digits("남은시간", rm, col).Blank()
	f.Progress(p, col).Rule()
	f.Menu("1", "새 타이머 설정", "기존 타이머는 자동 취소", false)
	f.Menu("2", "타이머 취소", "", a.cur == nil)
	f.Menu("3", "홈으로", "Esc 도 가능", false)
	f.Rule().Message(a.msg, a.msgTone)
	return f.String()
}

func (a *timerApp) viewNew(c Ctx) string {
	f := NewFrame(c)
	f.Row(sgBold("새 타이머", colorAccent)).Rule()
	if a.cur != nil {
		f.Row(sg("현재 예약  ", colorTextMuted), sg(a.cur.Target.Format("15:04:05")+"  (남은 "+fmtDur(time.Until(a.cur.Target))+")", colorWarn))
		f.Row(sg("새로 설정하면 기존 타이머는 자동 취소됨", colorTextMuted))
	} else {
		f.Row(sg("현재 예약된 타이머 없음", colorTextMuted))
	}
	f.Blank()
	f.Row(sg("입력 예  ", colorTextMuted), sg("30", colorText), sg("  /  ", colorTextMuted), sg("90", colorText), sg("  /  ", colorTextMuted), sg("1h30m", colorText), sg("  /  ", colorTextMuted), sg("2h", colorText))
	f.Row(sg("Enter = 30분    0 / Esc = 뒤로", colorTextMuted))
	f.Below("", a.input.View())
	if a.inputErr != "" {
		f.Below(fg(a.inputErr, colorDanger))
	}
	return f.String()
}
