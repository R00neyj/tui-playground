//go:build windows

package main

import (
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
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

var (
	brJobOnce sync.Once
	brJob     windows.Handle
)

// brBindLife 는 브라우저를 이 프로그램과 운명을 같이하는 작업 개체에 넣음 (프로그램이 갑자기 죽어도 브라우저가 남지 않음)
func brBindLife(cmd *exec.Cmd) {
	brJobOnce.Do(func() {
		h, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
			windows.CloseHandle(h)
			return
		}
		brJob = h
	})
	if brJob == 0 || cmd.Process == nil {
		return
	}
	ph, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(ph)
	_ = windows.AssignProcessToJobObject(brJob, ph)
}
