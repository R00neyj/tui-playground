//go:build !windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 가짜 명령 실행기: 호출 기록을 남기고 미리 정한 결과를 돌려줌. 진짜 shutdown 은 절대 부르지 않음.
type fakeRun struct {
	calls   []string
	results map[string]fakeResult // "이름 인자..." 의 마지막 이름(basename) + 인자 -> 결과
}

type fakeResult struct {
	code int
	out  string
}

func (f *fakeRun) run(_ time.Duration, name string, args ...string) (int, string, bool) {
	key := strings.TrimSuffix(filepath.Base(name), ".exe") // Windows 에서 컴파일만 흉내 낼 때 붙는 확장자 제거
	if len(args) > 0 {
		key += " " + strings.Join(args, " ")
	}
	f.calls = append(f.calls, key)
	if r, ok := f.results[key]; ok {
		return r.code, r.out, false
	}
	return 0, "", false
}

// setupPlatform 은 OS, 권한, 파일 존재 여부를 가짜로 바꿔 끼움
func setupPlatform(t *testing.T, goos string, euid int, existing map[string]bool, results map[string]fakeResult) *fakeRun {
	t.Helper()
	f := &fakeRun{results: results}
	oOS, oEUID, oRun, oEx := platformOS, platformEUID, platformRun, platformExists
	platformOS, platformEUID, platformRun = goos, func() int { return euid }, f.run
	platformExists = func(p string) bool { return existing[p] }
	resetPlatformState()
	t.Cleanup(func() {
		platformOS, platformEUID, platformRun, platformExists = oOS, oEUID, oRun, oEx
		resetPlatformState()
	})
	return f
}

func resetPlatformState() {
	platformDetail, platformCancelWhy = "", ""
	platformCancelOKAt, platformCancelFailAt, platformCancelFail = time.Time{}, time.Time{}, 0
}

// setClock 은 platformNow 를 가짜 시계로 바꾸고 시계를 앞으로 보내는 함수를 돌려줌
func setClock(t *testing.T) func(time.Duration) {
	t.Helper()
	old := platformNow
	now := time.Unix(1_700_000_000, 0)
	platformNow = func() time.Time { return now }
	t.Cleanup(func() { platformNow = old })
	return func(d time.Duration) { now = now.Add(d) }
}

func TestPlatformLinuxScheduleArgs(t *testing.T) {
	f := setupPlatform(t, "linux", 1000, nil, nil)
	for sec, want := range map[int64]string{1800: "+30", 61: "+2", 60: "+1", 1: "+1", 5400: "+90"} {
		f.calls = nil
		if c := platformSchedule(sec); c != 0 {
			t.Fatalf("sec=%d code=%d", sec, c)
		}
		if len(f.calls) != 1 || !strings.HasSuffix(f.calls[0], "shutdown -P "+want) {
			t.Errorf("sec=%d calls=%v, want -P %s", sec, f.calls, want)
		}
	}
}

func TestPlatformDarwinScheduleArgs(t *testing.T) {
	f := setupPlatform(t, "darwin", 0, nil, map[string]fakeResult{"pgrep -x shutdown": {1, ""}})
	if c := platformSchedule(600); c != 0 {
		t.Fatalf("code=%d", c)
	}
	if got := f.calls[len(f.calls)-1]; got != "shutdown -h +10" {
		t.Errorf("calls=%v", f.calls)
	}
}

func TestPlatformLinuxAlreadyScheduled(t *testing.T) {
	f := setupPlatform(t, "linux", 0, map[string]bool{systemdScheduleFile: true}, nil)
	if c := platformSchedule(600); c != codeAlreadyScheduled {
		t.Fatalf("이미 예약됨을 알려야 함: %d", c)
	}
	if len(f.calls) != 0 {
		t.Errorf("알리는 호출에서는 shutdown 을 실행하면 안 됨: %v", f.calls)
	}
	// 호출한 쪽이 취소한 뒤 다시 부르면 그대로 실행해야 함 (무한히 1190 만 돌려주면 안 됨)
	if !platformCancel() {
		t.Fatal("취소 실패")
	}
	f.calls = nil
	if c := platformSchedule(600); c != 0 || len(f.calls) != 1 {
		t.Errorf("재시도는 실행돼야 함: code=%d calls=%v", c, f.calls)
	}
}

