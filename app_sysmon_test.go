package main

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const gib = 1 << 30

// sysmonFake 는 한 번 읽을 때마다 다른 값을 돌려주는 가짜 읽기. 한 번 = CPU 항목을 읽은 때로 세고,
// 같은 회차의 나머지 항목(메모리·디스크·가동 시간)은 그 회차의 값에서 해당 필드만 돌려줌.
type sysmonFake struct {
	mu    sync.Mutex
	calls int
	cur   sysmonSample
	mod   func(n int, s *sysmonSample)
}

func (f *sysmonFake) build() sysmonSample {
	f.calls++
	s := sysmonSample{
		CPU: float64(10 * f.calls), CPUOK: true, Cores: 8,
		MemUsed: 10 * gib, MemTotal: 16 * gib, MemOK: true,
		Disks: []sysmonDisk{
			{Label: "C:", Used: 339 * gib, Total: 476 * gib},
			{Label: "D:", Used: 354 * gib, Total: 931 * gib},
		},
		DisksOK: true, Uptime: 17*3600 + 32*60, UptimeOK: true,
	}
	if f.mod != nil {
		f.mod(f.calls, &s)
	}
	return s
}

func (f *sysmonFake) read(item sysmonItem) sysmonSample {
	f.mu.Lock()
	defer f.mu.Unlock()
	if item == sysmonCPU {
		f.cur = f.build()
	}
	return sysmonPart(item, f.cur)
}

// sysmonPart 는 s 에서 item 에 해당하는 필드만 남김
func sysmonPart(item sysmonItem, s sysmonSample) sysmonSample {
	var o sysmonSample
	switch item {
	case sysmonCPU:
		o.CPU, o.CPUOK, o.Cores = s.CPU, s.CPUOK, s.Cores
	case sysmonMem:
		o.MemUsed, o.MemTotal, o.MemOK = s.MemUsed, s.MemTotal, s.MemOK
	case sysmonDrives:
		o.Disks, o.DisksOK = s.Disks, s.DisksOK
	case sysmonUp:
		o.Uptime, o.UptimeOK = s.Uptime, s.UptimeOK
	}
	return o
}

// sysmonReadAll 은 항목별 읽기를 차례로 돌려 한 덩어리로 합침
func sysmonReadAll(read func(sysmonItem) sysmonSample) sysmonSample {
	var s sysmonSample
	for i := range sysmonItemN {
		p := read(i)
		switch i {
		case sysmonCPU:
			s.CPU, s.CPUOK, s.Cores = p.CPU, p.CPUOK, p.Cores
		case sysmonMem:
			s.MemUsed, s.MemTotal, s.MemOK = p.MemUsed, p.MemTotal, p.MemOK
		case sysmonDrives:
			s.Disks, s.DisksOK = p.Disks, p.DisksOK
		case sysmonUp:
			s.Uptime, s.UptimeOK = p.Uptime, p.UptimeOK
		}
	}
	return s
}

func setupSysmon(t *testing.T, mod func(n int, s *sysmonSample)) (*sysmonFake, *sysmonApp) {
	t.Helper()
	f := &sysmonFake{mod: mod}
	old := sysmonRead
	sysmonRead = f.read
	t.Cleanup(func() { sysmonRead = old })
	return f, newSysmonApp(&Env{}).(*sysmonApp)
}

// run 은 명령이 돌려주는 메시지를 앱에 다시 먹임 (Batch 는 풀어서 샘플 메시지만 먹이고 틱은 건너뜀)
func sysmonRun(a *sysmonApp, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch m := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range m {
			sysmonRun(a, c)
		}
	case sysmonSampleMsg:
		a.Update(m)
	}
}

func sysmonTick(a *sysmonApp) appTickMsg { return appTickMsg{Gen: a.gen, Tag: a.tag()} }

