//go:build !wayland

package main

import (
	"github.com/go-gl/glfw/v3.4/glfw"
)

func init() {
	// 设置 X11 WM_CLASS，使 dock/任务栏能正确识别应用
	glfw.WindowHintString(glfw.X11ClassName, "PortView")
	glfw.WindowHintString(glfw.X11InstanceName, "PortView")
}
