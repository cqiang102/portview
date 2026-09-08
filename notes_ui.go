// PortView
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
package main

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

func (pv *PortViewer) editNote() {
	if pv.selRow < 0 || pv.selRow >= len(pv.filtered) {
		dialog.ShowInformation("提示", "请先选择一行", pv.win)
		return
	}
	e := pv.filtered[pv.selRow]
	m := pv.meta.Get(e.Port)

	groups := pv.meta.Groups()
	names := make([]string, len(groups))
	for i, g := range groups {
		names[i] = g.Name
	}
	gs := widget.NewCheckGroup(names, nil)
	gs.SetSelected(pv.meta.PortBelongsToCustom(e.Port))

	// 备注输入框
	ne := widget.NewEntry()
	ne.SetText(m.Note)
	ne.SetPlaceHolder("添加备注...")

	// 字数计数
	countLabel := widget.NewLabel(fmt.Sprintf("%d/%d", len([]rune(m.Note)), maxNoteLen))
	countLabel.Alignment = fyne.TextAlignTrailing
	countLabel.TextStyle.Italic = true

	// 实时截断 + 更新计数
	updateCount := func() {
		n := len([]rune(ne.Text))
		if n > maxNoteLen {
			ne.SetText(string([]rune(ne.Text)[:maxNoteLen]))
			n = maxNoteLen
		}
		countLabel.SetText(fmt.Sprintf("%d/%d", n, maxNoteLen))
	}
	ne.OnChanged = func(string) { updateCount() }

	var dlg dialog.Dialog

	// 用透明矩形强制弹窗最小宽度 420px
	wSpacer := canvas.NewRectangle(color.Transparent)
	wSpacer.SetMinSize(fyne.NewSize(420, 1))
	dlgContent := container.NewVBox(
		wSpacer,
		widget.NewForm(
			widget.NewFormItem("分组", gs),
		),
		ne,
		countLabel,
		widget.NewSeparator(),
		container.NewHBox(
			layout.NewSpacer(),
			widget.NewButton("取消", func() { dlg.Hide() }),
			widget.NewButton("保存", func() {
				note := strings.TrimSpace(strings.ReplaceAll(ne.Text, "\n", " "))
				if len([]rune(note)) > maxNoteLen {
					note = string([]rune(note)[:maxNoteLen])
				}
				if err := pv.meta.SaveNote(e.Port, note, gs.Selected); err != nil {
					dialog.ShowError(fmt.Errorf("保存备注失败: %w", err), pv.win)
					return
				}
				pv.applyFilter()
				dlg.Hide()
			}),
		),
	)
	dlg = dialog.NewCustomWithoutButtons(fmt.Sprintf("端口 %d — 备注", e.Port), dlgContent, pv.win)
	dlg.Show()
}
