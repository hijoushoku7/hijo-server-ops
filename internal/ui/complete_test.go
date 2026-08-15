package ui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
)

func TestModelCompletions(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		players []string
		want    []string
	}{
		{name: "tell", input: "tell ", players: []string{"Alice", "Bob"}, want: []string{"Alice", "Bob"}},
		{name: "tell 語が出そろった位置", input: "tell", players: []string{"Alice", "Bob"}, want: []string{"Alice", "Bob"}},
		{name: "tell 大文字小文字を無視", input: "tell al", players: []string{"Alice", "Bob"}, want: []string{"Alice"}},
		{name: "time set", input: "time set n", want: []string{"night", "noon"}},
		{name: "time", input: "time", want: []string{"set"}},
		{name: "time 空白", input: "time ", want: []string{"set"}},
		{name: "time 打ちかけ", input: "time s", want: []string{"set"}},
		{name: "weather", input: "weather ", want: []string{"clear", "rain", "thunder"}},
		{name: "weather 語が出そろった位置", input: "weather", want: []string{"clear", "rain", "thunder"}},
		{name: "time set 語が出そろった位置", input: "time set", want: []string{"day", "night", "noon", "midnight"}},
		{name: "tell 引数の後", input: "tell Alice ", players: []string{"Alice"}, want: nil},
		{name: "weather 引数の後", input: "weather clear ", want: nil},
		{name: "対象外", input: "gamemode ", want: nil},
		// 先頭に空白があってもコマンド語を見失わない。
		{name: "先頭の空白", input: "  weather ", want: []string{"clear", "rain", "thunder"}},
		// 空白が 2 つ続いても並びの一致は変わらない。
		{name: "連続する空白", input: "weather  c", want: []string{"clear"}},
		{name: "連続する空白 打ちかけ無し", input: "weather  ", want: []string{"clear", "rain", "thunder"}},
		{name: "連続する空白 time set", input: "time set  n", want: []string{"night", "noon"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := newTestModel()
			model.input = []rune(test.input)
			model.playerList = test.players
			if got := model.completions(); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("completions() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestModelInsertsTimeSetCompletionOnTab(t *testing.T) {
	for _, input := range []string{"time", "time ", "time s"} {
		model := newTestModel()
		model.input = []rune(input)
		_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if got := string(model.input); got != "time set " {
			t.Fatalf("input %q: got %q, want %q", input, got, "time set ")
		}
	}
}

// TestModelKeepsSpacingWhenInserting は打った空白をそのまま残すことを見る。
// 候補で置き換えるのは打ちかけの語だけで、その手前には触らない。
func TestModelKeepsSpacingWhenInserting(t *testing.T) {
	model := newTestModel()
	model.input = []rune("weather  c")

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	if got := string(model.input); got != "weather  clear " {
		t.Fatalf("input = %q, want %q", got, "weather  clear ")
	}
}

func TestModelCompletionHint(t *testing.T) {
	tests := []struct {
		input   string
		players []string
		want    string
	}{
		{input: "weather", want: " clear"},
		{input: "weather cl", want: "ear"},
		{input: "time", want: " set"},
		{input: "tell ", players: []string{"Alice", "Bob"}, want: "Alice"},
		{input: "tell al", players: []string{"Alice"}, want: ""},
	}
	for _, test := range tests {
		model := newTestModel()
		model.input = []rune(test.input)
		model.playerList = test.players
		if got := model.completionHint(); got != test.want {
			t.Errorf("input %q: completionHint() = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestModelCompletionHintFollowsCursor(t *testing.T) {
	model := newTestModel()
	model.input = []rune("weather ")
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := model.completionHint(); got != "rain" {
		t.Fatalf("completionHint() = %q, want %q", got, "rain")
	}
}

func TestModelConsoleLineShowsHintWithoutOverflow(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 72, Height: 24})
	model.input = []rune("weather")

	line := model.consoleLine()
	if !strings.Contains(line, dimStyle.Render(" clear")) {
		t.Fatalf("Console 行に灰色の候補が無い: %q", line)
	}
	if width := stringWidth(line); width != model.layout.width-2 {
		t.Fatalf("Console 行の桁数 = %d, want %d", width, model.layout.width-2)
	}

	model.input = []rune(strings.Repeat("x", 512) + " weather")
	if width := stringWidth(model.consoleLine()); width != model.layout.width-2 {
		t.Fatalf("長い入力の Console 行の桁数 = %d, want %d", width, model.layout.width-2)
	}
}

func TestModelKeybarShowsCompletionForCompletableInput(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	model.input = []rune("weather")
	if bar := stripANSI(model.keybar()); !strings.Contains(bar, msg.BarComplete) {
		t.Fatalf("keybar = %q", bar)
	}
	model.input = []rune("say hello")
	if bar := stripANSI(model.keybar()); !strings.Contains(bar, msg.BarConsoleTab) {
		t.Fatalf("keybar = %q", bar)
	}
}

func TestModelCompletesCaseInsensitiveCandidateWithoutHint(t *testing.T) {
	model := newTestModel()
	model.input = []rune("tell al")
	model.playerList = []string{"Alice"}
	if got := model.completionHint(); got != "" {
		t.Fatalf("completionHint() = %q, want empty", got)
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if got := string(model.input); got != "tell Alice " {
		t.Fatalf("input = %q, want %q", got, "tell Alice ")
	}
}

func TestModelInsertsSingleCompletionOnTab(t *testing.T) {
	model := newTestModel()
	model.input = []rune("weather cl")

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	if got := string(model.input); got != "weather clear " {
		t.Fatalf("input = %q, want %q", got, "weather clear ")
	}
	if model.completionOpen {
		t.Fatal("候補が1件なのにポップアップが開いている")
	}
}

func TestModelOpensAndConfirmsMultipleCompletions(t *testing.T) {
	actions := make(chan Action, 1)
	model := New(actions, nil, 0, DefaultSettings(), ServerInfo{})
	model.input = []rune("time set n")

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !model.completionOpen || model.completionCursor != 0 {
		t.Fatalf("open = %v, cursor = %d", model.completionOpen, model.completionCursor)
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if got := string(model.input); got != "time set noon " {
		t.Fatalf("input = %q, want %q", got, "time set noon ")
	}
	if model.completionOpen {
		t.Fatal("確定後もポップアップが開いている")
	}
	if len(actions) != 0 {
		t.Fatal("候補の Enter でコマンドが送信された")
	}
}

func TestModelClosesCompletionOnEscapeWithoutLeavingInput(t *testing.T) {
	model := newTestModel()
	model.input = []rune("weather ")
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	if model.completionOpen {
		t.Fatal("Esc で候補が閉じていない")
	}
	if !model.editingConsole() {
		t.Fatalf("mode = %d, panel = %d, focus = %d", model.mode, model.panel, model.consoleFocus)
	}
	if got := string(model.input); got != "weather " {
		t.Fatalf("input = %q, want %q", got, "weather ")
	}
}

func TestModelKeepsTabFocusCycleWithoutCompletions(t *testing.T) {
	model := newTestModel()
	model.input = []rune("say hello")

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	if model.consoleFocus != consoleRestart {
		t.Fatalf("consoleFocus = %d, want %d", model.consoleFocus, consoleRestart)
	}
}

func TestModelRefreshesOpenCompletionsWhileTyping(t *testing.T) {
	model := newTestModel()
	model.playerList = []string{"Alice", "Bob"}
	model.input = []rune("tell ")
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	pressRune(model, 'a')
	if !model.completionOpen {
		t.Fatal("候補が残る入力でポップアップが閉じた")
	}
	if got := model.completions(); !reflect.DeepEqual(got, []string{"Alice"}) {
		t.Fatalf("completions() = %#v, want %#v", got, []string{"Alice"})
	}
	pressRune(model, 'z')
	if model.completionOpen {
		t.Fatal("候補が0件になってもポップアップが開いている")
	}
}

// TestModelViewOverlaysCompletionAboveConsole は候補が Console のすぐ上に
// 重なり、下地の桁数と行数を変えないことを見る。overlay は幅を測って詰め直す
// ので、ずれると画面全体が崩れる。
func TestModelViewOverlaysCompletionAboveConsole(t *testing.T) {
	model := newTestModel()
	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model.playerList = []string{"Alice", "Bob"}
	model.input = []rune("tell ")
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	lines := strings.Split(model.View().Content, "\n")
	if len(lines) != 24 {
		t.Fatalf("行数 = %d, want 24", len(lines))
	}
	for index, line := range lines {
		if width := stringWidth(line); width != 80 {
			t.Fatalf("%d 行目の桁数 = %d, want 80", index, width)
		}
	}
	// Console 枠は下から数えて footerHeight + keybarHeight 行。候補の箱は
	// その 1 行上で閉じるので、最後の候補は下枠のさらに 1 行上に出る。
	consoleTop := 24 - footerHeight - keybarHeight
	if !strings.Contains(lines[consoleTop-2], "Bob") {
		t.Fatalf("候補が Console の直上に無い:\n%s", strings.Join(lines[consoleTop-3:], "\n"))
	}
	if !strings.Contains(lines[consoleTop+1], "tell ") {
		t.Fatalf("入力欄が候補で潰れている: %q", lines[consoleTop+1])
	}
}

func TestModelCompletesInputWithLeadingSlash(t *testing.T) {
	model := newTestModel()
	model.playerList = []string{"Hijoushoku"}
	model.input = []rune("/tell hi")

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	if got := string(model.input); got != "/tell Hijoushoku " {
		t.Fatalf("input = %q, want %q", got, "/tell Hijoushoku ")
	}
}
