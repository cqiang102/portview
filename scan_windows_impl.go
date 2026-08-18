// PortView - Windows 平台实现（netstat + tasklist + PowerShell）
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
package main

import (
	"fmt"
	"strconv"
	"strings"
)

// ============================================================
// Windows 端口扫描（netstat -ano）
// ============================================================

// windowsProcInfo Windows 进程信息（名称、可执行路径、内存）
type windowsProcInfo struct {
	name    string
	exePath string
	memMB   float64
}

// getPortsWindows Windows 版本：使用 netstat -ano 扫描监听端口
func getPortsWindows() ([]PortEntry, error) {
	raw, err := execCmdWindows("netstat", "-ano")
	if err != nil {
		return nil, fmt.Errorf("netstat 失败: %w", err)
	}
	// 一次性批量获取进程信息，避免逐 PID 起进程（wmic 已废弃，改用 tasklist + PowerShell）
	procInfo := loadWindowsProcesses()
	entries := parseNetstatOutput(raw, procInfo)

	// 补全空闲端口
	seen := make(map[int]bool)
	for _, e := range entries {
		seen[e.Port] = true
	}
	for p := 0; p <= 65535; p++ {
		if !seen[p] {
			entries = append(entries, PortEntry{Port: p, Status: "空闲"})
		}
	}
	return entries, nil
}

// parseNetstatOutput 解析 netstat -ano 输出，进程信息从 procInfo 补全
func parseNetstatOutput(raw string, procInfo map[int]windowsProcInfo) []PortEntry {
	entries := make([]PortEntry, 0, 100)

	// netstat -ano 输出格式：
	// Proto  Local Address          Foreign Address        State           PID
	// TCP    0.0.0.0:8080           0.0.0.0:0              LISTENING       1234
	// UDP    0.0.0.0:5353           *:*                                    9012
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Proto") || strings.HasPrefix(line, "Active") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		proto := strings.ToLower(f[0])
		if proto != "tcp" && proto != "udp" {
			continue
		}

		// 本地地址：0.0.0.0:8080 或 [::]:22
		local := f[1]
		idx := strings.LastIndex(local, ":")
		if idx < 0 {
			continue
		}
		port := atoi(local[idx+1:])
		if port == 0 || port > 65535 {
			continue
		}

		// PID 在最后一列（UDP 行没有状态列，同样取最后一列）
		pid := 0
		if len(f) >= 4 {
			pid = atoi(f[len(f)-1])
		}

		// 进程信息
		pn := ""
		ep := ""
		memMB := 0.0
		if pid > 0 {
			if info, ok := procInfo[pid]; ok {
				pn, ep, memMB = info.name, info.exePath, info.memMB
			}
			if pn == "" {
				pn = fmt.Sprintf("PID:%d", pid)
			}
		}

		// 状态映射：UDP 行无状态列（最后一列是 PID），统一显示 LISTEN
		status := "UNKNOWN"
		if proto == "udp" {
			status = "LISTEN"
		} else if len(f) >= 4 {
			status = f[3]
		}

		entries = append(entries, PortEntry{
			Port: port, Protocol: proto, PID: pid,
			ProcessName: pn, Status: status,
			MemoryMB: memMB, ExePath: ep, LocalAddr: local,
		})
	}
	return entries
}

// loadWindowsProcesses 批量获取所有 Windows 进程信息：
// tasklist 一次拿进程名+内存，PowerShell 一次拿可执行路径
func loadWindowsProcesses() map[int]windowsProcInfo {
	m := make(map[int]windowsProcInfo)

	// tasklist /FO CSV /NH 全量输出，一次获取进程名和内存
	if out, err := execCmdWindows("tasklist", "/FO", "CSV", "/NH"); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "\"") || strings.Contains(line, "INFO:") {
				continue
			}
			// 格式: "process.exe","1234","Console","1","123,456 K"
			parts := strings.Split(line, "\",\"")
			if len(parts) < 5 {
				continue
			}
			name := strings.Trim(parts[0], "\"")
			pid := atoi(strings.Trim(parts[1], "\""))
			if pid <= 0 {
				continue
			}
			memStr := strings.ReplaceAll(strings.Trim(parts[len(parts)-1], "\""), ",", "")
			memStr = strings.TrimSuffix(memStr, " K")
			info := m[pid]
			info.name = name
			info.memMB = atof(strings.TrimSpace(memStr)) / 1024.0
			m[pid] = info
		}
	}

	// PowerShell 一次获取所有进程的可执行路径（wmic 已废弃，Win11 24H2 起不再提供）
	if out, err := powershellOut(`Get-CimInstance Win32_Process | ForEach-Object { "{0}|{1}" -f $_.ProcessId, $_.ExecutablePath }`); err == nil {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			pidStr, exe, ok := strings.Cut(line, "|")
			if !ok {
				continue
			}
			pid := atoi(pidStr)
			if pid <= 0 {
				continue
			}
			info := m[pid]
			info.exePath = strings.TrimSpace(exe)
			m[pid] = info
		}
	}
	return m
}

