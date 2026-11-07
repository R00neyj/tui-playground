package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

// ---- 호스트와 홈 ----

func key(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestRegistrySane(t *testing.T) {
	seen := map[string]bool{}
	for _, info := range registry {
		if info.ID == "" || info.Name == "" || info.Desc == "" || info.New == nil {
			t.Errorf("등록 정보가 비어 있음: %+v", info)
		}
		if seen[info.ID] {
			t.Errorf("앱 ID 가 겹침: %s", info.ID)
		}
		seen[info.ID] = true
	}
}

func TestHomeViewAligned(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	m := newModel()
	for _, sz := range testSizes {
		c := testCtx(sz[0], sz[1])
		v := m.viewHome(c)
		assertFrame(t, "home", c, v)
		for _, info := range registry {
			if !strings.Contains(plain(v), info.Name) {
				t.Errorf("홈에 앱 이름이 없음: %s", info.Name)
			}
		}
	}
}

func TestHostFlow(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	var mod tea.Model = newModel()
	mod, _ = mod.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	mod, _ = mod.Update(key("x")) // 인트로 건너뛰기
	if mod.(model).ph != phHome {
		t.Fatalf("인트로가 건너뛰어지지 않음")
	}
	mod, _ = mod.Update(key("down"))
	if mod.(model).sel != 1 {
		t.Fatalf("아래 키로 선택이 이동하지 않음: %d", mod.(model).sel)
	}
	mod, _ = mod.Update(key("1")) // 숫자키로 바로 열기
	if m := mod.(model); m.ph != phApp || m.sel != 0 || m.app == nil {
		t.Fatalf("1번 앱이 열리지 않음: %+v", m.ph)
	}
	mod, _ = mod.Update(goHomeMsg{})
	if m := mod.(model); m.ph != phHome || m.app != nil || m.sel != 0 {
		t.Fatalf("홈으로 복귀하지 않음")
	}
	for _, k := range []string{"q", "esc"} { // F-01: 홈에서 Esc/q 로 종료
		_, cmd := mod.Update(key(k))
		if cmd == nil {
			t.Fatalf("홈에서 %s 로 종료되지 않음", k)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("홈에서 %s 의 결과가 종료가 아님", k)
		}
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
	m.introStart = time.Now().Add(-2 * time.Second)
	if v := plain(m.viewIntro()); !strings.Contains(v, "터 미 널   플 레 이 그 라 운 드") {
		t.Errorf("부제목이 없음:\n%s", v)
	}
	m.w, m.h = 30, 20
	_ = m.viewIntro()
}

// 등록된 모든 앱에 공통으로 적용하는 검사 (새 앱도 registry 에 넣으면 자동으로 검사받음)
func TestAllAppsRender(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	env := &Env{Boot: bootUnix()}
	for _, info := range registry {
		for _, sz := range testSizes {
			a := info.New(env)
			if a == nil {
				t.Fatalf("%s: New 가 nil 을 돌려줌", info.ID)
			}
			c := testCtx(sz[0], sz[1])
			v := a.View(c)
			assertFrame(t, info.ID, c, v)
			// 화면이 창 높이를 넘으면 가운데 정렬 때 윗부분(머리줄·탭)이 잘려 나감. 큰 숫자를 쓰는 높이(21)보다 낮은 창에서도 한 줄 숫자로 들어와야 함
			if n := strings.Count(v, "\n") + 1; n > sz[1] {
				t.Errorf("%s: %dx%d 창에서 화면이 %d줄로 창 높이를 넘음", info.ID, sz[0], sz[1], n)
			}
		}
	}
}

func TestFreshAppEscGoesHome(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	env := &Env{Boot: bootUnix()}
	for _, info := range registry {
		a := info.New(env)
		_, cmd := a.Update(key("esc"))
		if cmd == nil {
			t.Errorf("%s: 처음 상태에서 Esc 를 눌러도 홈으로 가지 않음", info.ID)
			continue
		}
		if _, ok := cmd().(goHomeMsg); !ok {
			t.Errorf("%s: Esc 의 결과가 goHomeMsg 가 아님", info.ID)
		}
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
}

// ---- 종료 타이머 로직 (shutdown 은 가짜로 대체) ----

type fakeSD struct {
	calls   []string
	results []int // schedule 호출 순서대로 돌려줄 코드, 모자라면 0
	cancels int
}

func (f *fakeSD) schedule(sec int64) int {
	f.calls = append(f.calls, "s"+strconv.FormatInt(sec, 10))
	if len(f.results) == 0 {
		return 0
	}
	r := f.results[0]
	f.results = f.results[1:]
	return r
}

func (f *fakeSD) cancel() bool { f.calls = append(f.calls, "a"); f.cancels++; return true }

func setupTimer(t *testing.T, results ...int) (*fakeSD, *timerApp) {
	t.Helper()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	f := &fakeSD{results: results}
	oldS, oldC := shutdownSchedule, shutdownCancel
	shutdownSchedule, shutdownCancel = f.schedule, f.cancel
	t.Cleanup(func() { shutdownSchedule, shutdownCancel = oldS, oldC })
	env := &Env{Boot: bootUnix()}
	return f, newTimerApp(env).(*timerApp)
}

func TestTimerSetNew(t *testing.T) {
	f, a := setupTimer(t)
	a.setTimer(1800)
	if a.cur == nil || len(f.calls) != 1 || a.msgTone != ToneOk {
		t.Fatalf("cur=%v calls=%v msg=%q", a.cur, f.calls, a.msg)
	}
	if loadTimer(a.env.Boot) == nil {
		t.Fatal("상태 파일이 저장되지 않음")
	}
}

func TestTimerReplacesExisting(t *testing.T) {
	f, a := setupTimer(t, codeAlreadyScheduled, 0)
	a.setTimer(600)
	if len(f.calls) != 3 || f.calls[1] != "a" || a.cur == nil || !strings.Contains(a.msg, "기존 예약 취소됨") {
		t.Fatalf("calls=%v msg=%q", f.calls, a.msg)
	}
}

func TestTimerFailKeepsOld(t *testing.T) {
	f, a := setupTimer(t)
	a.setTimer(3600)
	f.calls = nil
	f.results = []int{codeAlreadyScheduled, 5, 0} // 중복, 새 예약 실패, 복구 성공
	oldTarget := a.cur.Target
	a.setTimer(60)
	if a.cur == nil || !a.cur.Target.Equal(oldTarget) || !strings.Contains(a.msg, "기존 타이머 유지됨") {
		t.Fatalf("cur=%v msg=%q calls=%v", a.cur, a.msg, f.calls)
	}
	if !strings.HasPrefix(f.calls[len(f.calls)-1], "s") {
		t.Fatalf("복구 호출이 없음: %v", f.calls)
	}
}

func TestTimerFailRestoreFails(t *testing.T) {
	f, a := setupTimer(t)
	a.setTimer(3600)
	f.results = []int{codeAlreadyScheduled, 5, 7}
	a.setTimer(60)
	if a.cur != nil || !strings.Contains(a.msg, "취소된 상태") {
		t.Fatalf("cur=%v msg=%q", a.cur, a.msg)
	}
	if _, err := os.Stat(statePath()); err == nil {
		t.Fatal("상태 파일이 남아 있음")
	}
}

func TestTimerFirstFailsOtherCode(t *testing.T) {
	f, a := setupTimer(t)
	a.setTimer(3600)
	old := a.cur
	f.calls = nil
	f.results = []int{5}
	a.setTimer(60)
	if a.cur != old || len(f.calls) != 1 || a.msgTone != ToneDanger {
		t.Fatalf("기존 예약이 유지돼야 함: calls=%v msg=%q", f.calls, a.msg)
	}
}

func TestTimerSyncOtherWindowCancel(t *testing.T) {
	_, a := setupTimer(t)
	a.setTimer(3600)
	os.Remove(statePath()) // 다른 창에서 취소
	a.sync()
	if a.cur != nil || !strings.Contains(a.msg, "다른 곳에서 취소됨") {
		t.Fatalf("cur=%v msg=%q", a.cur, a.msg)
	}
}

func TestTimerStateWriteFailure(t *testing.T) {
	_, a := setupTimer(t)
	os.MkdirAll(statePath(), 0o755) // 상태 파일 경로를 디렉터리로 막아 쓰기 실패를 유도
	os.WriteFile(statePath()+`\keep`, []byte("x"), 0o600)
	a.setTimer(3600)
	if a.cur == nil || !a.cur.NoFile || a.msgTone != ToneWarn {
		t.Fatalf("cur=%v msg=%q", a.cur, a.msg)
	}
	a.sync()
	if a.cur == nil {
		t.Fatal("NoFile 타이머가 sync 에서 사라짐")
	}
}

func TestTimerStaleBootDropped(t *testing.T) {
	_, a := setupTimer(t)
	a.setTimer(3600)
	if loadTimer(a.env.Boot+3600) != nil {
		t.Fatal("다른 부팅의 상태 파일이 유지됨")
	}
}

func TestTimerStop(t *testing.T) {
	_, a := setupTimer(t)
	a.setTimer(3600)
	a.stopTimer()
	if a.cur != nil || loadTimer(a.env.Boot) != nil || a.msgTone != ToneOk {
		t.Fatalf("cur=%v msg=%q", a.cur, a.msg)
	}
}

func TestTimerViewsAligned(t *testing.T) {
	_, a := setupTimer(t)
	for _, sz := range testSizes {
		c := testCtx(sz[0], sz[1])
		a.ph = tpMain
		a.cur = nil
		assertFrame(t, "timer-idle", c, a.View(c))
		a.cur = &Timer{Start: fixedNow.Add(-22 * time.Minute), Target: fixedNow.Add(38 * time.Minute)}
		assertFrame(t, "timer-active", c, a.View(c))
		a.cur = &Timer{Start: fixedNow.Add(-1 * time.Hour), Target: fixedNow.Add(42 * time.Second)}
		a.say("예약 실패 (오류 5) / 기존 타이머 유지됨", ToneDanger)
		assertFrame(t, "timer-danger", c, a.View(c))
		a.ph = tpNew
		assertFrame(t, "timer-new", c, a.View(c))
		a.inputErr = "잘못된 입력."
		assertFrame(t, "timer-new-err", c, a.View(c))
	}
}

func TestTimerProgressMoves(t *testing.T) {
	_, a := setupTimer(t)
	a.cur = &Timer{Start: fixedNow.Add(-15 * time.Minute), Target: fixedNow.Add(15 * time.Minute)}
	c := testCtx(100, 40)
	if v := plain(a.View(c)); !strings.Contains(v, " 50%") {
		t.Errorf("50%% 표시가 없음:\n%s", v)
	}
}

func TestTimerStatusForHome(t *testing.T) {
	_, a := setupTimer(t)
	if st := timerStatus(a.env, time.Now()); st.Badge != "" || st.Text != "예약 없음" {
		t.Errorf("예약이 없을 때 상태: %+v", st)
	}
	a.setTimer(3600)
	if st := timerStatus(a.env, time.Now()); st.Badge != "예약됨" || st.Tone != ToneOk || st.Hint == "" {
		t.Errorf("예약 중 상태: %+v", st)
	}
}

// ---- 플랫폼이 알려주는 실패 이유 (macOS/Linux 권한 문제 등) ----

func fakeReasons(t *testing.T, fail func(int) string, cancel func() string) {
	t.Helper()
	oldF, oldC := shutdownFailReason, shutdownCancelReason
	shutdownFailReason, shutdownCancelReason = fail, cancel
	t.Cleanup(func() { shutdownFailReason, shutdownCancelReason = oldF, oldC })
}

func TestTimerScheduleFailShowsReason(t *testing.T) {
	_, a := setupTimer(t, 13)
	fakeReasons(t, func(c int) string {
		if c == 13 {
			return "권한 없음 (sudo 로 실행해야 함)"
		}
		return ""
	}, func() string { return "" })
	a.setTimer(600)
	if !strings.Contains(a.msg, "권한 없음 (sudo 로 실행해야 함)") || strings.Contains(a.msg, "오류 13") || a.msgTone != ToneDanger {
		t.Fatalf("이유가 화면에 보이지 않음: %q", a.msg)
	}
}

func TestTimerCancelFailKeepsExistingOnReplace(t *testing.T) {
	f, a := setupTimer(t)
	a.setTimer(3600)
	old := a.cur
	f.calls = nil
	f.results = []int{codeAlreadyScheduled}
	fakeReasons(t, func(int) string { return "" }, func() string { return "권한 없음" })
	cancelOld := shutdownCancel
	shutdownCancel = func() bool { f.calls = append(f.calls, "a-fail"); return false }
	t.Cleanup(func() { shutdownCancel = cancelOld })
	a.setTimer(60)
	if a.cur != old || !strings.Contains(a.msg, "기존 타이머 유지됨") || !strings.Contains(a.msg, "권한 없음") {
		t.Fatalf("취소 실패 시 기존 예약이 유지돼야 함: cur=%v msg=%q", a.cur, a.msg)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "s") && c != "s3600" && c != "s60" {
			t.Fatalf("예상 밖 호출: %v", f.calls)
		}
	}
	if len(f.calls) != 2 { // 첫 schedule(중복) + 취소 시도 뒤 재시도하지 않아야 함
		t.Fatalf("취소에 실패했는데 재예약을 시도함: %v", f.calls)
	}
	if loadTimer(a.env.Boot) == nil {
		t.Fatal("상태 파일이 지워짐")
	}
}

func TestTimerStopCancelFailKeepsState(t *testing.T) {
	_, a := setupTimer(t)
	a.setTimer(3600)
	fakeReasons(t, func(int) string { return "" }, func() string { return "권한 없음" })
	old := shutdownCancel
	shutdownCancel = func() bool { return false }
	t.Cleanup(func() { shutdownCancel = old })
	a.stopTimer()
	if a.cur == nil || loadTimer(a.env.Boot) == nil || !strings.Contains(a.msg, "예약은 그대로 남아 있음") || a.msgTone != ToneDanger {
		t.Fatalf("취소 실패인데 예약이 지워지거나 안내가 틀림: cur=%v msg=%q", a.cur, a.msg)
	}
}
