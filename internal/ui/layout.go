package ui

const (
	minimumWidth  = 72
	minimumHeight = 20
	statsHeight   = 10
	footerHeight  = 3
)

type layout struct {
	width         int
	height        int
	ready         bool
	bodyHeight    int
	leftWidth     int
	rightWidth    int
	chatHeight    int
	commandHeight int
	graphWidth    int
}

func calculateLayout(width, height int) layout {
	result := layout{width: width, height: height}
	if width < minimumWidth || height < minimumHeight {
		return result
	}

	result.ready = true
	result.bodyHeight = height - statsHeight - footerHeight
	result.leftWidth = width * 2 / 5
	result.rightWidth = width - result.leftWidth
	result.chatHeight = result.bodyHeight / 2
	result.commandHeight = result.bodyHeight - result.chatHeight
	result.graphWidth = max(0, width-2-6)
	return result
}

func (current layout) chatLines() int {
	if !current.ready {
		return 0
	}
	return max(0, current.chatHeight-2)
}

func (current layout) commandLines() int {
	if !current.ready {
		return 0
	}
	return max(0, current.commandHeight-2)
}

func (current layout) logLines() int {
	if !current.ready {
		return 0
	}
	return max(0, current.bodyHeight-2)
}

func (current layout) leftContentWidth() int {
	if !current.ready {
		return 0
	}
	return max(0, current.leftWidth-2)
}

func (current layout) rightContentWidth() int {
	if !current.ready {
		return 0
	}
	return max(0, current.rightWidth-2)
}
