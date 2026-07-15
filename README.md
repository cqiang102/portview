# PortView 端口查看器

跨平台端口扫描与进程管理工具，Go + Fyne 构建。

> 🤖 Vibe Coding — 全程由 DeepSeek 辅助编码

## 功能

- 扫描 0–65535 端口，识别 TCP/UDP 占用进程
- 进程详情：PID、内存、可执行路径
- Kill 进程、打开可执行文件位置
- 自定义分组管理，端口备注持久化
- 按端口排序、按占用状态/协议筛选
- 关键词搜索（端口号 / PID / 进程名）

## 安装

### Linux

```bash
# .deb
sudo dpkg -i portview_*.deb

# .rpm
sudo rpm -i portview-*.rpm

# AppImage
chmod +x portview-*.AppImage && ./portview-*.AppImage
```

### macOS

打开 `.dmg`，拖入 Applications。

### Windows

双击 `portview.exe`。

## 开发

```bash
git clone git@github.com:cqiang102/portview.git
cd portview
go build -o portview .
./portview
```

依赖：Go 1.23+、Fyne v2、Linux 需 `libgl1-mesa-dev xorg-dev`。

## 构建与打包

```bash
# 本地构建
go build -ldflags="-s -w" -o portview .

# Windows 交叉编译（Linux 上需 mingw64）
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc \
  go build -ldflags="-s -w -H windowsgui" -o portview.exe .
```

CI 通过 GitHub Actions 自动构建：

| 平台 | 产物 | 架构 |
|------|------|------|
| Linux | `.deb` `.rpm` `.AppImage` | x86_64 |
| macOS | `.dmg` | arm64 |
| Windows | `.exe` | x86_64 |

## 协议

Apache 2.0 © lacia.cq@qq.com
