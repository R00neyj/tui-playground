package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/sys/windows"
)

// Timer 는 이 프로그램이 예약한 종료 타이머 정보
type Timer struct {
	Start, Target time.Time
	NoFile        bool // 상태 파일 저장에 실패한 경우 (파일 동기화 생략)
}

const isoLayout = "2006-01-02T15:04:05.0000000-07:00"

func statePath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "shutdown_timer_go.state")
}

// ---- shutdown.exe 호출 ----

// 테스트에서 바꿔 끼울 수 있도록 변수로 둠
var runShutdown = func(args ...string) int {
	exe := filepath.Join(os.Getenv("SystemRoot"), "System32", "shutdown.exe")
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	err := cmd.Run()
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// ---- 부팅 시각 ----

type timeOfDayInfo struct {
	BootTime, CurrentTime, TimeZoneBias int64
	TimeZoneId                          uint32
	Reserved                            uint32
	BootTimeBias, SleepTimeBias         uint64
}

// bootUnix 는 마지막 부팅 시각(유닉스 초). 재부팅 후 남은 상태 파일을 걸러내는 데 씀
func bootUnix() int64 {
	var info timeOfDayInfo
	var ret uint32
	err := windows.NtQuerySystemInformation(3 /* SystemTimeOfDayInformation */, unsafe.Pointer(&info), uint32(unsafe.Sizeof(info)), &ret)
	if err == nil && info.BootTime > 0 {
		return (info.BootTime - 116444736000000000) / 10000000
	}
	// 실패 시: 현재 시각 - 가동 시간
	return time.Now().Add(-time.Duration(windows.DurationSinceBoot())).Unix()
}

// ---- 상태 파일 ----

func writeState(t *Timer, boot int64) error {
	body := strings.Join([]string{t.Start.Format(isoLayout), t.Target.Format(isoLayout), strconv.FormatInt(boot, 10)}, "\r\n") + "\r\n"
	p := statePath()
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p) // 다른 창이 읽는 도중 반쯤 쓰인 파일을 보지 않도록 교체 방식으로 저장
}

func loadTimer(boot int64) *Timer {
	p := statePath()
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(string(data)), "\r", ""), "\n")
	drop := func() *Timer { os.Remove(p); return nil }
	if len(lines) < 3 {
		return drop()
	}
	s, e1 := time.Parse(time.RFC3339Nano, strings.TrimSpace(lines[0]))
	t, e2 := time.Parse(time.RFC3339Nano, strings.TrimSpace(lines[1]))
	b, e3 := strconv.ParseInt(strings.TrimSpace(lines[2]), 10, 64)
	if e1 != nil || e2 != nil || e3 != nil {
		return drop()
	}
	if d := b - boot; d > 2 || d < -2 { // 다른 부팅에서 만든 예약
		return drop()
	}
	if !t.After(time.Now()) {
		return drop()
	}
	return &Timer{Start: s, Target: t}
}

// ---- 입력 파싱 ----

var minRe = regexp.MustCompile(`^(?:(\d{1,6})h)?(?:(\d{1,9})m?)?$`)

// parseMin: 분 단위로 반환. 빈 입력=30, 잘못된 입력=-1, 0=뒤로
func parseMin(s string) int64 {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 30
	}
	m := minRe.FindStringSubmatch(s)
	if m == nil {
		return -1
	}
	var h, mi int64
	if m[1] != "" {
		h, _ = strconv.ParseInt(m[1], 10, 64)
	}
	if m[2] != "" {
		mi, _ = strconv.ParseInt(m[2], 10, 64)
	}
	tot := h*60 + mi
	if tot > 5256000 { // shutdown.exe /t 최대값(10년)
		return -1
	}
	return tot
}

func fmtDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%02d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

// ---- 타이머 동작 ----

func (m *model) say(msg string, col lipgloss.Color) { m.msg, m.msgCol = msg, col }

func (m *model) setTimer(sec int64) {
	old := m.cur
	secS := strconv.FormatInt(sec, 10)
	code := runShutdown("/s", "/t", secS)
	had := false
	if code == 1190 { // 이미 예약된 종료가 있음 -> 취소 후 재시도
		had = true
		runShutdown("/a")
		code = runShutdown("/s", "/t", secS)
	}
	if code != 0 {
		if had {
			// 기존 예약이 이 프로그램이 건 것이면 남은 시간만큼 복구 시도
			var left int64
			if old != nil {
				left = int64(time.Until(old.Target).Seconds())
			}
			if left > 0 && runShutdown("/s", "/t", strconv.FormatInt(left, 10)) == 0 {
				m.say(fmt.Sprintf("예약 실패 (오류 %d) / 기존 타이머 유지됨", code), cRed)
				return
			}
			os.Remove(statePath())
			m.cur = nil
			m.say(fmt.Sprintf("예약 실패 (오류 %d) / 기존 예약은 취소된 상태", code), cRed)
		} else {
			m.say(fmt.Sprintf("예약 실패 (shutdown 오류 %d)", code), cRed)
		}
		return
	}
	n := time.Now()
	t := n.Add(time.Duration(sec) * time.Second)
	m.cur = &Timer{Start: n, Target: t}
	os.Remove(statePath())
	col, warn := cGreen, ""
	if err := writeState(m.cur, m.boot); err != nil {
		m.cur.NoFile = true
		col, warn = cYellow, " / 상태 파일 저장 실패"
	}
	txt := fmtDur(time.Duration(sec)*time.Second) + " 후 종료 예약 (" + t.Format("15:04:05") + ")"
	if had {
		txt += " / 기존 예약 취소됨"
	}
	m.say(txt+warn, col)
}

func (m *model) stopTimer() {
	ok := runShutdown("/a") == 0
	os.Remove(statePath())
	m.cur = nil
	if ok {
		m.say("종료 예약 취소 완료.", cGreen)
	} else {
		m.say("예약된 종료 없음.", cYellow)
	}
}

// sync: 다른 창에서 취소/변경했거나 만료됐으면 화면 상태를 상태 파일에 맞춤
func (m *model) sync() {
	old := m.cur
	if old != nil && old.NoFile {
		if !old.Target.After(time.Now()) {
			m.cur = nil
			m.say("", cGray)
		}
		return
	}
	nw := loadTimer(m.boot)
	switch {
	case old != nil && nw == nil:
		if !old.Target.After(time.Now()) {
			m.say("", cGray)
		} else {
			m.say("예약이 다른 곳에서 취소됨", cYellow)
		}
	case old != nil && nw != nil && !nw.Target.Equal(old.Target):
		m.say("예약이 다른 곳에서 변경됨", cYellow)
	}
	m.cur = nw
}
