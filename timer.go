package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Timer 는 종료 타이머 앱이 예약한 종료 정보
type Timer struct {
	Start, Target time.Time
	NoFile        bool // 상태 파일 저장에 실패한 경우 (파일 동기화 생략)
}

const isoLayout = "2006-01-02T15:04:05.0000000-07:00"

// 이미 종료가 예약돼 있을 때 shutdown 이 돌려주는 코드 (Windows ERROR_SHUTDOWN_IS_SCHEDULED).
// 다른 OS 구현도 같은 상황이면 이 값을 돌려줌.
const codeAlreadyScheduled = 1190

// 플랫폼 구현(platform_*.go)을 테스트에서 바꿔 끼울 수 있게 변수로 둠
var (
	shutdownSchedule = platformSchedule
	shutdownCancel   = platformCancel
)

func bootUnix() int64 { return platformBootUnix() }

func statePath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "shutdown_timer_go.state")
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
	if tot > 5256000 { // shutdown /t 최대값(10년)
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
