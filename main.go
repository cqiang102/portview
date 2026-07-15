// PortView - 端口扫描与进程管理工具
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
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

// ============================================================
// 数据结构 — 端口元信息、自定义分组、持久化
// ============================================================

// PortMeta 端口的备注和所属自定义分组
type PortMeta struct {
	Group string `json:"group"` // 所属自定义分组名
	Note  string `json:"note"`  // 备注文本，最长 100 字符
}

// CustomGroup 用户自定义端口分组
type CustomGroup struct {
	Name  string `json:"name"`  // 分组名称
	Ports []int  `json:"ports"` // 包含的端口列表
}

// StoreData 持久化到磁盘的完整数据结构
type StoreData struct {
	CustomGroups []CustomGroup    `json:"custom_groups"` // 自定义分组列表
	PortNotes    map[int]PortMeta `json:"port_notes"`    // 端口→备注映射
}

// defaultGroups 返回内置的默认分组（Web、数据库、SSH 等）
func defaultGroups() []CustomGroup {
	return []CustomGroup{
		{Name: "🌐 Web服务", Ports: []int{80, 443, 8080, 8443, 3000, 5000, 8000, 8888, 9090}},
		{Name: "💾 数据库", Ports: []int{3306, 5432, 6379, 27017, 1433, 1521, 9042}},
		{Name: "🔐 远程访问", Ports: []int{22, 3389, 5900, 5901, 6000, 6001}},
		{Name: "📧 邮件服务", Ports: []int{25, 110, 143, 587, 993, 995}},
		{Name: "🛠️ 开发工具", Ports: []int{5173, 5174, 24678, 9229, 30000}},
		{Name: "📡 网络服务", Ports: []int{53, 67, 68, 69, 123, 389, 636}},
	}
}

// PortMetaStore 线程安全的分组和备注存储，序列化为 JSON
type PortMetaStore struct {
	mu   sync.RWMutex // 读写锁
	data StoreData    // 内存数据
	path string       // JSON 文件路径
}

// load 从磁盘加载数据，首次使用时自动创建默认分组
func (s *PortMetaStore) load() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = StoreData{}
	d, err := os.ReadFile(s.path)
	if err != nil {
		// 首次使用，创建默认分组
		s.data.CustomGroups = defaultGroups()
		s.data.PortNotes = make(map[int]PortMeta)
		s.save()
		return
	}
	// 尝试新格式（含 CustomGroups）
	if err := json.Unmarshal(d, &s.data); err != nil {
		// 旧格式兼容：仅有 port→PortMeta 的 map
		old := make(map[int]PortMeta)
		if err2 := json.Unmarshal(d, &old); err2 == nil {
			s.data.PortNotes = old
		}
		s.data.CustomGroups = defaultGroups()
	}
	if s.data.PortNotes == nil {
		s.data.PortNotes = make(map[int]PortMeta)
	}
}

// save 将数据写回磁盘（JSON 格式）
func (s *PortMetaStore) save() {
	os.MkdirAll(filepath.Dir(s.path), 0755)
	d, _ := json.MarshalIndent(s.data, "", "  ")
	os.WriteFile(s.path, d, 0644)
}

// Get 读取某个端口的备注
func (s *PortMetaStore) Get(port int) PortMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.PortNotes[port]
}

// Set 写入某个端口的备注
func (s *PortMetaStore) Set(port int, m PortMeta) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.PortNotes[port] = m
}

// PortBelongsToCustom 返回某端口所属的所有自定义分组名
func (s *PortMetaStore) PortBelongsToCustom(port int) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []string
	for _, g := range s.data.CustomGroups {
		for _, p := range g.Ports {
			if p == port {
				out = append(out, g.Name)
				break
			}
		}
	}
	return out
}

// ResetAll 清除所有自定义分组和备注，恢复默认
func (s *PortMetaStore) ResetAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = StoreData{
		CustomGroups: defaultGroups(),
		PortNotes:    make(map[int]PortMeta),
	}
	s.save()
}