func TestSysmonRenderStates(t *testing.T) {
	cases := map[string]func(n int, s *sysmonSample){
		"정상":      nil,
		"CPU 높음":  func(n int, s *sysmonSample) { s.CPU = 92 },
		"메모리 실패":  func(n int, s *sysmonSample) { s.MemOK = false },
		"디스크 실패":  func(n int, s *sysmonSample) { s.DisksOK = false },
		"전부 실패":   func(n int, s *sysmonSample) { *s = sysmonSample{} },
		"드라이브 많음": func(n int, s *sysmonSample) { s.Disks = sysmonManyDisks(25) },
		"디스크 높음":  func(n int, s *sysmonSample) { s.Disks = []sysmonDisk{{Label: "C:", Used: 95 * gib, Total: 100 * gib}} },
	}
	for name, mod := range cases {
		_, a := setupSysmon(t, mod)
		for _, paused := range []bool{false, true} {
			a.paused = paused
			sysmonRun(a, a.sample())
			for _, sz := range testSizes {
				c := testCtx(sz[0], sz[1])
				v := a.View(c)
				assertFrame(t, "sysmon/"+name, c, v)
				if h := len(strings.Split(v, "\n")); c.H >= 17 && h > c.H {
					t.Errorf("sysmon/%s %dx%d: 화면 %d줄이 창 높이를 넘음", name, c.W, c.H, h)
				}
			}
		}
	}
}

func sysmonManyDisks(n int) []sysmonDisk {
	var d []sysmonDisk
	for i := range n {
		d = append(d, sysmonDisk{Label: fmt.Sprintf("/mnt/disk%d", i), Used: gib, Total: 10 * gib})
	}
	return d
}

func TestSysmonScreenText(t *testing.T) {
	_, a := setupSysmon(t, nil)
	c := testCtx(80, 24)
	if v := plain(a.View(c)); !strings.Contains(v, "처음 값을 읽는 중") {
		t.Errorf("첫 결과 전 안내가 없음:\n%s", v)
	}
	a.apply((&sysmonFake{}).build())
	v := plain(a.View(c))
	for _, want := range []string{"CPU", "10%", "8 코어", "메모리", "63%", "10.0 / 16.0 GB", "C:", "339 / 476 GB !", "D:", "354 / 931 GB", "가동 시간", "17시간 32분",
		"[p] 일시정지", "[+][-] 갱신 주기 (1초)", "[Esc] 홈으로"} {
		if !strings.Contains(v, want) {
			t.Errorf("화면에 %q 없음:\n%s", want, v)
		}
	}
}

func TestSysmonRefreshesEachInterval(t *testing.T) {
	f, a := setupSysmon(t, nil)
	sysmonRun(a, a.Init())
	if f.calls != 1 || len(a.cpuH) != 1 {
		t.Fatalf("Init 에서 첫 읽기가 안 됨: calls=%d hist=%d", f.calls, len(a.cpuH))
	}
	for i := 2; i <= 5; i++ {
		_, cmd := a.Update(sysmonTick(a))
		sysmonRun(a, cmd)
		if f.calls != i || a.s.CPU != float64(10*i) {
			t.Fatalf("틱 %d: calls=%d cpu=%v", i, f.calls, a.s.CPU)
		}
	}
	if len(a.cpuH) != 5 || len(a.memH) != 5 {
		t.Errorf("추이 길이 cpu=%d mem=%d", len(a.cpuH), len(a.memH))
	}
}

func TestSysmonHistoryCappedAt60(t *testing.T) {
	_, a := setupSysmon(t, nil)
	for range 100 {
		a.apply((&sysmonFake{}).build())
	}
	if len(a.cpuH) != 60 || len(a.memH) != 60 {
		t.Errorf("추이는 최근 60회만 남아야 함: cpu=%d mem=%d", len(a.cpuH), len(a.memH))
	}
}

func TestSysmonBusySkipsTick(t *testing.T) {
	f, a := setupSysmon(t, nil)
	a.Init() // 읽기가 진행 중인 상태 (결과를 아직 안 받음)
	_, cmd := a.Update(sysmonTick(a))
	sysmonRun(a, cmd)
	if f.calls != 0 {
		t.Errorf("읽기가 진행 중인데 또 읽음: %d", f.calls)
	}
}

