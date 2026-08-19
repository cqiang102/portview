// PortView - 端口扫描与进程管理工具
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
package main

import (
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// doubleClickInterval 双击判定间隔
const doubleClickInterval = 350 * time.Millisecond

// ============================================================
// 应用主结构和 GUI
// ============================================================

type PortViewer struct {
	entries   []PortEntry     // 完整扫描结果（65536 条）
	filtered  []PortEntry     // 过滤/排序后的展示数据
	table     *widget.Table   // 端口列表表格
	status    *widget.Label   // 底部状态栏
	sysInfo   *widget.Label   // 系统信息（CPU/内存/GPU）
	win       fyne.Window     // 主窗口
	selRow    int             // 当前选中行（-1 = 未选中）
	meta      *PortMetaStore  // 持久化存储
	lastClick time.Time       // 上次点击时间（双击检测用）
	groupSel  *widget.Select  // 分组筛选下拉框
	searchBox *widget.Entry   // 搜索输入框
}

func main() {
	// 创建应用，设置唯一 ID 和图标
	a := app.NewWithID("PortView")
	a.SetIcon(resourceIconPng)
	w := a.NewWindow("端口查看器")
	w.Resize(fyne.NewSize(1300, 760))

	// 加载持久化数据
	meta := &PortMetaStore{path: os.ExpandEnv("$HOME/.portview/notes.json")}
	meta.load()

	pv := &PortViewer{win: w, selRow: -1, meta: meta,
		entries:  make([]PortEntry, 0),
		filtered: make([]PortEntry, 0)}

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
			row := tci.Row - 1
			if row < 0 || row >= len(pv.filtered) {
				return
			}
			e := pv.filtered[row]
			occ := e.PID > 0 // 端口是否被占用
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
	pv.table.SetColumnWidth(0, 100)  // 端口
	pv.table.SetColumnWidth(1, 50)   // 协议
	pv.table.SetColumnWidth(2, 60)   // PID
	pv.table.SetColumnWidth(3, 150)  // 进程名
	pv.table.SetColumnWidth(4, 65)   // 状态
	pv.table.SetColumnWidth(5, 80)   // 内存
	pv.table.SetColumnWidth(6, 130)  // 分组
	pv.table.SetColumnWidth(7, 250)  // 备注

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
			sort.Slice(pv.entries, func(i, j int) bool {
				return pv.entries[i].Port < pv.entries[j].Port
			})
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
		container.NewVBox(topBar, btnRow2, widget.NewSeparator()), // 顶部
		container.NewVBox(pv.sysInfo, widget.NewSeparator(), pv.status), // 底部
		nil, nil,
		container.NewPadded(pv.table), // 中央
	)
	w.SetContent(content)

	// ---- 初始化 ----
	initGroupSelect(pv)    // 填充分组下拉选项
	updateSysInfo(pv.sysInfo) // 异步获取系统信息

	// 启动后自动刷新
	go func() {
		time.Sleep(100 * time.Millisecond)
		safeDo(pv, pv.refresh)
	}()

	w.ShowAndRun()
}

func safeDo(pv *PortViewer, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			pv.status.SetText(fmt.Sprintf("⚠️ 出错了: %v", r))
		}
	}()
	fn()
}

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
	for _, g := range pv.meta.data.CustomGroups {
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
	if !valid {
		pv.groupSel.SetSelected("🏷️ 全部")
	}
}

func (pv *PortViewer) manageGroups() {
	items := make([]fyne.CanvasObject, 0)
	items = append(items,
		widget.NewLabelWithStyle("自定义分组管理", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator())

	// 列出每个自定义分组
	for i, g := range pv.meta.data.CustomGroups {
		idx := i
		ports := make([]string, len(g.Ports))
		for j, p := range g.Ports {
			ports[j] = strconv.Itoa(p)
		}

		row := container.NewHBox(
			widget.NewLabel(fmt.Sprintf("🔖 %s (%d)", g.Name, len(g.Ports))),
			layout.NewSpacer(),
			widget.NewButton("编辑", func() { pv.editGroup(idx) }),
			widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
				dialog.ShowConfirm("删除", fmt.Sprintf("删除「%s」？", pv.meta.data.CustomGroups[idx].Name),
					func(ok bool) {
						if !ok {
							return
						}
						pv.meta.data.CustomGroups = append(
							pv.meta.data.CustomGroups[:idx], pv.meta.data.CustomGroups[idx+1:]...)
						if err := pv.meta.save(); err != nil {
							dialog.ShowError(fmt.Errorf("删除分组失败: %w", err), pv.win)
							return
						}
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
				pv.rebuildGroupList()
				pv.applyFilter()
				pv.status.SetText("已重置为默认分组")
			}, pv.win)
		}))

	scroll := container.NewVScroll(container.NewVBox(items...))
	scroll.SetMinSize(fyne.NewSize(420, 400))
	dialog.ShowCustom("分组管理", "关闭", scroll, pv.win)
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
			pv.meta.data.CustomGroups = append(pv.meta.data.CustomGroups, CustomGroup{Name: name, Ports: ports})
			if err := pv.meta.save(); err != nil {
				dialog.ShowError(fmt.Errorf("新增分组失败: %w", err), pv.win)
				return
			}
			pv.rebuildGroupList()
			pv.applyFilter()
		}, pv.win)
}

