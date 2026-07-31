package ui

import "charm.land/lipgloss/v2"

// frame はパネルの枠線。通常・選択中・フォーカス中の 3 状態を
// 罫線の太さと色で区別する。幅は 3 状態とも 1 セルで変わらない。
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

// styled は枠に設定の色を当てる。罫線の太さは状態ごとに固定で、
// 設定で変わるのは色だけ。
func (model *Model) styled(box frame, color string) frame {
	if color == "" {
		return box
	}
	box.style = box.style.Foreground(lipgloss.Color(color))
	return box
}

// displayFrame は選択対象でないパネル（Stats / Meters / Graph）の枠。
func (model *Model) displayFrame() frame {
	return model.styled(plainFrame, model.settings.FrameColor)
}

func (model *Model) frameFor(target panel) frame {
	if model.panel != target {
		return model.displayFrame()
	}
	if model.mode == modeFocus {
		return model.styled(focusedFrame, model.settings.FocusedFrameColor)
	}
	return model.styled(selectedFrame, model.settings.SelectedFrameColor)
}
