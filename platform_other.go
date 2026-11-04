//go:build !windows

package main

// macOS / Linux 용 종료 예약 구현 (F-06).
// 이 코드는 Windows PC 에서 작성했고 실제 macOS / Linux 에서는 실행해 보지 못함 (교차 컴파일과 가짜 실행기 테스트만 함).
// 근거 (확인한 것 / 확인 못 한 것은 보고서 참고):
//   - Linux(systemd): shutdown -P +분 은 logind 에 예약을 맡기고 바로 끝남. shutdown -c 가 취소 (man shutdown: "Cancel a pending shutdown").
//     logind 의 ScheduleShutdown / CancelScheduledShutdown 은 즉시 끄기와 같은 polkit 권한을 요구함 -> 일반 사용자는 거부될 수 있음.
//     예약이 이미 있을 때 덮어쓰는지는 문서에 없음 (그래서 먼저 예약 여부를 보고 1190 으로 알림).
//     예약 정보가 /run/systemd/shutdown/scheduled 파일에 있다는 것은 문서가 아니라 알려진 동작을 따른 것이라 확인 못 함.
//     https://www.freedesktop.org/software/systemd/man/latest/shutdown.html
//     https://www.freedesktop.org/software/systemd/man/latest/org.freedesktop.login1.html
//   - macOS: shutdown -h +분 은 관리자(root)용 (ss64 의 예시는 모두 sudo). 취소 옵션이 없고 "shutdown 프로세스를 종료(SIGTERM)" 하면 취소됨.
//     이미 예약이 있을 때의 동작은 문서에 없음 (프로세스가 따로 하나 더 생길 수 있어서 pgrep 으로 먼저 확인함).
//     https://ss64.com/mac/shutdown.html

import (
	"math"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/host"
)

// 권한이 없어 거부됨 (EACCES 값). 화면에는 platformFailReason 으로 이유를 풀어 씀.
const codePermissionDenied = 13

// shutdown 실행 파일을 못 찾았거나 실행 자체가 안 됨
const codeNotRunnable = -1

const (
	systemdScheduleFile = "/run/systemd/shutdown/scheduled" // systemd 가 예약을 적어두는 파일
	systemdRunDir       = "/run/systemd/system"             // systemd 로 부팅했을 때만 있는 폴더
	shutdownWait        = 3 * time.Second                   // 이 시간 안에 안 끝나는 shutdown 은 앞에서 대기 중인 것으로 보고 성공 처리
	cancelWindow        = 5 * time.Second                   // 취소 직후 "취소하고 다시 예약" 흐름으로 보는 시간 (타이머 앱의 취소 -> 재예약 호출 간격)
)

// 테스트에서 바꿔 끼울 수 있게 변수로 둠
var (
	platformOS       = runtime.GOOS
	platformEUID     = os.Geteuid
	platformRun      = execCapture
	platformExists   = func(p string) bool { _, err := os.Stat(p); return err == nil }
	platformReadFile = os.ReadFile
	platformBootFn   = host.BootTime
	platformNow      = time.Now
)

var (
	platformDetail string // 마지막 실패의 첫 줄 (platformFailReason 이 씀)

	// 취소 -> 재예약 흐름 판별용. 한 번 켜지면 계속 남는 플래그 대신 시각만 적고 cancelWindow 안에서만 믿음.
	platformCancelOKAt   time.Time // 마지막으로 취소에 성공한 시각
	platformCancelFailAt time.Time // 예약이 남아 있는데 취소에 실패한 시각
	platformCancelFail   int       // 그때 platformSchedule 이 돌려줄 코드
	platformCancelWhy    string    // 가장 최근 platformCancel 이 예약이 남은 채 실패했을 때의 이유. 아니면 ""
)

// ---- 명령 실행 ----

