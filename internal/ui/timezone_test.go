package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
)

func TestTimeOffsetForRoundsAndUsesNearestDate(t *testing.T) {
	tests := []struct {
		name         string
		now          time.Time
		hour, minute int
		want         int
	}{
		{
			name: "next day",
			now:  time.Date(2026, 8, 8, 23, 10, 0, 0, time.UTC),
			hour: 1, minute: 30, want: 150,
		},
		{
			name: "previous day",
			now:  time.Date(2026, 8, 8, 0, 10, 0, 0, time.UTC),
			hour: 23, minute: 30, want: -30,
		},
		{
			name: "positive rounding",
			now:  time.Date(2026, 8, 8, 12, 16, 0, 0, time.UTC),
			hour: 13, minute: 30, want: 60,
		},
		{
			name: "negative twelve hour tie",
			now:  time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
			hour: 0, minute: 0, want: 720,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := timeOffsetFor(test.hour, test.minute, test.now); got != test.want {
				t.Fatalf("offset = %d, want %d", got, test.want)
			}
		})
	}
}

func TestClockBaseUsesDisplayedClockTime(t *testing.T) {
	// UTC から 5:45 ずれたローカル時刻でも、絶対時刻ではなく画面上の
	// 14:20 を丸めるので 14:30 になる。
	clock := time.Date(2026, 8, 8, 14, 20, 0, 0, time.FixedZone("test", 5*3600+45*60))
	if got := clockBase(clock); got != 14*60+30 {
		t.Fatalf("clock base = %d", got)
	}
	// 23:45 は日をまたいで 00:00 へ折り返す。24:00 のまま返すと、初期値の
	// 時が 24 になって表示も次の計算も壊れる。
	if got := clockBase(time.Date(2026, 8, 8, 23, 45, 0, 0, time.UTC)); got != 0 {
		t.Fatalf("clock base at 23:45 = %d", got)
	}
}

// TestReopenAndConfirmKeepsOffset は開いてそのまま OK を押しただけで
// オフセットが動かないことを見る。初期値と確定時の丸めが揃っていないと、
// 12:15 のような半端な時刻で押すたびに 30 分ずつずれていく。
func TestReopenAndConfirmKeepsOffset(t *testing.T) {
	clocks := []time.Time{
		time.Date(2026, 8, 8, 12, 15, 30, 0, time.UTC),
		time.Date(2026, 8, 8, 12, 45, 1, 0, time.UTC),
		time.Date(2026, 8, 8, 23, 50, 0, 0, time.UTC),
		time.Date(2026, 8, 8, 0, 5, 0, 0, time.UTC),
	}
	offsets := []int{0, 30, 60, 90, -90, -690, 720}
	for _, now := range clocks {
		for _, offset := range offsets {
			minutes := ((clockBase(now)+offset)%dayMinutes + dayMinutes) % dayMinutes
			got := timeOffsetFor(minutes/60, minutes%60, now)
			if got != offset {
				t.Errorf("now %s / offset %d: 表示 %02d:%02d から %d に変わった",
					now.Format("15:04:05"), offset, minutes/60, minutes%60, got)
			}
		}
	}
}

// TestTimeOffsetForStaysInRange は正規化の境界を見る。(-720, 720] の外へ
// 出ると、同じずれが日付をまたいだ別の値としても表せてしまう。
func TestTimeOffsetForStaysInRange(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	for minutes := 0; minutes < dayMinutes; minutes += 30 {
		got := timeOffsetFor(minutes/60, minutes%60, now)
		if got <= -720 || got > 720 {
			t.Fatalf("%02d:%02d -> %d", minutes/60, minutes%60, got)
		}
		if got%30 != 0 {
			t.Fatalf("%02d:%02d -> %d は 30 分刻みでない", minutes/60, minutes%60, got)
		}
	}
}

