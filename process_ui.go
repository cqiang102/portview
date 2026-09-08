// PortView
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

func (pv *PortViewer) showDetail() {
	if pv.selRow < 0 || pv.selRow >= len(pv.filtered) {
		dialog.ShowInformation("提示", "请先选择一行", pv.win)
		return
	}
	e := pv.filtered[pv.selRow]
	m := pv.meta.Get(e.Port)
	cg := pv.meta.PortBelongsToCustom(e.Port)

	// 空闲端口：只显示基本信息
	if e.PID <= 0 {
		msg := fmt.Sprintf("端口: %d\n分组: %s\n状态: %s\n地址: %s", e.Port, e.SysGroup(), e.Status, e.LocalAddr)
		if e.Occupied() {
			msg += "\n已观测到占用，但无法读取进程 PID"
		} else {
			msg += "\n未观测到占用不代表此端口可绑定"
		}
		if m.Note != "" {
			msg += "\n备注: " + m.Note
		}
		if len(cg) > 0 {
			msg += "\n自定义: " + strings.Join(cg, ",")
		}
		dialog.ShowInformation("端口信息", msg, pv.win)
		return
	}

	// 被占用端口：显示进程详情
	pid := e.PID
	detail := widget.NewLabel("正在读取进程信息...")
	active := true
	go func() {
		info, gpu, cmdline := readProcess(pid), readProcessGPU(pid), readCmdline(pid)
		msg := fmt.Sprintf("进程: %s (PID %d)\n%s\n%s路径: %s\n命令行: %s", e.ProcessName, pid, info, gpu, e.ExePath, cmdline)
		if m.Note != "" {
			msg = "📝 " + m.Note + "\n\n" + msg
		}
		fyne.Do(func() {
			if !pv.closed && active {
				detail.SetText(msg)
			}
		})
	}()
	content := container.NewVBox(
		detail,
		widget.NewSeparator(),
		container.NewHBox(
			layout.NewSpacer(),
			widget.NewButton("终止进程", func() { pv.confirmKill(e) }),
		),
	)
	dlg := dialog.NewCustom(fmt.Sprintf("端口 %d", e.Port), "关闭", content, pv.win)
	dlg.SetOnClosed(func() { active = false })
	dlg.Show()
}

func (pv *PortViewer) killSelected() {
	if pv.selRow < 0 || pv.selRow >= len(pv.filtered) {
		dialog.ShowInformation("提示", "请先选择一行", pv.win)
		return
	}
	e := pv.filtered[pv.selRow]
	if e.PID <= 0 {
		dialog.ShowInformation("提示", "未观测到占用，或无法读取进程 PID", pv.win)
		return
	}
	pv.confirmKill(e)
}

func (pv *PortViewer) confirmKill(e PortEntry) {
	if e.PID <= 0 {
		return
	}
	dialog.ShowConfirm("确认终止",
		fmt.Sprintf("终止「%s」(PID %d)？", e.ProcessName, e.PID),
		func(ok bool) {
			if !ok {
				return
			}
			go func() {
				err := killProcess(e.PID)
				fyne.Do(func() {
					if pv.closed {
						return
					}
					if err != nil {
						dialog.ShowError(fmt.Errorf("失败: %w", err), pv.win)
						return
					}
					pv.refresh()
				})
			}()
		}, pv.win)
}

func (pv *PortViewer) openSelected() {
	if pv.selRow < 0 || pv.selRow >= len(pv.filtered) {
		return
	}
	e := pv.filtered[pv.selRow]
	if e.ExePath == "" {
		dialog.ShowInformation("提示", "无路径", pv.win)
		return
	}
	// macOS 用 open，Windows 用 explorer，Linux 用 xdg-open
	var err error
	switch runtime.GOOS {
	case "darwin":
		err = exec.Command("open", filepath.Dir(e.ExePath)).Start()
	case "windows":
		// explorer 是 GUI 应用，无需 HideWindow；路径用 ShellExecute 更可靠
		err = openFileWindows(e.ExePath)
	default:
		err = exec.Command("xdg-open", filepath.Dir(e.ExePath)).Start()
	}
	if err != nil {
		dialog.ShowError(fmt.Errorf("打开位置失败: %w", err), pv.win)
	}
}
