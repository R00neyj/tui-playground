package main

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// 가짜 시계: 테스트가 직접 시간을 흘려보냄
type swClock struct{ now time.Time }

func (c *swClock) advance(d time.Duration) { c.now = c.now.Add(d) }

func setupStopwatch(t *testing.T) (*swClock, *stopwatchApp) {
	t.Helper()
	clk := &swClock{now: time.Date(2026, 11, 5, 14, 0, 0, 0, time.Local)}
	old := swNow
	swNow = func() time.Time { return clk.now }
	t.Cleanup(func() { swNow = old })
	return clk, newStopwatchApp(&Env{}).(*stopwatchApp)
}

// press 는 키를 보내고 앱과 결과 명령을 돌려줌
func swPress(a *stopwatchApp, k string) tea.Cmd {
	m, cmd := a.Update(key(k))
	if m != App(a) {
		panic("앱 인스턴스가 바뀜")
	}
	return cmd
}

func isGoHome(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(goHomeMsg)
	return ok
}

func swView(a *stopwatchApp, w, h int) string { return plain(a.View(testCtx(w, h))) }

func TestFmtSW(t *testing.T) {
	cases := map[time.Duration]string{
		0:                                     "00:00.0",
		99 * time.Millisecond:                 "00:00.0",
		100 * time.Millisecond:                "00:00.1",
		59*time.Second + 999*time.Millisecond: "00:59.9",
		time.Minute:                           "01:00.0",
		83*time.Second + 400*time.Millisecond: "01:23.4",
		100*time.Minute + 5*time.Second:       "100:05.0",
		-time.Second:                          "00:00.0",
	}
	for d, want := range cases {
		if got := fmtSW(d); got != want {
			t.Errorf("fmtSW(%v)=%q, want %q", d, got, want)
		}
	}
}

func TestStopwatchStartStopResume(t *testing.T) {
	clk, a := setupStopwatch(t)
	if cmd := swPress(a, " "); cmd == nil || a.ph != swRunning {
		t.Fatalf("Space 로 시작되지 않음: ph=%v cmd=%v", a.ph, cmd)
	}
	clk.advance(1234 * time.Millisecond)
	if got := fmtSW(a.elapsed()); got != "00:01.2" {
		t.Errorf("측정 중 경과=%s", got)
	}
	swPress(a, " ")
	if a.ph != swStopped {
		t.Fatalf("Space 로 정지되지 않음")
	}
	clk.advance(10 * time.Second) // 정지 중에는 시간이 흐르지 않음
	if got := fmtSW(a.elapsed()); got != "00:01.2" {
		t.Errorf("정지 중 경과가 변함=%s", got)
	}
	swPress(a, " ") // 이어서 측정
	clk.advance(2 * time.Second)
	if got := fmtSW(a.elapsed()); got != "00:03.2" {
		t.Errorf("다시 시작 후 경과=%s", got)
	}
}

// 틱이 밀려도 경과는 시작 시각 기준이라 어긋나지 않음
func TestStopwatchNoDrift(t *testing.T) {
	clk, a := setupStopwatch(t)
	swPress(a, " ")
	for i := 0; i < 5; i++ { // 틱을 하나도 안 보내고 시간만 흘림
		clk.advance(7 * time.Second)
	}
	if got := fmtSW(a.elapsed()); got != "00:35.0" {
		t.Errorf("경과=%s, want 00:35.0", got)
	}
}

func TestStopwatchLaps(t *testing.T) {
	clk, a := setupStopwatch(t)
	swPress(a, " ")
	clk.advance(31500 * time.Millisecond)
	swPress(a, "l")
	clk.advance(30200 * time.Millisecond)
	swPress(a, "l")
	clk.advance(21700 * time.Millisecond)
	swPress(a, "l")
	want := []swLap{
		{31500 * time.Millisecond, 31500 * time.Millisecond},
		{30200 * time.Millisecond, 61700 * time.Millisecond},
		{21700 * time.Millisecond, 83400 * time.Millisecond},
	}
	if len(a.laps) != 3 {
		t.Fatalf("랩 개수=%d", len(a.laps))
	}
	for i, w := range want {
		if a.laps[i] != w {
			t.Errorf("랩 %d = %+v, want %+v", i+1, a.laps[i], w)
		}
	}
	v := swView(a, 80, 24)
	for _, s := range []string{"랩 3   00:21.7   누적 01:23.4", "랩 2   00:30.2   누적 01:01.7", "랩 1   00:31.5   누적 00:31.5"} {
		if !strings.Contains(v, s) {
			t.Errorf("랩 줄이 없음: %q\n%s", s, v)
		}
	}
	// 네 번째 랩이 생기면 가장 오래된 랩 1 이 화면에서 빠짐 (기록은 남음)
	clk.advance(10 * time.Second)
	swPress(a, "l")
	v = swView(a, 80, 24)
	if strings.Contains(v, "랩 1 ") || !strings.Contains(v, "랩 4   00:10.0   누적 01:33.4") {
		t.Errorf("최근 3개만 보여야 함:\n%s", v)
	}
	if len(a.laps) != 4 {
		t.Errorf("기록된 랩=%d", len(a.laps))
	}
}

