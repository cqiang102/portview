// PortView - 跨平台系统信息与端口/进程扫描调度
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// ============================================================
// 系统信息（CPU / 内存 / GPU）
// ============================================================

// getCPU 读取 CPU 使用率（按平台分发）
func getCPU() string {
	switch runtime.GOOS {
	case "darwin":
		return getCPUDarwin()
	case "windows":
		return getCPUWindows()
	}
	return getCPULinux()
}

// getMem 读取内存使用率（按平台分发）
func getMem() string {
	switch runtime.GOOS {
	case "darwin":
		return getMemDarwin()
	case "windows":
		return getMemWindows()
	}
	return getMemLinux()
}

// getGPU 通过 nvidia-smi 读取 GPU 信息
func getGPU() string {
	out, _ := exec.Command("nvidia-smi",
		"--query-gpu=utilization.gpu,memory.used,memory.total,temperature.gpu",
		"--format=csv,noheader,nounits").Output()
	p := strings.Split(strings.TrimSpace(string(out)), ", ")
	if len(p) < 3 {
		return ""
	}
	return fmt.Sprintf("GPU: %s%% | %s/%s MB | %s°C", p[0], p[1], p[2], p[3])
}

// ============================================================
// 进程详情（按平台分发）
// ============================================================

// readProcess 读取进程状态、CPU、内存（按平台分发）
func readProcess(pid int) string {
	switch runtime.GOOS {
	case "darwin":
		return readProcessDarwin(pid)
	case "windows":
		return readProcessWindows(pid)
	}
	return readProcessLinux(pid)
}

// readProcessGPU 通过 nvidia-smi 查看某进程的 GPU 显存使用
func readProcessGPU(pid int) string {
	out, _ := exec.Command("nvidia-smi",
		"--query-compute-apps=pid,used_memory,name",
		"--format=csv,noheader,nounits").Output()
	ps := strconv.Itoa(pid)
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, ps+",") {
			continue
		}
		p := strings.SplitN(line, ", ", 3)
		if len(p) == 3 {
			return fmt.Sprintf("GPU显存: %s MB (%s)", p[1], p[2]) + "\n"
		}
	}
	return ""
}

// readCmdline 读取进程命令行（按平台分发）
func readCmdline(pid int) string {
	switch runtime.GOOS {
	case "darwin":
		return readCmdlineDarwin(pid)
	case "windows":
		return readCmdlineWindows(pid)
	}
	return readCmdlineLinux(pid)
}

// ============================================================
// 端口扫描调度与公共命令
// ============================================================

// getPorts 根据操作系统选择端口扫描方式
func getPorts() ([]PortEntry, error) {
	switch runtime.GOOS {
	case "darwin":
		return getPortsDarwin()
	case "windows":
		return getPortsWindows()
	default:
		return getPortsLinux()
	}
}

// execCmd 执行命令并返回 stdout
func execCmd(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return string(out), err
}

// killProcess 终止指定进程：Windows 用 taskkill，Linux/macOS 用 kill -9
func killProcess(pid int) error {
	if runtime.GOOS == "windows" {
		return exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid)).Run()
	}
	return exec.Command("kill", "-9", strconv.Itoa(pid)).Run()
}
