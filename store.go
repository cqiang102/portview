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
	Group string `json:"group"` // 所属自定义分组名
	Note  string `json:"note"`  // 备注文本，最长 100 字符
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

func (s *PortMetaStore) load() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = StoreData{}
	d, err := os.ReadFile(s.path)
	if err != nil {
		// 首次使用，创建默认分组
		s.data.CustomGroups = defaultGroups()
		s.data.PortNotes = make(map[int]PortMeta)
		_ = s.save()
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

func (s *PortMetaStore) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	d, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, d, 0644); err != nil {
		return fmt.Errorf("写入临时配置失败: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("保存配置失败: %w", err)
	}
	return nil
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

func (s *PortMetaStore) ResetAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = StoreData{
		CustomGroups: defaultGroups(),
		PortNotes:    make(map[int]PortMeta),
	}
	return s.save()
}
