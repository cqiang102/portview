package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func testStore(t *testing.T) *PortMetaStore {
	t.Helper()
	s := &PortMetaStore{path: filepath.Join(t.TempDir(), "notes.json")}
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	return s
}
func TestUnknownPIDIsOccupied(t *testing.T) {
	entries := parseSSOutput("tcp LISTEN 0 128 0.0.0.0:80 0.0.0.0:*\nudp UNCONN 0 0 0.0.0.0:53 0.0.0.0:*")
	entries = append(entries, PortEntry{Port: 1, Status: unobservedStatus})
	for _, e := range entries[:2] {
		if !e.Occupied() {
			t.Fatalf("missing PID is not a free port: %+v", e)
		}
	}
	if got := filterEntries(entries, nil, "📌 已占用", ""); len(got) != 2 {
		t.Fatal(got)
	}
	if got := filterEntries(entries, nil, "🅰 TCP", ""); len(got) != 1 || got[0].Port != 80 {
		t.Fatal(got)
	}
	if got := filterEntries(entries, nil, "🅱 UDP", ""); len(got) != 1 || got[0].Port != 53 {
		t.Fatal(got)
	}
	if entries[0].SysGroup() != "Web" {
		t.Fatal(entries[0].SysGroup())
	}
	sortEntries(entries, true)
	if entries[2].Occupied() {
		t.Fatal("unobserved port should sort last")
	}
}
func TestDarwinConnectedLocalPort(t *testing.T) {
	for _, proto := range []string{"tcp", "udp"} {
		var entries []PortEntry
		seen := map[int]bool{}
		parseLsofDarwin("app 123 user 4u IPv4 0x123 0t0 TCP 127.0.0.1:54321->127.0.0.1:443 (ESTABLISHED)", proto, seen, &entries, map[int]darwinProcInfo{123: {}})
		if len(entries) != 1 || entries[0].Port != 54321 || seen[443] {
			t.Fatalf("remote port parsed as local: %+v", entries)
		}
		if entries[0].LocalAddr != "127.0.0.1:54321" {
			t.Fatal(entries[0])
		}
		if proto == "tcp" && entries[0].Status != "ESTABLISHED" {
			t.Fatal(entries[0])
		}
	}
}
func TestGPUIncompleteAndMultipleDevices(t *testing.T) {
	for _, s := range []string{"", "1, 2, 3", "error"} {
		if formatGPU(s) != "" {
			t.Fatal(s)
		}
	}
	if got := formatGPU("1, 2, 3, 4\n5, 6, 7, 8\n"); strings.Count(got, "GPU:") != 2 {
		t.Fatal(got)
	}
}
func TestLegacyStoreMigration(t *testing.T) {
	for _, raw := range []string{
		`{"18080":{"group":"old","note":"保留"}}`,
		`{"custom_groups":[{"name":"old","ports":[80]}],"port_notes":{"18080":{"group":"old","note":"保留"}}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			s := testStore(t)
			if err := os.WriteFile(s.path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			if err := s.load(); err != nil {
				t.Fatal(err)
			}
			if s.Get(18080).Note != "保留" || s.Get(18080).Group != "" {
				t.Fatal(s.Get(18080))
			}
			if !reflect.DeepEqual(s.PortBelongsToCustom(18080), []string{"old"}) {
				t.Fatal(s.Groups())
			}
			if err := s.SaveGroup("old", CustomGroup{Name: "renamed", Ports: []int{18080}}); err != nil {
				t.Fatal(err)
			}
			if err := s.load(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(s.PortBelongsToCustom(18080), []string{"renamed"}) {
				t.Fatal(s.Groups())
			}
		})
	}
}
func TestStoreMembershipRoundTrip(t *testing.T) {
	s := testStore(t)
	for _, name := range []string{"a", "b"} {
		if err := s.SaveGroup("", CustomGroup{Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SaveNote(80, "hello", []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.PortBelongsToCustom(80), []string{"a", "b"}) {
		t.Fatal(s.Groups())
	}
	entries := []PortEntry{{Port: 80}}
	if len(filterEntries(entries, s.Groups(), "🔖 a", "")) != 1 {
		t.Fatal("membership not used by filter")
	}
	if err := s.SaveNote(80, "hello", nil); err != nil {
		t.Fatal(err)
	}
	if len(s.PortBelongsToCustom(80)) != 0 {
		t.Fatal("clearing memberships failed")
	}
	if err := s.DeleteGroup("a"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveGroup("a", CustomGroup{Name: "oops"}); err == nil {
		t.Fatal("stale group reference accepted")
	}
	if err := s.SaveGroup("", CustomGroup{Name: "b"}); err == nil {
		t.Fatal("duplicate accepted")
	}
}
func TestStoreFailureDoesNotCommit(t *testing.T) {
	s := testStore(t)
	before := s.Groups()
	// A regular file cannot serve as the parent config directory.
	s.path = filepath.Join(s.path, "notes.json")
	if err := s.DeleteGroup(before[0].Name); err == nil {
		t.Fatal("expected persistence error")
	}
	if !reflect.DeepEqual(s.Groups(), before) {
		t.Fatal("failed save changed in-memory data")
	}
}
func TestStoreCorruptionPreserved(t *testing.T) {
	s := testStore(t)
	raw := []byte(`{"broken"`)
	if err := os.WriteFile(s.path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.load(); err == nil {
		t.Fatal("invalid JSON silently accepted")
	}
	got, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatal("corrupted config overwritten")
	}
}
func TestStoreSnapshotAndConcurrentSave(t *testing.T) {
	s := testStore(t)
	snapshot := s.Groups()
	snapshot[0].Ports[0] = 12345
	if s.Groups()[0].Ports[0] == 12345 {
		t.Fatal("snapshot aliases storage")
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			s.Set(port, PortMeta{Note: "test"})
			if err := s.save(); err != nil {
				t.Error(err)
			}
			s.Groups()
		}(i)
	}
	wg.Wait()
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
}
func TestKillRejectsInvalidPID(t *testing.T) {
	for _, pid := range []int{0, -1} {
		if err := killProcess(pid); err == nil {
			t.Fatal("invalid PID accepted")
		}
	}
}

func TestConfigPathNewAndLegacy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := configPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dir, "PortView", "notes.json") {
		t.Fatal(got)
	}
	legacy := filepath.Join(home, ".portview", "notes.json")
	if err := os.MkdirAll(filepath.Dir(legacy), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = configPath()
	if err != nil || got != legacy {
		t.Fatalf("legacy config ignored: %s %v", got, err)
	}
}

func TestSSKeepsProcessNameAndPIDTogether(t *testing.T) {
	entries := parseSSOutput(`tcp LISTEN 0 128 0.0.0.0:8080 0.0.0.0:* users:(("worker",pid=101,fd=6),("master",pid=202,fd=6))`)
	if len(entries) != 1 || entries[0].ProcessName != "worker" || entries[0].PID != 101 {
		t.Fatal(entries)
	}
}
