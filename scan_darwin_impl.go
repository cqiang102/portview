// PortView - macOS 平台实现（lsof + ps）
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// ============================================================
// macOS 端口扫描（lsof）
// ============================================================

// darwinProcInfo macOS 进程信息缓存（一次刷新内按 PID 去重，减少 ps/lsof -p 调用）
type darwinProcInfo struct {
	exePath string
	memMB   float64
}

// getPortsDarwin macOS 版本：使用 lsof 扫描监听端口（TCP + UDP）
func getPortsDarwin() ([]PortEntry, error) {
	// -n -P 避免 DNS 反查和端口名转换
	// TCP 只取 LISTEN 状态；UDP 无状态过滤，直接列出全部 UDP socket
	rawTCP, errTCP := execCmd("lsof", "-iTCP", "-sTCP:LISTEN", "-n", "-P")
	rawUDP, errUDP := execCmd("lsof", "-iUDP", "-n", "-P")
	// lsof 没有匹配项时返回 exit 1，所以仅在两个命令都无输出时视为失败
	if (errTCP != nil && rawTCP == "") && (errUDP != nil && rawUDP == "") {
		return nil, fmt.Errorf("lsof 失败: %w", errTCP)
	}

	seen := make(map[int]bool)
	entries := make([]PortEntry, 0, 100)
	// 进程信息缓存：同一 PID 在 TCP/UDP 中重复出现时只查询一次
	cache := make(map[int]darwinProcInfo)
	parseLsofDarwin(rawTCP, "tcp", seen, &entries, cache)
	parseLsofDarwin(rawUDP, "udp", seen, &entries, cache)

	// 补全空闲端口
	for p := 0; p <= 65535; p++ {
		if !seen[p] {
			entries = append(entries, PortEntry{Port: p, Status: "空闲"})
		}
	}
	return entries, nil
}

// parseLsofDarwin 解析 macOS lsof 输出并追加到 entries
// baseProto 为 "tcp"/"udp"，IPv6 自动映射为 tcp6/udp6；cache 按 PID 缓存进程信息
func parseLsofDarwin(raw, baseProto string, seen map[int]bool, entries *[]PortEntry, cache map[int]darwinProcInfo) {
	// lsof 输出格式:
	// COMMAND   PID   USER   FD   TYPE             DEVICE SIZE/OFF NODE NAME
	// com.dock  1234  user   10u  IPv4 0x...       0t0  TCP *:8080 (LISTEN)
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "COMMAND") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 9 {
			continue
		}
		// f[0]=COMMAND, f[1]=PID, f[8]=NAME
		pid := atoi(f[1])
		nameField := f[8] // e.g. "*:8080" 或 "127.0.0.1:3000"

		// 提取端口号：取最后一个 : 之后的部分
		idx := strings.LastIndex(nameField, ":")
		if idx < 0 {
			continue
		}
		port := atoi(nameField[idx+1:])
		if port == 0 || port > 65535 {
			continue
		}
		seen[port] = true

		// 进程名
		pn := f[0]
		if pn == "" {
			pn = fmt.Sprintf("PID:%d", pid)
		}

		// 协议类型（lsof 第 4 列：IPv4/IPv6，映射为 tcp/tcp6 或 udp/udp6）
		proto := baseProto
		if len(f) >= 5 && f[4] == "IPv6" {
			proto = baseProto + "6"
		}

		// 状态：TCP 为 LISTEN，UDP 为 UNCONN（与 Linux ss 输出保持一致）
		status := "LISTEN"
		if baseProto == "udp" {
			status = "UNCONN"
		}

		// 读取 exe 路径与 RSS 内存（按 PID 缓存，避免重复调用 ps/lsof -p）
		ep, memMB := "", 0.0
		if pid > 0 {
			info, ok := cache[pid]
			if !ok {
				info = darwinProcInfo{exePath: getExePathDarwin(pid), memMB: getProcessMemDarwin(pid)}
				cache[pid] = info
			}
			ep, memMB = info.exePath, info.memMB
		}

		*entries = append(*entries, PortEntry{
			Port: port, Protocol: proto, PID: pid,
			ProcessName: pn, Status: status,
			MemoryMB: memMB, ExePath: ep, LocalAddr: nameField,
		})
	}
}

