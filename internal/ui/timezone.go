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
}

func (model *Model) openTimeModal() {
	now := time.Now().Add(time.Duration(model.settings.TimeOffsetMinutes) * time.Minute)
	hour, minute := roundedClock(now)
	model.timeModal = &timeModalState{
		hour:   hour,
		minute: minute,
	}
}

func roundedClock(value time.Time) (int, int) {
	sinceMidnight := time.Duration(value.Hour())*time.Hour +
		time.Duration(value.Minute())*time.Minute +
		time.Duration(value.Second())*time.Second +
		time.Duration(value.Nanosecond())
	minutes := int(sinceMidnight.Round(30*time.Minute)/time.Minute) % (24 * 60)
	return minutes / 60, minutes % 60
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
		offset := timeOffsetFor(state.hour, state.minute, time.Now())
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
// (-12h, +12h] へ収める。保存値は 30 分刻みなので差も同じ単位へ丸める。
func timeOffsetFor(hour, minute int, now time.Time) int {
	offset := hour*60 + minute - (now.Hour()*60 + now.Minute())
	offset = roundOffsetMinutes(offset)
	for offset <= -720 {
		offset += 24 * 60
	}
	for offset > 720 {
		offset -= 24 * 60
	}
	return offset
}

func roundOffsetMinutes(minutes int) int {
	if minutes >= 0 {
		return ((minutes + 15) / 30) * 30
	}
	return -(((-minutes + 15) / 30) * 30)
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

	offset := timeOffsetFor(state.hour, state.minute, time.Now())
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