// getGPUWindows Windows 版 GPU 信息（隐藏控制台窗口）
func getGPUWindows() string {
	out, _ := execCmdWindows("nvidia-smi",
		"--query-gpu=utilization.gpu,memory.used,memory.total,temperature.gpu",
		"--format=csv,noheader,nounits")
	p := strings.Split(strings.TrimSpace(out), ", ")
	if len(p) < 3 {
		return ""
	}
	return fmt.Sprintf("GPU: %s%% | %s/%s MB | %s°C", p[0], p[1], p[2], p[3])
}

// killProcessWindows Windows 版终止进程（隐藏控制台窗口）
func killProcessWindows(pid int) error {
	return runCmdWindows("taskkill", "/F", "/PID", strconv.Itoa(pid))
}

// readProcessWindows 通过 tasklist 获取 Windows 进程详情
func readProcessWindows(pid int) string {
	out, err := execCmdWindows("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid),
		"/FO", "CSV", "/NH")
	if err != nil || out == "" {
		return "状态: 已结束或无权限"
	}
	line := strings.TrimSpace(out)
	if !strings.HasPrefix(line, "\"") || strings.Contains(line, "INFO:") {
		return "状态: 已结束或无权限"
	}
	parts := strings.Split(line, "\",\"")
	if len(parts) < 5 {
		return fmt.Sprintf("PID %d (无法读取详情)", pid)
	}
	name := strings.Trim(parts[0], "\"")
	memStr := strings.Trim(parts[len(parts)-1], "\"")
	memStr = strings.ReplaceAll(memStr, ",", "")
	memStr = strings.TrimSuffix(memStr, " K")
	memStr = strings.TrimSpace(memStr)
	memKB := atof(memStr)
	memMB := memKB / 1024.0

	return fmt.Sprintf("进程: %s | 内存: %.1f MB | 线程: N/A", name, memMB)
}

// readProcessGPUWindows Windows 版 GPU 显存查询（隐藏控制台窗口）
func readProcessGPUWindows(pid int) string {
	out, _ := execCmdWindows("nvidia-smi",
		"--query-compute-apps=pid,used_memory,name",
		"--format=csv,noheader,nounits")
	ps := strconv.Itoa(pid)
	for _, line := range strings.Split(out, "\n") {
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

// readCmdlineWindows 通过 PowerShell 获取 Windows 进程命令行（替代已废弃的 wmic）
func readCmdlineWindows(pid int) string {
	script := fmt.Sprintf(`(Get-CimInstance Win32_Process -Filter "ProcessId = %d").CommandLine`, pid)
	out, err := powershellOut(script)
	if err != nil {
		return ""
	}
	return out
}

// ============================================================
// Windows 系统信息（PowerShell，替代已废弃的 wmic）
// ============================================================

// getCPUWindows Windows 版 CPU 使用率
func getCPUWindows() string {
	out, err := powershellOut(`(Get-CimInstance Win32_Processor | Measure-Object -Property LoadPercentage -Average).Average`)
	if err != nil {
		return "N/A"
	}
	if pct := atof(out); pct > 0 {
		return fmt.Sprintf("%.0f%%", pct)
	}
	return "N/A"
}

// getMemWindows Windows 版内存信息
func getMemWindows() string {
	out, err := powershellOut(`$os = Get-CimInstance Win32_OperatingSystem; "{0} {1}" -f $os.TotalVisibleMemorySize, $os.FreePhysicalMemory`)
	if err != nil {
		return "N/A"
	}
	f := strings.Fields(out)
	if len(f) >= 2 {
		totalKB := atof(f[0])
		freeKB := atof(f[1])
		if totalKB > 0 {
			usedKB := totalKB - freeKB
			totalGB := totalKB / 1024 / 1024
			usedGB := usedKB / 1024 / 1024
			pct := usedKB / totalKB * 100
			return fmt.Sprintf("%.1f%% (%.1f/%.0f GB)", pct, usedGB, totalGB)
		}
	}
	return "N/A"
}