// getProcessMemDarwin 通过 ps 获取进程 RSS 内存（MB）
func getProcessMemDarwin(pid int) float64 {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "rss=").Output()
	if err != nil {
		return 0
	}
	// ps rss= 返回 KB
	kb := atoi(strings.TrimSpace(string(out)))
	return float64(kb) / 1024.0
}

// getExePathDarwin 通过 lsof -d txt 获取可执行文件完整路径
func getExePathDarwin(pid int) string {
	out, err := exec.Command("lsof", "-p", strconv.Itoa(pid), "-a", "-d", "txt", "-Fn").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "n") && len(line) > 1 {
			return line[1:]
		}
	}
	return ""
}

// readProcessDarwin 通过 ps 获取 macOS 进程详情
func readProcessDarwin(pid int) string {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid),
		"-o", "state=", "-o", "%cpu=", "-o", "rss=", "-o", "nice=").Output()
	if err != nil {
		return "状态: 已结束或无权限"
	}
	f := strings.Fields(strings.TrimSpace(string(out)))
	if len(f) < 4 {
		return fmt.Sprintf("PID %d (无权限)", pid)
	}
	st := f[0]
	cpuP := atof(f[1])
	rssKB := atof(f[2])
	nice := atoi(f[3])
	rssM := rssKB / 1024.0

	stMap := map[string]string{
		"R": "运行中", "S": "休眠", "D": "不可中断",
		"Z": "僵尸", "T": "已停止",
	}
	if v, ok := stMap[st]; ok {
		st = v
	}

	return fmt.Sprintf("状态: %s | CPU: %.2f%% | 内存: %.1f MB | 优先级: %d | 线程: N/A",
		st, cpuP, rssM, nice)
}

// readCmdlineDarwin 通过 ps 获取 macOS 进程命令行
func readCmdlineDarwin(pid int) string {
	out, _ := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "args=").Output()
	return strings.TrimSpace(string(out))
}

// ============================================================
// macOS 系统信息
// ============================================================

// getCPUDarwin 通过 top 读取 CPU 使用率
func getCPUDarwin() string {
	out, err := exec.Command("top", "-l", "1", "-n", "0").Output()
	if err != nil {
		return "N/A"
	}
	re := regexp.MustCompile(`CPU usage:\s*([\d.]+)%\s*user,\s*([\d.]+)%\s*sys`)
	if m := re.FindStringSubmatch(string(out)); len(m) == 3 {
		user := atof(m[1])
		sys := atof(m[2])
		return fmt.Sprintf("%.1f%%", user+sys)
	}
	return "N/A"
}

// getMemDarwin macOS 版内存信息，通过 sysctl + vm_stat 获取
func getMemDarwin() string {
	totalOut, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return "N/A"
	}
	totalBytes := atof(strings.TrimSpace(string(totalOut)))
	if totalBytes == 0 {
		return "N/A"
	}

	vmOut, err := exec.Command("vm_stat").Output()
	if err != nil {
		return "N/A"
	}
	vmText := string(vmOut)
	getPages := func(key string) float64 {
		re := regexp.MustCompile(key + `:\s+(\d+)`)
		if m := re.FindStringSubmatch(vmText); len(m) == 2 {
			return atof(m[1])
		}
		return 0
	}
	pageSize := 16384.0 // macOS ARM 默认页大小 16KB
	freePages := getPages("Pages free")
	activePages := getPages("Pages active")
	inactivePages := getPages("Pages inactive")
	wiredPages := getPages("Pages wired down")
	usedPages := activePages + wiredPages + (inactivePages * 0.5)

	usedBytes := usedPages * pageSize
	totalGB := totalBytes / 1024 / 1024 / 1024
	usedGB := usedBytes / 1024 / 1024 / 1024
	pct := usedBytes / totalBytes * 100

	_ = freePages

	return fmt.Sprintf("%.1f%% (%.1f/%.0f GB)", pct, usedGB, totalGB)
}