// 취소하지 않고 다시 불러도 계속 1190 이어야 함 (예전 구현은 두 번째 호출부터 중복 검사를 건너뜀)
func TestPlatformAlreadyScheduledIsNotSticky(t *testing.T) {
	f := setupPlatform(t, "linux", 0, map[string]bool{systemdScheduleFile: true}, nil)
	for i := 0; i < 3; i++ {
		if c := platformSchedule(600); c != codeAlreadyScheduled {
			t.Fatalf("%d번째 호출 code=%d", i+1, c)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("shutdown 이 실행됨: %v", f.calls)
	}
}

// 취소에 성공한 흐름은 시간이 지나면 더는 중복 검사를 건너뛰지 않음 (그 사이 외부에서 건 예약을 조용히 덮지 않기 위해)
func TestPlatformCancelSuccessWindowExpires(t *testing.T) {
	adv := setClock(t)
	f := setupPlatform(t, "linux", 0, map[string]bool{systemdRunDir: true, systemdScheduleFile: true}, nil)
	if !platformCancel() {
		t.Fatal("취소 실패")
	}
	f.calls = nil
	adv(cancelWindow + time.Second)
	if c := platformSchedule(600); c != codeAlreadyScheduled || len(f.calls) != 0 {
		t.Errorf("창이 지난 뒤에는 1190: code=%d calls=%v", c, f.calls)
	}
}

// 취소가 실패한 채 재시도하는 흐름 (setTimer: 1190 -> 취소 -> 재예약 -> 복구 재예약)
func TestPlatformCancelFailureRetryFlowDarwin(t *testing.T) {
	adv := setClock(t)
	f := setupPlatform(t, "darwin", 501, nil, map[string]fakeResult{
		"pgrep -x shutdown": {0, "99"}, "pkill -x shutdown": {1, "Operation not permitted"},
	})
	if c := platformSchedule(600); c != codeAlreadyScheduled {
		t.Fatalf("첫 호출 code=%d", c)
	}
	if platformCancel() {
		t.Fatal("권한 없는 취소가 true")
	}
	if platformCancelFailReason() == "" || !strings.Contains(platformCancelFailReason(), "권한") {
		t.Errorf("취소 실패 이유가 비어 있음: %q", platformCancelFailReason())
	}
	f.calls = nil
	for i, sec := range []int64{600, 120} { // 재예약, 복구 재예약
		if c := platformSchedule(sec); c == 0 || c == codeAlreadyScheduled {
			t.Errorf("%d번째 재시도는 실패 사유 코드여야 함: %d", i+1, c)
		} else if c != codePermissionDenied {
			t.Errorf("%d번째 재시도 code=%d, want %d", i+1, c, codePermissionDenied)
		}
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "shutdown") {
			t.Errorf("취소에 실패한 상태에서 shutdown 을 실행함: %v", f.calls)
		}
	}
	// 시간이 지나면 다시 평범한 중복 알림(1190)으로 돌아가고 shutdown 은 여전히 실행하지 않음
	adv(cancelWindow + time.Second)
	if c := platformSchedule(600); c != codeAlreadyScheduled {
		t.Errorf("창이 지난 뒤 code=%d", c)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "shutdown") {
			t.Errorf("shutdown 이 실행됨: %v", f.calls)
		}
	}
	// 다음 취소 호출이 이전 실패 이유를 지움 (대기 중인 것이 사라진 경우)
	platformRun = (&fakeRun{results: map[string]fakeResult{"pgrep -x shutdown": {1, ""}}}).run
	if platformCancel() || platformCancelFailReason() != "" {
		t.Errorf("예약이 없어서 false 일 때는 실패 이유가 없어야 함: %q", platformCancelFailReason())
	}
}

