// PortView
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func (pv *PortViewer) buildUI() {
	// ---- 表格定义 ----
	headers := []string{"端口", "协议", "PID", "进程名", "状态", "内存", "分组", "备注"}
	pv.table = widget.NewTable(
		// 行数 = 数据行 + 1 表头
		func() (int, int) { return len(pv.filtered) + 1, len(headers) },
		// 单元格模板
		func() fyne.CanvasObject {
			l := widget.NewLabel("  ")
			l.Alignment = fyne.TextAlignCenter
			l.Wrapping = fyne.TextTruncate // 超出列宽截断
			return l
		},
		// 单元格渲染：第 0 行为表头，其余为数据
		func(tci widget.TableCellID, co fyne.CanvasObject) {
			label := co.(*widget.Label)
			if tci.Row == 0 {
				label.TextStyle.Bold = true
				label.SetText(headers[tci.Col])
				return
			}
			label.TextStyle.Bold = false
			row := tci.Row - 1
			if row < 0 || row >= len(pv.filtered) {
				return
			}
			e := pv.filtered[row]
			occ := e.PID > 0 // 是否可读取进程信息
			switch tci.Col {
			case 0:
				label.SetText(fmtPort(e.Port))
			case 1:
				label.SetText(e.Protocol)
			case 2:
				if occ {
					label.SetText(strconv.Itoa(e.PID))
				} else {
					label.SetText("-")
				}
			case 3:
				if occ {
					label.SetText(e.ProcessName)
				} else {
					label.SetText("-")
				}
			case 4:
				label.SetText(e.Status)
			case 5:
				if occ {
					label.SetText(fmt.Sprintf("%.1f MB", e.MemoryMB))
				} else {
					label.SetText("-")
				}
			case 6:
				// 优先显示自定义分组，否则使用系统分组
				g := e.SysGroup()
				if cg := pv.meta.PortBelongsToCustom(e.Port); len(cg) > 0 {
					g = strings.Join(cg, ",")
				}
				label.SetText(g)
			case 7:
				m := pv.meta.Get(e.Port)
				if m.Note != "" {
					label.SetText("📝 " + truncateNote(m.Note, 25))
				} else {
					label.SetText("")
				}
			}
		},
	)

	// 设置列宽
	pv.table.SetColumnWidth(0, 100) // 端口
	pv.table.SetColumnWidth(1, 50)  // 协议
	pv.table.SetColumnWidth(2, 60)  // PID
	pv.table.SetColumnWidth(3, 150) // 进程名
	pv.table.SetColumnWidth(4, 65)  // 状态
	pv.table.SetColumnWidth(5, 80)  // 内存
	pv.table.SetColumnWidth(6, 130) // 分组
	pv.table.SetColumnWidth(7, 250) // 备注

	// ---- 行选择（单击选中 + 双击编辑/详情） ----
	pv.table.OnSelected = func(tci widget.TableCellID) {
		if tci.Row == 0 {
			pv.table.UnselectAll()
			return // 忽略表头点击
		}
		row := tci.Row - 1
		if row < 0 || row >= len(pv.filtered) {
			pv.table.UnselectAll()
			return
		}
		now := time.Now()
		// 双击检测：同一行、间隔 < doubleClickInterval
		// 说明：Fyne 的 Table.Select 对已选中的单元格不会再次触发 OnSelected，
		// 因此单击后必须立即取消选中，下一次点击同一行才能重新触发回调做双击检测。
		if pv.selRow == row && now.Sub(pv.lastClick) < doubleClickInterval {
			pv.table.UnselectAll()
			if tci.Col == 7 {
				pv.editNote() // 双击备注列 → 编辑备注
			} else {
				pv.showDetail() // 双击其他列 → 查看详情
			}
			pv.selRow = -1
			return
		}
		// 单击：记录选中并立即取消选中（让下次点击可触发双击检测），状态栏提示
		pv.selRow = row
		pv.lastClick = now
		pv.table.UnselectAll()
		pv.status.SetText(fmt.Sprintf("已选中: 端口 %s", fmtPort(pv.filtered[row].Port)))
	}

	// ---- 顶部按钮栏 ----
	refreshBtn := widget.NewButtonWithIcon("刷新", theme.ViewRefreshIcon(), func() {
		safeDo(pv, pv.refresh)
	})
	detailBtn := widget.NewButtonWithIcon("详情", theme.InfoIcon(), func() {
		safeDo(pv, pv.showDetail)
	})
	killBtn := widget.NewButtonWithIcon("终止", theme.CancelIcon(), func() {
		safeDo(pv, pv.killSelected)
	})
	openBtn := widget.NewButtonWithIcon("位置", theme.FolderOpenIcon(), func() {
		safeDo(pv, pv.openSelected)
	})
	noteBtn := widget.NewButtonWithIcon("备注", theme.DocumentCreateIcon(), func() {
		safeDo(pv, pv.editNote)
	})
	groupBtn := widget.NewButtonWithIcon("分组管理", theme.SettingsIcon(), func() {
		safeDo(pv, pv.manageGroups)
	})

	// 分组下拉筛选 + 搜索框
	pv.groupSel = widget.NewSelect([]string{"🏷️ 全部"}, func(string) {})
	pv.searchBox = widget.NewEntry()
	pv.searchBox.SetPlaceHolder("搜索端口/PID/进程名...")

	// 排序按钮
	sortPortBtn := widget.NewButton("端口↑", func() {
		safeDo(pv, func() {
			pv.occupiedFirst = false
			sortEntries(pv.entries, false)
			pv.applyFilter()
		})
	})
	sortOccBtn := widget.NewButton("占用↑", func() { safeDo(pv, pv.sortOccupied) })

	// 状态栏和系统信息
	pv.sysInfo = widget.NewLabel("")
	pv.sysInfo.TextStyle.Monospace = true
	pv.status = widget.NewLabel("就绪 — 点击「刷新」")
	pv.status.TextStyle.Italic = true

	// ---- 布局 ----
	btnRow := container.NewHBox(refreshBtn, detailBtn, killBtn, openBtn, noteBtn, groupBtn,
		widget.NewSeparator(), pv.groupSel, widget.NewSeparator())
	topBar := container.NewBorder(nil, nil, btnRow, nil, pv.searchBox)
	btnRow2 := container.NewHBox(sortPortBtn, sortOccBtn)

	content := container.NewBorder(
		container.NewVBox(topBar, btnRow2, widget.NewSeparator()),       // 顶部
		container.NewVBox(pv.sysInfo, widget.NewSeparator(), pv.status), // 底部
		nil, nil,
		container.NewPadded(pv.table), // 中央
	)
	pv.win.SetContent(content)

	initGroupSelect(pv)
}
