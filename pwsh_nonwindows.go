// PortView - 非 Windows 平台的 PowerShell 占位实现
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
//go:build !windows

package main

import (
	"os/exec"
	"strings"
)

// powershellOut 非 Windows 平台不存在 powershell，仅为保持包可编译而提供占位实现。
func powershellOut(script string) (string, error) {
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	return strings.TrimSpace(string(out)), err
}