func TestPlatformCancelFailureRetryFlowLinux(t *testing.T) {
	setClock(t)
	f := setupPlatform(t, "linux", 1000, map[string]bool{systemdRunDir: true, systemdScheduleFile: true}, map[string]fakeResult{
		"shutdown -c": {1, "Failed to talk to logind: Access denied"},
	})
	if c := platformSchedule(600); c != codeAlreadyScheduled {
		t.Fatalf("첫 호출 code=%d", c)
	}
	if platformCancel() {
		t.Fatal("취소 실패인데 true")
	}
	if !strings.Contains(platformCancelFailReason(), "권한") {
		t.Errorf("이유=%q", platformCancelFailReason())
	}
	f.calls = nil
	if c := platformSchedule(600); c != codePermissionDenied {
		t.Errorf("code=%d", c)
	}
	if len(f.calls) != 0 {
		t.Errorf("취소 실패 직후에는 shutdown 을 실행하지 않아야 함: %v", f.calls)
	}
}

// 예약이 없어서 취소할 게 없는 경우는 "취소 실패" 가 아님
func TestPlatformCancelNothingPendingHasNoFailReason(t *testing.T) {
	setupPlatform(t, "linux", 1000, map[string]bool{systemdRunDir: true},
		map[string]fakeResult{"shutdown -c": {1, "Failed to talk to logind: Access denied"}})
	if platformCancel() {
		t.Error("true")
	}
	if platformCancelFailReason() != "" {
		t.Errorf("이유=%q", platformCancelFailReason())
	}
}

func TestPlatformDarwinAlreadyScheduled(t *testing.T) {
	f := setupPlatform(t, "darwin", 0, nil, map[string]fakeResult{"pgrep -x shutdown": {0, "1234"}})
	if c := platformSchedule(600); c != codeAlreadyScheduled {
		t.Fatalf("code=%d calls=%v", c, f.calls)
	}
}

func TestPlatformPermissionDenied(t *testing.T) {
	cases := []struct {
		goos string
		euid int
		out  string
	}{
		{"linux", 1000, "Failed to schedule shutdown: Interactive authentication required."},
		{"linux", 1000, "Failed to schedule shutdown: Access denied"},
		{"darwin", 501, "shutdown: NOT super-user"},
		{"darwin", 501, ""}, // 출력이 없어도 root 가 아니면 권한 문제로 봄
	}
	for _, c := range cases {
		setupPlatform(t, c.goos, c.euid, nil, map[string]fakeResult{
			"shutdown -P +10": {1, c.out}, "shutdown -h +10": {1, c.out}, "pgrep -x shutdown": {1, ""},
		})
		if code := platformSchedule(600); code != codePermissionDenied {
			t.Errorf("%s %q: code=%d", c.goos, c.out, code)
		}
		if r := platformFailReason(codePermissionDenied); !strings.Contains(r, "권한") || strings.Contains(r, "\n") {
			t.Errorf("이유 문구가 한 줄이 아니거나 권한 안내가 없음: %q", r)
		}
	}
}

func TestPlatformOtherFailureKeepsCode(t *testing.T) {
	setupPlatform(t, "linux", 0, nil, map[string]fakeResult{"shutdown -P +1": {2, "shutdown: something odd\nsecond line"}})
	if code := platformSchedule(30); code != 2 {
		t.Fatalf("code=%d", code)
	}
	if r := platformFailReason(2); r != "shutdown: something odd" {
		t.Errorf("reason=%q", r)
	}
}

