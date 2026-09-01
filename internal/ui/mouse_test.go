package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

	// 1 回目のクリックは仮選択で、フォーカスには入らない。
	_, _ = model.Update(tea.MouseClickMsg{
		X: model.layout.leftWidth, Y: statsHeight, Button: tea.MouseLeft,
	})
	if model.panel != panelLog || model.mode != modeSelect || !model.selected {
		t.Fatalf("panel click state = panel %d, mode %d, selected %t", model.panel, model.mode, model.selected)
	}
	// 同じパネルをもう一度押すと本選択。
	_, _ = model.Update(tea.MouseClickMsg{
		X: model.layout.leftWidth, Y: statsHeight, Button: tea.MouseLeft,
	})
	if model.panel != panelLog || model.mode != modeFocus {
		t.Fatalf("second click state = panel %d, mode %d", model.panel, model.mode)
	}
	// 別のパネルを押したら、そこの仮選択からやり直す。
	_, _ = model.Update(tea.MouseClickMsg{
		X: 0, Y: statsHeight + model.layout.graphHeight, Button: tea.MouseLeft,
	})
	if model.panel != panelChat || model.mode != modeSelect {
		t.Fatalf("other panel click state = panel %d, mode %d", model.panel, model.mode)
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

// 選択枠を消した後もホバーは出す。マウスで操作を再開するとっかかりが
// 無くなるため。
func TestModelHoverFrameShowsWithoutSelection(t *testing.T) {
	model := newTestModel()
	model.resize(100, 24)
	model.mode = modeSelect
	model.selected = false
	_, _ = model.Update(tea.MouseMotionMsg{X: model.layout.leftWidth, Y: statsHeight})
	if got := model.frameFor(panelLog); got.render("─") != hoverFrame.render("─") {
		t.Fatal("hover frame is not shown while nothing is selected")
	}
	if got := model.frameFor(panelChat); got.render("─") != plainFrame.render("─") {
		t.Fatal("non-hovered panel is not plain")
	}
}

// 開いたままのモーダルはサーバー停止で閉じない。終了画面ではキー処理と
// 同じく、モーダルより先に終了モーダルのログを見る。
func TestModelMouseWheelOnExitIgnoresOpenModal(t *testing.T) {
	model := newTestModel()
	model.resize(100, 24)
	model.settingsOpen = true
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

// メニューはマウスでも操作できる。ホバーは細線のまま文字色だけ変え、
// カーソルの当たっている項目だけ太線にする。
func TestModelMouseSelectsQuitMenuItem(t *testing.T) {
	actions := make(chan Action, 1)
	model := New(actions, nil, 0, DefaultSettings(), ServerInfo{})
	model.resize(100, 30)
	model.openQuitMenu()
	// 当たり判定は描画のたびに作る。
	_ = model.View()

	restart := model.quitMenuHits[quitMenuRestart]
	_, _ = model.Update(tea.MouseMotionMsg{X: restart.x0 + 1, Y: restart.y0 + 1})
	if model.quitMenuHover != quitMenuRestart {
		t.Fatalf("hover = %d", model.quitMenuHover)
	}
	box, _, _ := model.quitMenuModal()
	// カーソルは OPTIONS のまま。ホバーで字形は変わらない。
	if !strings.Contains(stripANSI(box), bigWords[quitMenuRestart][0]) ||
		!strings.Contains(stripANSI(box), heavyStrokes.Replace(bigWords[quitMenuOptions][0])) {
		t.Fatalf("menu = %q", stripANSI(box))
	}
	if !styledWith(box, bigWords[quitMenuRestart][0], selectionTextStyle) {
		t.Fatal("ホバーの色が変わっていない")
	}

	// 外したところは何も指さない。
	_, _ = model.Update(tea.MouseMotionMsg{X: 0, Y: 0})
	if model.quitMenuHover != -1 {
		t.Fatalf("hover = %d", model.quitMenuHover)
	}

	// クリックでその項目を選ぶ。RESTART は確認を挟む。
	_, _ = model.Update(tea.MouseClickMsg{
		Button: tea.MouseLeft, X: restart.x0 + 1, Y: restart.y0 + 1,
	})
	if model.quitMenuCursor != quitMenuRestart || !model.confirmOpen {
		t.Fatalf("cursor = %d, confirm = %t",
			model.quitMenuCursor, model.confirmOpen)
	}
	select {
	case action := <-actions:
		t.Fatalf("クリックで確認なしに再起動した: %#v", action)
	default:
	}
}

// 項目を外したクリックは「やめる」の意図と見て閉じる。
func TestModelMouseClickOutsideQuitMenuCloses(t *testing.T) {
	model := newTestModel()
	model.resize(100, 30)
	model.openQuitMenu()
	_ = model.View()

	_, _ = model.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 0, Y: 0})
	if model.quitMenuOpen {
		t.Fatal("メニューが閉じていない")
	}
}

// styledWith は body を含む行がそのスタイルで描かれているかを見る。中央寄せの
// 空白ごと Render するので、装飾済みの文字列そのものとは一致しない。
func styledWith(view, body string, style lipgloss.Style) bool {
	prefix, _, _ := strings.Cut(style.Render("x"), "x")
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(stripANSI(line), body) {
			return strings.Contains(line, prefix)
		}
	}
	return false
}
