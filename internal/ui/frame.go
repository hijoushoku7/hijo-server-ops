package ui

import "charm.land/lipgloss/v2"

// style は theme.go の init() が既定プリセットから流し込む。
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
	}
	selectedFrame = frame{
		topLeft:     "┏",
		topRight:    "┓",
		bottomLeft:  "┗",
		bottomRight: "┛",
		horizontal:  "━",
		vertical:    "┃",
	}
	hoverFrame = frame{
		topLeft:     "┌",
		topRight:    "┐",
		bottomLeft:  "└",
		bottomRight: "┘",
		horizontal:  "─",
		vertical:    "│",
	}
	focusedFrame = frame{
		topLeft:     "┏",
		topRight:    "┓",
		bottomLeft:  "┗",
		bottomRight: "┛",
		horizontal:  "━",
		vertical:    "┃",
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
