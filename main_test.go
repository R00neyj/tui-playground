package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestParseMin(t *testing.T) {
	cases := map[string]int64{
		"": 30, "30": 30, "90": 90, "1h30m": 90, "2h": 120, "1h30": 90, "0": 0, " 15 ": 15, "1H": 60,
		"abc": -1, "m": -1, "h": -1, "-5": -1, "1.5": -1, "5256001": -1, "5256000": 5256000,
	}
	for in, want := range cases {
		if got := parseMin(in); got != want {
			t.Errorf("parseMin(%q)=%d, want %d", in, got, want)
		}
	}
}

func TestBigRowsWidth(t *testing.T) {
	rows := bigRows("12:34:56")
	for i, r := range rows {
		if lipgloss.Width(r) != lipgloss.Width(rows[0]) {
			t.Errorf("row %d width %d != %d", i, lipgloss.Width(r), lipgloss.Width(rows[0]))
		}
	}
}

func checkAligned(t *testing.T, name, view string) {
	t.Helper()
	lines := strings.Split(view, "\n")
	// 박스 줄(│ ╭ ├ ╰ 로 시작)만 비교
	w := -1
	for _, l := range lines {
		plain := strings.TrimLeft(l, " ")
		if !(strings.HasPrefix(plain, "│") || strings.HasPrefix(plain, "╭") || strings.HasPrefix(plain, "├") || strings.HasPrefix(plain, "╰")) {
			continue
		}
		lw := lipgloss.Width(strings.TrimSpace(l))
		if w < 0 {
			w = lw
		} else if lw != w {
			t.Errorf("%s: 박스 줄 폭 불일치 %d != %d: %q", name, lw, w, l)
		}
	}
	if w < 0 {
		t.Errorf("%s: 박스 줄 없음", name)
	}
}

func TestViewsAligned(t *testing.T) {
	for _, sz := range [][2]int{{120, 40}, {80, 24}, {80, 21}, {60, 20}, {56, 17}} {
		m := newModel()
		m.w, m.h = sz[0], sz[1]
		m.ph = phMain
		m.say("메시지 확인 용 한글 문장입니다", cGreen)
		checkAligned(t, "idle", m.viewMain())
		m.cur = &Timer{Start: time.Now().Add(-5 * time.Minute), Target: time.Now().Add(25 * time.Minute)}
		checkAligned(t, "active", m.viewMain())
		m.cur = &Timer{Start: time.Now().Add(-1 * time.Hour), Target: time.Now().Add(100 * time.Hour)}
		checkAligned(t, "100h", m.viewMain())
		m.ph = phReserve
		checkAligned(t, "reserve", m.viewReserve())
		m.cur = nil
		checkAligned(t, "reserve-nocur", m.viewReserve())
	}
}

func TestProgressBarMoves(t *testing.T) {
	m := newModel()
	m.w, m.h = 100, 40
	m.ph = phMain
	m.cur = &Timer{Start: time.Now().Add(-15 * time.Minute), Target: time.Now().Add(15 * time.Minute)}
	v := m.viewMain()
	if !strings.Contains(v, " 50%") {
		t.Errorf("50%% 표시가 없음:\n%s", v)
	}
}

func TestIntroFrames(t *testing.T) {
	m := newModel()
	m.w, m.h = 100, 40
	for _, d := range []time.Duration{0, 300 * time.Millisecond, 900 * time.Millisecond, 1500 * time.Millisecond, 2000 * time.Millisecond} {
		m.introStart = time.Now().Add(-d)
		if m.viewIntro() == "" {
			t.Errorf("intro 비어 있음 @%v", d)
		}
	}
	// 끝 무렵에는 부제목이 "터 미 널   플 레 이 그 라 운 드" 형태로 완성돼야 함
	m.introStart = time.Now().Add(-2 * time.Second)
	if v := ansi.Strip(m.viewIntro()); !strings.Contains(v, "터 미 널   플 레 이 그 라 운 드") {
		t.Errorf("부제목이 없음:\n%s", v)
	}
	// 폭이 좁은 창에서도 로고가 잘리지 않아야 함
	m.w, m.h = 30, 20
	m.introStart = time.Now().Add(-2 * time.Second)
	_ = m.viewIntro()
}

func TestHeaderHasAppName(t *testing.T) {
	m := newModel()
	m.w, m.h = 100, 40
	m.ph = phMain
	if v := ansi.Strip(m.viewMain()); !strings.Contains(v, appName) {
		t.Errorf("헤더에 앱 이름이 없음")
	}
}

// ---- 예약 로직 (shutdown.exe 는 가짜로 대체) ----

type fakeSD struct {
	calls   [][]string
	results []int // 호출 순서대로 반환할 코드, 모자라면 0
}

func (f *fakeSD) run(args ...string) int {
	f.calls = append(f.calls, args)
	if len(f.results) == 0 {
		return 0
	}
	r := f.results[0]
	f.results = f.results[1:]
	return r
}

