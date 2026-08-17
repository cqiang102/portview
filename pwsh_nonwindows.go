// PortView - 非 Windows 平台的 PowerShell 占位实现
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
//go:build !windows

package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// powershellOut 非 Windows 平台不存在 powershell，仅为保持包可编译而提供占位实现。
func powershellOut(script string) (string, error) {
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	return strings.TrimSpace(string(out)), err
}

// execCmdWindows 在非 Windows 平台上就是普通 exec（无需隐藏窗口）
func execCmdWindows(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return string(out), err
}

// runCmdWindows 在非 Windows 平台上就是普通 exec（无需隐藏窗口）
func runCmdWindows(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

// getGPUNonWindows 非 Windows 平台的 GPU 信息
func getGPUNonWindows() string {
	out, _ := exec.Command("nvidia-smi",
		"--query-gpu=utilization.gpu,memory.used,memory.total,temperature.gpu",
		"--format=csv,noheader,nounits").Output()
	p := strings.Split(strings.TrimSpace(string(out)), ", ")
	if len(p) < 3 {
		return ""
	}
	return fmt.Sprintf("GPU: %s%% | %s/%s MB | %s°C", p[0], p[1], p[2], p[3])
}

// killProcessNonWindows 非 Windows 平台终止进程
func killProcessNonWindows(pid int) error {
	return exec.Command("kill", "-9", strconv.Itoa(pid)).Run()
}
