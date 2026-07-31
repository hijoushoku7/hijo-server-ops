package ui

const (
	minimumWidth  = 72
	minimumHeight = 21
	statsHeight   = 10
	footerHeight  = 3
	keybarHeight  = 1
	// historyLines は各ペインが保持する行数。表示行数と切り離すことで
	// 画面外へ流れた行まで遡れる。
	historyLines = 500
	// 上段 3 列のうち Meters と Players に確保する最小幅。
	// Meters は "CPU   100%" の下にバーを敷く 2 行構成。Players は
	// Minecraft のユーザー名上限である 16 文字がそのまま入る幅
	// （枠の 2 列を足して 18）。
	minimumMetersWidth  = 20
	minimumPlayersWidth = 18
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
	statsWidth    int
	metersWidth   int
	playersWidth  int
}

func calculateLayout(width, height int) layout {
	result := layout{width: width, height: height}
	if width < minimumWidth || height < minimumHeight {
		return result
	}

	result.ready = true
	result.bodyHeight = height - statsHeight - footerHeight - keybarHeight
	result.leftWidth = width * 2 / 5
	result.rightWidth = width - result.leftWidth
	result.chatHeight = result.bodyHeight / 2
	result.commandHeight = result.bodyHeight - result.chatHeight

	// 上段は Stats / Meters / Players の 3 列。Meters と Players は
	// 必要な最小幅を確保し、余りをすべて Stats に回す。Stats は行が長く、
	// 狭いほど削られて困るのがこの列だけのため。
	result.playersWidth = clamp(width*3/20, minimumPlayersWidth, 22)
	result.metersWidth = clamp(width/4, minimumMetersWidth, 28)
	result.statsWidth = width - result.playersWidth - result.metersWidth
	result.graphWidth = max(0, result.statsWidth-2-6)
	return result
}

func clamp(value, low, high int) int {
	return min(max(value, low), high)
}

func (current layout) playerLines() int {
	if !current.ready {
		return 0
	}
	return max(0, statsHeight-2)
}

func (current layout) playersContentWidth() int {
	if !current.ready {
		return 0
	}
	return max(0, current.playersWidth-2)
}

func (current layout) metersContentWidth() int {
	if !current.ready {
		return 0
	}
	return max(0, current.metersWidth-2)
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
