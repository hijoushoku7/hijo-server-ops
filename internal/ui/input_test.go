package ui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestModelMovesBetweenPanelsWithHJKL(t *testing.T) {
	tests := []struct {
		name  string
		start panel
		key   rune
		arrow rune
	}{
		{name: "左", start: panelLog, key: 'h', arrow: tea.KeyLeft},
		{name: "下", start: panelChat, key: 'j', arrow: tea.KeyDown},
		{name: "上", start: panelChat, key: 'k', arrow: tea.KeyUp},
		{name: "右", start: panelChat, key: 'l', arrow: tea.KeyRight},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			vimModel := newTestModel()
			vimModel.mode = modeSelect
			vimModel.panel = test.start
			arrowModel := newTestModel()
			arrowModel.mode = modeSelect
			arrowModel.panel = test.start

			pressRune(vimModel, test.key)
			_, _ = arrowModel.Update(tea.KeyPressMsg{Code: test.arrow})

			if vimModel.panel != arrowModel.panel {
				t.Fatalf("%c の移動先 = %d, 矢印の移動先 = %d",
					test.key, vimModel.panel, arrowModel.panel)
			}
		})
	}
}

func TestModelTypesHJKLInConsoleInput(t *testing.T) {
	model := newTestModel()
	for _, key := range "hjkl" {
		pressRune(model, key)
	}

	if got := string(model.input); got != "hjkl" {
		t.Fatalf("input = %q, want %q", got, "hjkl")
	}
	if model.panel != panelConsole || !model.editingConsole() {
		t.Fatalf("panel = %d, mode = %d, focus = %d",
			model.panel, model.mode, model.consoleFocus)
	}
}

func TestModelUsesHJKLAsNavigationOnConsoleButtons(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	pressRune(model, 'h')

	if got := string(model.input); got != "" {
		t.Fatalf("input = %q, want empty", got)
	}
	if model.consoleFocus != consoleRestart {
		t.Fatalf("consoleFocus = %d, want %d", model.consoleFocus, consoleRestart)
	}
}

func TestModelScrollsChatAndLogWithJK(t *testing.T) {
	tests := []struct {
		name  string
		panel panel
	}{
		{name: "Chat", panel: panelChat},
		{name: "Log", panel: panelLog},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := newTestModel()
			model.resize(80, 24)
			model.mode = modeFocus
			model.panel = test.panel
			buffer, viewport := model.focusedBuffer()
			for index := 0; index < viewport.height*3; index++ {
				buffer.Add(logRecord{text: fmt.Sprintf("line %d", index)})
			}

			pressRune(model, 'k')
			if offset := buffer.Offset(viewport); offset != 1 {
				t.Fatalf("k の後の offset = %d, want 1", offset)
			}
			pressRune(model, 'j')
			if offset := buffer.Offset(viewport); offset != 0 {
				t.Fatalf("j の後の offset = %d, want 0", offset)
			}
		})
	}
}

func TestModelDoesNotConvertModifiedHJKL(t *testing.T) {
	model := newTestModel()
	model.mode = modeSelect
	model.panel = panelLog
	_, _ = model.Update(tea.KeyPressMsg{Code: 'h', Mod: tea.ModCtrl})

	if model.panel != panelLog {
		t.Fatalf("Ctrl+h で panel = %d, want %d", model.panel, panelLog)
	}
}

func TestHJKLArrowKeyClearsTextOnlyWhenConverted(t *testing.T) {
	tests := []struct {
		key  rune
		want rune
	}{
		{key: 'h', want: tea.KeyLeft},
		{key: 'j', want: tea.KeyDown},
		{key: 'k', want: tea.KeyUp},
		{key: 'l', want: tea.KeyRight},
	}
	for _, test := range tests {
		got := hjklArrowKey(tea.Key{Code: test.key, Text: string(test.key)})
		if got.Code != test.want || got.Text != "" {
			t.Errorf("%c: key = %#v, want Code %d and empty Text",
				test.key, got, test.want)
		}
	}

	want := tea.Key{Code: 'h', Text: "h", Mod: tea.ModAlt}
	if got := hjklArrowKey(want); got != want {
		t.Fatalf("Alt+h = %#v, want %#v", got, want)
	}
}

func TestModelUsesHJKLInExitView(t *testing.T) {
	model := newTestModel()
	model.resize(80, 24)
	model.exit = &exitState{}
	pressRune(model, 'l')
	if model.exit.button != 1 {
		t.Fatalf("l の後の button = %d, want 1", model.exit.button)
	}
	pressRune(model, 'h')
	if model.exit.button != 0 {
		t.Fatalf("h の後の button = %d, want 0", model.exit.button)
	}

	model.exit.closed = true
	_, viewport := model.focusedBuffer()
	for index := 0; index < viewport.height*3; index++ {
		model.logs.Add(logRecord{text: fmt.Sprintf("line %d", index)})
	}
	pressRune(model, 'k')
	if offset := model.logs.Offset(viewport); offset != 1 {
		t.Fatalf("k の後の offset = %d, want 1", offset)
	}
	pressRune(model, 'j')
	if offset := model.logs.Offset(viewport); offset != 0 {
		t.Fatalf("j の後の offset = %d, want 0", offset)
	}
}

func pressRune(model *Model, key rune) {
	_, _ = model.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
}
