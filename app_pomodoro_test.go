package main

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// 시계와 벨을 가짜로 바꾼 포모도로 앱
type pomoEnv struct {
	now   time.Time
	bells int
}

func setupPomo(t *testing.T) (*pomoEnv, *pomodoroApp) {
	t.Helper()
	e := &pomoEnv{now: fixedNow}
	oldNow, oldBell := pomoNow, pomoBell
	pomoNow = func() time.Time { return e.now }
	pomoBell = func() { e.bells++ }
	t.Cleanup(func() { pomoNow, pomoBell = oldNow, oldBell })
	return e, newPomodoroApp(&Env{}).(*pomodoroApp)
}

func (e *pomoEnv) advance(d time.Duration) { e.now = e.now.Add(d) }

// send 는 키를 보내고 돌려받은 명령을 그대로 돌려줌
func pomoSend(a *pomodoroApp, m tea.Msg) tea.Cmd {
	_, cmd := a.Update(m)
	return cmd
}

// tick 은 앱이 지금 기다리는 틱을 보냄
func (a *pomodoroApp) tick() tea.Cmd { return pomoSend(a, appTickMsg{Gen: a.gen, Tag: a.tag}) }

func pomoIsHome(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(goHomeMsg)
	return ok
}

func pomoViewAt(a *pomodoroApp, w, h int) string {
	c := testCtx(w, h)
	c.Now = pomoNow()
	return plain(a.View(c))
}

func TestPomoDefaultIdle(t *testing.T) {
	_, a := setupPomo(t)
	if a.focusMin != 25 || a.breakMin != 5 || a.phase != pomoIdle {
		t.Fatalf("기본값/상태: %d/%d phase=%d", a.focusMin, a.breakMin, a.phase)
	}
	v := pomoViewAt(a, 56, 17) // 한 줄 숫자
	for _, want := range []string{"대기", "오늘 완료  0회", "25:00", "시작", "리셋", "설정", "집중 25분 · 휴식 5분", "홈으로"} {
		if !strings.Contains(v, want) {
			t.Errorf("대기 화면에 %q 없음:\n%s", want, v)
		}
	}
	if strings.Contains(v, "진행 중이면 확인함") {
		t.Errorf("대기 중에는 확인 안내가 없어야 함")
	}
	if !strings.Contains(pomoViewAt(a, 120, 40), "██") {
		t.Errorf("큰 창에서는 큰 숫자를 써야 함")
	}
}

func TestPomoStartPauseResumeReset(t *testing.T) {
	e, a := setupPomo(t)
	if cmd := pomoSend(a, key(" ")); cmd == nil || a.phase != pomoFocus || a.paused {
		t.Fatalf("시작 안 됨: phase=%d cmd=%v", a.phase, cmd)
	}
	e.advance(10*time.Minute + 30*time.Second)
	if got := a.remaining(e.now); got != 14*time.Minute+30*time.Second {
		t.Fatalf("남은 시간 %v", got)
	}
	v := pomoViewAt(a, 56, 17)
	if !strings.Contains(v, "집중") || !strings.Contains(v, "14:30") || !strings.Contains(v, "일시정지") || !strings.Contains(v, "진행 중이면 확인함") {
		t.Errorf("집중 화면:\n%s", v)
	}

	pomoSend(a, key(" ")) // 일시정지
	if !a.paused {
		t.Fatal("일시정지 안 됨")
	}
	e.advance(7 * time.Minute) // 멈춘 동안은 줄지 않음
	if got := a.remaining(e.now); got != 14*time.Minute+30*time.Second {
		t.Fatalf("일시정지 중 남은 시간이 변함: %v", got)
	}
	c := testCtx(56, 17)
	c.Now = e.now
	v = plain(a.View(c))
	if !strings.Contains(v, "일시정지 ") || !strings.Contains(v, "집중 · 오늘 완료  0회") || !strings.Contains(v, "일시정지됨. Space 로 다시 시작함") || !strings.Contains(v, "14:30") {
		t.Errorf("일시정지 화면:\n%s", v)
	}

	if cmd := pomoSend(a, key(" ")); cmd == nil || a.paused { // 재개
		t.Fatal("재개 안 됨")
	}
	e.advance(30 * time.Second)
	if got := a.remaining(e.now); got != 14*time.Minute {
		t.Fatalf("재개 뒤 남은 시간 %v", got)
	}

	pomoSend(a, key("r"))
	if a.phase != pomoIdle || a.done != 0 || a.remaining(e.now) != 25*time.Minute {
		t.Fatalf("리셋 안 됨: phase=%d done=%d", a.phase, a.done)
	}
}

