package main

import (
	"os"
	"testing"
)

// ---------- SysGroup ----------

func TestSysGroup_SSH(t *testing.T) {
	e := PortEntry{Port: 22, PID: 1234}
	if g := e.SysGroup(); g != "SSH" {
		t.Errorf("端口 22 应为 SSH，得到 %s", g)
	}
}

func TestSysGroup_Web(t *testing.T) {
	for _, p := range []int{80, 443, 8080, 8443} {
		e := PortEntry{Port: p, PID: 1}
		if g := e.SysGroup(); g != "Web" {
			t.Errorf("端口 %d 应为 Web，得到 %s", p, g)
		}
	}
}

func TestSysGroup_DB(t *testing.T) {
	for _, p := range []int{3306, 5432, 6379, 27017} {
		e := PortEntry{Port: p, PID: 1}
		if g := e.SysGroup(); g != "数据库" {
			t.Errorf("端口 %d 应为 数据库，得到 %s", p, g)
		}
	}
}

func TestSysGroup_System(t *testing.T) {
	e := PortEntry{Port: 123, PID: 456}
	if g := e.SysGroup(); g != "系统" {
		t.Errorf("端口 123 (占用) 应为 系统，得到 %s", g)
	}
}

func TestSysGroup_App(t *testing.T) {
	e := PortEntry{Port: 8088, PID: 999}
	if g := e.SysGroup(); g != "应用" {
		t.Errorf("端口 8088 (占用) 应为 应用，得到 %s", g)
	}
}

func TestSysGroup_FreeSystem(t *testing.T) {
	e := PortEntry{Port: 512, PID: 0}
	if g := e.SysGroup(); g != "系统" {
		t.Errorf("空闲系统端口应返回 系统，得到 %s", g)
	}
}

func TestSysGroup_FreeRegistered(t *testing.T) {
	e := PortEntry{Port: 18000, PID: 0}
	if g := e.SysGroup(); g != "注册" {
		t.Errorf("空闲注册端口应返回 注册，得到 %s", g)
	}
}

func TestSysGroup_FreeDynamic(t *testing.T) {
	e := PortEntry{Port: 60000, PID: 0}
	if g := e.SysGroup(); g != "动态" {
		t.Errorf("空闲动态端口应返回 动态，得到 %s", g)
	}
}

func TestSysGroup_DNS(t *testing.T) {
	e := PortEntry{Port: 53, PID: 1}
	if g := e.SysGroup(); g != "DNS" {
		t.Errorf("端口 53 应为 DNS，得到 %s", g)
	}
}

// ---------- fmtPort ----------

func TestFmtPort_Known(t *testing.T) {
	cases := map[int]string{
		22: "22 (SSH)", 80: "80 (HTTP)", 443: "443 (HTTPS)",
		3306: "3306 (MySQL)", 6379: "6379 (Redis)",
	}
	for p, expected := range cases {
		if got := fmtPort(p); got != expected {
			t.Errorf("fmtPort(%d) = %s，期望 %s", p, got, expected)
		}
	}
}

func TestFmtPort_AllKnown(t *testing.T) {
	cases := map[int]string{
		22: "22 (SSH)", 80: "80 (HTTP)", 443: "443 (HTTPS)",
		3306: "3306 (MySQL)", 5432: "5432 (PG)", 6379: "6379 (Redis)",
		8080: "8080 (HTTP-alt)", 27017: "27017 (Mongo)",
		53: "53 (DNS)", 25: "25 (SMTP)", 3389: "3389 (RDP)",
	}
	for p, expected := range cases {
		if got := fmtPort(p); got != expected {
			t.Errorf("fmtPort(%d) = %s，期望 %s", p, got, expected)
		}
	}
}

func TestFmtPort_Unknown(t *testing.T) {
	if got := fmtPort(9999); got != "9999" {
		t.Errorf("fmtPort(9999) = %s，期望 9999", got)
	}
}

func TestFmtPort_Zero(t *testing.T) {
	if got := fmtPort(0); got != "0 (保留)" {
		t.Errorf("fmtPort(0) = %s，期望 0 (保留)", got)
	}
}

func TestFmtPort_LowPort(t *testing.T) {
	// 1023 以下但不在已知列表中，返回纯数字
	got := fmtPort(123)
	if got != "123" {
		t.Errorf("fmtPort(123) = %s，期望 123", got)
	}
}

// ---------- uniquePorts ----------

