# PortView - 端口查看器

Linux 桌面端口扫描与管理工具，基于 [Fyne](https://fyne.io/) GUI 框架。

扫描全部 65536 个端口，展示占用进程详情，支持分组、备注、筛选、终止进程。

## 功能

| 功能 | 说明 |
|------|------|
| 🔍 端口扫描 | 调用 `ss -tulnp` 扫描所有 TCP/UDP 端口 |
| 📋 进程详情 | CPU、内存、GPU 占用、命令行、状态 |
| 🎯 智能分组 | 自动分类（Web/数据库/SSH/系统/动态），支持自定义分组 |
| 🔎 实时筛选 | 按分组、协议、占用状态过滤，支持搜索端口/PID/进程名 |
| 📝 备注管理 | 为端口添加备注，持久化保存 |
| 🔫 终止进程 | 一键 kill 选中进程 |
| 📂 打开位置 | 打开进程可执行文件所在目录 |
| 💻 系统概览 | 顶部实时 CPU/内存/GPU 利用率 |

## 安装

### 前置要求

- Go 1.21+
- Linux（依赖 `/proc`、`ss`、可选 `nvidia-smi`）
- Fyne 依赖：`go mod download` 会自动处理

### 编译

```bash
git clone https://github.com/cqiang102/portview.git
cd portview
go build -ldflags="-s -w" -o portview .
./portview
```

### 交叉编译

```bash
# 当前系统
./build.sh linux

# macOS / Windows 需在对应系统上编译
```

## 使用

1. 启动后点击 **「刷新」** 扫描端口
2. 点击表格行选中端口
3. 工具栏按钮：详情 / 终止 / 位置 / 备注
4. 顶部下拉框按分组筛选，搜索框模糊搜索
5. **「分组管理」** 创建/编辑自定义端口分组

## 数据存储

备注和自定义分组保存在 `~/.portview/notes.json`（JSON 格式）。

## 技术栈

- [Fyne](https://fyne.io/) v2.8 — Go 跨平台 GUI
- Linux `/proc` — 进程信息
- `ss` — 端口列表
- `nvidia-smi` — GPU 信息（可选）

## 许可

MIT License