func TestPomoDeadlineBasedNotTickCount(t *testing.T) {
	e, a := setupPomo(t)
	pomoSend(a, key(" "))
	// 틱이 하나도 안 왔어도 시계만 가면 남은 시간이 줄어듦
	e.advance(5 * time.Minute)
	if got := a.remaining(e.now); got != 20*time.Minute {
		t.Fatalf("deadline 기준이 아님: %v", got)
	}
}

func TestPomoSegmentEndSwitchesWithBell(t *testing.T) {
	e, a := setupPomo(t)
	pomoSend(a, key(" "))
	e.advance(25*time.Minute - time.Second)
	a.tick()
	if a.phase != pomoFocus || e.bells != 0 {
		t.Fatalf("종료 전인데 전환됨")
	}
	e.advance(time.Second)
	cmd := a.tick()
	if a.phase != pomoBreak || a.done != 1 || e.bells != 1 || cmd == nil {
		t.Fatalf("집중→휴식 전환 실패: phase=%d done=%d bells=%d", a.phase, a.done, e.bells)
	}
	if !strings.Contains(a.msg, "집중 구간 끝. 휴식 5분을 시작함") {
		t.Errorf("메시지: %q", a.msg)
	}
	v := pomoViewAt(a, 56, 17)
	if !strings.Contains(v, "휴식") || !strings.Contains(v, "05:00") || !strings.Contains(v, "오늘 완료  1회") {
		t.Errorf("휴식 화면:\n%s", v)
	}
	if a.tone() != ToneOk {
		t.Errorf("휴식은 ToneOk 여야 함")
	}

	e.advance(5 * time.Minute)
	a.tick()
	if a.phase != pomoFocus || a.done != 1 || e.bells != 2 || a.remaining(e.now) != 25*time.Minute {
		t.Fatalf("휴식→집중 전환 실패: phase=%d done=%d bells=%d rem=%v", a.phase, a.done, e.bells, a.remaining(e.now))
	}
	if a.tone() != ToneAccent {
		t.Errorf("집중은 ToneAccent 여야 함")
	}

	// 한 사이클 더 돌면 2회
	e.advance(25 * time.Minute)
	a.tick()
	if a.done != 2 {
		t.Errorf("완료 횟수 %d", a.done)
	}
}

func TestPomoLateTickNoDrift(t *testing.T) {
	e, a := setupPomo(t)
	pomoSend(a, key(" "))
	e.advance(25*time.Minute + 2*time.Second) // 틱이 2초 늦음
	a.tick()
	// 다음 구간은 원래 종료 시각에서 이어지므로 5분에서 2초 줄어든 상태
	if got := a.remaining(e.now); got != 5*time.Minute-2*time.Second {
		t.Fatalf("늦은 틱 뒤 남은 시간 %v", got)
	}
	// 오래 멈췄다 돌아온 경우(절전)는 구간을 건너뛰지 않고 새 구간을 처음부터 셈
	e.advance(3 * time.Hour)
	a.tick()
	if a.phase != pomoFocus || a.done != 1 || a.remaining(e.now) != 25*time.Minute {
		t.Fatalf("절전 복귀: phase=%d done=%d rem=%v", a.phase, a.done, a.remaining(e.now))
	}
}