func (pv *PortViewer) editGroup(idx int) {
	g := pv.meta.data.CustomGroups[idx]
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
			sort.Ints(ports)
			pv.meta.data.CustomGroups[idx] = CustomGroup{Name: name, Ports: uniquePorts(ports)}
			if err := pv.meta.save(); err != nil {
				dialog.ShowError(fmt.Errorf("保存分组失败: %w", err), pv.win)
				return
			}
			pv.rebuildGroupList()
			pv.applyFilter()
		}, pv.win)
}

func (pv *PortViewer) applyFilter() {
	if pv.table == nil {
		return
	}
	sel := pv.groupSel.Selected                         // 分组选择
	q := strings.ToLower(strings.TrimSpace(pv.searchBox.Text)) // 搜索关键词

	// 判断是否选中了自定义分组
	var customTarget *CustomGroup
	for _, g := range pv.meta.data.CustomGroups {
		if "🔖 "+g.Name == sel {
			customTarget = &g
			break
		}
	}

	pv.filtered = make([]PortEntry, 0, len(pv.entries))
	for _, e := range pv.entries {
		// 分组筛选
		if sel != "🏷️ 全部" {
			switch {
			case sel == "📌 已占用":
				if e.PID <= 0 {
					continue
				}
			case sel == "🅰 TCP":
				if e.Protocol != "" && e.Protocol != "tcp" && e.Protocol != "tcp6" && e.Protocol != "-" {
					continue
				}
			case sel == "🅱 UDP":
				if e.Protocol != "" && e.Protocol != "udp" && e.Protocol != "udp6" && e.Protocol != "-" {
					continue
				}
			case sel == "⚙️ 系统(占用)":
				if e.PID <= 0 || e.Port > 1023 {
					continue
				}
			case sel == "🌐 Web":
				if e.PID <= 0 || !matchAny(e.Port, 80, 443, 8080, 8443, 3000, 5000, 8000, 8888, 9090) {
					continue
				}
			case sel == "💾 数据库":
				if e.PID <= 0 || !matchAny(e.Port, 3306, 5432, 6379, 27017, 1433, 1521, 9042) {
					continue
				}
			case sel == "🔐 SSH":
				if e.Port != 22 {
					continue
				}
			case sel == "🔁 动态":
				if e.Port < 49152 {
					continue
				}
			case customTarget != nil:
				found := false
				for _, p := range customTarget.Ports {
					if e.Port == p {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
		}

		// 关键词搜索（匹配端口号、PID、进程名、状态）
		if q != "" {
			ps, pids := strconv.Itoa(e.Port), strconv.Itoa(e.PID)
			if !strings.Contains(ps, q) && !strings.Contains(pids, q) &&
				!strings.Contains(strings.ToLower(e.ProcessName), q) &&
				!strings.Contains(strings.ToLower(e.Status), q) {
				continue
			}
		}
		pv.filtered = append(pv.filtered, e)
	}
	pv.table.Refresh()
}

func (pv *PortViewer) editNote() {
	if pv.selRow < 0 || pv.selRow >= len(pv.filtered) {
		dialog.ShowInformation("提示", "请先选择一行", pv.win)
		return
	}
	e := pv.filtered[pv.selRow]
	m := pv.meta.Get(e.Port)

	// 分组下拉（含 "(无)" 选项）
	names := make([]string, len(pv.meta.data.CustomGroups))
	for i, g := range pv.meta.data.CustomGroups {
		names[i] = g.Name
	}
	gs := widget.NewSelect(append([]string{"(无)"}, names...), nil)
	if m.Group != "" {
		gs.SetSelected(m.Group)
	} else {
		gs.SetSelected("(无)")
	}

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
				g := gs.Selected
				if g == "(无)" {
					g = ""
				}
				note := strings.TrimSpace(strings.ReplaceAll(ne.Text, "\n", " "))
				if len([]rune(note)) > maxNoteLen {
					note = string([]rune(note)[:maxNoteLen])
				}
				pv.meta.Set(e.Port, PortMeta{Group: g, Note: note})
				if err := pv.meta.save(); err != nil {
					dialog.ShowError(fmt.Errorf("保存备注失败: %w", err), pv.win)
					return
				}
				pv.table.Refresh()
				dlg.Hide()
			}),
		),
	)
	dlg = dialog.NewCustomWithoutButtons(fmt.Sprintf("端口 %d — 备注", e.Port), dlgContent, pv.win)
	dlg.Show()
}