func TestTimeModalOpensFromSettingsAndHandlesKeys(t *testing.T) {
	model := New(make(chan Action, 1), nil, 0, DefaultSettings())
	model.resize(100, 40)
	model.settingsOpen = true
	model.settingCursor = len(settingItems) - 1
	settingsView := stripANSI(model.View().Content)
	if !strings.Contains(settingsView, msg.OptSystemTime) ||
		!strings.Contains(settingsView, "["+msg.TimeSettingButton+"]") {
		t.Fatalf("settings view:\n%s", settingsView)
	}

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if model.timeModal != nil {
		t.Fatal("right opened the time modal")
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.timeModal == nil || !model.settingsOpen {
		t.Fatalf("timeModal = %#v, settingsOpen = %t", model.timeModal, model.settingsOpen)
	}
	if model.timeModal.minute != 0 && model.timeModal.minute != 30 {
		t.Fatalf("minute = %d", model.timeModal.minute)
	}
	content := stripANSI(model.View().Content)
	if !strings.Contains(content, msg.TimeModalTitle) ||
		!strings.Contains(content, msg.LabelTimezone) {
		t.Fatalf("view:\n%s", content)
	}

	model.timeModal = &timeModalState{hour: 23}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if model.timeModal.hour != 0 {
		t.Fatalf("hour after up = %d", model.timeModal.hour)
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if model.timeModal.hour != 23 || model.timeModal.field != 1 || model.timeModal.minute != 30 {
		t.Fatalf("timeModal = %#v", model.timeModal)
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if model.timeModal.field != 0 || model.timeModal.minute != 0 {
		t.Fatalf("timeModal = %#v", model.timeModal)
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.timeModal != nil || !model.settingsOpen {
		t.Fatalf("timeModal = %#v, settingsOpen = %t", model.timeModal, model.settingsOpen)
	}
}

func TestTimeModalOKSavesAndInvalidatesExistingLogTimestamps(t *testing.T) {
	var saved []Settings
	model := New(make(chan Action, 1), func(settings Settings) error {
		saved = append(saved, settings)
		return nil
	}, 0, DefaultSettings())
	model.resize(100, 40)
	timestamp := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	record := logRecord{
		timestamp: timestamp, timestampSource: serverlog.TimestampLog,
		kind: serverlog.KindOther, text: "line",
	}
	model.logs.Add(record)
	model.chat.Add(record)
	viewport := bufferViewport{width: 30, height: 1}
	_ = model.logs.Window(viewport)
	_ = model.chat.Window(viewport)

	target := time.Now().Add(2 * time.Hour).Round(30 * time.Minute)
	model.timeModal = &timeModalState{hour: target.Hour(), minute: target.Minute()}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	offset := model.settings.TimeOffsetMinutes
	if model.timeModal != nil || len(saved) != 1 || saved[0].TimeOffsetMinutes != offset {
		t.Fatalf("timeModal = %#v, saved = %#v", model.timeModal, saved)
	}
	if offset == 0 || model.logs.wrapValid || model.chat.wrapValid {
		t.Fatalf("offset = %d, logs valid = %t, chat valid = %t",
			offset, model.logs.wrapValid, model.chat.wrapValid)
	}
	want := timestamp.Add(time.Duration(offset) * time.Minute).Format("15:04")
	for name, buffer := range map[string]*lineBuffer{"logs": &model.logs, "chat": &model.chat} {
		line := buffer.Window(viewport)[0]
		if !strings.HasPrefix(line.prefix, want) {
			t.Errorf("%s prefix = %q, want %q", name, line.prefix, want)
		}
	}
}

func TestExitModalUsesTimeOffset(t *testing.T) {
	settings := DefaultSettings()
	settings.TimeOffsetMinutes = 90
	model := New(make(chan Action, 1), nil, 0, settings)
	model.resize(100, 40)
	model.exit = &exitState{
		exitedAt: time.Date(2026, 8, 8, 23, 15, 0, 0, time.UTC),
	}

	box, _, _ := model.exitModal()
	if content := stripANSI(box); !strings.Contains(content, "2026-08-09 00:45:00") {
		t.Fatalf("modal:\n%s", content)
	}
}