func TestPomoTicksIgnoredWhenStale(t *testing.T) {
	e, a := setupPomo(t)
	pomoSend(a, key(" "))
	oldTag := a.tag
	e.advance(26 * time.Minute)
	if cmd := pomoSend(a, appTickMsg{Gen: a.gen + 1, Tag: a.tag}); cmd != nil || a.phase != pomoFocus {
		t.Errorf("다른 gen 의 틱을 처리함")
	}
	pomoSend(a, key(" ")) // 일시정지 (이미 종료 시각이 지났으므로 전환 뒤 정지)
	pomoSend(a, key(" ")) // 재개 -> 새 틱 줄기
	if a.tag == oldTag {
		t.Fatal("재개해도 틱 줄기가 그대로임")
	}
	done := a.done
	if cmd := pomoSend(a, appTickMsg{Gen: a.gen, Tag: oldTag}); cmd != nil || a.done != done {
		t.Errorf("옛 틱 줄기를 처리함")
	}
	// 일시정지 중 도착한 틱은 이어지지 않음
	pomoSend(a, key(" "))
	if cmd := a.tick(); cmd != nil {
		t.Errorf("일시정지 중에 틱이 이어짐")
	}
	// 리셋 뒤 도착한 틱도 마찬가지
	pomoSend(a, key(" "))
	pomoSend(a, key("r"))
	if cmd := a.tick(); cmd != nil {
		t.Errorf("대기 중에 틱이 이어짐")
	}
}

func TestPomoEscBehavior(t *testing.T) {
	_, a := setupPomo(t)
	if !pomoIsHome(pomoSend(a, key("esc"))) {
		t.Fatal("대기 중 Esc 는 바로 홈")
	}

	// 집중 중: 확인 -> n 이면 계속
	pomoSend(a, key(" "))
	if cmd := pomoSend(a, key("esc")); cmd != nil || !a.confirm {
		t.Fatal("진행 중 Esc 는 확인을 띄워야 함")
	}
	v := pomoViewAt(a, 80, 24)
	if !strings.Contains(v, "진행 중인 타이머가 멈춥니다. 홈으로 나갈까요?") || !strings.Contains(v, "[y] 나감") || !strings.Contains(v, "[n] 계속") {
		t.Errorf("확인 문구:\n%s", v)
	}
	if cmd := pomoSend(a, key(" ")); cmd != nil || !a.confirm || a.paused { // 확인 중 다른 키는 무시
		t.Error("확인 중 다른 키가 먹힘")
	}
	pomoSend(a, key("n"))
	if a.confirm || a.phase != pomoFocus || a.paused {
		t.Fatal("n 은 계속이어야 함")
	}

	// 일시정지 중에도 확인 -> y 면 홈, 타이머는 멈춤
	pomoSend(a, key(" "))
	pomoSend(a, key("esc"))
	if !a.confirm {
		t.Fatal("일시정지 중 Esc 도 확인")
	}
	if !pomoIsHome(pomoSend(a, key("y"))) {
		t.Fatal("y 는 홈으로 나가야 함")
	}
	if a.phase != pomoIdle || a.running() {
		t.Fatal("나가면 타이머가 멈춰야 함")
	}

	// 휴식 중 Esc, 그리고 확인창에서 Esc 는 계속
	e2, b := setupPomo(t)
	pomoSend(b, key(" "))
	e2.advance(25 * time.Minute)
	b.tick()
	pomoSend(b, key("esc"))
	pomoSend(b, key("esc"))
	if b.confirm || b.phase != pomoBreak {
		t.Error("확인창에서 Esc 는 계속이어야 함")
	}
}

func TestPomoCtrlKeysIgnored(t *testing.T) {
	_, a := setupPomo(t)
	for _, k := range []string{"q", "x", "1"} {
		if cmd := pomoSend(a, key(k)); cmd != nil || a.phase != pomoIdle || a.screen != pomoScrMain {
			t.Errorf("%q 가 동작을 일으킴", k)
		}
	}
}

func TestPomoParseMin(t *testing.T) {
	cases := map[string]int{"25": 25, " 5 ": 5, "1": 1, "999": 999, "007": 7}
	for in, want := range cases {
		if got, e := pomoParseMin(in); e != "" || got != want {
			t.Errorf("pomoParseMin(%q)=%d,%q", in, got, e)
		}
	}
	for _, in := range []string{"", " ", "0", "00", "-5", "abc", "1.5", "+3", "2분", "1000", "99999999999999999999"} {
		if _, e := pomoParseMin(in); e == "" {
			t.Errorf("pomoParseMin(%q) 를 거부해야 함", in)
		}
	}
	if _, e := pomoParseMin("0"); e != "1 이상의 숫자만 입력할 수 있음." {
		t.Errorf("오류 문구: %q", e)
	}
}

