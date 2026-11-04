//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// 새 프로세스 그룹으로 띄워서 끝낼 때 자식까지 한꺼번에 정리함
func brPrepCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func brKillTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
}