func TestUniquePorts(t *testing.T) {
	result := uniquePorts([]int{5, 3, 3, 1, 5, 2})
	expected := []int{1, 2, 3, 5}
	if len(result) != len(expected) {
		t.Fatalf("uniquePorts 长度 %d，期望 %d: %v", len(result), len(expected), result)
	}
	for i := range expected {
		if result[i] != expected[i] {
			t.Errorf("位置 %d: 得到 %d，期望 %d", i, result[i], expected[i])
		}
	}
}

func TestUniquePorts_Empty(t *testing.T) {
	if r := uniquePorts([]int{}); len(r) != 0 {
		t.Errorf("空输入应返回空，得到 %v", r)
	}
}

func TestUniquePorts_Single(t *testing.T) {
	if r := uniquePorts([]int{42}); len(r) != 1 || r[0] != 42 {
		t.Errorf("单元素应返回自身: %v", r)
	}
}

func TestUniquePorts_AlreadySorted(t *testing.T) {
	r := uniquePorts([]int{1, 2, 3, 4, 5})
	expected := []int{1, 2, 3, 4, 5}
	for i := range expected {
		if r[i] != expected[i] {
			t.Errorf("位置 %d: 得到 %d，期望 %d", i, r[i], expected[i])
		}
	}
}

// ---------- parsePorts ----------

func TestParsePorts_Single(t *testing.T) {
	r := parsePorts("80")
	if len(r) != 1 || r[0] != 80 {
		t.Errorf("解析 '80' 失败: %v", r)
	}
}

func TestParsePorts_Comma(t *testing.T) {
	r := parsePorts("80,443,8080")
	if len(r) != 3 || r[0] != 80 || r[1] != 443 || r[2] != 8080 {
		t.Errorf("解析 '80,443,8080' 失败: %v", r)
	}
}

func TestParsePorts_Range(t *testing.T) {
	r := parsePorts("8000-8005")
	if len(r) != 6 || r[0] != 8000 || r[5] != 8005 {
		t.Errorf("解析 '8000-8005' 失败: %v", r)
	}
}

func TestParsePorts_Mixed(t *testing.T) {
	r := parsePorts("80,443,8000-8002")
	if len(r) != 5 {
		t.Errorf("解析混合格式失败，长度 %d: %v", len(r), r)
	}
}

func TestParsePorts_Empty(t *testing.T) {
	if r := parsePorts(""); len(r) != 0 {
		t.Errorf("空字符串应返回空: %v", r)
	}
}

func TestParsePorts_Invalid(t *testing.T) {
	r := parsePorts("abc,,-1")
	if len(r) != 0 {
		t.Errorf("无效输入应返回空: %v", r)
	}
}

func TestParsePorts_Spaces(t *testing.T) {
	r := parsePorts(" 80 , 443 , 8000-8002 ")
	if len(r) != 5 {
		t.Errorf("带空格的解析失败，长度 %d: %v", len(r), r)
	}
}

func TestParsePorts_RangeFull(t *testing.T) {
	r := parsePorts("1-65535")
	if len(r) != 65535 {
		t.Errorf("全端口范围应为 65535 个，得到 %d", len(r))
	}
	if r[0] != 1 || r[65534] != 65535 {
		t.Errorf("边界错误: first=%d, last=%d", r[0], r[65534])
	}
}

func TestParsePorts_ReverseRange(t *testing.T) {
	// 反向范围应被忽略
	r := parsePorts("8000-7999")
	if len(r) != 0 {
		t.Errorf("反向范围应返回空: %v", r)
	}
}

func TestParsePorts_OutOfRange(t *testing.T) {
	r := parsePorts("0,65536,99999")
	if len(r) != 0 {
		t.Errorf("超范围端口应被忽略: %v", r)
	}
}

func TestParsePorts_Duplicates(t *testing.T) {
	r := parsePorts("80,80,80,443")
	if len(r) != 2 {
		t.Errorf("重复端口应去重，得到 %v", r)
	}
}

// ---------- matchAny ----------

func TestMatchAny_Hit(t *testing.T) {
	if !matchAny(80, 80, 443, 8080) {
		t.Errorf("matchAny(80, ...) 应返回 true")
	}
}

func TestMatchAny_Miss(t *testing.T) {
	if matchAny(3000, 80, 443, 8080) {
		t.Errorf("matchAny(3000, ...) 应返回 false")
	}
}

func TestMatchAny_Empty(t *testing.T) {
	if matchAny(80) {
		t.Errorf("matchAny(80) 无目标应返回 false")
	}
}

// ---------- PortMetaStore ----------