func TestPlatformNotRunnable(t *testing.T) {
	setupPlatform(t, "linux", 0, nil, map[string]fakeResult{"shutdown -P +1": {codeNotRunnable, "exec: not found"}})
	if code := platformSchedule(30); code != codeNotRunnable {
		t.Fatalf("code=%d", code)
	}
	if platformFailReason(codeNotRunnable) == "" {
		t.Error("이유가 비어 있음")
	}
}

func TestPlatformLinuxCancel(t *testing.T) {
	// systemd: 예약이 있었으면 true, 없었으면 -c 가 0 이어도 false
	f := setupPlatform(t, "linux", 0, map[string]bool{systemdRunDir: true, systemdScheduleFile: true}, nil)
	if !platformCancel() || f.calls[0] != "shutdown -c" {
		t.Errorf("예약이 있을 때 취소: calls=%v", f.calls)
	}
	setupPlatform(t, "linux", 0, map[string]bool{systemdRunDir: true}, nil)
	if platformCancel() {
		t.Error("예약이 없었는데 true")
	}
	// 취소 실패(권한 등)는 false
	setupPlatform(t, "linux", 1000, map[string]bool{systemdRunDir: true, systemdScheduleFile: true},
		map[string]fakeResult{"shutdown -c": {1, "Failed to talk to logind: Access denied"}})
	if platformCancel() {
		t.Error("취소 실패인데 true")
	}
}

func TestPlatformDarwinCancel(t *testing.T) {
	f := setupPlatform(t, "darwin", 0, nil, map[string]fakeResult{"pgrep -x shutdown": {0, "99"}})
	if !platformCancel() || f.calls[len(f.calls)-1] != "pkill -x shutdown" {
		t.Errorf("calls=%v", f.calls)
	}
	f = setupPlatform(t, "darwin", 0, nil, map[string]fakeResult{"pgrep -x shutdown": {1, ""}})
	if platformCancel() || len(f.calls) != 1 {
		t.Errorf("대기 중인 shutdown 이 없으면 false 이고 종료 시도도 없어야 함: %v", f.calls)
	}
	setupPlatform(t, "darwin", 501, nil, map[string]fakeResult{"pgrep -x shutdown": {0, "99"}, "pkill -x shutdown": {1, "Operation not permitted"}})
	if platformCancel() {
		t.Error("권한 없는 취소가 true")
	}
}

// ---- 부팅 시각 ----

func TestPlatformBootFallback(t *testing.T) {
	oFn, oRead, oNow := platformBootFn, platformReadFile, platformNow
	t.Cleanup(func() { platformBootFn, platformReadFile, platformNow = oFn, oRead, oNow })
	platformBootFn = func() (uint64, error) { return 0, errors.New("fail") }
	now := time.Unix(1_700_000_000, 400_000_000)
	platformNow = func() time.Time { return now }
	platformReadFile = func(string) ([]byte, error) { return []byte("1000.40 5000.00\n"), nil }
	setupPlatform(t, "linux", 0, nil, nil)
	if got := platformBootUnix(); got != 1_700_000_000-1000 {
		t.Errorf("/proc/uptime 대체값=%d", got)
	}
	// 같은 부팅이면 호출 시각이 달라도 2초 안에서 같아야 함
	now = now.Add(1500 * time.Millisecond)
	platformReadFile = func(string) ([]byte, error) { return []byte("1001.90 5000.00\n"), nil }
	if d := platformBootUnix() - (1_700_000_000 - 1000); d > 1 || d < -1 {
		t.Errorf("대체값이 흔들림: %d", d)
	}
}

func TestPlatformBootUnknownIsStable(t *testing.T) {
	oFn, oRead := platformBootFn, platformReadFile
	t.Cleanup(func() { platformBootFn, platformReadFile = oFn, oRead })
	platformBootFn = func() (uint64, error) { return 0, errors.New("fail") }
	platformReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }
	setupPlatform(t, "linux", 0, nil, nil)
	a := platformBootUnix()
	time.Sleep(1100 * time.Millisecond)
	if b := platformBootUnix(); a != b {
		t.Errorf("구하지 못했을 때 값이 호출마다 달라짐: %d vs %d", a, b)
	}
}