// execCapture 는 명령을 실행하고 (종료 코드, 출력, 시간 초과 여부) 를 돌려줌.
// 출력은 파이프가 아니라 임시 파일로 받음: BSD shutdown 은 예약 후 자식 프로세스가 남아 파이프를 계속 잡고 있어서
// 파이프로 받으면 예약 시각까지 안 끝날 수 있음. 시간 안에 안 끝나면 (sysvinit 등은 앞에서 기다림) 계속 돌게 두고 성공으로 봄.
func execCapture(wait time.Duration, name string, args ...string) (int, string, bool) {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C") // 오류 문구 판별을 위해 영어로 고정
	var out *os.File
	if f, err := os.CreateTemp("", "tui-playground-*.out"); err == nil {
		out = f
		cmd.Stdout, cmd.Stderr = f, f
		defer os.Remove(f.Name())
		defer f.Close()
	}
	if err := cmd.Start(); err != nil {
		return codeNotRunnable, err.Error(), false
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var werr error
	select {
	case werr = <-done:
	case <-time.After(wait):
		return 0, "", true
	}
	text := ""
	if out != nil {
		if b, err := os.ReadFile(out.Name()); err == nil {
			text = strings.TrimSpace(string(b))
		}
	}
	if werr == nil {
		return 0, text, false
	}
	if ee, ok := werr.(*exec.ExitError); ok && ee.ExitCode() > 0 {
		return ee.ExitCode(), text, false
	}
	return codeNotRunnable, text, false
}

// findBin 은 PATH 에서 찾고, 없으면 흔한 위치에서 찾음 (일반 사용자 PATH 에는 /usr/sbin 이 없는 배포판이 많음)
func findBin(name string, dirs ...string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, d := range dirs {
		if p := d + "/" + name; platformExists(p) {
			return p
		}
	}
	return name
}

func shutdownBin() string { return findBin("shutdown", "/usr/sbin", "/sbin", "/usr/bin") }

func run(wait time.Duration, name string, args ...string) (int, string) {
	code, out, _ := platformRun(wait, name, args...)
	return code, out
}

// ---- 예약 / 취소 ----

// platformScheduled 는 이 PC 에 이미 예약된 종료가 있는지 알아냄. 알 수 없으면 false.
func platformScheduled() bool {
	if platformOS == "darwin" {
		code, _ := run(shutdownWait, findBin("pgrep", "/usr/bin"), "-x", "shutdown")
		return code == 0
	}
	return platformExists(systemdScheduleFile)
}

// platformSchedule 은 sec 초 뒤 종료를 예약함 (분 단위로 올림). 0 이면 성공.
// 이미 예약이 있으면 codeAlreadyScheduled, 권한이 없으면 codePermissionDenied, 실행 불가면 codeNotRunnable,
// 그 밖에는 shutdown 의 종료 코드를 돌려줌.
// 호출한 쪽(타이머 앱)은 1190 을 받으면 platformCancel 후 다시 부름. 그 사이 취소가 실패했으면 기존 예약이 남은 것이므로
// shutdown 을 실행하지 않고 실패 코드를 돌려줌.
func platformSchedule(sec int64) int {
	if platformScheduled() {
		now := platformNow()
		switch {
		case within(platformCancelOKAt, now):
			// 방금 취소에 성공했음. 대기 중이던 shutdown 프로세스가 아직 사라지지 않았을 수 있어서 그대로 예약함
		case within(platformCancelFailAt, now):
			// 방금 취소에 실패해서 기존 예약이 그대로 남아 있음. shutdown 을 또 실행하면 예약이 겹치거나 덮이므로 실행하지 않음
			return platformCancelFail
		default:
			return codeAlreadyScheduled
		}
	}
	mins := (sec + 59) / 60
	if mins < 1 {
		mins = 1 // +0 은 즉시 종료이므로 최소 1분
	}
	arg := "+" + strconv.FormatInt(mins, 10)
	var args []string
	if platformOS == "darwin" {
		args = []string{"-h", arg} // macOS: -h 가 종료 (BSD)
	} else {
		args = []string{"-P", arg} // Linux: -P 가 전원 끄기. -h 는 sysvinit 에서 halt 만 하고 끌 수도 있음
	}
	code, out := run(shutdownWait, shutdownBin(), args...)
	if code == 0 {
		platformDetail = ""
		return 0
	}
	return classifyFailure(code, out)
}

// within 은 t 가 기록돼 있고 now 로부터 cancelWindow 안인지
func within(t, now time.Time) bool {
	return !t.IsZero() && now.Sub(t) >= 0 && now.Sub(t) < cancelWindow
}

// classifyFailure 는 shutdown 실패 출력을 보고 코드를 정함
func classifyFailure(code int, out string) int {
	platformDetail = firstLine(out)
	low := strings.ToLower(out)
	if code == codeNotRunnable {
		return codeNotRunnable
	}
	if platformOS == "darwin" && platformEUID() != 0 { // macOS shutdown 은 root 만 가능
		return codePermissionDenied
	}
	for _, k := range []string{"permission denied", "not permitted", "access denied", "not super-user", "superuser", "must be root",
		"authentication required", "authentication is required", "not authorized", "interactive authentication"} {
		if strings.Contains(low, k) {
			return codePermissionDenied
		}
	}
	if strings.Contains(low, "already") { // sysvinit 계열의 "이미 실행 중" 류 (확인 못 한 추정)
		return codeAlreadyScheduled
	}
	return code
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// platformFailReason 은 platformSchedule 이 돌려준 실패 코드를 화면에 한 줄로 보여줄 이유로 풀어줌. 모르면 "".
func platformFailReason(code int) string {
	switch code {
	case codePermissionDenied:
		if platformOS == "darwin" {
			return "권한 없음 (sudo 로 실행해야 함)"
		}
		return "권한 없음 (sudo 로 실행하거나 polkit 권한이 필요함)"
	case codeNotRunnable:
		return "shutdown 명령을 실행할 수 없음"
	}
	return platformDetail
}

// platformCancelFailReason 은 가장 최근 platformCancel 이 "예약이 남아 있는데 취소하지 못함" 으로 끝났을 때 그 이유(한 줄)를 돌려줌.
// 취소에 성공했거나 취소할 예약이 없었으면 "". platformCancel 이 false 여도 예약이 없어서인지 취소에 실패해서인지는 이 함수로 구별함.
func platformCancelFailReason() string { return platformCancelWhy }

// platformCancel 은 예약된 종료를 취소함. 취소할 예약이 있었으면 true.
func platformCancel() bool {
	platformCancelWhy = ""
	var ok bool
	if platformOS == "darwin" {
		ok = cancelDarwin()
	} else {
		ok = cancelLinux()
	}
	if ok {
		platformCancelOKAt, platformCancelFailAt = platformNow(), time.Time{}
	}
	return ok
}

// failCancel 은 예약이 남은 채 취소에 실패했음을 기록함
func failCancel(code int, out string) bool {
	platformCancelFail = classifyFailure(code, out)
	platformCancelFailAt, platformCancelOKAt = platformNow(), time.Time{}
	platformCancelWhy = platformFailReason(platformCancelFail)
	if platformCancelWhy == "" {
		platformCancelWhy = "예약 취소 실패 (오류 " + strconv.Itoa(code) + ")"
	}
	return false
}

func cancelDarwin() bool {
	// macOS shutdown 에는 취소 옵션이 없어서 대기 중인 프로세스를 종료함. root 가 아니면 실패함
	if code, _ := run(shutdownWait, findBin("pgrep", "/usr/bin"), "-x", "shutdown"); code != 0 {
		return false // 대기 중인 shutdown 이 없음
	}
	code, out := run(shutdownWait, findBin("pkill", "/usr/bin"), "-x", "shutdown")
	if code != 0 {
		return failCancel(code, out)
	}
	return true
}

func cancelLinux() bool {
	had := platformExists(systemdScheduleFile)
	code, out := run(shutdownWait, shutdownBin(), "-c")
	if code != 0 {
		if had {
			return failCancel(code, out)
		}
		platformDetail = firstLine(out)
		return false
	}
	// systemd 는 취소할 예약이 없어도 -c 가 성공(0)으로 끝나서 파일로 따로 확인함
	if platformExists(systemdRunDir) && !had {
		return false
	}
	return true
}

// ---- 부팅 시각 ----

// platformBootUnix 는 마지막 부팅 시각(유닉스 초). 재부팅 후 남은 상태 파일을 걸러내는 데 씀.
// 구하지 못하면 0 을 돌려줌: 호출할 때마다 달라지는 값(예: 현재 시각)을 쓰면 앱을 다시 켤 때마다 상태 파일이 버려지므로,
// 항상 같은 값인 0 으로 두어 상태 파일을 유지함 (재부팅 감지만 못 함).
func platformBootUnix() int64 {
	if b, err := platformBootFn(); err == nil && b > 0 {
		return int64(b)
	}
	if platformOS == "linux" {
		if data, err := platformReadFile("/proc/uptime"); err == nil {
			if f := strings.Fields(string(data)); len(f) > 0 {
				if up, err := strconv.ParseFloat(f[0], 64); err == nil && up >= 0 {
					now := float64(platformNow().UnixNano()) / 1e9
					return int64(math.Round(now - up))
				}
			}
		}
	}
	if platformOS == "darwin" {
		// "{ sec = 1700000000, usec = 123456 } Mon Nov  5 ..." 꼴
		if code, out := run(shutdownWait, findBin("sysctl", "/usr/sbin", "/sbin"), "-n", "kern.boottime"); code == 0 {
			if i := strings.Index(out, "sec = "); i >= 0 {
				rest := out[i+len("sec = "):]
				if j := strings.IndexAny(rest, ", }"); j > 0 {
					if v, err := strconv.ParseInt(rest[:j], 10, 64); err == nil && v > 0 {
						return v
					}
				}
			}
		}
	}
	return 0
}
