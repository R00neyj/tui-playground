//go:build windows

package main

import (
	"os/exec"
	"strconv"
	"syscall"
)

func brPrepCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

// brKillTree 는 브라우저와 그 자식 프로세스(렌더러 등)를 모두 끝냄
func brKillTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	k := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	k.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = k.Run()
	_ = cmd.Process.Kill()
}
