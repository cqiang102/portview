// PortView - Windows PowerShell 执行（隐藏控制台窗口）
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
//go:build windows

package main

import (
	"os/exec"
	"strings"
	"syscall"
)

// powershellOut 执行 PowerShell 命令并返回 stdout（wmic 的现代替代）。
// 通过 CREATE_NO_WINDOW（HideWindow）隐藏控制台窗口，避免每次启动/刷新弹出黑窗。
func powershellOut(script string) (string, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive",
		"-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// execCmdWindows 在 Windows 上执行外部命令并返回 stdout。
// 通过 HideWindow 确保不弹出控制台窗口（适用于 netstat、tasklist、nvidia-smi 等）。
func execCmdWindows(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	return string(out), err
}

// runCmdWindows 在 Windows 上执行外部命令（不获取输出），确保不弹出控制台窗口。
func runCmdWindows(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}
