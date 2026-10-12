package main

import (
	"errors"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// ============================================================
// /proc/[pid]/stat 解析（进程名含空格/括号时字段不能错位）
// ============================================================

// makeProcStat 构造一条 /proc/[pid]/stat 内容。comm 为进程名（可能含空格与括号），
// 其后依次是字段 3..24，其中只填入被测字段，其余为 0。
func makeProcStat(comm string, utime, stime, nice, threads, rss int) string {
	vals := make([]string, 22)
	for i := range vals {
		vals[i] = "0"
	}
	vals[0] = "S"                    // 3 state
	vals[11] = strconv.Itoa(utime)   // 14 utime
	vals[12] = strconv.Itoa(stime)   // 15 stime
	vals[16] = strconv.Itoa(nice)    // 19 nice
	vals[17] = strconv.Itoa(threads) // 20 num_threads
	vals[21] = strconv.Itoa(rss)     // 24 rss
	return "1234 (" + comm + ") " + strings.Join(vals, " ")
}

func TestParseProcStat(t *testing.T) {
	st, ok := parseProcStat(makeProcStat("nginx", 120, 30, -5, 7, 2048))
	if !ok {
		t.Fatal("合法 stat 内容被拒绝")
	}
	if st.State != "S" || st.UTime != 120 || st.STime != 30 || st.Nice != -5 || st.Threads != 7 || st.RSSPages != 2048 {
		t.Fatalf("字段解析错误: %+v", st)
	}
}

func TestParseProcStat_CommWithSpacesAndParens(t *testing.T) {
	// 进程名含空格与 ')' 时，旧的 strings.Fields 方案会让所有字段整体错位。
	st, ok := parseProcStat(makeProcStat("Web Content (Renderer)", 200, 50, 3, 11, 4096))
	if !ok {
		t.Fatal("含空格的 comm 被拒绝")
	}
	if st.UTime != 200 || st.STime != 50 || st.Nice != 3 || st.Threads != 11 || st.RSSPages != 4096 {
		t.Fatalf("comm 含空格时字段错位: %+v", st)
	}
}

func TestParseProcStat_Invalid(t *testing.T) {
	for _, raw := range []string{"", "1234 nginx S 0 0 0", "1234 (nginx) S 0 0"} {
		if _, ok := parseProcStat(raw); ok {
			t.Fatalf("非法内容被接受: %q", raw)
		}
	}
}

// ============================================================
// /proc/stat 解析（瞬时 CPU 需要两次采样求差）
// ============================================================

func TestParseCPUStat(t *testing.T) {
	content := "cpu  100 20 30 400 5 0 0 0 0 0\ncpu0 10 1 2 30 0 0 0 0 0 0\nintr 123\n"
	total, idle, ok := parseCPUStat(content)
	if !ok {
		t.Fatal("合法的 /proc/stat 被拒绝")
	}
	if total != 555 || idle != 400 {
		t.Fatalf("total=%d idle=%d，期望 555/400", total, idle)
	}
}

func TestParseCPUStat_Invalid(t *testing.T) {
	for _, content := range []string{"", "intr 1 2 3\nctxt 9", "cpu 1 2 3"} {
		if _, _, ok := parseCPUStat(content); ok {
			t.Fatalf("非法内容被接受: %q", content)
		}
	}
}

// ============================================================
// 协议标签（ss 的 Netid 不区分 IPv6，按地址判断）
// ============================================================

func TestProtoForAddr(t *testing.T) {
	cases := []struct {
		netid, addr, want string
	}{
		{"tcp", "0.0.0.0:8080", "tcp"},
		{"tcp", "127.0.0.1:54321", "tcp"},
		{"tcp", "[::]:22", "tcp6"},
		{"udp", "0.0.0.0:5353", "udp"},
		{"udp", "[::]:53", "udp6"},
		{"raw", "[::]:1", "raw"},
	}
	for _, c := range cases {
		if got := protoForAddr(c.netid, c.addr); got != c.want {
			t.Errorf("protoForAddr(%q, %q) = %q，期望 %q", c.netid, c.addr, got, c.want)
		}
	}
}

// ============================================================
// 列表展示口径：已占用但 PID 不可见 ≠ 未观测到占用
// ============================================================

func TestPortEntryTextHelpers(t *testing.T) {
	withPID := PortEntry{Port: 80, Protocol: "tcp", PID: 42, ProcessName: "nginx", MemoryMB: 12.5}
	if withPID.PIDText() != "42" || withPID.ProcessText() != "nginx" || withPID.MemoryText() != "12.5 MB" {
		t.Fatalf("PID 可见时展示错误: %q %q %q", withPID.PIDText(), withPID.ProcessText(), withPID.MemoryText())
	}

	// 观测到占用但读不到 PID（权限不足，或 TIME-WAIT 等无主 socket）：
	// 用词保持中性，只区分"占用但进程未知"与"未观测到占用"
	noPID := PortEntry{Port: 80, Protocol: "tcp", Status: "LISTEN"}
	if noPID.PIDText() != "未知" {
		t.Errorf("PID 列 = %q，期望 未知", noPID.PIDText())
	}
	if noPID.ProcessText() != "进程未知" {
		t.Errorf("进程名列 = %q", noPID.ProcessText())
	}
	if noPID.MemoryText() != "-" {
		t.Errorf("内存列 = %q，期望 -", noPID.MemoryText())
	}

	// 未观测到占用
	free := PortEntry{Port: 9999, Status: unobservedStatus}
	if free.PIDText() != "-" || free.ProcessText() != "-" || free.MemoryText() != "-" {
		t.Fatalf("未观测端口展示错误: %q %q %q", free.PIDText(), free.ProcessText(), free.MemoryText())
	}
}

// ============================================================
// 自定义分组选项文本（集中构造/解析）
// ============================================================

func TestCustomGroupLabelRoundTrip(t *testing.T) {
	const name = "我的服务"
	if got := customGroupLabel(name); got != "🔖 "+name {
		t.Fatalf("customGroupLabel = %q", got)
	}
	got, ok := customGroupName(customGroupLabel(name))
	if !ok || got != name {
		t.Fatalf("往返解析失败: %q %v", got, ok)
	}
	// 分组名本身含前缀时只剥离一次
	tricky := "a🔖 b"
	if got, ok := customGroupName(customGroupLabel(tricky)); !ok || got != tricky {
		t.Fatalf("含前缀的分组名解析失败: %q %v", got, ok)
	}
	// 内建筛选项与空名不应被当作自定义分组
	for _, label := range []string{"🏷️ 全部", "📌 已占用", customGroupPrefix, ""} {
		if _, ok := customGroupName(label); ok {
			t.Errorf("%q 不应解析为自定义分组", label)
		}
	}
}

func TestFilterUsesCentralGroupLabel(t *testing.T) {
	groups := []CustomGroup{{Name: "我的服务", Ports: []int{8080}}}
	entries := []PortEntry{{Port: 8080, Protocol: "tcp"}, {Port: 80, Protocol: "tcp"}}
	if got := filterEntries(entries, groups, customGroupLabel("我的服务"), ""); len(got) != 1 || got[0].Port != 8080 {
		t.Fatalf("自定义分组筛选失败: %+v", got)
	}
	// 前缀不完整/名字不存在时不应误匹配
	if got := filterEntries(entries, groups, "🔖 不存在", ""); len(got) != 0 {
		t.Fatalf("不存在的分组应筛出空结果: %+v", got)
	}
}

// ============================================================
// Windows "打开位置"：路径含逗号时 explorer /select 无法解析
// ============================================================

func TestRevealTargetForWindows(t *testing.T) {
	if target, sel := revealTargetForWindows(`C:\nginx\nginx.exe`); !sel || target != `C:\nginx\nginx.exe` {
		t.Fatalf("普通路径应选中文件: %q %v", target, sel)
	}
	// 期望值写死为字面量：不能依赖宿主平台（非 Windows 上 filepath.Dir 会得到 "."）
	target, sel := revealTargetForWindows(`C:\my,app\nginx.exe`)
	if sel {
		t.Fatal("含逗号路径不应使用 /select")
	}
	if target != `C:\my,app` {
		t.Fatalf("含逗号路径应退化为所在目录: %q", target)
	}
	if target, sel := revealTargetForWindows(`C:/my,app/nginx.exe`); sel || target != "C:/my,app" {
		t.Fatalf("正斜杠路径同样应退化: %q %v", target, sel)
	}
}

// ============================================================
// 优雅终止：先 SIGTERM，宽限期后仍存活才 SIGKILL
// ============================================================

func TestGracefulKill_TerminatesOnSIGTERM(t *testing.T) {
	var sent []syscall.Signal
	sleeps := 0
	send := func(_ int, sig syscall.Signal) error {
		if sig == syscall.Signal(0) {
			return syscall.ESRCH // 已退出
		}
		sent = append(sent, sig)
		return nil
	}
	if err := gracefulKill(1234, send, func(time.Duration) { sleeps++ }); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || sent[0] != syscall.SIGTERM {
		t.Fatalf("应只发送 SIGTERM: %v", sent)
	}
	if sleeps != 1 {
		t.Fatalf("应在第一次轮询后结束，sleeps=%d", sleeps)
	}
}

func TestGracefulKill_EscalatesToSIGKILL(t *testing.T) {
	var sent []syscall.Signal
	sleeps := 0
	send := func(_ int, sig syscall.Signal) error {
		if sig != syscall.Signal(0) {
			sent = append(sent, sig)
		}
		return nil // 一直存活
	}
	if err := gracefulKill(1234, send, func(time.Duration) { sleeps++ }); err != nil {
		t.Fatal(err)
	}
	// 宽限期内每 killPollInterval 轮询一次，共 killGracePeriod/killPollInterval 次
	want := int(killGracePeriod / killPollInterval)
	if len(sent) != 2 || sent[0] != syscall.SIGTERM || sent[1] != syscall.SIGKILL {
		t.Fatalf("应依次发送 SIGTERM/SIGKILL: %v", sent)
	}
	if sleeps != want {
		t.Fatalf("宽限期内应轮询 %d 次，实际 %d", want, sleeps)
	}
}

func TestGracefulKill_ReturnsSendError(t *testing.T) {
	boom := errors.New("boom")
	sleeps := 0
	err := gracefulKill(1234, func(_ int, _ syscall.Signal) error { return boom }, func(time.Duration) { sleeps++ })
	if !errors.Is(err, boom) {
		t.Fatalf("应返回发送失败的错误: %v", err)
	}
	if sleeps != 0 {
		t.Fatal("发送失败后不应继续等待")
	}
}

func TestGracefulKill_ProcessAlreadyGone(t *testing.T) {
	// 进程已不存在时应视为成功，而不是弹窗报错。
	// 注意用 os.ErrProcessDone：os.Process.Signal 会把内核 ESRCH 归一化成它。
	sleeps := 0
	var sent []syscall.Signal
	send := func(_ int, sig syscall.Signal) error {
		sent = append(sent, sig)
		return os.ErrProcessDone
	}
	if err := gracefulKill(1234, send, func(time.Duration) { sleeps++ }); err != nil {
		t.Fatalf("进程已退出应返回成功: %v", err)
	}
	if len(sent) != 1 || sent[0] != syscall.SIGTERM {
		t.Fatalf("应只尝试一次 SIGTERM: %v", sent)
	}
	if sleeps != 0 {
		t.Fatal("进程已退出时不应等待")
	}
}

func TestGracefulKill_GoneOnSIGKILLIsSuccess(t *testing.T) {
	// 宽限期内进程消失：SIGKILL 返回"进程不存在"也应视为成功
	send := func(_ int, sig syscall.Signal) error {
		switch sig {
		case syscall.SIGTERM:
			return nil
		case syscall.Signal(0):
			return nil // 一直存活，触发升级
		default:
			return os.ErrProcessDone
		}
	}
	if err := gracefulKill(1234, send, func(time.Duration) {}); err != nil {
		t.Fatalf("SIGKILL 遇到进程已退出应视为成功: %v", err)
	}
}

// TestKillProcessAlreadyGone 走真实信号路径（不经 fake send），
// 确保 killProcess 对已不存在的 PID 返回成功，而不是弹窗报错。
func TestKillProcessAlreadyGone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 终止进程走 taskkill 分支，不使用 POSIX 信号")
	}
	// 远大于各平台 pid_max 的 PID，保证不存在，避免误杀真实进程。
	if err := killProcess(1 << 30); err != nil {
		t.Fatalf("目标进程已不存在时应视为成功，实际: %v", err)
	}
}

