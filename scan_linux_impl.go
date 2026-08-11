// PortView - Linux 平台实现（ss + /proc）
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ============================================================
// Linux 端口扫描（ss + /proc）
// ============================================================

// getPortsLinux 调用 ss -tulnp 扫描所有监听端口，补全空闲端口
func getPortsLinux() ([]PortEntry, error) {
	raw, err := execCmd("ss", "-tulnp")
	if err != nil {
		return nil, fmt.Errorf("ss 失败: %w", err)
	}
	entries := parseSSOutput(raw)

	// 补全未出现在 ss 输出中的空闲端口
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

// parseSSOutput 解析 ss -tulnp 输出，返回监听端口条目
func parseSSOutput(raw string) []PortEntry {
	entries := make([]PortEntry, 0, 100)
	// 解析 ss 输出：提取进程名和 PID
	re := regexp.MustCompile(`"([^"]+)".*pid=(\d+)`)
	pageSize := os.Getpagesize()

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Netid") {
			continue // 跳过表头
		}
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		addr := f[4] // 本地地址，如 0.0.0.0:8080 或 [::]:22
		idx := strings.LastIndex(addr, ":")
		if idx < 0 {
			continue
		}
		port := atoi(addr[idx+1:])
		if port == 0 || port > 65535 {
			continue
		}

		// 提取进程名和 PID
		pn, pid := "", 0
		if len(f) > 5 {
			if m := re.FindStringSubmatch(strings.Join(f[5:], " ")); len(m) >= 3 {
				pn, pid = m[1], atoi(m[2])
			}
		}

		// 读取 exe 路径和 comm（进程名备用）
		ep := ""
		if pid > 0 {
			ep, _ = os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
			if pn == "" {
				if d, _ := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid)); len(d) > 0 {
					pn = strings.TrimSpace(string(d))
				}
			}
			if pn == "" {
				pn = fmt.Sprintf("PID:%d", pid)
			}
		}

		// 从 /proc/[pid]/statm 读取 RSS 内存（页大小用系统实际值）
		memMB := 0.0
		if pid > 0 {
			if d, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid)); err == nil {
				f := strings.Fields(string(d))
				if len(f) >= 2 {
					rss, _ := strconv.Atoi(f[1])
					memMB = float64(rss) * float64(pageSize) / 1024 / 1024
				}
			}
		}

		entries = append(entries, PortEntry{
			Port: port, Protocol: f[0], PID: pid,
			ProcessName: pn, Status: f[1],
			MemoryMB: memMB, ExePath: ep, LocalAddr: addr,
		})
	}
	return entries
}

// readProcessLinux 从 /proc/[pid]/stat 读取进程状态、CPU、内存
func readProcessLinux(pid int) string {
	d, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "状态: 已结束或无权限"
	}
	f := strings.Fields(string(d))
	if len(f) < 24 {
		return ""
	}

	// 进程状态映射
	st := map[string]string{
		"R": "运行中", "S": "休眠", "D": "不可中断",
		"Z": "僵尸", "T": "已停止",
	}[f[2]]
	if st == "" {
		st = f[2]
	}

	ut, _ := strconv.Atoi(f[13])  // 用户态 CPU 时间
	sti, _ := strconv.Atoi(f[14]) // 内核态 CPU 时间
	rss, _ := strconv.Atoi(f[23]) // RSS 页数
	nice, _ := strconv.Atoi(f[18])
	thr, _ := strconv.Atoi(f[19]) // 线程数
	pageSize := os.Getpagesize()
	rssM := float64(rss) * float64(pageSize) / 1024 / 1024 // RSS 转 MB

	// CPU 使用率 = (ut+sti) / uptime 秒
	ud, _ := os.ReadFile("/proc/uptime")
	us := 0.0
	fmt.Sscanf(string(ud), "%f", &us)
	cpuP := 0.0
	if us > 0 {
		cpuP = float64(ut+sti) / 100 / us * 100
	}

	// 内存占比
	mt := uint64(0)
	if d2, _ := os.ReadFile("/proc/meminfo"); d2 != nil {
		for _, l := range strings.Split(string(d2), "\n") {
			fmt.Sscanf(l, "MemTotal: %d kB", &mt)
		}
	}
	mp := 0.0
	if mt > 0 {
		mp = float64(rss) * float64(pageSize) / 1024 / float64(mt) * 100
	}

	return fmt.Sprintf("状态: %s | CPU: %.2f%% | 内存: %.1f MB (%.2f%%) | 优先级: %d | 线程: %d",
		st, cpuP, rssM, mp, nice, thr)
}

// readCmdlineLinux 读取 Linux 进程命令行（/proc）
func readCmdlineLinux(pid int) string {
	d, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	return strings.ReplaceAll(strings.TrimSpace(string(d)), "\x00", " ")
}

// ============================================================
// Linux 系统信息（/proc）
// ============================================================

// getCPULinux 从 /proc/stat 读取 CPU 使用率
func getCPULinux() string {
	d, _ := os.ReadFile("/proc/stat")
	for _, line := range strings.Split(string(d), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 5 {
			break
		}
		t, id := 0, 0
		for i, v := range f[1:] {
			n, _ := strconv.Atoi(v)
			t += n
			if i == 3 {
				id = n
			}
		}
		if t > 0 {
			return fmt.Sprintf("%.1f%%", float64(t-id)/float64(t)*100)
		}
	}
	return "N/A"
}

// getMemLinux 从 /proc/meminfo 读取内存使用率
func getMemLinux() string {
	d, _ := os.ReadFile("/proc/meminfo")
	t, a := 0, 0
	for _, line := range strings.Split(string(d), "\n") {
		fmt.Sscanf(line, "MemTotal: %d kB", &t)
		fmt.Sscanf(line, "MemAvailable: %d kB", &a)
	}
	if t == 0 {
		return "N/A"
	}
	u := t - a
	return fmt.Sprintf("%.1f%% (%d/%d GB)", float64(u)/float64(t)*100, u/1024/1024, t/1024/1024)
}