func pomoType(a *pomodoroApp, s string) {
	for _, r := range s {
		pomoSend(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func TestPomoSetupFlow(t *testing.T) {
	_, a := setupPomo(t)
	pomoSend(a, key("e"))
	if a.screen != pomoScrSetup || a.inputs[0].Value() != "25" || a.inputs[1].Value() != "5" || a.field != 0 {
		t.Fatalf("설정 화면 초기값: screen=%d %q %q", a.screen, a.inputs[0].Value(), a.inputs[1].Value())
	}
	v := pomoViewAt(a, 80, 24)
	for _, want := range []string{"포모도로 설정", "집중과 휴식 시간을 분 단위로 입력함", "현재 값  집중 25분  /  휴식 5분", "입력 예  25  /  50  /  5", "Enter = 저장    Esc = 취소", "집중(분)  > 25", "휴식(분)  > 5"} {
		if !strings.Contains(v, want) {
			t.Errorf("설정 화면에 %q 없음:\n%s", want, v)
		}
	}

	pomoType(a, "50") // 처음 치는 글자는 기존 값을 대체함
	if a.inputs[0].Value() != "50" {
		t.Fatalf("집중 입력값 %q", a.inputs[0].Value())
	}
	pomoSend(a, key("enter"))
	if a.field != 1 || a.screen != pomoScrSetup {
		t.Fatalf("다음 칸으로 가야 함: field=%d", a.field)
	}
	pomoType(a, "10")
	pomoSend(a, key("enter"))
	if a.screen != pomoScrMain || a.focusMin != 50 || a.breakMin != 10 {
		t.Fatalf("저장 실패: %d/%d", a.focusMin, a.breakMin)
	}
	v = pomoViewAt(a, 56, 17)
	if !strings.Contains(v, "50:00") || !strings.Contains(v, "집중 50분 · 휴식 10분") || !strings.Contains(v, "설정을 저장함") {
		t.Errorf("저장 뒤 화면:\n%s", v)
	}
}

func TestPomoSetupEditKeepsDigits(t *testing.T) {
	_, a := setupPomo(t)
	pomoSend(a, key("e"))
	pomoSend(a, tea.KeyMsg{Type: tea.KeyBackspace}) // 지우기는 기존 값에서 이어서
	if a.inputs[0].Value() != "2" {
		t.Fatalf("백스페이스 결과 %q", a.inputs[0].Value())
	}
	pomoType(a, "5")
	if a.inputs[0].Value() != "25" {
		t.Fatalf("이어 쓰기 결과 %q", a.inputs[0].Value())
	}
}

func TestPomoSetupInvalid(t *testing.T) {
	for _, bad := range []string{"0", "abc", "-1", "1000"} {
		_, a := setupPomo(t)
		pomoSend(a, key("e"))
		pomoType(a, bad)
		pomoSend(a, key("enter"))
		if a.inputErr == "" || a.field != 0 || a.screen != pomoScrSetup || a.focusMin != 25 {
			t.Errorf("%q 를 거부해야 함: err=%q field=%d", bad, a.inputErr, a.field)
		}
		if v := pomoViewAt(a, 80, 24); !strings.Contains(v, a.inputErr) {
			t.Errorf("오류 문구가 화면에 없음: %q", a.inputErr)
		}
		pomoType(a, "7") // 다시 입력하면 오류가 사라짐
		if a.inputErr != "" {
			t.Errorf("입력하면 오류 문구가 지워져야 함")
		}
	}
	// 휴식 칸 오류
	_, a := setupPomo(t)
	pomoSend(a, key("e"))
	pomoSend(a, key("enter"))
	pomoType(a, "0")
	pomoSend(a, key("enter"))
	if a.inputErr != pomoErrMin || a.field != 1 || a.breakMin != 5 {
		t.Errorf("휴식 0 거부 실패: err=%q field=%d break=%d", a.inputErr, a.field, a.breakMin)
	}
	if v := pomoViewAt(a, 80, 24); !strings.Contains(v, "1 이상의 숫자만 입력할 수 있음.") {
		t.Errorf("오류 문구:\n%s", v)
	}
}

func TestPomoSetupEscCancels(t *testing.T) {
	_, a := setupPomo(t)
	pomoSend(a, key("e"))
	pomoType(a, "99")
	if cmd := pomoSend(a, key("esc")); cmd != nil || a.screen != pomoScrMain || a.focusMin != 25 {
		t.Fatalf("Esc 는 저장 없이 돌아가야 함 (홈으로 가면 안 됨): cmd=%v focus=%d", cmd, a.focusMin)
	}
}

func TestPomoSetupWhileRunningAppliesNext(t *testing.T) {
	e, a := setupPomo(t)
	pomoSend(a, key(" "))
	pomoSend(a, key("e"))
	pomoType(a, "30")
	pomoSend(a, key("enter"))
	pomoType(a, "2")
	pomoSend(a, key("enter"))
	if a.remaining(e.now) != 25*time.Minute {
		t.Fatalf("진행 중인 구간은 그대로여야 함: %v", a.remaining(e.now))
	}
	e.advance(25 * time.Minute)
	a.tick()
	if a.remaining(e.now) != 2*time.Minute {
		t.Fatalf("다음 구간은 새 휴식 값을 써야 함: %v", a.remaining(e.now))
	}
}

func TestPomoViewsAligned(t *testing.T) {
	e, a := setupPomo(t)
	type state struct {
		name string
		prep func()
	}
	reset := func() { a.reset(); a.screen = pomoScrMain; a.focusMin, a.breakMin = 25, 5; a.say("", ToneInfo) }
	states := []state{
		{"idle", func() {}},
		{"focus", func() { a.start(e.now) }},
		{"paused", func() { a.start(e.now); a.pause(e.now) }},
		{"break", func() { a.start(e.now); e.advance(25 * time.Minute); a.advance(e.now); e.now = fixedNow }},
		{"confirm", func() { a.start(e.now); a.confirm = true }},
		{"many", func() { a.focusMin, a.breakMin = 999, 999; a.done = 123; a.start(e.now); a.pause(e.now) }},
		{"setup", func() { a.openSetup() }},
		{"setup-err", func() { a.openSetup(); a.inputErr = pomoErrMax }},
	}
	for _, st := range states {
		for _, sz := range testSizes {
			reset()
			st.prep()
			c := testCtx(sz[0], sz[1])
			c.Now = e.now
			assertFrame(t, "pomo-"+st.name, c, a.View(c))
		}
	}
}

func TestPomoMessageLinesFit(t *testing.T) {
	// 가장 긴 메시지와 확인 문구가 가장 좁은 창에서도 한 줄에 들어가는지 (assertFrame 은 넘침을 잡음)
	e, a := setupPomo(t)
	a.focusMin, a.breakMin = 999, 999
	a.start(e.now)
	e.advance(999 * time.Minute)
	a.advance(e.now)
	c := testCtx(56, 17)
	c.Now = e.now
	assertFrame(t, "pomo-long-msg", c, a.View(c))
	a.confirm = true
	assertFrame(t, "pomo-long-confirm", c, a.View(c))
	if v := plain(a.View(c)); !strings.Contains(v, "[y] 나감") || !strings.Contains(v, "[n] 계속") {
		t.Errorf("좁은 창 확인 줄에 키 안내가 없음:\n%s", v)
	}
}

func TestPomoProgress(t *testing.T) {
	e, a := setupPomo(t)
	pomoSend(a, key(" "))
	e.advance(5 * time.Minute) // 25분 중 5분 = 20%
	c := testCtx(100, 40)
	c.Now = e.now
	if v := plain(a.View(c)); !strings.Contains(v, " 20%") {
		t.Errorf("20%% 표시가 없음:\n%s", v)
	}
	if v := pomoViewAt(pomoFresh(t), 100, 40); !strings.Contains(v, "  0%") {
		t.Errorf("대기 중 0%% 표시가 없음")
	}
}

func pomoFresh(t *testing.T) *pomodoroApp { _, a := setupPomo(t); return a }

func TestPomoRegistered(t *testing.T) {
	if pomodoroInfo.ID != "pomodoro" || pomodoroInfo.New == nil {
		t.Fatalf("등록 정보: %+v", pomodoroInfo)
	}
	a, b := pomodoroInfo.New(&Env{}).(*pomodoroApp), pomodoroInfo.New(&Env{}).(*pomodoroApp)
	if a.gen == b.gen {
		t.Error("앱마다 gen 이 달라야 함")
	}
}
