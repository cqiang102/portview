// PortView
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
package main

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func initGroupSelect(pv *PortViewer) {
	options := buildGroupOptions(pv)
	pv.groupSel.Options = options
	if len(options) > 0 {
		pv.groupSel.SetSelected("🏷️ 全部")
	}
	pv.groupSel.OnChanged = func(s string) { pv.applyFilter() }
	pv.searchBox.OnChanged = func(string) { pv.applyFilter() }
}

func buildGroupOptions(pv *PortViewer) []string {
	out := []string{"🏷️ 全部", "📌 已占用", "🅰 TCP", "🅱 UDP",
		"⚙️ 系统(占用)", "🌐 Web", "💾 数据库", "🔐 SSH", "🔁 动态"}
	for _, g := range pv.meta.Groups() {
		out = append(out, "🔖 "+g.Name)
	}
	return out
}

func (pv *PortViewer) rebuildGroupList() {
	options := buildGroupOptions(pv)
	pv.groupSel.Options = options
	cur := pv.groupSel.Selected
	valid := false
	for _, o := range options {
		if o == cur {
			valid = true
			break
		}
	}
	pv.groupSel.Refresh()
	if !valid {
		pv.groupSel.SetSelected("🏷️ 全部")
	}
}

func (pv *PortViewer) manageGroups() {
	if pv.groupDialog != nil {
		pv.groupDialog.Hide()
	}
	items := make([]fyne.CanvasObject, 0)
	items = append(items,
		widget.NewLabelWithStyle("自定义分组管理", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator())

	// 列出每个自定义分组
	for _, g := range pv.meta.Groups() {

		row := container.NewHBox(
			widget.NewLabel(fmt.Sprintf("🔖 %s (%d)", g.Name, len(g.Ports))),
			layout.NewSpacer(),
			widget.NewButton("编辑", func() { pv.editGroup(g) }),
			widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
				dialog.ShowConfirm("删除", fmt.Sprintf("删除「%s」？", g.Name),
					func(ok bool) {
						if !ok {
							return
						}
						if err := pv.meta.DeleteGroup(g.Name); err != nil {
							dialog.ShowError(fmt.Errorf("删除分组失败: %w", err), pv.win)
							return
						}
						pv.manageGroups()
						pv.rebuildGroupList()
						pv.applyFilter()
					}, pv.win)
			}),
		)
		items = append(items, row, widget.NewSeparator())
	}

	// 新增 + 重置按钮
	items = append(items, widget.NewSeparator(),
		widget.NewButtonWithIcon("➕ 新增分组", theme.ContentAddIcon(), func() { pv.addGroup() }),
		widget.NewButtonWithIcon("🔄 重置为默认", theme.ViewRefreshIcon(), func() {
			dialog.ShowConfirm("重置", "清除所有自定义分组和备注？", func(ok bool) {
				if !ok {
					return
				}
				if err := pv.meta.ResetAll(); err != nil {
					dialog.ShowError(fmt.Errorf("重置失败: %w", err), pv.win)
					return
				}
				pv.manageGroups()
				pv.rebuildGroupList()
				pv.applyFilter()
				pv.status.SetText("已重置为默认分组")
			}, pv.win)
		}))

	scroll := container.NewVScroll(container.NewVBox(items...))
	scroll.SetMinSize(fyne.NewSize(420, 400))
	pv.groupDialog = dialog.NewCustom("分组管理", "关闭", scroll, pv.win)
	pv.groupDialog.Show()
}

func (pv *PortViewer) addGroup() {
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("分组名 (如: 我的服务)")
	portsEntry := widget.NewEntry()
	portsEntry.SetPlaceHolder("端口号，逗号或范围 (如: 3000,5000,8000-8010)")
	dialog.ShowForm("新增分组", "创建", "取消",
		[]*widget.FormItem{{Text: "名称", Widget: nameEntry}, {Text: "端口", Widget: portsEntry}},
		func(ok bool) {
			if !ok {
				return
			}
			name := strings.TrimSpace(nameEntry.Text)
			ports := parsePorts(portsEntry.Text)
			if name == "" || len(ports) == 0 {
				return
			}
			if err := pv.meta.SaveGroup("", CustomGroup{Name: name, Ports: ports}); err != nil {
				dialog.ShowError(fmt.Errorf("新增分组失败: %w", err), pv.win)
				return
			}
			pv.manageGroups()
			pv.rebuildGroupList()
			pv.applyFilter()
		}, pv.win)
}

func (pv *PortViewer) editGroup(g CustomGroup) {
	nameEntry := widget.NewEntry()
	nameEntry.SetText(g.Name)
	ps := make([]string, len(g.Ports))
	for i, p := range g.Ports {
		ps[i] = strconv.Itoa(p)
	}
	portsEntry := widget.NewEntry()
	portsEntry.SetText(strings.Join(ps, ","))

	dialog.ShowForm(fmt.Sprintf("编辑「%s」", g.Name), "保存", "取消",
		[]*widget.FormItem{{Text: "名称", Widget: nameEntry}, {Text: "端口", Widget: portsEntry}},
		func(ok bool) {
			if !ok {
				return
			}
			name := strings.TrimSpace(nameEntry.Text)
			ports := parsePorts(portsEntry.Text)
			if name == "" {
				return
			}
			if err := pv.meta.SaveGroup(g.Name, CustomGroup{Name: name, Ports: ports}); err != nil {
				dialog.ShowError(fmt.Errorf("保存分组失败: %w", err), pv.win)
				return
			}
			pv.manageGroups()
			pv.rebuildGroupList()
			pv.applyFilter()
		}, pv.win)
}
