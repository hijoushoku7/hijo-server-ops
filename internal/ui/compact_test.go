package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
)

func TestCompactLayoutBounds(t *testing.T) {
	small := calculateLayout(44, 22)
	if !small.ready || !small.compact {
		t.Fatalf("layout = %#v", small)
	}
	// 画面をちょうど使い切る。
	total := small.playersHeight + small.chatHeight + footerHeight + keybarHeight
	if total != small.height {
		t.Fatalf("height = %d, want %d", total, small.height)
	}
	if small.playersWidth != 44 || small.leftWidth != 44 {
		t.Fatalf("layout = %#v", small)
	}

	for _, size := range [][2]int{{compactMinWidth - 1, 22}, {44, compactMinHeight - 1}} {
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
	model.resize(44, 22)
	model.status = "running"

	view := stripANSI(model.View().Content)
	lines := strings.Split(view, "\n")
	if len(lines) != 22 {
		t.Fatalf("lines = %d:\n%s", len(lines), view)
	}
	// キーバー以外は枠で 44 桁ちょうどに揃う。
	for index, line := range lines[:len(lines)-1] {
		if width := stringWidth(line); width != 44 {
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
	model.resize(44, 22)
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
	model.resize(44, 22)
	if model.panel != panelChat {
		t.Fatalf("panel = %d", model.panel)
	}
}

// 終了モーダルの「ログを読む」は Log を全面に出してそこへフォーカスする。
// 立ち直って compact のダッシュボードへ戻ったとき、画面に無い Log へキーが
// 吸われたままにしない。
func TestCompactLeavesHiddenLogAfterRestart(t *testing.T) {
	for _, restore := range []string{"auto", "manual"} {
		model := New(nil, nil, 0, DefaultSettings(), ServerInfo{})
		model.resize(44, 22)
		_, _ = model.Update(ProcessExitedMsg{Err: errors.New("boom"), ExitCode: 1})
		model.closeExitModal()
		if model.panel != panelLog {
			t.Fatalf("%s: panel = %d", restore, model.panel)
		}

		if restore == "auto" {
			model.exit.autoRestart = true
		}
		model.onServerStarted()
		if restore == "auto" {
			// 自動再起動はモーダルを残す。Enter で閉じたときに戻す。
			_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		}
		if model.exit != nil || model.panel != panelChat {
			t.Fatalf("%s: exit = %v, panel = %d", restore, model.exit, model.panel)
		}
	}
}

func TestCompactPanelAtMapsRows(t *testing.T) {
	layout := calculateLayout(44, 22)
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
	if _, ok := layout.panelAt(5, 21); ok {
		t.Fatalf("keybar row is selectable")
	}
}

// TestCompactMinimumFitsModals は下限の端末で、重ねるモーダルが切れずに
// 収まることを確かめる。compact の下限はここが決めている（→ layout.go）。
// 下限を下げたりモーダルを大きくしたりすると、ここが落ちる。
func TestCompactMinimumFitsModals(t *testing.T) {
	model := New(nil, nil, 0, DefaultSettings(), ServerInfo{})
	model.resize(compactMinWidth, compactMinHeight)

	// メニュー: 3 項目の大きい文字が縮まず、画面内に収まる。
	model.openQuitMenu()
	box, x, y := model.quitMenuModal()
	lines := strings.Split(box, "\n")
	if got := stringWidth(lines[0]); got != model.quitMenuBox.x1-model.quitMenuBox.x0+1 ||
		x+got > compactMinWidth || y+len(lines) > compactMinHeight {
		t.Fatalf("menu = %dx%d at (%d,%d)", got, len(lines), x, y)
	}
	// 3 項目とも字形の幅そのままで、当たり判定が画面の中に収まっている。
	for item, hit := range model.quitMenuHits {
		if hit.x1-hit.x0+1 != stringWidth(bigWords[item][0]) ||
			hit.x1 >= compactMinWidth || hit.y1 >= compactMinHeight {
			t.Fatalf("item %d = %#v:\n%s", item, hit, stripANSI(box))
		}
	}
	model.quitMenuOpen = false

	// 終了モーダル: 三択が全部読める。エラー行を並べても押し出されない。
	for index := 0; index < exitErrorLineLimit+2; index++ {
		model.addLog(serverlog.Entry{
			Kind:    serverlog.KindOther,
			Message: fmt.Sprintf("java.lang.Exception: crash %d", index),
		})
	}
	_, _ = model.Update(ProcessExitedMsg{Err: errors.New("crashed"), ExitCode: 1})
	box, _, y = model.exitModal()
	view := stripANSI(box)
	if y+len(strings.Split(box, "\n")) > compactMinHeight {
		t.Fatalf("exit modal overflows: y = %d\n%s", y, view)
	}
	for _, button := range []string{
		msg.ExitButtonLogs, msg.ExitButtonRestart, msg.ExitButtonQuit,
	} {
		if !strings.Contains(view, button) {
			t.Fatalf("exit modal hides %q:\n%s", button, view)
		}
	}
	model.exit = nil

	// プレイヤーのコマンド一覧: 末尾の項目まで画面内。
	model.playerList = []string{"Alice"}
	model.panel = panelPlayers
	model.mode = modeFocus
	model.playerStage = playerStageCommands
	model.playerTarget = "Alice"
	box, _, y = model.commandModal()
	if y+len(strings.Split(box, "\n")) > compactMinHeight ||
		!strings.Contains(stripANSI(box), playerCommands[len(playerCommands)-1].label) {
		t.Fatalf("command modal overflows at y = %d:\n%s", y, stripANSI(box))
	}
}

// ← → で中段の Chat と Log を入れ替える。Log もスクロールできること、
// 入れ替えたまま Players へ抜けて戻っても表示が維持されることを見る。
func TestCompactSwapsChatAndLog(t *testing.T) {
	model := New(nil, nil, 0, DefaultSettings(), ServerInfo{})
	model.resize(44, 22)
	for index := 0; index < 50; index++ {
		model.addLog(serverlog.Entry{
			Kind:    serverlog.KindOther,
			Raw:     fmt.Sprintf("line %d", index),
			Message: fmt.Sprintf("line %d", index),
		})
	}
	model.panel = panelChat
	model.mode = modeSelect
	model.selected = true

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if model.panel != panelLog {
		t.Fatalf("panel = %d", model.panel)
	}
	view := stripANSI(model.View().Content)
	if !strings.Contains(view, "Log") || !strings.Contains(view, "line 49") {
		t.Fatalf("view does not show the log:\n%s", view)
	}

	// フォーカスして遡れる。
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if model.logs.Following() {
		t.Fatal("log did not scroll")
	}

	// Players へ抜けて戻っても Log のまま。
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if model.panel != panelPlayers {
		t.Fatalf("panel = %d", model.panel)
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if model.panel != panelLog {
		t.Fatalf("panel = %d", model.panel)
	}

	// もう一度 ← で Chat へ戻る。
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if model.panel != panelChat || model.compactLog {
		t.Fatalf("panel = %d, compactLog = %v", model.panel, model.compactLog)
	}
}
