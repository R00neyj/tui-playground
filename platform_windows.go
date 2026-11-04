//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

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

// platformSchedule 은 sec 초 뒤 종료를 예약함. 0 이면 성공, 이미 예약돼 있으면 codeAlreadyScheduled.
func platformSchedule(sec int64) int {
	return runShutdown("/s", "/t", strconv.FormatInt(sec, 10))
}

// platformCancel 은 예약된 종료를 취소함. 취소할 예약이 있었으면 true.
func platformCancel() bool { return runShutdown("/a") == 0 }

// ---- 부팅 시각 ----

type timeOfDayInfo struct {
	BootTime, CurrentTime, TimeZoneBias int64
	TimeZoneId                          uint32
	Reserved                            uint32
	BootTimeBias, SleepTimeBias         uint64
}

// platformBootUnix 는 마지막 부팅 시각(유닉스 초). 재부팅 후 남은 상태 파일을 걸러내는 데 씀
func platformBootUnix() int64 {
	var info timeOfDayInfo
	var ret uint32
	err := windows.NtQuerySystemInformation(3 /* SystemTimeOfDayInformation */, unsafe.Pointer(&info), uint32(unsafe.Sizeof(info)), &ret)
	if err == nil && info.BootTime > 0 {
		return (info.BootTime - 116444736000000000) / 10000000
	}
	// 실패 시: 현재 시각 - 가동 시간
	return time.Now().Add(-time.Duration(windows.DurationSinceBoot())).Unix()
}
