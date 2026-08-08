package ui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
)

type timeModalState struct {
	hour   int
	minute int
	field  int
	// base は開いた時点のシステムの時計。確定時に読み直さないのは、
	// 開いている間に丸めの境目（毎時 15 分・45 分）をまたぐと、指を
	// 触れていないのに 30 分ずれるため。画面に出した時刻からの差として
	// 扱うので、入力の意味も「今映っている時刻を何時何分に直すか」で一貫する。
	base int
}

func (model *Model) openTimeModal() {
	base := clockBase(time.Now())
	minutes := ((base+model.settings.TimeOffsetMinutes)%dayMinutes + dayMinutes) %
		dayMinutes
	model.timeModal = &timeModalState{
		hour:   minutes / 60,
		minute: minutes % 60,
		base:   base,
	}
}

const dayMinutes = 24 * 60

// clockBase は画面に出す時刻の基準を、システムの時計から 30 分刻みで返す。
// 初期値の組み立てと確定時のずれ計算が同じ値（timeModalState.base）を使う
// ので、開いてそのまま OK を押してもオフセットは変わらない。丸めを
// どちらか一方だけに掛けると、12:15 のような半端な時刻で押すたびに
// 30 分ずつ動く。
//
// 絶対時刻ではなく画面上の時計を丸めるので、システムの地域オフセットが
// 30 分刻みでなくても結果は変わらない。
func clockBase(value time.Time) int {
	sinceMidnight := time.Duration(value.Hour())*time.Hour +
		time.Duration(value.Minute())*time.Minute +
		time.Duration(value.Second())*time.Second +
		time.Duration(value.Nanosecond())
	return int(sinceMidnight.Round(30*time.Minute)/time.Minute) % dayMinutes
}

func (model *Model) handleTimeModalKey(key tea.Key) (tea.Model, tea.Cmd) {
	state := model.timeModal
	switch key.Code {
	case tea.KeyEscape:
		model.timeModal = nil
	case tea.KeyLeft:
		state.field = max(0, state.field-1)
	case tea.KeyRight:
		state.field = min(1, state.field+1)
	case tea.KeyUp:
		state.change(1)
	case tea.KeyDown:
		state.change(-1)
	case tea.KeyEnter, tea.KeyKpEnter:
		offset := timeOffsetFor(state.hour, state.minute, state.base)
		model.settings.TimeOffsetMinutes = offset
		model.saveSettings()
		model.chat.SetTimeOffset(offset)
		model.logs.SetTimeOffset(offset)
		model.timeModal = nil
	}
	return model, nil
}

func (state *timeModalState) change(step int) {
	if state.field == 1 {
		state.minute = 30 - state.minute
		return
	}
	state.hour = (state.hour + step + 24) % 24
}

// timeOffsetFor は日付を入力させない時計同士の差を、最も近い日付として
// (-12h, +12h] へ収める。入力も基準も 30 分刻みなので、ここでの丸めは要らない。
func timeOffsetFor(hour, minute, base int) int {
	offset := hour*60 + minute - base
	for offset <= -dayMinutes/2 {
		offset += dayMinutes
	}
	for offset > dayMinutes/2 {
		offset -= dayMinutes
	}
	return offset
}

func (model *Model) timeSettingsModal() (string, int, int) {
	state := model.timeModal
	hour := formatTwoDigits(state.hour)
	minute := formatTwoDigits(state.minute)
	hourField := "‹ " + hour + " ›"
	minuteField := "‹ " + minute + " ›"
	if state.field == 0 {
		hourField = selectedStyle.Render(hourField)
	} else {
		minuteField = selectedStyle.Render(minuteField)
	}

	offset := timeOffsetFor(state.hour, state.minute, state.base)
	lines := []string{
		msg.LabelCurrentTime + "  " + hourField + " : " + minuteField,
		"",
		msg.LabelTimeDrift + ": " + formatTimeOffset(offset),
	}
	width := 34
	for _, line := range lines {
		width = max(width, stringWidth(line)+4)
	}
	width = min(width, model.layout.width)
	height := len(lines) + 2
	x := max(0, (model.layout.width-width)/2)
	y := max(0, (model.layout.height-height)/2)
	return renderPanel(msg.TimeModalTitle, lines, width, height, false, modalFrame), x, y
}

func formatTwoDigits(value int) string {
	return fmt.Sprintf("%02d", value)
}
