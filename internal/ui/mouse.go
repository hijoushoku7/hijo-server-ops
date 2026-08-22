package ui

import tea "charm.land/bubbletea/v2"

func (model *Model) mouseDiscarded() bool {
	return model.settingsOpen || model.timeModal != nil || model.completionOpen ||
		(model.mode == modeFocus && model.panel == panelPlayers &&
			model.playerStage == playerStageCommands)
}

func (model *Model) handleMouseMotion(message tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	if model.exit != nil || model.mouseDiscarded() {
		return model, nil
	}
	target, ok := model.layout.panelAt(message.X, message.Y)
	model.hovering = ok
	if ok {
		model.hover = target
	}
	return model, nil
}

func (model *Model) handleMouseClick(message tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if model.exit != nil || model.mouseDiscarded() || message.Button != tea.MouseLeft {
		return model, nil
	}
	if index, ok := model.playerAt(message.X, message.Y); ok {
		model.playerCursor = index
		model.playerTarget = model.playerList[index]
		model.mode = modeFocus
		model.panel = panelPlayers
		model.playerStage = playerStageCommands
		model.commandCursor = 0
		model.selected = true
		return model, nil
	}
	if target, ok := model.layout.panelAt(message.X, message.Y); ok {
		model.panel = target
		model.selected = true
	}
	return model, nil
}

func (model *Model) handleMouseWheel(message tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if model.mouseDiscarded() {
		return model, nil
	}
	delta := 0
	switch message.Button {
	case tea.MouseWheelUp:
		delta = 1
	case tea.MouseWheelDown:
		delta = -1
	default:
		return model, nil
	}
	// 終了モーダル中はログが全面に出ており、ダッシュボードとは幅も高さも
	// 違う。focusedBuffer から全面用の viewport を取る。
	if model.exit != nil {
		buffer, viewport := model.focusedBuffer()
		buffer.Scroll(delta*3, viewport)
		return model, nil
	}
	target, ok := model.layout.panelAt(message.X, message.Y)
	if !ok {
		return model, nil
	}
	if target == panelPlayers {
		if len(model.playerList) > 0 {
			model.playerCursor = clamp(model.playerCursor-delta, 0, len(model.playerList)-1)
		}
		return model, nil
	}
	buffer, viewport := model.bufferFor(target)
	if buffer != nil {
		buffer.Scroll(delta*3, viewport)
	}
	return model, nil
}

// playerAt は Players の本文行に対応するプレイヤー一覧の添字を返す。
func (model *Model) playerAt(x, y int) (int, bool) {
	if !model.layout.ready || y < 1 || y > statsHeight-2 {
		return 0, false
	}
	left := model.layout.statsWidth + model.layout.metersWidth
	if x <= left || x >= left+model.layout.playersWidth-1 {
		return 0, false
	}
	index := windowStart(model.playerCursor, len(model.playerList), model.layout.playerLines()) + y - 1
	if index < 0 || index >= len(model.playerList) {
		return 0, false
	}
	return index, true
}
