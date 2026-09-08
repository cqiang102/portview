// PortView
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
package main

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

// All viewer state belongs to the UI thread. Workers publish only through fyne.Do.
func (pv *PortViewer) refresh() {
	if pv.closed {
		return
	}
	if pv.refreshing {
		pv.refreshPending = true
		return
	}
	pv.refreshing = true
	pv.status.SetText("扫描中...")
	go func() {
		entries, err := getPorts()
		fyne.Do(func() {
			if pv.closed {
				return
			}
			if err != nil {
				pv.status.SetText("扫描失败")
				dialog.ShowError(err, pv.win)
				pv.finishRefresh()
				return
			}
			pv.entries = entries
			sortEntries(pv.entries, pv.occupiedFirst)
			pv.applyFilter()
			ports := make(map[int]bool)
			for _, e := range entries {
				if e.Occupied() {
					ports[e.Port] = true
				}
			}
			pv.status.SetText(fmt.Sprintf("共 65536 个端口号，%d 个观测到占用（未观测不代表可绑定）", len(ports)))
		})
		if err != nil {
			return
		}
		cpu, mem, gpu := getCPU(), getMem(), getGPU()
		info := fmt.Sprintf("💻 CPU: %s | 🧠 内存: %s", cpu, mem)
		if gpu != "" {
			info += " | " + gpu
		}
		fyne.Do(func() {
			if !pv.closed {
				pv.sysInfo.SetText(info)
				pv.finishRefresh()
			}
		})
	}()
}

func (pv *PortViewer) applyFilter() {
	// A row index has no identity once the data changes.
	pv.selRow = -1
	pv.lastClick = time.Time{}
	if pv.table == nil {
		return
	}
	pv.table.UnselectAll()
	pv.filtered = filterEntries(pv.entries, pv.meta.Groups(), pv.groupSel.Selected, pv.searchBox.Text)
	pv.table.Refresh()
	pv.status.SetText(fmt.Sprintf("显示 %d 条，选择已清除", len(pv.filtered)))
}
func (pv *PortViewer) sortOccupied() {
	pv.occupiedFirst = true
	sortEntries(pv.entries, true)
	pv.applyFilter()
}

func (pv *PortViewer) finishRefresh() {
	pv.refreshing = false
	if pv.refreshPending {
		pv.refreshPending = false
		pv.refresh()
	}
}