func updateSysInfo(label *widget.Label) {
	go func() {
		cpu := getCPU()
		mem := getMem()
		gpu := getGPU()
		g := ""
		if gpu != "" {
			g = " | " + gpu
		}
		label.SetText(fmt.Sprintf("💻 CPU: %s | 🧠 内存: %s%s", cpu, mem, g))
	}()
}

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
		msg := fmt.Sprintf("端口: %d\n分组: %s\n状态: 空闲", e.Port, e.SysGroup())
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
	info := readProcess(pid)    // 从 /proc 读取
	gpu := readProcessGPU(pid)  // GPU 显存使用
	cmdline := readCmdline(pid) // 完整命令行

	msg := fmt.Sprintf("进程: %s (PID %d)\n%s\n%s路径: %s\n命令行: %s",
		e.ProcessName, pid, info, gpu, e.ExePath, cmdline)
	if m.Note != "" {
		msg = "📝 " + m.Note + "\n\n" + msg
	}
	content := container.NewVBox(
		widget.NewLabel(msg),
		widget.NewSeparator(),
		container.NewHBox(
			layout.NewSpacer(),
			widget.NewButton("终止进程", func() {
				if err := killProcess(pid); err != nil {
					dialog.ShowError(fmt.Errorf("失败: %w", err), pv.win)
					return
				}
				pv.refresh()
			}),
		),
	)
	dialog.ShowCustom(fmt.Sprintf("端口 %d", e.Port), "关闭", content, pv.win)
}

func (pv *PortViewer) refresh() {
	pv.status.SetText("扫描中...")
	entries, err := getPorts()
	if err != nil {
		dialog.ShowError(err, pv.win)
		return
	}
	pv.entries = entries
	pv.sortOccupied() // 默认按"占用在前"排序
	pv.applyFilter()
	updateSysInfo(pv.sysInfo)
	occ := 0
	for _, e := range entries {
		if e.PID > 0 {
			occ++
		}
	}
	msg := fmt.Sprintf("共 65536 个端口，%d 个被占用", occ)
	if runtime.GOOS == "linux" && os.Geteuid() != 0 {
		msg += " ⚠️ 非 root 运行，可能看不到部分进程信息，建议用 sudo 运行"
	}
	pv.status.SetText(msg)
}

func (pv *PortViewer) killSelected() {
	if pv.selRow < 0 || pv.selRow >= len(pv.filtered) {
		dialog.ShowInformation("提示", "请先选择一行", pv.win)
		return
	}
	e := pv.filtered[pv.selRow]
	if e.PID <= 0 {
		dialog.ShowInformation("提示", "端口空闲", pv.win)
		return
	}
	dialog.ShowConfirm("确认终止",
		fmt.Sprintf("终止「%s」(PID %d)？", e.ProcessName, e.PID),
		func(ok bool) {
			if !ok {
				return
			}
			if err := killProcess(e.PID); err != nil {
				dialog.ShowError(fmt.Errorf("失败: %w", err), pv.win)
				return
			}
			pv.refresh()
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

func (pv *PortViewer) sortOccupied() {
	sort.Slice(pv.entries, func(i, j int) bool {
		io, jo := pv.entries[i].PID > 0, pv.entries[j].PID > 0
		if io != jo {
			return io // 占用的排前面
		}
		return pv.entries[i].Port < pv.entries[j].Port
	})
	pv.applyFilter()
	pv.status.SetText("已按「占用在前」排序")
}
