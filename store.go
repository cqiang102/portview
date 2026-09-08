// PortView - 数据存储：端口备注与自定义分组持久化
// Copyright 2026 lacia.cq@qq.com
// License: Apache 2.0
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ============================================================
// 数据结构 — 端口元信息、自定义分组、持久化
// ============================================================

type PortMeta struct {
	Group string `json:"group,omitempty"` // 仅用于兼容旧版，运行时归属由 CustomGroups.Ports 管理
	Note  string `json:"note"`            // 备注文本，最长 100 字符
}

type CustomGroup struct {
	Name  string `json:"name"`  // 分组名称
	Ports []int  `json:"ports"` // 包含的端口列表
}

type StoreData struct {
	CustomGroups []CustomGroup    `json:"custom_groups"` // 自定义分组列表
	PortNotes    map[int]PortMeta `json:"port_notes"`    // 端口→备注映射
}

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

type PortMetaStore struct {
	mu   sync.RWMutex // 读写锁
	data StoreData    // 内存数据
	path string       // JSON 文件路径
}

// configPath retains existing installations; new installs use the OS config directory.
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err == nil {
		legacy := filepath.Join(home, ".portview", "notes.json")
		if _, err := os.Stat(legacy); err == nil {
			return legacy, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("读取配置目录失败: %w", err)
	}
	return filepath.Join(dir, "PortView", "notes.json"), nil
}

func (s *PortMetaStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = StoreData{CustomGroups: defaultGroups(), PortNotes: make(map[int]PortMeta)}
	raw, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return s.saveLocked(s.data)
	}
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("配置格式错误: %w", err)
	}
	if fields == nil {
		return fmt.Errorf("配置必须为 JSON 对象")
	}
	_, hasGroups := fields["custom_groups"]
	_, hasNotes := fields["port_notes"]
	data := StoreData{}
	if hasGroups || hasNotes {
		if err := json.Unmarshal(raw, &data); err != nil {
			return err
		}
	} else {
		data.CustomGroups = defaultGroups()
		if err := json.Unmarshal(raw, &data.PortNotes); err != nil {
			return err
		}
	}
	if data.PortNotes == nil {
		data.PortNotes = make(map[int]PortMeta)
	}
	// Merge duplicate legacy names, and migrate legacy per-port assignments once.
	groups := []CustomGroup{}
	for _, g := range data.CustomGroups {
		found := false
		for i := range groups {
			if groups[i].Name == g.Name {
				groups[i].Ports = uniquePorts(append(groups[i].Ports, g.Ports...))
				found = true
				break
			}
		}
		if !found {
			g.Ports = uniquePorts(g.Ports)
			groups = append(groups, g)
		}
	}
	data.CustomGroups = groups
	for port, m := range data.PortNotes {
		if m.Group == "" {
			continue
		}
		i := groupIndex(data, m.Group)
		if i < 0 {
			data.CustomGroups = append(data.CustomGroups, CustomGroup{Name: m.Group})
			i = len(data.CustomGroups) - 1
		}
		data.CustomGroups[i].Ports = uniquePorts(append(data.CustomGroups[i].Ports, port))
		m.Group = ""
		data.PortNotes[port] = m
	}
	s.data = data
	return nil
}

func (s *PortMetaStore) save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(s.data)
}
func (s *PortMetaStore) saveLocked(data StoreData) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".notes-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.path)
}

func cloneData(data StoreData) StoreData {
	out := StoreData{PortNotes: make(map[int]PortMeta), CustomGroups: nil}
	for k, v := range data.PortNotes {
		out.PortNotes[k] = v
	}
	out.CustomGroups = cloneGroups(data.CustomGroups)
	return out
}

// update commits memory only after persistence succeeds.
func (s *PortMetaStore) update(fn func(*StoreData) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneData(s.data)
	if err := fn(&next); err != nil {
		return err
	}
	if err := s.saveLocked(next); err != nil {
		return err
	}
	s.data = next
	return nil
}
func (s *PortMetaStore) Groups() []CustomGroup {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneGroups(s.data.CustomGroups)
}
func (s *PortMetaStore) Get(port int) PortMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.PortNotes[port]
}
func (s *PortMetaStore) Set(port int, m PortMeta) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.PortNotes[port] = m
}
func (s *PortMetaStore) PortBelongsToCustom(port int) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var names []string
	for _, g := range s.data.CustomGroups {
		for _, p := range g.Ports {
			if p == port {
				names = append(names, g.Name)
				break
			}
		}
	}
	return names
}
func groupIndex(data StoreData, name string) int {
	for i, g := range data.CustomGroups {
		if g.Name == name {
			return i
		}
	}
	return -1
}
func (s *PortMetaStore) SaveGroup(oldName string, g CustomGroup) error {
	return s.update(func(d *StoreData) error {
		if g.Name == "" {
			return fmt.Errorf("分组名称不能为空")
		}
		idx := groupIndex(*d, oldName)
		if oldName != "" && idx < 0 {
			return fmt.Errorf("分组已不存在，请重新打开")
		}
		if other := groupIndex(*d, g.Name); other >= 0 && other != idx {
			return fmt.Errorf("分组名称已存在")
		}
		for _, p := range g.Ports {
			if p < 1 || p > 65535 {
				return fmt.Errorf("端口必须在 1–65535 之间")
			}
		}
		g.Ports = uniquePorts(g.Ports)
		if idx < 0 {
			d.CustomGroups = append(d.CustomGroups, g)
		} else {
			d.CustomGroups[idx] = g
		}
		return nil
	})
}
func (s *PortMetaStore) DeleteGroup(name string) error {
	return s.update(func(d *StoreData) error {
		idx := groupIndex(*d, name)
		if idx < 0 {
			return fmt.Errorf("分组已不存在")
		}
		d.CustomGroups = append(d.CustomGroups[:idx], d.CustomGroups[idx+1:]...)
		return nil
	})
}
func (s *PortMetaStore) SaveNote(port int, note string, names []string) error {
	return s.update(func(d *StoreData) error {
		if port < 0 || port > 65535 {
			return fmt.Errorf("无效端口: %d", port)
		}
		if len([]rune(note)) > maxNoteLen {
			return fmt.Errorf("备注最多 %d 个字符", maxNoteLen)
		}
		for _, name := range names {
			if groupIndex(*d, name) < 0 {
				return fmt.Errorf("分组 %s 已不存在", name)
			}
		}
		for i, g := range d.CustomGroups {
			ports := []int{}
			for _, p := range g.Ports {
				if p != port {
					ports = append(ports, p)
				}
			}
			for _, name := range names {
				if name == g.Name {
					ports = append(ports, port)
					break
				}
			}
			d.CustomGroups[i].Ports = uniquePorts(ports)
		}
		d.PortNotes[port] = PortMeta{Note: note}
		return nil
	})
}
func (s *PortMetaStore) ResetAll() error {
	return s.update(func(d *StoreData) error {
		*d = StoreData{CustomGroups: defaultGroups(), PortNotes: make(map[int]PortMeta)}
		return nil
	})
}

func cloneGroups(groups []CustomGroup) []CustomGroup {
	out := make([]CustomGroup, len(groups))
	for i, g := range groups {
		out[i] = CustomGroup{Name: g.Name, Ports: append([]int(nil), g.Ports...)}
	}
	return out
}
