package ui

import tea "charm.land/bubbletea/v2"

func (model *Model) mouseDiscarded() bool {
	return model.settingsOpen || model.timeModal != nil || model.completionOpen ||
		model.quitMenuOpen || model.confirmOpen ||
		(model.mode == modeFocus && model.panel == panelPlayers &&
			model.playerStage == playerStageCommands)
}

func (model *Model) handleMouseMotion(message tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	// メニューだけは重なっている間もホバーを追う。確認モーダル中は背後を
	// 触らせない。
	if model.exit == nil && model.quitMenuOpen && !model.confirmOpen {
		if item, ok := model.quitMenuItemAt(message.X, message.Y); ok {
			model.quitMenuHover = item
		} else {
			model.quitMenuHover = -1
		}
		return model, nil
	}
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
	if model.exit != nil || message.Button != tea.MouseLeft {
		return model, nil
	}
	// メニューは画面の中央に大きく出る。外したクリックは「やめる」の意図と
	// 見て閉じる。
	if model.confirmOpen {
		model.confirmOpen = false
		return model, nil
	}
	if model.quitMenuOpen {
		if item, ok := model.quitMenuItemAt(message.X, message.Y); ok {
			return model.activateQuitMenu(item)
		}
		model.quitMenuOpen = false
		return model, nil
	}
	if model.mouseDiscarded() {
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
		// 1 回目のクリックは仮選択（選択モードで枠だけ）。同じパネルをもう
		// 一度押すとフォーカスへ入る。キーボードの「矢印で選ぶ → Enter」と
		// 同じ 2 段にしておき、1 回のクリックで入力やスクロールの当たり先が
		// 変わらないようにする。
		if model.mode == modeSelect && model.selected && model.panel == target {
			model.mode = modeFocus
			return model, nil
		}
		model.panel = target
		model.mode = modeSelect
		model.selected = true
	}
	return model, nil
}

func (model *Model) handleMouseWheel(message tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
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
	// 違う。focusedBuffer から全面用の viewport を取る。開いたままの
	// モーダルは終了時に閉じないので、キー処理と同じく exit を先に見る。
	if model.exit != nil {
		buffer, viewport := model.focusedBuffer()
		buffer.Scroll(delta*3, viewport)
		return model, nil
	}
	if model.mouseDiscarded() {
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
