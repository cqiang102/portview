// PortView
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"
)

func main() {
	a := app.NewWithID("PortView")
	a.SetIcon(resourceIconPng)
	w := a.NewWindow("端口查看器")
	w.Resize(fyne.NewSize(1300, 760))
	path, err := configPath()
	if err != nil {
		dialog.ShowError(err, w)
		w.ShowAndRun()
		return
	}
	meta := &PortMetaStore{path: path}
	if err := meta.load(); err != nil {
		dialog.ShowError(err, w)
		w.ShowAndRun()
		return
	}
	pv := &PortViewer{win: w, selRow: -1, meta: meta, occupiedFirst: true}
	pv.buildUI()
	w.SetOnClosed(func() { pv.closed = true })
	pv.refresh()
	// 延迟在后台检查更新，不阻塞首屏；可在更新弹窗中关闭。
	pv.startAutoUpdateCheck()
	w.ShowAndRun()
}
