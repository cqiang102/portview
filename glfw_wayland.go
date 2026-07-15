//go:build wayland && !x11

package main

import (
	"github.com/go-gl/glfw/v3.4/glfw"
)

func init() {
	// 在 Fyne 创建窗口前设置 Wayland app_id，使 dock 能正确匹配 .desktop 文件
	glfw.WindowHintString(glfw.WaylandAppID, "PortView")
}