// ============================================================
// 端口条目模型
// ============================================================

// PortEntry 单个端口的信息
type PortEntry struct {
	Port        int     // 端口号 0-65535
	Protocol    string  // 协议：tcp/tcp6/udp/udp6
	PID         int     // 占用进程 PID，0 表示空闲
	ProcessName string  // 进程名
	Status      string  // 连接状态：LISTEN/ESTABLISHED/空闲
	MemoryMB    float64 // 进程 RSS 内存（MB）
	ExePath     string  // 可执行文件路径
	LocalAddr   string  // 本地地址（含 IP）
}

// SysGroup 根据端口号和状态自动判断系统分组
func (e *PortEntry) SysGroup() string {
	// 被占用端口按端口号分类
	if e.PID > 0 {
		switch {
		case e.Port == 22:
			return "SSH"
		case e.Port == 80 || e.Port == 443 || e.Port == 8080 || e.Port == 8443:
			return "Web"
		case e.Port == 3306 || e.Port == 5432 || e.Port == 6379 || e.Port == 27017:
			return "数据库"
		case e.Port == 53:
			return "DNS"
		case e.Port <= 1023:
			return "系统"
		default:
			return "应用"
		}
	}
	// 空闲端口按范围分类：系统(0-1023) / 注册(1024-49151) / 动态(49152+)
	if e.Port <= 1023 {
		return "系统"
	}
	if e.Port <= 49151 {
		return "注册"
	}
	return "动态"
}

// ============================================================
// 应用主结构和 GUI
// ============================================================

// PortViewer 应用核心结构，持有所有状态和 UI 引用
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

	// 创建窗口："端口查看器"
	// 注：Wayland 上 NewWindow("")+显式 SetTitle 优于直接传中文标题，
	//     避免 GLFW 初始化时序导致 dock 显示"未知"
	w := a.NewWindow("")
	w.Resize(fyne.NewSize(1300, 760))
	w.SetTitle("端口查看器")

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
		now := time.Now()
		// 双击检测：同一行、间隔 < 350ms
		if pv.selRow+1 == tci.Row && now.Sub(pv.lastClick) < 350*time.Millisecond {
			pv.table.UnselectAll()
			if tci.Col == 7 {
				pv.editNote() // 双击备注列 → 编辑备注
			} else {
				pv.showDetail() // 双击其他列 → 查看详情
			}
			return
		}
		// 单击：立即取消选中（让下次点击可触发双击检测），更新选中状态
		pv.selRow = tci.Row - 1
		pv.lastClick = now
		pv.table.UnselectAll()
		pv.status.SetText(fmt.Sprintf("已选中: 端口 %s", fmtPort(pv.filtered[pv.selRow].Port)))
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

// safeDo 统一 panic 恢复，避免单次操作崩溃导致程序退出
func safeDo(pv *PortViewer, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			pv.status.SetText(fmt.Sprintf("⚠️ 出错了: %v", r))
		}
	}()
	fn()
}

// ============================================================
// 分组管理逻辑
// ============================================================

// initGroupSelect 初始化分组下拉列表并绑定筛选事件
func initGroupSelect(pv *PortViewer) {
	options := buildGroupOptions(pv)
	pv.groupSel.Options = options
	if len(options) > 0 {
		pv.groupSel.SetSelected("🏷️ 全部")
	}
	pv.groupSel.OnChanged = func(s string) { pv.applyFilter() }
	pv.searchBox.OnChanged = func(string) { pv.applyFilter() }
}

// buildGroupOptions 构建分组下拉的所有选项
func buildGroupOptions(pv *PortViewer) []string {
	out := []string{"🏷️ 全部", "📌 已占用", "🅰 TCP", "🅱 UDP",
		"⚙️ 系统(占用)", "🌐 Web", "💾 数据库", "🔐 SSH", "🔁 动态"}
	for _, g := range pv.meta.data.CustomGroups {
		out = append(out, "🔖 "+g.Name)
	}
	return out
}

