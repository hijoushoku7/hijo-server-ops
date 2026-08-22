package ui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestLayoutPanelAt(t *testing.T) {
	current := calculateLayout(100, 24)
	tests := []struct {
		name string
		x, y int
		want panel
		ok   bool
	}{
		{name: "Stats", x: 0, y: 0},
		{name: "Meters", x: current.statsWidth, y: 0},
		{name: "Players", x: current.statsWidth + current.metersWidth, y: 0, want: panelPlayers, ok: true},
		{name: "Graph", x: 0, y: statsHeight},
		{name: "Chat", x: 0, y: statsHeight + current.graphHeight, want: panelChat, ok: true},
		{name: "Log", x: current.leftWidth, y: statsHeight, want: panelLog, ok: true},
		{name: "Console", x: 0, y: statsHeight + current.bodyHeight, want: panelConsole, ok: true},
		{name: "Keybar", x: 0, y: statsHeight + current.bodyHeight + footerHeight},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := current.panelAt(test.x, test.y)
			if got != test.want || ok != test.ok {
				t.Fatalf("panelAt(%d, %d) = (%d, %t), want (%d, %t)", test.x, test.y, got, ok, test.want, test.ok)
			}
		})
	}
	if _, ok := (layout{}).panelAt(0, 0); ok {
		t.Fatal("unready layout accepted a panel")
	}
}

func TestModelMouseWheelScrollsBuffersAndPlayers(t *testing.T) {
	model := newTestModel()
	model.resize(100, 24)
	for _, target := range []panel{panelChat, panelLog} {
		buffer, viewport := model.bufferFor(target)
		for index := 0; index < viewport.height*3; index++ {
			buffer.Add(logRecord{text: fmt.Sprintf("line %d", index)})
		}
		x, y := 1, statsHeight+model.layout.graphHeight
		if target == panelLog {
			x, y = model.layout.leftWidth, statsHeight
		}
		_, _ = model.Update(tea.MouseWheelMsg{X: x, Y: y, Button: tea.MouseWheelUp})
		if offset := buffer.Offset(viewport); offset != 3 {
			t.Fatalf("%s offset = %d, want 3", target.title(), offset)
		}
	}
	model.playerList = []string{"alice", "bob"}
	_, _ = model.Update(tea.MouseWheelMsg{
		X: model.layout.statsWidth + model.layout.metersWidth,
		Y: 0, Button: tea.MouseWheelDown,
	})
	if model.playerCursor != 1 {
		t.Fatalf("player cursor = %d, want 1", model.playerCursor)
	}
}

func TestModelMouseIgnoresModalAndClickOpensPlayerCommands(t *testing.T) {
	model := newTestModel()
	model.resize(100, 24)
	model.playerList = []string{"alice"}
	x := model.layout.statsWidth + model.layout.metersWidth + 1

	// 通常のクリックはモードを変えず、選択対象だけを差し替える。
	_, _ = model.Update(tea.MouseClickMsg{
		X: model.layout.leftWidth, Y: statsHeight, Button: tea.MouseLeft,
	})
	if model.panel != panelLog || model.mode != modeFocus || !model.selected {
		t.Fatalf("panel click state = panel %d, mode %d, selected %t", model.panel, model.mode, model.selected)
	}

	_, _ = model.Update(tea.MouseClickMsg{X: x, Y: 1, Button: tea.MouseLeft})
	if model.panel != panelPlayers || model.mode != modeFocus || model.playerStage != playerStageCommands || model.playerTarget != "alice" {
		t.Fatalf("click state = panel %d, mode %d, stage %d, target %q", model.panel, model.mode, model.playerStage, model.playerTarget)
	}

	// コマンドモーダル中は背後のクリックとホイールを捨てる。
	model.playerCursor = 0
	_, _ = model.Update(tea.MouseWheelMsg{X: x, Y: 0, Button: tea.MouseWheelDown})
	_, _ = model.Update(tea.MouseClickMsg{X: 0, Y: statsHeight + model.layout.bodyHeight, Button: tea.MouseLeft})
	if model.panel != panelPlayers || model.playerCursor != 0 {
		t.Fatalf("modal accepted mouse input: panel %d, cursor %d", model.panel, model.playerCursor)
	}
}

func TestModelMouseMotionOnlyKeepsSelectablePanel(t *testing.T) {
	model := newTestModel()
	model.resize(100, 24)
	model.mode = modeSelect
	_, _ = model.Update(tea.MouseMotionMsg{
		X: model.layout.leftWidth, Y: statsHeight,
	})
	if !model.hovering || model.hover != panelLog {
		t.Fatalf("hover = (%d, %t), want Log", model.hover, model.hovering)
	}
	_, _ = model.Update(tea.MouseMotionMsg{X: 0, Y: 0})
	if model.hovering {
		t.Fatal("Stats panel remained selectable while hovering")
	}
}

// 終了モーダル中のログは全面表示で、ダッシュボードとは幅も高さも違う。
// ダッシュボード側の viewport で遡ると位置がずれる。
func TestModelMouseWheelUsesFullScreenViewportOnExit(t *testing.T) {
	model := newTestModel()
	model.resize(100, 24)
	model.exit = &exitState{closed: true}
	_, viewport := model.focusedBuffer()
	for index := 0; index < viewport.height*3; index++ {
		model.logs.Add(logRecord{text: fmt.Sprintf("line %d", index)})
	}
	_, _ = model.Update(tea.MouseWheelMsg{X: 1, Y: 1, Button: tea.MouseWheelUp})
	if offset := model.logs.Offset(viewport); offset != 3 {
		t.Fatalf("offset = %d, want 3", offset)
	}
}