func TestStopwatchLapIgnoredWhenNotRunning(t *testing.T) {
	clk, a := setupStopwatch(t)
	swPress(a, "l") // 대기 중
	if len(a.laps) != 0 {
		t.Fatalf("대기 중 l 이 기록됨")
	}
	swPress(a, " ")
	clk.advance(time.Second)
	swPress(a, " ")
	swPress(a, "l") // 정지 중
	if len(a.laps) != 0 {
		t.Fatalf("정지 중 l 이 기록됨")
	}
}

func TestStopwatchReset(t *testing.T) {
	clk, a := setupStopwatch(t)
	swPress(a, " ")
	clk.advance(5 * time.Second)
	swPress(a, "l")
	swPress(a, "r") // 측정 중 리셋
	if a.ph != swIdle || a.elapsed() != 0 || len(a.laps) != 0 {
		t.Fatalf("리셋되지 않음: %+v", a)
	}
	clk.advance(time.Minute)
	if a.elapsed() != 0 {
		t.Fatalf("리셋 후 시간이 흐름")
	}
	swPress(a, " ")
	clk.advance(2 * time.Second)
	swPress(a, " ")
	swPress(a, "r") // 정지 중 리셋
	if a.ph != swIdle || a.elapsed() != 0 {
		t.Fatalf("정지 중 리셋되지 않음")
	}
	if !isGoHome(swPress(a, "esc")) { // 리셋 후에는 기록이 없으니 바로 홈
		t.Errorf("리셋 후 Esc 가 바로 홈으로 가지 않음")
	}
}

func TestStopwatchEscRules(t *testing.T) {
	clk, a := setupStopwatch(t)
	// 대기 중: 바로 홈
	if !isGoHome(swPress(a, "esc")) {
		t.Fatalf("대기 중 Esc 는 바로 홈")
	}
	// 측정 중: 확인
	swPress(a, " ")
	clk.advance(3 * time.Second)
	if cmd := swPress(a, "esc"); cmd != nil || !a.confirm {
		t.Fatalf("측정 중 Esc 는 확인을 띄워야 함")
	}
	if !strings.Contains(swView(a, 100, 30), "측정 중인 시간이 사라집니다. 홈으로 나갈까요?   [y] 나감   [n] 계속") {
		t.Errorf("확인 문구가 없음:\n%s", swView(a, 100, 30))
	}
	// 확인 중에는 시간이 계속 흐르고 다른 키는 무시됨
	swPress(a, "r")
	swPress(a, "l")
	swPress(a, " ")
	if a.ph != swRunning || len(a.laps) != 0 || a.base != 0 {
		t.Fatalf("확인 중 다른 키가 동작함")
	}
	// n: 계속
	if cmd := swPress(a, "n"); cmd != nil || a.confirm || a.ph != swRunning {
		t.Fatalf("n 으로 계속되지 않음")
	}
	// Esc 두 번째는 확인 취소
	swPress(a, "esc")
	swPress(a, "esc")
	if a.confirm {
		t.Fatalf("확인 중 Esc 가 확인을 닫지 않음")
	}
	// y: 홈
	swPress(a, "esc")
	if !isGoHome(swPress(a, "y")) {
		t.Fatalf("y 로 홈에 가지 않음")
	}
	// 정지 + 기록 있음: 확인
	swPress(a, "n") // (y 후에도 앱 상태는 남아 있으므로 확인을 닫고 이어감)
	swPress(a, " ")
	if a.ph != swStopped {
		t.Fatalf("정지되지 않음")
	}
	if cmd := swPress(a, "esc"); cmd != nil || !a.confirm {
		t.Fatalf("정지 상태에 기록이 있으면 Esc 는 확인")
	}
	if !strings.Contains(swView(a, 100, 30), "측정 기록이 사라집니다") {
		t.Errorf("정지 상태 확인 문구가 없음")
	}
}