// rebuildGroupList 分组变更后重建下拉列表
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

// ============================================================
// 分组管理弹窗
// ============================================================

// manageGroups 打开分组管理弹窗，显示所有自定义分组
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
						pv.meta.save()
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
				pv.meta.ResetAll()
				pv.rebuildGroupList()
				pv.applyFilter()
				pv.status.SetText("已重置为默认分组")
			}, pv.win)
		}))

	scroll := container.NewVScroll(container.NewVBox(items...))
	scroll.SetMinSize(fyne.NewSize(420, 400))
	dialog.ShowCustom("分组管理", "关闭", scroll, pv.win)
}

// addGroup 弹出新增分组表单
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
			pv.meta.save()
			pv.rebuildGroupList()
			pv.applyFilter()
		}, pv.win)
}

// editGroup 弹出编辑分组表单
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
			pv.meta.save()
			pv.rebuildGroupList()
			pv.applyFilter()
		}, pv.win)
}

// parsePorts 解析端口字符串，支持逗号分隔和范围 (如 "3000,5000,8000-8010")
func parsePorts(s string) []int {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// 范围：8000-8010
		if strings.Contains(part, "-") {
			r := strings.SplitN(part, "-", 2)
			s, e := atoi(strings.TrimSpace(r[0])), atoi(strings.TrimSpace(r[1]))
			if s > 0 && e > 0 && s <= e && e <= 65535 {
				for p := s; p <= e; p++ {
					out = append(out, p)
				}
			}
		} else if p := atoi(part); p > 0 && p <= 65535 {
			out = append(out, p)
		}
	}
	return uniquePorts(out)
}

// uniquePorts 去重并排序端口列表
func uniquePorts(ports []int) []int {
	seen := make(map[int]bool)
	var out []int
	for _, p := range ports {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Ints(out)
	return out
}

// ============================================================
// 搜索与筛选
// ============================================================

// applyFilter 根据分组选择和搜索关键词过滤端口列表
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

// matchAny 检查值是否在目标列表中
func matchAny(p int, targets ...int) bool {
	for _, t := range targets {
		if p == t {
			return true
		}
	}
	return false
}

// ============================================================
// 备注编辑
// ============================================================

const maxNoteLen = 100 // 备注最大字符数（按 rune 计，支持中文）

// truncateNote 截断备注文本，超出部分用 ... 替代
func truncateNote(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "..."
}

// editNote 打开备注编辑弹窗
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
				pv.meta.save()
				pv.table.Refresh()
				dlg.Hide()
			}),
		),
	)
	dlg = dialog.NewCustomWithoutButtons(fmt.Sprintf("端口 %d — 备注", e.Port), dlgContent, pv.win)
	dlg.Show()
}

// ============================================================
// 系统信息（CPU / 内存 / GPU）
// ============================================================

// updateSysInfo 异步获取并更新系统信息显示
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

// getCPU 从 /proc/stat 读取 CPU 使用率
func getCPU() string {
	d, _ := os.ReadFile("/proc/stat")
	for _, line := range strings.Split(string(d), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 5 {
			break
		}
		t, id := 0, 0
		for i, v := range f[1:] {
			n, _ := strconv.Atoi(v)
			t += n
			if i == 3 {
				id = n
			}
		}
		if t > 0 {
			return fmt.Sprintf("%.1f%%", float64(t-id)/float64(t)*100)
		}
	}
	return "N/A"
}

// getMem 从 /proc/meminfo 读取内存使用率
func getMem() string {
	d, _ := os.ReadFile("/proc/meminfo")
	t, a := 0, 0
	for _, line := range strings.Split(string(d), "\n") {
		fmt.Sscanf(line, "MemTotal: %d kB", &t)
		fmt.Sscanf(line, "MemAvailable: %d kB", &a)
	}
	if t == 0 {
		return "N/A"
	}
	u := t - a
	return fmt.Sprintf("%.1f%% (%d/%d GB)", float64(u)/float64(t)*100, u/1024/1024, t/1024/1024)
}