func TestPlatformBootDarwinSysctl(t *testing.T) {
	oFn := platformBootFn
	t.Cleanup(func() { platformBootFn = oFn })
	platformBootFn = func() (uint64, error) { return 0, errors.New("fail") }
	setupPlatform(t, "darwin", 0, nil, map[string]fakeResult{"sysctl -n kern.boottime": {0, "{ sec = 1700000123, usec = 5 } Tue Nov 14 22:15:23 2023"}})
	if got := platformBootUnix(); got != 1700000123 {
		t.Errorf("sysctl 대체값=%d", got)
	}
}

// ---- 실제 프로세스 실행 (테스트 바이너리 자신을 자식으로 씀) ----

func TestExecCaptureReal(t *testing.T) {
	if os.Getenv("TUI_HELPER") != "" { // 자식으로 실행된 경우
		return
	}
	self, _ := os.Executable()
	run := func(mode string, wait time.Duration) (int, string, bool) {
		t.Setenv("TUI_HELPER", mode)
		return execCapture(wait, self, "-test.run=TestHelperProcess")
	}
	if c, out, to := run("ok", 5*time.Second); c != 0 || to || !strings.Contains(out, "hello") {
		t.Errorf("정상 종료: %d %q %v", c, out, to)
	}
	if c, out, _ := run("fail", 5*time.Second); c != 3 || !strings.Contains(out, "boom") {
		t.Errorf("실패 종료: %d %q", c, out)
	}
	start := time.Now()
	if c, _, to := run("slow", 300*time.Millisecond); c != 0 || !to || time.Since(start) > 3*time.Second {
		t.Errorf("오래 걸리는 명령은 시간 초과 성공으로 돌아와야 함: %d %v %v", c, to, time.Since(start))
	}
	if c, _, _ := execCapture(time.Second, "/no/such/binary-xyz"); c != codeNotRunnable {
		t.Errorf("없는 명령: %d", c)
	}
}

func TestHelperProcess(t *testing.T) {
	switch os.Getenv("TUI_HELPER") {
	case "ok":
		os.Stdout.WriteString("hello\n")
		os.Exit(0)
	case "fail":
		os.Stderr.WriteString("boom\n")
		os.Exit(3)
	case "slow":
		time.Sleep(2 * time.Second)
		os.Exit(0)
	}
}

// ---- 상태 파일 경로 ----

func TestStatePathFallsBackToUserCacheDir(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")
	cache := t.TempDir()
	old := stateUserCacheDir
	stateUserCacheDir = func() (string, error) { return cache, nil }
	t.Cleanup(func() { stateUserCacheDir = old })
	want := filepath.Join(cache, "TUI-playground", "shutdown_timer_go.state")
	if got := statePath(); got != want {
		t.Fatalf("statePath=%q, want %q", got, want)
	}
	// 폴더가 만들어져 있어서 상태 파일을 바로 쓸 수 있어야 함
	if err := writeState(&Timer{Start: time.Now(), Target: time.Now().Add(time.Hour)}, 5); err != nil {
		t.Fatalf("writeState: %v", err)
	}
	if loadTimer(5) == nil {
		t.Error("저장한 상태를 다시 읽지 못함")
	}
}

func TestStatePathPrefersLocalAppData(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	if got := statePath(); got != filepath.Join(dir, "shutdown_timer_go.state") {
		t.Errorf("statePath=%q", got)
	}
}

func TestStatePathNoCacheDirUsesTemp(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")
	old := stateUserCacheDir
	stateUserCacheDir = func() (string, error) { return "", exec.ErrNotFound }
	t.Cleanup(func() { stateUserCacheDir = old })
	if got := statePath(); filepath.Dir(got) != os.TempDir() {
		t.Errorf("statePath=%q", got)
	}
}
