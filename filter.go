// PortView
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
package main

import (
	"sort"
	"strconv"
	"strings"
)

func filterEntries(entries []PortEntry, groups []CustomGroup, sel, query string) []PortEntry {
	q := strings.ToLower(strings.TrimSpace(query))
	// 判断是否选中了自定义分组
	var customTarget *CustomGroup
	for _, g := range groups {
		if "🔖 "+g.Name == sel {
			customTarget = &g
			break
		}
	}

	targets := make(map[int]bool)
	if customTarget != nil {
		for _, port := range customTarget.Ports {
			targets[port] = true
		}
	}
	filtered := make([]PortEntry, 0, len(entries))
	for _, e := range entries {
		// 分组筛选
		if sel != "🏷️ 全部" {
			switch {
			case sel == "📌 已占用":
				if !e.Occupied() {
					continue
				}
			case sel == "🅰 TCP":
				if e.Protocol != "tcp" && e.Protocol != "tcp6" {
					continue
				}
			case sel == "🅱 UDP":
				if e.Protocol != "udp" && e.Protocol != "udp6" {
					continue
				}
			case sel == "⚙️ 系统(占用)":
				if !e.Occupied() || e.Port > 1023 {
					continue
				}
			case sel == "🌐 Web":
				if !e.Occupied() || !matchAny(e.Port, 80, 443, 8080, 8443, 3000, 5000, 8000, 8888, 9090) {
					continue
				}
			case sel == "💾 数据库":
				if !e.Occupied() || !matchAny(e.Port, 3306, 5432, 6379, 27017, 1433, 1521, 9042) {
					continue
				}
			case sel == "🔐 SSH":
				if e.Port != 22 {
					continue
				}
			case sel == "🔁 动态":
				if e.Port < 49152 {
					continue
				}
			case customTarget != nil:
				found := targets[e.Port]
				if !found {
					continue
				}
			}
		}

		// 关键词搜索（匹配端口号、PID、进程名、状态）
		if q != "" {
			ps, pids := strconv.Itoa(e.Port), strconv.Itoa(e.PID)
			if !strings.Contains(ps, q) && !strings.Contains(pids, q) &&
				!strings.Contains(strings.ToLower(e.ProcessName), q) &&
				!strings.Contains(strings.ToLower(e.Status), q) {
				continue
			}
		}
		filtered = append(filtered, e)
	}
	return filtered
}

func sortEntries(entries []PortEntry, occupiedFirst bool) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if occupiedFirst && a.Occupied() != b.Occupied() {
			return a.Occupied()
		}
		if a.Port != b.Port {
			return a.Port < b.Port
		}
		if a.Protocol != b.Protocol {
			return a.Protocol < b.Protocol
		}
		if a.LocalAddr != b.LocalAddr {
			return a.LocalAddr < b.LocalAddr
		}
		return a.PID < b.PID
	})
}