// getGPU 通过 nvidia-smi 读取 GPU 信息
func getGPU() string {
	out, _ := exec.Command("nvidia-smi",
		"--query-gpu=utilization.gpu,memory.used,memory.total,temperature.gpu",
		"--format=csv,noheader,nounits").Output()
	p := strings.Split(strings.TrimSpace(string(out)), ", ")
	if len(p) < 3 {
		return ""
	}
	return fmt.Sprintf("GPU: %s%% | %s/%s MB | %s°C", p[0], p[1], p[2], p[3])
}

// ============================================================
// 进程详情弹窗
// ============================================================

// showDetail 打开进程详情弹窗（双击非备注列触发）
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
				exec.Command("kill", "-9", strconv.Itoa(pid)).Run()
				pv.refresh()
			}),
		),
	)
	dialog.ShowCustom(fmt.Sprintf("端口 %d", e.Port), "关闭", content, pv.win)
}

// readProcess 从 /proc/[pid]/stat 读取进程状态、CPU、内存
func readProcess(pid int) string {
	d, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "状态: 已结束或无权限"
	}
	f := strings.Fields(string(d))
	if len(f) < 24 {
		return ""
	}

	// 进程状态映射
	st := map[string]string{
		"R": "运行中", "S": "休眠", "D": "不可中断",
		"Z": "僵尸", "T": "已停止",
	}[f[2]]
	if st == "" {
		st = f[2]
	}

	ut, _ := strconv.Atoi(f[13])  // 用户态 CPU 时间
	sti, _ := strconv.Atoi(f[14]) // 内核态 CPU 时间
	rss, _ := strconv.Atoi(f[23]) // RSS 页数（每页 4KB）
	nice, _ := strconv.Atoi(f[18])
	thr, _ := strconv.Atoi(f[19]) // 线程数
	rssM := float64(rss*4) / 1024 // RSS 转 MB

	// CPU 使用率 = (ut+sti) / uptime 秒
	ud, _ := os.ReadFile("/proc/uptime")
	us := 0.0
	fmt.Sscanf(string(ud), "%f", &us)
	cpuP := 0.0
	if us > 0 {
		cpuP = float64(ut+sti) / 100 / us * 100
	}

	// 内存占比
	mt := uint64(0)
	if d2, _ := os.ReadFile("/proc/meminfo"); d2 != nil {
		for _, l := range strings.Split(string(d2), "\n") {
			fmt.Sscanf(l, "MemTotal: %d kB", &mt)
		}
	}
	mp := 0.0
	if mt > 0 {
		mp = float64(rss*4) / float64(mt) * 100
	}

	return fmt.Sprintf("状态: %s | CPU: %.2f%% | 内存: %.1f MB (%.2f%%) | 优先级: %d | 线程: %d",
		st, cpuP, rssM, mp, nice, thr)
}

// readProcessGPU 通过 nvidia-smi 查看某进程的 GPU 显存使用
func readProcessGPU(pid int) string {
	out, _ := exec.Command("nvidia-smi",
		"--query-compute-apps=pid,used_memory,name",
		"--format=csv,noheader,nounits").Output()
	ps := strconv.Itoa(pid)
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, ps+",") {
			continue
		}
		p := strings.SplitN(line, ", ", 3)
		if len(p) == 3 {
			return fmt.Sprintf("GPU显存: %s MB (%s)", p[1], p[2]) + "\n"
		}
	}
	return ""
}

// readCmdline 读取 /proc/[pid]/cmdline 并替换 \x00 为空格
func readCmdline(pid int) string {
	d, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	return strings.ReplaceAll(strings.TrimSpace(string(d)), "\x00", " ")
}

// ============================================================
// 端口扫描（ss + /proc）
// ============================================================

// refresh 重新扫描端口并更新 UI
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
	pv.status.SetText(fmt.Sprintf("共 65536 个端口，%d 个被占用", occ))
}