func TestSysmonPauseResume(t *testing.T) {
	f, a := setupSysmon(t, nil)
	sysmonRun(a, a.Init())
	_, cmd := a.Update(key("p"))
	if !a.paused || cmd != nil {
		t.Fatalf("p 로 일시정지가 안 됨: paused=%v", a.paused)
	}
	if v := plain(a.View(testCtx(80, 24))); !strings.Contains(v, "[p] 재개") || !strings.Contains(v, "일시정지됨") {
		t.Errorf("일시정지 화면 문구가 없음:\n%s", v)
	}
	// 일시정지 중에는 틱이 와도 읽지 않음
	before, calls := a.s, f.calls
	_, cmd = a.Update(appTickMsg{Gen: a.gen, Tag: "sysmon0"})
	sysmonRun(a, cmd)
	if f.calls != calls || a.s.CPU != before.CPU {
		t.Errorf("일시정지 중에 값이 바뀜")
	}
	// 일시정지 직전에 시작된 읽기가 늦게 도착해도 값이 고정돼야 함
	a.Update(sysmonSampleMsg{Gen: a.gen, Item: sysmonCPU, S: sysmonSample{CPU: 99, CPUOK: true}})
	if a.s.CPU != before.CPU {
		t.Errorf("일시정지 중 늦게 온 결과가 반영됨: cpu=%v", a.s.CPU)
	}
	// 재개하면 바로 읽고 틱이 다시 걸림
	_, cmd = a.Update(key("p"))
	if a.paused || cmd == nil {
		t.Fatalf("p 로 재개가 안 됨")
	}
	sysmonRun(a, cmd)
	if f.calls != calls+1 || a.s.CPU != float64(10*f.calls) {
		t.Errorf("재개 후 값이 갱신되지 않음: calls=%d", f.calls)
	}
}

func TestSysmonStaleTicksIgnored(t *testing.T) {
	f, a := setupSysmon(t, nil)
	oldTag := a.tag()
	a.Update(key("p"))
	a.Update(key("p")) // 사슬 번호가 바뀜
	for name, m := range map[string]appTickMsg{
		"옛 사슬":   {Gen: a.gen, Tag: oldTag},
		"다른 gen": {Gen: a.gen + 1, Tag: a.tag()},
	} {
		_, cmd := a.Update(m)
		if cmd != nil {
			t.Errorf("%s 틱에 반응함", name)
		}
	}
	if _, cmd := a.Update(sysmonSampleMsg{Gen: a.gen + 1, Item: sysmonCPU, S: f.build()}); cmd != nil || a.got[sysmonCPU] {
		t.Errorf("다른 gen 의 결과가 반영됨")
	}
}

func TestSysmonIntervalSteps(t *testing.T) {
	_, a := setupSysmon(t, nil)
	if sysmonIntervals[a.iv] != time.Second {
		t.Fatalf("기본 주기가 1초가 아님: %v", sysmonIntervals[a.iv])
	}
	want := []time.Duration{2 * time.Second, 5 * time.Second, 5 * time.Second}
	for _, w := range want { // 끝 단계(5초)에서 더 올려도 그대로
		a.Update(key("+"))
		if got := sysmonIntervals[a.iv]; got != w {
			t.Fatalf("+ 후 주기 %v, 기대 %v", got, w)
		}
	}
	for _, w := range []time.Duration{2 * time.Second, time.Second, 500 * time.Millisecond, 500 * time.Millisecond} {
		a.Update(key("-"))
		if got := sysmonIntervals[a.iv]; got != w {
			t.Fatalf("- 후 주기 %v, 기대 %v", got, w)
		}
	}
	if v := plain(a.View(testCtx(80, 24))); !strings.Contains(v, "(0.5초)") {
		t.Errorf("주기 표시가 없음:\n%s", v)
	}
	// 주기를 바꾸면 옛 틱은 무시됨
	old := a.tag()
	a.Update(key("+"))
	if _, cmd := a.Update(appTickMsg{Gen: a.gen, Tag: old}); cmd != nil {
		t.Errorf("주기 변경 전 틱에 반응함")
	}
}

func TestSysmonIntervalLabelFitsNarrow(t *testing.T) {
	_, a := setupSysmon(t, nil)
	a.iv = 0
	c := testCtx(56, 17)
	v := plain(a.View(c))
	if !strings.Contains(v, "0.5초") || !strings.Contains(v, "Esc") {
		t.Errorf("좁은 창에서 주기/Esc 안내가 빠짐:\n%s", v)
	}
	assertFrame(t, "sysmon narrow", c, a.View(c))
}

