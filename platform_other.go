//go:build !windows

package main

// macOS / Linux 용 초기 구현 (F-06). 이 PC 는 Windows 라서 실제 동작은 확인하지 못했고, 빌드(GOOS 교차 컴파일)만 가능함.
// 구현 시 확인할 것: 명령 형태, 필요한 권한(대개 관리자), 이미 예약돼 있을 때의 동작.

import (
	"os/exec"
	"strconv"
	"time"

	"github.com/shirou/gopsutil/v4/host"
)

// platformSchedule 은 sec 초 뒤 종료를 예약함 (분 단위로 올림). 0 이면 성공.
func platformSchedule(sec int64) int {
	min := (sec + 59) / 60
	if err := exec.Command("shutdown", "-h", "+"+strconv.FormatInt(min, 10)).Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		return -1
	}
	return 0
}

// platformCancel 은 예약된 종료를 취소함 (Linux: shutdown -c).
func platformCancel() bool { return exec.Command("shutdown", "-c").Run() == nil }

// platformBootUnix 는 마지막 부팅 시각(유닉스 초)
func platformBootUnix() int64 {
	if b, err := host.BootTime(); err == nil && b > 0 {
		return int64(b)
	}
	return time.Now().Unix()
}