// getPorts 调用 ss -tulnp 扫描所有监听端口，补全空闲端口
func getPorts() ([]PortEntry, error) {
	raw, err := execCmd("ss", "-tulnp")
	if err != nil {
		return nil, fmt.Errorf("ss 失败: %w", err)
	}
	seen := make(map[int]bool)
	entries := make([]PortEntry, 0, 100)

	// 解析 ss 输出：提取进程名和 PID
	re := regexp.MustCompile(`"([^"]+)".*pid=(\d+)`)

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Netid") {
			continue // 跳过表头
		}
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		addr := f[4] // 本地地址，如 0.0.0.0:8080 或 [::]:22
		idx := strings.LastIndex(addr, ":")
		if idx < 0 {
			continue
		}
		port := atoi(addr[idx+1:])
		if port == 0 {
			continue
		}

		// 提取进程名和 PID
		pn, pid := "", 0
		if len(f) > 5 {
			if m := re.FindStringSubmatch(strings.Join(f[5:], " ")); len(m) >= 3 {
				pn, pid = m[1], atoi(m[2])
			}
		}
		seen[port] = true

		// 读取 exe 路径和 comm（进程名备用）
		ep := ""
		if pid > 0 {
			ep, _ = os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
			if pn == "" {
				if d, _ := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid)); len(d) > 0 {
					pn = strings.TrimSpace(string(d))
				}
			}
			if pn == "" {
				pn = fmt.Sprintf("PID:%d", pid)
			}
		}

		// 从 /proc/[pid]/statm 读取 RSS 内存
		memMB := 0.0
		if pid > 0 {
			if d, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid)); err == nil {
				f := strings.Fields(string(d))
				if len(f) >= 2 {
					rss, _ := strconv.Atoi(f[1])
					memMB = float64(rss) * 4096 / 1024 / 1024
				}
			}
		}

		entries = append(entries, PortEntry{
			Port: port, Protocol: f[0], PID: pid,
			ProcessName: pn, Status: f[1],
			MemoryMB: memMB, ExePath: ep, LocalAddr: addr,
		})
	}

	// 补全未出现在 ss 输出中的空闲端口
	for p := 0; p <= 65535; p++ {
		if !seen[p] {
			entries = append(entries, PortEntry{Port: p, Status: "空闲"})
		}
	}
	return entries, nil
}

// execCmd 执行命令并返回 stdout
func execCmd(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return string(out), err
}

// ============================================================
// 用户操作（终止进程、打开位置、排序）
// ============================================================

// killSelected 终止当前选中端口的进程（SIGKILL）
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
			if err := exec.Command("kill", "-9", strconv.Itoa(e.PID)).Run(); err != nil {
				dialog.ShowError(fmt.Errorf("失败: %w", err), pv.win)
				return
			}
			pv.refresh()
		}, pv.win)
}

// openSelected 用系统文件管理器打开当前进程的可执行文件目录
func (pv *PortViewer) openSelected() {
	if pv.selRow < 0 || pv.selRow >= len(pv.filtered) {
		return
	}
	e := pv.filtered[pv.selRow]
	if e.ExePath == "" {
		dialog.ShowInformation("提示", "无路径", pv.win)
		return
	}
	exec.Command("xdg-open", filepath.Dir(e.ExePath)).Start()
}

// sortOccupied 按"占用在前、端口号升序"排列
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

// atoi 字符串转 int，失败返回 0
func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// fmtPort 格式化端口号，知名端口显示名称
func fmtPort(p int) string {
	// 知名端口映射
	m := map[int]string{
		22: "SSH", 80: "HTTP", 443: "HTTPS", 3306: "MySQL",
		5432: "PG", 6379: "Redis", 8080: "HTTP-alt", 27017: "Mongo",
		53: "DNS", 25: "SMTP", 3389: "RDP",
	}
	if p == 0 {
		return "0 (保留)"
	}
	if n, ok := m[p]; ok {
		return fmt.Sprintf("%d (%s)", p, n)
	}
	if p < 1024 {
		return strconv.Itoa(p)
	}
	return strconv.Itoa(p)
}