func TestSysmonReadFailureKeepsGoing(t *testing.T) {
	f, a := setupSysmon(t, func(n int, s *sysmonSample) {
		if n == 2 {
			s.MemOK = false
		}
	})
	sysmonRun(a, a.Init())
	_, cmd := a.Update(sysmonTick(a))
	sysmonRun(a, cmd)
	v := plain(a.View(testCtx(80, 24)))
	if !strings.Contains(v, "메모리 값을 읽지 못함") {
		t.Errorf("읽기 실패 안내가 없음:\n%s", v)
	}
	var memLine string
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, "메모리 ") && strings.Contains(l, "━") {
			memLine = l
		}
	}
	if !strings.Contains(memLine, "-") || strings.Contains(memLine, "%") && !strings.Contains(memLine, "   -") {
		t.Errorf("메모리 줄이 - 로 표시되지 않음: %q", memLine)
	}
	if !strings.Contains(v, "20%") { // CPU 는 계속 갱신됨
		t.Errorf("CPU 는 계속 갱신돼야 함:\n%s", v)
	}
	if len(a.memH) != 1 || len(a.cpuH) != 2 {
		t.Errorf("실패한 항목은 추이에 더하지 않아야 함: cpu=%d mem=%d", len(a.cpuH), len(a.memH))
	}
	// 다음 갱신에서 회복
	_, cmd = a.Update(sysmonTick(a))
	sysmonRun(a, cmd)
	if v := plain(a.View(testCtx(80, 24))); strings.Contains(v, "읽지 못함") || f.calls != 3 {
		t.Errorf("다음 갱신에서 회복되지 않음 (calls=%d):\n%s", f.calls, v)
	}
}

func TestSysmonAllFail(t *testing.T) {
	_, a := setupSysmon(t, func(n int, s *sysmonSample) { *s = sysmonSample{} })
	sysmonRun(a, a.Init())
	for _, sz := range testSizes {
		c := testCtx(sz[0], sz[1])
		v := plain(a.View(c))
		if !strings.Contains(v, "읽지 못함") || !strings.Contains(v, "-") {
			t.Errorf("%dx%d: 전부 실패했을 때 안내가 없음:\n%s", sz[0], sz[1], v)
		}
	}
}

func TestSysmonItemCmdSurvivesPanic(t *testing.T) {
	old := sysmonRead
	t.Cleanup(func() { sysmonRead = old })
	sysmonRead = func(sysmonItem) sysmonSample { panic("boom") }
	var running atomic.Bool
	running.Store(true)
	m := sysmonItemCmd(7, sysmonMem, &running)().(sysmonSampleMsg)
	if m.Gen != 7 || m.Item != sysmonMem || m.S.MemOK || running.Load() {
		t.Errorf("패닉이 그 항목만 실패로 바뀌지 않음: %+v running=%v", m, running.Load())
	}
}

// 항목 하나(디스크)의 읽기가 멈춰도 나머지는 계속 갱신되고, 멈춘 읽기가 고루틴으로 쌓이지 않아야 함
func TestSysmonHungItemDoesNotBlockOthers(t *testing.T) {
	f := &sysmonFake{}
	release := make(chan struct{})
	var once sync.Once
	var diskCalls atomic.Int32
	oldRead, oldT := sysmonRead, sysmonTimeout
	t.Cleanup(func() {
		once.Do(func() { close(release) })
		sysmonRead, sysmonTimeout = oldRead, oldT
	})
	sysmonTimeout = 30 * time.Millisecond
	sysmonRead = func(item sysmonItem) sysmonSample {
		if item == sysmonDrives {
			diskCalls.Add(1)
			<-release
			return sysmonSample{Disks: []sysmonDisk{{Label: "C:", Used: gib, Total: 10 * gib}}, DisksOK: true}
		}
		return f.read(item)
	}
	a := newSysmonApp(&Env{}).(*sysmonApp)
	base := runtime.NumGoroutine()
	const rounds = 10
	for range rounds {
		sysmonRun(a, a.sample())
	}
	if !a.s.CPUOK || !a.s.MemOK || !a.s.UptimeOK || a.s.DisksOK {
		t.Fatalf("멈춘 디스크만 실패여야 함: %+v", a.s)
	}
	if len(a.cpuH) != rounds || len(a.memH) != rounds || a.s.CPU != float64(10*rounds) {
		t.Errorf("나머지 항목이 계속 갱신되지 않음: cpuH=%d memH=%d cpu=%v", len(a.cpuH), len(a.memH), a.s.CPU)
	}
	if n := diskCalls.Load(); n != 1 {
		t.Errorf("멈춘 읽기가 끝나기 전에 디스크를 다시 읽음: %d회", n)
	}
	if extra := runtime.NumGoroutine() - base; extra > 2 {
		t.Errorf("멈춘 읽기 고루틴이 쌓임: +%d", extra)
	}
	if m, _ := a.message(72); !strings.Contains(m, "디스크 값을 읽지 못함") || strings.Contains(m, "CPU") || strings.Contains(m, "메모리") || strings.Contains(m, "가동") {
		t.Errorf("안내가 디스크만 가리켜야 함: %q", m)
	}
	c := testCtx(80, 24)
	v := plain(a.View(c))
	for _, want := range []string{"100%", "10.0 / 16.0 GB", "17시간 32분"} { // CPU(마지막 회차 100%)·메모리·가동 시간은 그대로 보임
		if !strings.Contains(v, want) {
			t.Errorf("화면에 %q 없음:\n%s", want, v)
		}
	}
	assertFrame(t, "sysmon/hung", c, a.View(c))

	// 멈췄던 읽기가 끝나면 다음 회차부터 디스크도 다시 읽힘
	once.Do(func() { close(release) })
	deadline := time.Now().Add(2 * time.Second)
	for a.running[sysmonDrives].Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	sysmonRun(a, a.sample())
	if !a.s.DisksOK || diskCalls.Load() != 2 {
		t.Errorf("멈춤이 풀린 뒤 디스크가 회복되지 않음: ok=%v calls=%d", a.s.DisksOK, diskCalls.Load())
	}
}