func setup(t *testing.T, results ...int) (*fakeSD, model) {
	t.Helper()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	f := &fakeSD{results: results}
	old := runShutdown
	runShutdown = f.run
	t.Cleanup(func() { runShutdown = old })
	m := newModel()
	m.boot = bootUnix()
	return f, m
}

func TestSetTimerNew(t *testing.T) {
	f, m := setup(t)
	m.setTimer(1800)
	if m.cur == nil || len(f.calls) != 1 || m.msgCol != cGreen {
		t.Fatalf("cur=%v calls=%v msg=%q", m.cur, f.calls, m.msg)
	}
	if loadTimer(m.boot) == nil {
		t.Fatal("상태 파일이 저장되지 않음")
	}
}

func TestSetTimerReplacesExisting(t *testing.T) {
	f, m := setup(t, 1190, 0, 0) // /s -> 이미 예약됨, /a -> ok, /s -> ok
	m.setTimer(600)
	if len(f.calls) != 3 || f.calls[1][0] != "/a" || m.cur == nil || !strings.Contains(m.msg, "기존 예약 취소됨") {
		t.Fatalf("calls=%v msg=%q", f.calls, m.msg)
	}
}

func TestSetTimerFailKeepsOld(t *testing.T) {
	f, m := setup(t)
	m.setTimer(3600) // 기존 타이머
	f.calls = nil
	f.results = []int{1190, 0, 5, 0} // /s 중복, /a ok, /s 실패, 복구 /s ok
	oldTarget := m.cur.Target
	m.setTimer(60)
	if m.cur == nil || !m.cur.Target.Equal(oldTarget) || !strings.Contains(m.msg, "기존 타이머 유지됨") {
		t.Fatalf("cur=%v msg=%q calls=%v", m.cur, m.msg, f.calls)
	}
	if last := f.calls[len(f.calls)-1]; last[0] != "/s" {
		t.Fatalf("복구 호출이 없음: %v", f.calls)
	}
}

func TestSetTimerFailRestoreFails(t *testing.T) {
	f, m := setup(t)
	m.setTimer(3600)
	f.results = []int{1190, 0, 5, 7}
	m.setTimer(60)
	if m.cur != nil || !strings.Contains(m.msg, "취소된 상태") {
		t.Fatalf("cur=%v msg=%q", m.cur, m.msg)
	}
	if _, err := os.Stat(statePath()); err == nil {
		t.Fatal("상태 파일이 남아 있음")
	}
}

func TestSetTimerFirstFailsOtherCode(t *testing.T) {
	f, m := setup(t)
	m.setTimer(3600)
	old := m.cur
	f.calls = nil
	f.results = []int{5}
	m.setTimer(60)
	if m.cur != old || len(f.calls) != 1 || m.msgCol != cRed {
		t.Fatalf("기존 예약이 유지돼야 함: calls=%v msg=%q", f.calls, m.msg)
	}
}

func TestSyncOtherWindowCancel(t *testing.T) {
	_, m := setup(t)
	m.setTimer(3600)
	os.Remove(statePath()) // 다른 창에서 취소
	m.sync()
	if m.cur != nil || !strings.Contains(m.msg, "다른 곳에서 취소됨") {
		t.Fatalf("cur=%v msg=%q", m.cur, m.msg)
	}
}

func TestSyncStateWriteFailure(t *testing.T) {
	_, m := setup(t)
	// 상태 파일 경로를 디렉터리로 막아 쓰기 실패를 유도
	os.MkdirAll(statePath(), 0o755)
	os.WriteFile(statePath()+`\keep`, []byte("x"), 0o600) // 비어 있지 않아야 Remove 로 지워지지 않음
	m.setTimer(3600)
	if m.cur == nil || !m.cur.NoFile || m.msgCol != cYellow {
		t.Fatalf("cur=%v msg=%q", m.cur, m.msg)
	}
	m.sync()
	if m.cur == nil {
		t.Fatal("NoFile 타이머가 sync 에서 사라짐")
	}
}

func TestStaleBootDropped(t *testing.T) {
	_, m := setup(t)
	m.setTimer(3600)
	if loadTimer(m.boot+3600) != nil {
		t.Fatal("다른 부팅의 상태 파일이 유지됨")
	}
}

func TestBootUnixStable(t *testing.T) {
	a := bootUnix()
	time.Sleep(1100 * time.Millisecond)
	b := bootUnix()
	if d := a - b; d > 2 || d < -2 {
		t.Errorf("부팅 시각이 흔들림: %d vs %d", a, b)
	}
	if up := time.Now().Unix() - a; up < 0 || up > 3650*86400 {
		t.Errorf("가동 시간이 이상함: %d초", up)
	}
	t.Logf("boot=%s", time.Unix(a, 0).Format(time.RFC3339))
}

func TestStopTimer(t *testing.T) {
	_, m := setup(t)
	m.setTimer(3600)
	m.stopTimer()
	if m.cur != nil || loadTimer(m.boot) != nil || m.msgCol != cGreen {
		t.Fatalf("cur=%v msg=%q", m.cur, m.msg)
	}
}
