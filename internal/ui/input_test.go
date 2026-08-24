package ui

import (
	"errors"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
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

// TestModelCtrlCStopsTheServerFirst は ^C が即終了ではなく stop の送信に
// なり、二度目で従来どおり終わることを見る。1 度目で終わると supervisor が
// サーバーを短い猶予で殺し、ワールドの保存が間に合わない。
func TestModelCtrlCStopsTheServerFirst(t *testing.T) {
	actions := make(chan Action, 1)
	model := New(actions, nil, 1, DefaultSettings(), ServerInfo{})
	model.resize(80, 24)

	_, command := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if command != nil {
		t.Fatalf("command = %T", command())
	}
	if action := <-actions; action.Kind != ActionSendCommand || action.Command != "stop" {
		t.Fatalf("action = %#v", action)
	}
	if !model.quitting || model.status != msg.StatusStopping {
		t.Fatalf("quitting = %t, status = %q", model.quitting, model.status)
	}

	// 停止待ちの間は ^C 以外を受けない。
	_, _ = model.Update(ActionResultMsg{Action: Action{Kind: ActionSendCommand, Command: "stop"}})
	_, command = model.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if command != nil || model.settingsOpen {
		t.Fatalf("settingsOpen = %t, command = %T", model.settingsOpen, command)
	}

	_, command = model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf("command = %T", command())
	}
}

// TestModelCtrlCSkipsAutoRestart は ^C で止めた世代を自動再起動が起こし直さ
// ないことを見る。stop 後の終了コードが非 0 でもクラッシュ扱いにしない。
func TestModelCtrlCSkipsAutoRestart(t *testing.T) {
	actions := make(chan Action, 1)
	model := New(actions, nil, 1, DefaultSettings(), ServerInfo{})
	model.resize(80, 24)
	model.settings.AutoRestart = true

	_, _ = model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	<-actions
	_, _ = model.Update(ProcessExitedMsg{Generation: 1, ExitCode: 1})

	if model.exit.autoRestart {
		t.Fatalf("exit = %#v", model.exit)
	}
}

// TestModelCtrlCStopsWaitingWhenSendFails は stop を送れなかったときに待ちを
// やめることを見る。送れていないのに待つと、次の ^C まで終われない。
func TestModelCtrlCStopsWaitingWhenSendFails(t *testing.T) {
	actions := make(chan Action, 1)
	model := New(actions, nil, 1, DefaultSettings(), ServerInfo{})
	model.resize(80, 24)

	_, _ = model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	<-actions
	_, _ = model.Update(ActionResultMsg{
		Action: Action{Kind: ActionSendCommand, Command: "stop"},
		Err:    errors.New("boom"),
	})

	if model.quitting {
		t.Fatal("quitting は解除されていない")
	}
}

// TestModelCtrlCQuitsWhenTheServerIsGone は待つ相手がいないときに ^C が
// その場で終わることを見る。
func TestModelCtrlCQuitsWhenTheServerIsGone(t *testing.T) {
	model := newTestModel()
	model.resize(80, 24)
	_, _ = model.Update(ProcessExitedMsg{ExitCode: 0})

	_, command := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf("command = %T", command())
	}
}