// 디스크 사용률 70% 이상은 막대 색 말고도 글자(값 칸의 "!" 와 안내 줄)로 보여야 함
func TestSysmonDiskWarnHasTextCue(t *testing.T) {
	_, a := setupSysmon(t, nil)
	sysmonRun(a, a.sample())
	for _, sz := range testSizes {
		c := testCtx(sz[0], sz[1])
		v := plain(a.View(c))
		var cLine, dLine string
		for _, l := range strings.Split(v, "\n") {
			switch {
			case strings.Contains(l, "C:") && strings.Contains(l, "━"):
				cLine = l
			case strings.Contains(l, "D:") && strings.Contains(l, "━"):
				dLine = l
			}
		}
		if !strings.Contains(cLine, "!") {
			t.Errorf("%dx%d: 사용률 71%% 드라이브에 글자 표시가 없음: %q\n%s", sz[0], sz[1], cLine, v)
		}
		if strings.Contains(dLine, "!") {
			t.Errorf("%dx%d: 사용률 38%% 드라이브에 경고 표시가 있음: %q", sz[0], sz[1], dLine)
		}
	}
	if m, tone := a.message(72); m != "디스크(C:) 사용률이 높음" || tone != ToneInfo {
		t.Errorf("디스크 부하 안내: %q %v", m, tone)
	}
	// 다른 항목이 실패해도 드라이브 줄의 표시는 남음
	a.applyItem(sysmonMem, sysmonSample{})
	if v := plain(a.View(testCtx(80, 24))); !strings.Contains(v, "339 / 476 GB !") {
		t.Errorf("다른 항목 실패 시 디스크 경고 표시가 사라짐:\n%s", v)
	}
	// 값이 아주 커도 표시가 잘리지 않음
	if got := sysmonDiskVal(sysmonDisk{Used: 12000 * gib, Total: 15000 * gib}); !strings.HasSuffix(got, " !") || lipgloss.Width(got) > meterValW {
		t.Errorf("큰 값에서 표시가 깨짐: %q", got)
	}
}

func TestSysmonWarnColor(t *testing.T) {
	cases := []struct {
		p    float64
		want lipgloss.Color
	}{{0, colorAccent}, {0.69, colorAccent}, {0.70, colorWarn}, {0.92, colorWarn}, {1, colorWarn}}
	for _, c := range cases {
		if got := sysmonColor(c.p); got != c.want {
			t.Errorf("sysmonColor(%v)=%v, want %v", c.p, got, c.want)
		}
	}
	if sysmonColor(0.95) == colorDanger {
		t.Error("높은 사용률에 빨강을 쓰면 안 됨")
	}
}