func TestStopwatchTicks(t *testing.T) {
	_, a := setupStopwatch(t)
	cmd := swPress(a, " ")
	if cmd == nil {
		t.Fatal("시작할 때 틱이 걸리지 않음")
	}
	tick := appTickMsg{Gen: a.gen, Tag: a.tag()}
	if _, c := a.Update(tick); c == nil {
		t.Errorf("측정 중 자기 틱에 다음 틱이 걸리지 않음")
	}
	if _, c := a.Update(appTickMsg{Gen: a.gen + 1000, Tag: a.tag()}); c != nil {
		t.Errorf("다른 gen 의 틱을 처리함")
	}
	// 정지 후 바로 다시 시작: 옛 틱 체인은 끊기고 새 체인만 이어짐
	swPress(a, " ")
	if _, c := a.Update(tick); c != nil {
		t.Errorf("정지 중 틱을 처리함")
	}
	swPress(a, " ")
	if _, c := a.Update(tick); c != nil {
		t.Errorf("다시 시작한 뒤 옛 틱이 이어짐 (틱 이중 발생)")
	}
	if _, c := a.Update(appTickMsg{Gen: a.gen, Tag: a.tag()}); c == nil {
		t.Errorf("새 틱이 이어지지 않음")
	}
	// 앱을 새로 만들면 gen 이 달라 옛 앱의 틱이 무시됨
	b := newStopwatchApp(&Env{}).(*stopwatchApp)
	if b.gen == a.gen {
		t.Errorf("gen 이 같음")
	}
}

func TestStopwatchSpaceKeyName(t *testing.T) {
	_, a := setupStopwatch(t)
	// 실제 터미널의 스페이스 키는 KeySpace 로 들어옴
	a.Update(tea.KeyMsg{Type: tea.KeySpace})
	if a.ph != swRunning {
		t.Fatalf("KeySpace 로 시작되지 않음")
	}
}

func TestStopwatchViewAligned(t *testing.T) {
	clk, a := setupStopwatch(t)
	states := []struct {
		name string
		do   func()
	}{
		{"대기", func() {}},
		{"측정 중", func() { swPress(a, " ") }},
		{"랩 1개", func() { clk.advance(75 * time.Second); swPress(a, "l") }},
		{"랩 3개 이상", func() {
			for i := 0; i < 4; i++ {
				clk.advance(61*time.Second + 300*time.Millisecond)
				swPress(a, "l")
			}
		}},
		{"나가기 확인", func() { swPress(a, "esc") }},
		{"확인 닫음", func() { swPress(a, "n") }},
		{"정지", func() { swPress(a, " ") }},
		{"정지 확인", func() { swPress(a, "esc") }},
		{"긴 시간", func() { swPress(a, "n"); a.base = 1234*time.Minute + 5*time.Second }},
		{"리셋", func() { swPress(a, "r") }},
	}
	for _, st := range states {
		st.do()
		for _, sz := range testSizes {
			c := testCtx(sz[0], sz[1])
			v := a.View(c)
			assertFrame(t, "stopwatch/"+st.name, c, v)
			if h := strings.Count(v, "\n") + 1; h > c.H {
				t.Errorf("stopwatch/%s %dx%d: 높이 %d 가 창 높이를 넘음", st.name, sz[0], sz[1], h)
			}
		}
	}
}

// 와이어프레임 80x24 에서 큰 숫자와 모든 문구가 보임
func TestStopwatchWireframeCopy(t *testing.T) {
	_, a := setupStopwatch(t)
	v := swView(a, 80, 24)
	for _, s := range []string{"대기", "██", "랩 기록 없음", "[Space]  시작", "[l]  랩    측정 중에만 기록됨", "[r]  리셋", "[Esc]  홈으로"} {
		if !strings.Contains(v, s) {
			t.Errorf("대기 화면에 %q 없음:\n%s", s, v)
		}
	}
	swPress(a, " ")
	v = swView(a, 80, 24)
	for _, s := range []string{"측정 중", "[Space]  정지", "[l]  랩", "[Esc]  홈으로    측정 중이면 확인함"} {
		if !strings.Contains(v, s) {
			t.Errorf("측정 화면에 %q 없음:\n%s", s, v)
		}
	}
	swPress(a, " ")
	v = swView(a, 80, 24)
	if !strings.Contains(v, "정지") || !strings.Contains(v, "측정 기록이 있으면 확인함") {
		t.Errorf("정지 화면 문구 없음:\n%s", v)
	}
	// 작은 창에서는 한 줄 숫자
	if v = swView(a, 56, 17); !strings.Contains(v, "경과시간  00:00.0") {
		t.Errorf("한 줄 숫자가 없음:\n%s", v)
	}
}
