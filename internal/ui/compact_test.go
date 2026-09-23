package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestCompactLayoutBounds(t *testing.T) {
	small := calculateLayout(40, 20)
	if !small.ready || !small.compact {
		t.Fatalf("layout = %#v", small)
	}
	// 画面をちょうど使い切る。
	total := small.playersHeight + small.chatHeight + footerHeight + keybarHeight
	if total != small.height {
		t.Fatalf("height = %d, want %d", total, small.height)
	}
	if small.playersWidth != 40 || small.leftWidth != 40 {
		t.Fatalf("layout = %#v", small)
	}

	for _, size := range [][2]int{{compactMinWidth - 1, 20}, {40, compactMinHeight - 1}} {
		if layout := calculateLayout(size[0], size[1]); layout.ready {
			t.Fatalf("%dx%d is ready: %#v", size[0], size[1], layout)
		}
	}
	if full := calculateLayout(100, 30); full.compact {
		t.Fatalf("layout = %#v", full)
	}
}

func TestCompactViewShowsPlayersChatAndConsole(t *testing.T) {
	model := New(nil, nil, 0, DefaultSettings(), ServerInfo{})
	model.resize(40, 20)
	model.status = "running"

	view := stripANSI(model.View().Content)
	lines := strings.Split(view, "\n")
	if len(lines) != 20 {
		t.Fatalf("lines = %d:\n%s", len(lines), view)
	}
	// キーバー以外は枠で 40 桁ちょうどに揃う。
	for index, line := range lines[:len(lines)-1] {
		if width := stringWidth(line); width != 40 {
			t.Fatalf("line %d width = %d:\n%s", index, width, view)
		}
	}
	for _, want := range []string{"Players 0", "Chat", "Console · running"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view does not show %q:\n%s", want, view)
		}
	}
	// Stats / Meters / Graph / Log は出さない。
	for _, unwanted := range []string{"Meters", "Graph", "Heap "} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("view still shows %q:\n%s", unwanted, view)
		}
	}
}

func TestCompactMovesBetweenThreePanels(t *testing.T) {
	model := New(nil, nil, 0, DefaultSettings(), ServerInfo{})
	model.resize(40, 20)
	model.panel = panelPlayers
	model.mode = modeSelect
	model.selected = true

	// Players → Chat → Console と降り、Console で止まる。Log は挟まない。
	want := []panel{panelChat, panelConsole, panelConsole}
	for index, expected := range want {
		_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		if model.panel != expected {
			t.Fatalf("step %d: panel = %d, want %d", index, model.panel, expected)
		}
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if model.panel != panelPlayers {
		t.Fatalf("panel = %d", model.panel)
	}

	// 大きい端末で Log を選んでから縮めても取り残されない。
	model.resize(100, 30)
	model.panel = panelLog
	model.resize(40, 20)
	if model.panel != panelChat {
		t.Fatalf("panel = %d", model.panel)
	}
}

func TestCompactPanelAtMapsRows(t *testing.T) {
	layout := calculateLayout(40, 20)
	cases := []struct {
		y      int
		target panel
	}{
		{0, panelPlayers},
		{layout.playersHeight - 1, panelPlayers},
		{layout.playersHeight, panelChat},
		{layout.consoleY() - 1, panelChat},
		{layout.consoleY(), panelConsole},
		{layout.consoleY() + footerHeight - 1, panelConsole},
	}
	for _, test := range cases {
		target, ok := layout.panelAt(5, test.y)
		if !ok || target != test.target {
			t.Fatalf("y = %d: panel = %d, ok = %t", test.y, target, ok)
		}
	}
	// 最下行はキーバー。
	if _, ok := layout.panelAt(5, 19); ok {
		t.Fatalf("keybar row is selectable")
	}
}
