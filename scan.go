// PortView - 跨平台系统信息与端口/进程扫描调度
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
package main

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
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
	if runtime.GOOS == "windows" {
		return getGPUWindows()
	}
	out, _ := commandOutput("nvidia-smi",
		"--query-gpu=utilization.gpu,memory.used,memory.total,temperature.gpu",
		"--format=csv,noheader,nounits")
	return formatGPU(string(out))
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

// readProcessGPU 通过 nvidia-smi 查看某进程的 GPU 显存使用（按平台分发）
func readProcessGPU(pid int) string {
	if runtime.GOOS == "windows" {
		return readProcessGPUWindows(pid)
	}
	out, _ := commandOutput("nvidia-smi",
		"--query-compute-apps=pid,used_memory,name",
		"--format=csv,noheader,nounits")
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
	out, err := commandOutput(name, args...)
	return string(out), err
}

const (
	// killGracePeriod 是发出 SIGTERM 后等待进程自行退出的宽限期。
	killGracePeriod = 3 * time.Second
	// killPollInterval 是宽限期内检查进程是否仍存活的间隔。
	killPollInterval = 100 * time.Millisecond
)

// killProcess 终止指定进程。
// Windows 使用 taskkill /F；Linux/macOS 先发 SIGTERM 让进程自行清理并退出，
// 宽限期结束后仍存活时再升级为 SIGKILL，避免一律强杀导致状态残留。
func killProcess(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("无效 PID: %d", pid)
	}
	if runtime.GOOS == "windows" {
		return killProcessWindows(pid)
	}
	return gracefulKill(pid, sendSignal, time.Sleep)
}

// gracefulKill 实现"先 TERM 后 KILL"的两阶段终止流程。
// send/sleep 以参数注入，便于在不真正操作进程的情况下测试流程。
// 进程本就不存在时视为目标已达成，不报错。
func gracefulKill(pid int, send func(pid int, sig syscall.Signal) error, sleep func(time.Duration)) error {
	if err := send(pid, syscall.SIGTERM); err != nil {
		if processGone(err) {
			return nil
		}
		return err
	}
	for waited := time.Duration(0); waited < killGracePeriod; waited += killPollInterval {
		sleep(killPollInterval)
		if !processAlive(pid, send) {
			return nil
		}
	}
	if err := send(pid, syscall.SIGKILL); err != nil && !processGone(err) {
		return err
	}
	return nil
}

// processGone 判断错误是否表示"目标进程已不存在"。
// os.Process.Signal 会把内核的 ESRCH 归一化为 os.ErrProcessDone，两者都要识别。
func processGone(err error) bool {
	return err != nil && (errors.Is(err, syscall.ESRCH) || errors.Is(err, os.ErrProcessDone))
}

// processAlive 通过信号 0 判断进程是否仍存在。
// EPERM 表示进程存在但不属于当前用户，仍应视为存活。
func processAlive(pid int, send func(pid int, sig syscall.Signal) error) bool {
	err := send(pid, syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

func commandOutput(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = time.Second
	return cmd.Output()
}
func formatGPU(raw string) string {
	reader := csv.NewReader(strings.NewReader(raw))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return ""
	}
	var out []string
	for _, p := range rows {
		if len(p) != 4 {
			continue
		}
		out = append(out, fmt.Sprintf("GPU: %s%% | %s/%s MB | %s°C", p[0], p[1], p[2], p[3]))
	}
	return strings.Join(out, " | ")
}
