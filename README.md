# PortView 端口查看器

跨平台端口扫描与进程管理工具，Go + Fyne 构建。

## 功能

- 扫描 0–65535 端口，识别 TCP/UDP 占用进程
- 进程详情：PID、内存、可执行路径
- Kill 进程、打开可执行文件位置
- 自定义分组（Web/数据库/SSH 等），端口备注持久化
- 按端口排序、按占用状态筛选
- 搜索过滤（端口号/PID/进程名）

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
```bash
# 打开 .dmg，拖入 Applications
```

### Windows
```bash
# 双击 portview.exe
```

## 开发

```bash
git clone git@github.com:cqiang102/portview.git
cd portview
go build -o portview .
./portview
```

需要 Go 1.23+ 及 Fyne 依赖（Linux 需 `libgl1-mesa-dev xorg-dev`）。

## 构建

```bash
go build -ldflags="-s -w" -o portview .

# 跨平台（需 mingw64）
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc go build -o portview.exe .
```

CI 通过 GitHub Actions 自动构建 `.deb/.rpm/.AppImage/.dmg/.exe`。

## 协议

Apache 2.0
