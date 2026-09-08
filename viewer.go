// PortView
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
package main

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// doubleClickInterval 双击判定间隔
const doubleClickInterval = 350 * time.Millisecond

// ============================================================
// 应用主结构和 GUI
// ============================================================

type PortViewer struct {
	refreshing     bool
	refreshPending bool
	closed         bool
	occupiedFirst  bool
	groupDialog    dialog.Dialog

	entries   []PortEntry    // socket 条目及未观测到占用的端口
	filtered  []PortEntry    // 过滤/排序后的展示数据
	table     *widget.Table  // 端口列表表格
	status    *widget.Label  // 底部状态栏
	sysInfo   *widget.Label  // 系统信息（CPU/内存/GPU）
	win       fyne.Window    // 主窗口
	selRow    int            // 当前选中行（-1 = 未选中）
	meta      *PortMetaStore // 持久化存储
	lastClick time.Time      // 上次点击时间（双击检测用）
	groupSel  *widget.Select // 分组筛选下拉框
	searchBox *widget.Entry  // 搜索输入框
}

func safeDo(pv *PortViewer, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			pv.status.SetText(fmt.Sprintf("⚠️ 出错了: %v", r))
		}
	}()
	fn()
}
