// PortView - 更新检查界面
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
//
// 只负责"查询结果 -> 界面提示"，不下载也不替换文件。
package main

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// autoUpdateDelay 是启动后自动检查的延迟，先让首屏完成渲染与首次扫描。
const autoUpdateDelay = 3 * time.Second

// versionText 返回界面展示用的当前版本：可解析时显示 vX.Y.Z。
func versionText() string {
	if _, ok := parseVersion(buildVersion); !ok {
		return "开发版本"
	}
	return "v" + buildVersion
}

// startAutoUpdateCheck 启动后延迟在后台检查一次更新；可在更新弹窗中关闭。
// 自动检查失败保持静默，只有手动检查才提示错误。
func (pv *PortViewer) startAutoUpdateCheck() {
	if pv.meta == nil || !pv.meta.AutoCheckUpdate() {
		return
	}
	go func() {
		time.Sleep(autoUpdateDelay)
		info, err := checkForUpdate(context.Background(), buildVersion)
		if err != nil {
			return
		}
		fyne.Do(func() {
			if pv.closed {
				return
			}
			pv.applyUpdateInfo(info)
			// 同一个新版本只主动提示一次，避免每次启动都弹窗。
			if info.Newer && pv.meta.NotifiedVersion() != info.Latest {
				if err := pv.meta.SetNotifiedVersion(info.Latest); err != nil {
					dialog.ShowError(fmt.Errorf("保存设置失败: %w", err), pv.win)
				}
				pv.showUpdateDialog(info)
			}
		})
	}()
}

// checkUpdateManual 手动检查：无论结果如何都给出反馈。
func (pv *PortViewer) checkUpdateManual() {
	if pv.updateChecking {
		return
	}
	pv.updateChecking = true
	if pv.updateBtn != nil {
		pv.updateBtn.Disable()
	}
	pv.status.SetText("正在检查更新...")
	go func() {
		info, err := checkForUpdate(context.Background(), buildVersion)
		fyne.Do(func() {
			pv.updateChecking = false
			if pv.updateBtn != nil {
				pv.updateBtn.Enable()
			}
			if pv.closed {
				return
			}
			if err != nil {
				pv.status.SetText("检查更新失败")
				dialog.ShowError(fmt.Errorf("检查更新失败: %w", err), pv.win)
				return
			}
			pv.applyUpdateInfo(info)
			pv.showUpdateDialog(info)
		})
	}()
}

// applyUpdateInfo 把检查结果反映到版本标签与状态栏。
// 版本标签不参与刷新流程，因此提示不会被扫描结果覆盖。
func (pv *PortViewer) applyUpdateInfo(info UpdateInfo) {
	if pv.version == nil {
		return
	}
	if info.Newer {
		pv.version.SetText(fmt.Sprintf("%s → 新版本 %s", versionText(), info.Latest))
		pv.status.SetText(fmt.Sprintf("发现新版本 %s（当前 %s），点击「检查更新」查看", info.Latest, versionText()))
		return
	}
	pv.version.SetText(versionText())
}

// updateDialogText 生成弹窗正文。
func updateDialogText(info UpdateInfo) string {
	if info.Newer {
		text := fmt.Sprintf("发现新版本 %s（当前 %s）", info.Latest, versionText())
		if info.Notes != "" {
			text += "\n\n更新说明：\n" + info.Notes
		}
		return text
	}
	if info.Latest == "" {
		return "没有查询到已发布的版本。"
	}
	if _, ok := parseVersion(buildVersion); !ok {
		return fmt.Sprintf("当前为开发版本（%s），最新发布版本为 %s。", buildVersion, info.Latest)
	}
	return fmt.Sprintf("当前已是最新版本（%s），最新发布版本为 %s。", versionText(), info.Latest)
}

// showUpdateDialog 展示检查结果，并提供"启动时自动检查更新"开关。
func (pv *PortViewer) showUpdateDialog(info UpdateInfo) {
	auto := widget.NewCheck("启动时自动检查更新", nil)
	auto.SetChecked(pv.meta.AutoCheckUpdate())
	auto.OnChanged = func(enabled bool) {
		if err := pv.meta.SetAutoCheckUpdate(enabled); err != nil {
			dialog.ShowError(fmt.Errorf("保存设置失败: %w", err), pv.win)
		}
	}

	body := widget.NewLabel(updateDialogText(info))
	body.Wrapping = fyne.TextWrapWord

	var dlg dialog.Dialog
	buttons := container.NewHBox(layout.NewSpacer())
	if info.Newer {
		buttons.Add(widget.NewButton("打开下载页", func() {
			u, err := url.Parse(info.URL)
			if err != nil {
				dialog.ShowError(fmt.Errorf("下载页地址无效: %w", err), pv.win)
				return
			}
			if err := fyne.CurrentApp().OpenURL(u); err != nil {
				dialog.ShowError(fmt.Errorf("打开浏览器失败: %w", err), pv.win)
			}
		}))
	}
	buttons.Add(widget.NewButton("关闭", func() { dlg.Hide() }))

	content := container.NewVBox(
		body,
		widget.NewSeparator(),
		auto,
		widget.NewSeparator(),
		buttons,
	)
	dlg = dialog.NewCustomWithoutButtons("检查更新", content, pv.win)
	dlg.Show()
}
