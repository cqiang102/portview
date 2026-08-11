// PortView - 领域模型与通用工具函数
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ============================================================
// 端口条目模型
// ============================================================

type PortEntry struct {
	Port        int     // 端口号 0-65535
	Protocol    string  // 协议：tcp/tcp6/udp/udp6
	PID         int     // 占用进程 PID，0 表示空闲
	ProcessName string  // 进程名
	Status      string  // 连接状态：LISTEN/ESTABLISHED/空闲
	MemoryMB    float64 // 进程 RSS 内存（MB）
	ExePath     string  // 可执行文件路径
	LocalAddr   string  // 本地地址（含 IP）
}

func (e *PortEntry) SysGroup() string {
	// 被占用端口按端口号分类
	if e.PID > 0 {
		switch {
		case e.Port == 22:
			return "SSH"
		case e.Port == 80 || e.Port == 443 || e.Port == 8080 || e.Port == 8443:
			return "Web"
		case e.Port == 3306 || e.Port == 5432 || e.Port == 6379 || e.Port == 27017:
			return "数据库"
		case e.Port == 53:
			return "DNS"
		case e.Port <= 1023:
			return "系统"
		default:
			return "应用"
		}
	}
	// 空闲端口按范围分类：系统(0-1023) / 注册(1024-49151) / 动态(49152+)
	if e.Port <= 1023 {
		return "系统"
	}
	if e.Port <= 49151 {
		return "注册"
	}
	return "动态"
}

func parsePorts(s string) []int {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// 范围：8000-8010
		if strings.Contains(part, "-") {
			r := strings.SplitN(part, "-", 2)
			s, e := atoi(strings.TrimSpace(r[0])), atoi(strings.TrimSpace(r[1]))
			if s > 0 && e > 0 && s <= e && e <= 65535 {
				for p := s; p <= e; p++ {
					out = append(out, p)
				}
			}
		} else if p := atoi(part); p > 0 && p <= 65535 {
			out = append(out, p)
		}
	}
	return uniquePorts(out)
}

func uniquePorts(ports []int) []int {
	seen := make(map[int]bool)
	var out []int
	for _, p := range ports {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Ints(out)
	return out
}

const maxNoteLen = 100 // 备注最大字符数（按 rune 计，支持中文）

func matchAny(p int, targets ...int) bool {
	for _, t := range targets {
		if p == t {
			return true
		}
	}
	return false
}

func truncateNote(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "..."
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func atof(s string) float64 {
	n, _ := strconv.ParseFloat(s, 64)
	return n
}

func fmtPort(p int) string {
	// 知名端口映射
	m := map[int]string{
		22: "SSH", 80: "HTTP", 443: "HTTPS", 3306: "MySQL",
		5432: "PG", 6379: "Redis", 8080: "HTTP-alt", 27017: "Mongo",
		53: "DNS", 25: "SMTP", 3389: "RDP",
	}
	if p == 0 {
		return "0 (保留)"
	}
	if n, ok := m[p]; ok {
		return fmt.Sprintf("%d (%s)", p, n)
	}
	if p < 1024 {
		return strconv.Itoa(p)
	}
	return strconv.Itoa(p)
}
