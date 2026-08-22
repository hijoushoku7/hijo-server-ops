package ui

import "charm.land/lipgloss/v2"

type frame struct {
	topLeft     string
	topRight    string
	bottomLeft  string
	bottomRight string
	horizontal  string
	vertical    string
	style       lipgloss.Style
}

var (
	plainFrame = frame{
		topLeft:     "┌",
		topRight:    "┐",
		bottomLeft:  "└",
		bottomRight: "┘",
		horizontal:  "─",
		vertical:    "│",
		style:       lipgloss.NewStyle().Foreground(lipgloss.Color("#777777")),
	}
	selectedFrame = frame{
		topLeft:     "┏",
		topRight:    "┓",
		bottomLeft:  "┗",
		bottomRight: "┛",
		horizontal:  "━",
		vertical:    "┃",
		style: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8BE9FD")).
			Bold(true),
	}
	hoverFrame = frame{
		topLeft:     "┌",
		topRight:    "┐",
		bottomLeft:  "└",
		bottomRight: "┘",
		horizontal:  "─",
		vertical:    "│",
		style:       lipgloss.NewStyle().Foreground(lipgloss.Color("#8BE9FD")),
	}
	focusedFrame = frame{
		topLeft:     "┏",
		topRight:    "┓",
		bottomLeft:  "┗",
		bottomRight: "┛",
		horizontal:  "━",
		vertical:    "┃",
		style: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F1FA8C")).
			Bold(true),
	}
	// modalFrame はフォーカス中と同じ色の細枠。重ねて出すモーダルが
	// 呼び出し元のパネルと同じものだと分かり、かつ枠の太さで区別できる。
	modalFrame = frame{
		topLeft:     "┌",
		topRight:    "┐",
		bottomLeft:  "└",
		bottomRight: "┘",
		horizontal:  "─",
		vertical:    "│",
		style:       focusedFrame.style,
	}
)

func (box frame) render(value string) string {
	return box.style.Render(value)
}

func (model *Model) frameFor(target panel) frame {
	if model.mode == modeSelect {
		// ホバーは選択枠を消した後も出す。マウスだけで操作を再開する
		// とっかかりが無くなるため。
		if model.hovering && model.hover == target &&
			(!model.selected || model.panel != target) {
			return hoverFrame
		}
		if !model.selected || model.panel != target {
			return plainFrame
		}
		return selectedFrame
	}
	if model.panel != target {
		return plainFrame
	}
	return focusedFrame
}
