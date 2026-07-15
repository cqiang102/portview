#!/bin/bash
# PortView 交叉编译脚本
# 用法: ./build.sh [linux|darwin|windows|all]

export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"
export GOPROXY=https://goproxy.cn,direct
cd "$(dirname "$0")"

OS="${1:-linux}"

build() {
    local os=$1 arch=$2 suffix=$3
    echo "编译 $os/$arch ..."
    CGO_ENABLED=1 GOOS=$os GOARCH=$arch \
        go build -ldflags="-s -w" -o "portview-${os}-${arch}${suffix}" .
}

case "$OS" in
    linux)
        build linux amd64 ""
        ;;
    darwin)
        # macOS 需要 CGO，但交叉编译需要 macOS SDK，建议在 Mac 上直接编译
        echo "macOS 建议在 Mac 上直接编译: GOOS=darwin GOARCH=amd64 go build"
        echo "或者安装 osxcross 工具链后取消下面注释"
        # build darwin amd64 ""
        ;;
    windows)
        # Windows 交叉编译需要 MinGW
        echo "Windows 建议在 Windows 上直接编译"
        echo "或者安装 mingw-w64 后: GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc go build"
        # build windows amd64 ".exe"
        ;;
    all)
        build linux amd64 ""
        echo ""
        echo "---"
        echo "Mac: GOOS=darwin GOARCH=amd64 go build -ldflags='-s -w' -o portview-macos ."
        echo "Win: GOOS=windows GOARCH=amd64 go build -ldflags='-s -w' -o portview.exe ."
        ;;
esac

echo "完成"
ls -lh portview* 2>/dev/null