func TestProcessGoneClassification(t *testing.T) {
	// os.Process.Signal 对内核 ESRCH 的归一化结果
	if !processGone(os.ErrProcessDone) {
		t.Fatal("os.ErrProcessDone 应判定为进程已不存在")
	}
	if !processGone(syscall.ESRCH) {
		t.Fatal("ESRCH 应判定为进程已不存在")
	}
	for _, err := range []error{nil, syscall.EPERM, syscall.EINVAL, errors.New("boom")} {
		if processGone(err) {
			t.Fatalf("%v 不应判定为进程已不存在", err)
		}
	}
}

func TestProcessAlive(t *testing.T) {
	// EPERM 表示进程存在但不属于当前用户，仍视为存活
	if !processAlive(1, func(_ int, _ syscall.Signal) error { return syscall.EPERM }) {
		t.Fatal("EPERM 应视为存活")
	}
	if processAlive(1, func(_ int, _ syscall.Signal) error { return syscall.ESRCH }) {
		t.Fatal("ESRCH 应视为已退出")
	}
	if !processAlive(1, func(_ int, _ syscall.Signal) error { return nil }) {
		t.Fatal("信号成功应视为存活")
	}
}

// ============================================================
// macOS CPU / 内存采样
// ============================================================

func TestParseTopCPU_TakesLastSample(t *testing.T) {
	raw := "CPU usage: 10.0% user, 5.0% sys, 85.0% idle\n" +
		"Processes: 400 total\n" +
		"CPU usage: 2.5% user, 1.5% sys, 96.0% idle\n"
	pct, ok := parseTopCPU(raw)
	if !ok {
		t.Fatal("未解析出 CPU 采样")
	}
	if pct != 4.0 {
		t.Fatalf("应取最后一次采样(2.5+1.5)，得到 %v", pct)
	}
	if _, ok := parseTopCPU("no cpu here"); ok {
		t.Fatal("无采样时不应返回成功")
	}
}

func TestFormatMemDarwinPageSize(t *testing.T) {
	vmText := "Mach Virtual Memory Statistics: (page size of 4096 bytes)\n" +
		"Pages free:                                100.\n" +
		"Pages active:                             1000.\n" +
		"Pages inactive:                            200.\n" +
		"Pages wired down:                          400.\n"
	const total = 1 << 30 // 1 GiB
	// usedPages = 1000 + 400 + 200*0.5 = 1500
	appleSilicon := formatMemDarwin(vmText, total, 16384)
	intel := formatMemDarwin(vmText, total, 4096)
	if appleSilicon == intel {
		t.Fatal("不同页大小必须得到不同结果")
	}
	if intel != "0.6% (0.0/1 GB)" {
		t.Fatalf("4KB 页大小结果错误: %q", intel)
	}
	if appleSilicon != "2.3% (0.0/1 GB)" {
		t.Fatalf("16KB 页大小结果错误: %q", appleSilicon)
	}
	if got := formatMemDarwin(vmText, 0, 4096); got != "N/A" {
		t.Fatalf("非法总内存应返回 N/A: %q", got)
	}
}
