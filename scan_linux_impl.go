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
	"time"
)

const (
	// Linux USER_HZ：/proc/[pid]/stat 中 utime/stime 的时钟单位，主流内核为 100。
	linuxClockTicks = 100.0
	// cpuSampleInterval 两次采样之间的间隔：瞬时占用率必须用差值计算。
	cpuSampleInterval = 200 * time.Millisecond
)

// ============================================================
// Linux 端口扫描（ss + /proc）
// ============================================================

// getPortsLinux 调用 ss -taunp 扫描所有 TCP/UDP socket，补全未观测到占用的端口
func getPortsLinux() ([]PortEntry, error) {
	raw, err := execCmd("ss", "-taunp")
	if err != nil {
		return nil, fmt.Errorf("ss 失败: %w", err)
	}
	entries := parseSSOutput(raw)

	// 补全未出现在 ss 输出中的未观测到占用的端口
	seen := make(map[int]bool)
	for _, e := range entries {
		seen[e.Port] = true
	}
	for p := 0; p <= 65535; p++ {
		if !seen[p] {
			entries = append(entries, PortEntry{Port: p, Status: unobservedStatus})
		}
	}
	return entries, nil
}

// parseSSOutput 解析 ss -taunp 输出，返回 socket 条目
func parseSSOutput(raw string) []PortEntry {
	entries := make([]PortEntry, 0, 100)
	// 解析 ss 输出：提取进程名和 PID
	re := regexp.MustCompile(`"([^"]+)",pid=(\d+)`)
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
			Port: port, Protocol: protoForAddr(f[0], addr), PID: pid,
			ProcessName: pn, Status: f[1],
			MemoryMB: memMB, ExePath: ep, LocalAddr: addr,
		})
	}
	return entries
}

// protoForAddr 统一协议标签。ss 的 Netid 列对 IPv6 socket 仍可能显示 tcp/udp，
// 因此按本地地址是否带 IPv6 前缀判断，与 macOS 实现保持一致（tcp/tcp6/udp/udp6）。
func protoForAddr(netid, addr string) string {
	proto := strings.ToLower(netid)
	if proto != "tcp" && proto != "udp" {
		return netid
	}
	if strings.HasPrefix(addr, "[") || strings.Contains(addr, "]:") {
		return proto + "6"
	}
	return proto
}

// linuxProcStat 是 /proc/[pid]/stat 中本工具关心的字段。
type linuxProcStat struct {
	State    string
	UTime    int
	STime    int
	Nice     int
	Threads  int
	RSSPages int
}

// parseProcStat 解析 /proc/[pid]/stat 内容。
// comm（字段 2）被括号包裹且进程名可能包含空格与 ')'，因此从最后一个 ')' 之后开始
// 按字段解析，避免进程名含空格导致后续字段整体错位。字段号 n 对应 rest[n-3]。
func parseProcStat(raw string) (linuxProcStat, bool) {
	var st linuxProcStat
	i := strings.LastIndex(raw, ")")
	if i < 0 {
		return st, false
	}
	rest := strings.Fields(raw[i+1:])
	if len(rest) < 22 {
		return st, false
	}
	st.State = rest[0]           // 3 state
	st.UTime = atoi(rest[11])    // 14 utime
	st.STime = atoi(rest[12])    // 15 stime
	st.Nice = atoi(rest[16])     // 19 nice
	st.Threads = atoi(rest[17])  // 20 num_threads
	st.RSSPages = atoi(rest[21]) // 24 rss
	return st, true
}

// readProcessLinux 从 /proc/[pid]/stat 读取进程状态、CPU、内存
func readProcessLinux(pid int) string {
	d, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "状态: 已结束或无权限"
	}
	st, ok := parseProcStat(string(d))
	if !ok {
		return ""
	}

	// 进程状态映射
	state := map[string]string{
		"R": "运行中", "S": "休眠", "D": "不可中断",
		"Z": "僵尸", "T": "已停止",
	}[st.State]
	if state == "" {
		state = st.State
	}

	// CPU 使用率：间隔采样求差，得到瞬时占用率（而非开机以来的平均值）。
	cpuText := "N/A"
	time.Sleep(cpuSampleInterval)
	if d2, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
		if st2, ok := parseProcStat(string(d2)); ok {
			delta := float64((st2.UTime+st2.STime)-(st.UTime+st.STime)) / linuxClockTicks
			if delta >= 0 {
				cpuText = fmt.Sprintf("%.2f%%", delta/cpuSampleInterval.Seconds()*100)
			}
		}
	}

	pageSize := os.Getpagesize()
	rssM := float64(st.RSSPages) * float64(pageSize) / 1024 / 1024 // RSS 转 MB

	// 内存占比
	mt := uint64(0)
	if d2, _ := os.ReadFile("/proc/meminfo"); d2 != nil {
		for _, l := range strings.Split(string(d2), "\n") {
			fmt.Sscanf(l, "MemTotal: %d kB", &mt)
		}
	}
	mp := 0.0
	if mt > 0 {
		mp = float64(st.RSSPages) * float64(pageSize) / 1024 / float64(mt) * 100
	}

	return fmt.Sprintf("状态: %s | CPU: %s | 内存: %.1f MB (%.2f%%) | 优先级: %d | 线程: %d",
		state, cpuText, rssM, mp, st.Nice, st.Threads)
}

// readCmdlineLinux 读取 Linux 进程命令行（/proc）
func readCmdlineLinux(pid int) string {
	d, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	return strings.ReplaceAll(strings.TrimSpace(string(d)), "\x00", " ")
}

// ============================================================
// Linux 系统信息（/proc）
// ============================================================

// getCPULinux 从 /proc/stat 读取 CPU 占用率。
// 单次读取得到的是"开机以来平均值"，因此间隔采样两次求差，得到瞬时占用率。
func getCPULinux() string {
	t1, i1, ok := readProcStatCPU()
	if !ok {
		return "N/A"
	}
	time.Sleep(cpuSampleInterval)
	t2, i2, ok := readProcStatCPU()
	if !ok {
		return "N/A"
	}
	dt, di := t2-t1, i2-i1
	if dt <= 0 {
		return "N/A"
	}
	return fmt.Sprintf("%.1f%%", float64(dt-di)/float64(dt)*100)
}

func readProcStatCPU() (int, int, bool) {
	d, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	return parseCPUStat(string(d))
}

// parseCPUStat 解析 /proc/stat 的 "cpu " 汇总行，返回 (total, idle)。
// 字段顺序：user nice system idle iowait irq softirq steal ...
func parseCPUStat(content string) (int, int, bool) {
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 5 {
			return 0, 0, false
		}
		total, idle := 0, 0
		for i, v := range f[1:] {
			total += atoi(v)
			if i == 3 {
				idle = atoi(v)
			}
		}
		return total, idle, total > 0
	}
	return 0, 0, false
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