func TestSysmonMessages(t *testing.T) {
	_, a := setupSysmon(t, nil)
	a.apply(sysmonSample{CPU: 92, CPUOK: true, MemUsed: 8, MemTotal: 10, MemOK: true, DisksOK: true, UptimeOK: true})
	if m, tone := a.message(50); m != "CPU·메모리 사용률이 높음" || tone != ToneInfo {
		t.Errorf("부하 높음 안내: %q %v", m, tone)
	}
	a.apply(sysmonSample{CPU: 92, CPUOK: true, MemUsed: 5, MemTotal: 10, MemOK: false, DisksOK: true, UptimeOK: true})
	if m, tone := a.message(72); tone != ToneDanger || m != "메모리 값을 읽지 못함. 다음 갱신 때 다시 시도함" {
		t.Errorf("실패 안내: %q %v", m, tone)
	}
	a.apply(sysmonSample{})
	if m, _ := a.message(50); lipgloss.Width(m) > 46 {
		t.Errorf("안내가 박스 폭을 넘음: %q", m)
	}
	a.apply(sysmonSample{CPU: 10, CPUOK: true, MemUsed: 1, MemTotal: 10, MemOK: true, DisksOK: true, UptimeOK: true})
	if m, _ := a.message(50); m != "" {
		t.Errorf("정상일 때 안내가 있음: %q", m)
	}
	a.apply(sysmonSample{CPU: 92, CPUOK: true, MemUsed: 8, MemTotal: 10, MemOK: true, DisksOK: true, UptimeOK: true,
		Disks: []sysmonDisk{{Label: "C:", Used: 8, Total: 10}, {Label: "D:", Used: 1, Total: 10}, {Label: "E:", Used: 9, Total: 10}}})
	if m, _ := a.message(80); m != "CPU·메모리·디스크(C:,E:) 사용률이 높음" {
		t.Errorf("디스크 포함 부하 안내: %q", m)
	}
	// 일부 항목만 도착했을 때는 도착 안 한 항목을 실패로 보지 않음
	_, b := setupSysmon(t, nil)
	b.applyItem(sysmonMem, sysmonSample{MemUsed: 1, MemTotal: 10, MemOK: true})
	if m, tone := b.message(50); m != "처음 값을 읽는 중" || tone != ToneInfo {
		t.Errorf("일부만 도착: %q %v", m, tone)
	}
}

func TestSysmonUptimeFormat(t *testing.T) {
	cases := map[uint64]string{0: "0분", 59: "0분", 3600: "1시간 0분", 17*3600 + 32*60: "17시간 32분", 2*86400 + 3*3600 + 12*60: "2일 3시간 12분"}
	for in, want := range cases {
		if got := sysmonUptime(in); got != want {
			t.Errorf("sysmonUptime(%d)=%q, want %q", in, got, want)
		}
	}
}

func TestSysmonEscGoesHome(t *testing.T) {
	_, a := setupSysmon(t, nil)
	a.Update(key("p")) // 일시정지 중에도 Esc 는 바로 홈으로
	_, cmd := a.Update(key("esc"))
	if cmd == nil {
		t.Fatal("Esc 로 홈에 가지 않음")
	}
	if _, ok := cmd().(goHomeMsg); !ok {
		t.Error("Esc 의 결과가 goHomeMsg 가 아님")
	}
}

// 실제 PC 에서 읽어 봄. 값은 PC 마다 다르니 모양만 확인하고 값은 로그로 남김 (go test -v -run SysmonReal).
func TestSysmonReadReal(t *testing.T) {
	s := sysmonReadAll(sysmonReadReal)
	t.Logf("cpu ok=%v %.1f%% cores=%d | mem ok=%v %.1f/%.1f GB | disks ok=%v n=%d | uptime ok=%v %s",
		s.CPUOK, s.CPU, s.Cores, s.MemOK, sysmonGB(s.MemUsed), sysmonGB(s.MemTotal), s.DisksOK, len(s.Disks), s.UptimeOK, sysmonUptime(s.Uptime))
	for _, d := range s.Disks {
		t.Logf("disk %s %.0f/%.0f GB", d.Label, sysmonGB(d.Used), sysmonGB(d.Total))
	}
	if s.CPUOK && (s.CPU < 0 || s.CPU > 100.0001) {
		t.Errorf("CPU 값 범위 이상: %v", s.CPU)
	}
	if s.MemOK && s.MemUsed > s.MemTotal {
		t.Errorf("메모리 사용량이 전체보다 큼")
	}
	for _, d := range s.Disks {
		if d.Used > d.Total {
			t.Errorf("디스크 %s 사용량이 전체보다 큼", d.Label)
		}
	}
}