func TestMetaStore_DefaultGroups(t *testing.T) {
	store := &PortMetaStore{path: "/tmp/test_portview.json"}
	store.load()

	if len(store.data.CustomGroups) == 0 {
		t.Error("默认分组不应为空")
	}

	found := false
	for _, g := range store.data.CustomGroups {
		if g.Name == "🌐 Web服务" {
			found = true
			has80 := false
			for _, p := range g.Ports {
				if p == 80 { has80 = true; break }
			}
			if !has80 { t.Error("Web服务分组应包含端口 80") }
			break
		}
	}
	if !found { t.Error("未找到 Web服务 默认分组") }

	os.Remove("/tmp/test_portview.json")
}

func TestMetaStore_SetAndGet(t *testing.T) {
	store := &PortMetaStore{path: "/tmp/test_portview2.json"}
	store.load()

	store.Set(8080, PortMeta{Note: "测试服务", Group: "自定义"})
	m := store.Get(8080)
	if m.Note != "测试服务" || m.Group != "自定义" {
		t.Errorf("Set/Get 失败: %+v", m)
	}

	os.Remove("/tmp/test_portview2.json")
}

func TestMetaStore_PortBelongsToCustom(t *testing.T) {
	store := &PortMetaStore{path: "/tmp/test_portview3.json"}
	store.load()

	// Web 服务包含端口 80
	groups := store.PortBelongsToCustom(80)
	found := false
	for _, g := range groups {
		if g == "🌐 Web服务" { found = true; break }
	}
	if !found {
		t.Errorf("端口 80 应属于 Web服务，得到: %v", groups)
	}

	os.Remove("/tmp/test_portview3.json")
}

func TestMetaStore_Reset(t *testing.T) {
	store := &PortMetaStore{path: "/tmp/test_portview4.json"}
	store.load()
	store.Set(9999, PortMeta{Note: "临时"})
	store.ResetAll()

	m := store.Get(9999)
	if m.Note != "" {
		t.Errorf("重置后备注应清空，得到 %s", m.Note)
	}
	if len(store.data.CustomGroups) == 0 {
		t.Error("重置后应有默认分组")
	}

	os.Remove("/tmp/test_portview4.json")
}

func TestMetaStore_ConcurrentGetSet(t *testing.T) {
	store := &PortMetaStore{path: "/tmp/test_portview_conc.json"}
	store.load()

	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func(p int) {
			store.Set(p, PortMeta{Note: "conc"})
			store.Get(p)
			done <- true
		}(i)
	}
	for i := 0; i < 100; i++ {
		<-done
	}

	os.Remove("/tmp/test_portview_conc.json")
}

// ---------- atoi ----------

func TestAtoi(t *testing.T) {
	if n := atoi("42"); n != 42 {
		t.Errorf("atoi('42') = %d", n)
	}
}

func TestAtoi_Invalid(t *testing.T) {
	if n := atoi("abc"); n != 0 {
		t.Errorf("atoi('abc') 应为 0，得到 %d", n)
	}
}

func TestAtoi_Empty(t *testing.T) {
	if n := atoi(""); n != 0 {
		t.Errorf("atoi('') 应为 0，得到 %d", n)
	}
}

func TestAtoi_Negative(t *testing.T) {
	if n := atoi("-5"); n != -5 {
		t.Errorf("atoi('-5') = %d，期望 -5", n)
	}
}

func TestAtoi_LargeNumber(t *testing.T) {
	if n := atoi("65535"); n != 65535 {
		t.Errorf("atoi('65535') = %d", n)
	}
}

func TestAtoi_LeadingZeros(t *testing.T) {
	if n := atoi("0080"); n != 80 {
		t.Errorf("atoi('0080') = %d", n)
	}
}

// ---------- sysGroup boundary ----------

func TestSysGroup_Port1023Occupied(t *testing.T) {
	e := PortEntry{Port: 1023, PID: 100}
	if g := e.SysGroup(); g != "系统" {
		t.Errorf("端口 1023 (占用) 应为 系统，得到 %s", g)
	}
}

func TestSysGroup_Port1024Occupied(t *testing.T) {
	e := PortEntry{Port: 1024, PID: 100}
	if g := e.SysGroup(); g != "应用" {
		t.Errorf("端口 1024 (占用) 应为 应用，得到 %s", g)
	}
}

func TestSysGroup_Port49151Free(t *testing.T) {
	e := PortEntry{Port: 49151, PID: 0}
	if g := e.SysGroup(); g != "注册" {
		t.Errorf("端口 49151 (空闲) 应为 注册，得到 %s", g)
	}
}

func TestSysGroup_Port49152Free(t *testing.T) {
	e := PortEntry{Port: 49152, PID: 0}
	if g := e.SysGroup(); g != "动态" {
		t.Errorf("端口 49152 (空闲) 应为 动态，得到 %s", g)
	}
}
