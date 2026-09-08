package main

import (
	"fyne.io/fyne/v2/test"
	"testing"
	"time"
)

func TestFilterAndSortClearSelection(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	pv := &PortViewer{win: a.NewWindow("test"), meta: testStore(t), selRow: -1}
	pv.buildUI()
	pv.entries = []PortEntry{{Port: 8080, PID: 42, Protocol: "tcp"}, {Port: 80, PID: 43, Protocol: "tcp"}}
	pv.applyFilter()
	pv.selRow = 0
	pv.lastClick = time.Now()
	pv.searchBox.SetText("80")
	if pv.selRow != -1 || !pv.lastClick.IsZero() {
		t.Fatal("filter retained stale selection")
	}
	pv.selRow = 0
	pv.sortOccupied()
	if pv.selRow != -1 {
		t.Fatal("sort retained stale selection")
	}
	if pv.filtered[0].Port != 80 {
		t.Fatal(pv.filtered)
	}
}
