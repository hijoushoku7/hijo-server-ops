package ui

import tea "charm.land/bubbletea/v2"

func (model *Model) mouseDiscarded() bool {
	return model.settingsOpen || model.timeModal != nil || model.completionOpen ||
		model.quitMenuOpen || model.confirmOpen
}

// commandModalOpen はプレイヤーへのコマンド一覧が出ているかを返す。
func (model *Model) commandModalOpen() bool {
	return model.mode == modeFocus && model.panel == panelPlayers &&
		model.playerStage == playerStageCommands
}

// commandHitboxes はコマンドモーダルの枠込みの範囲と、本文行だけの範囲を返す。
func (model *Model) commandHitboxes() (frame, list hitbox) {
	x, y, width, height := model.commandModalBounds()
	frame = hitbox{x0: x, x1: x + width - 1, y0: y, y1: y + height - 1}
	list = hitbox{
		x0: x + 1, x1: x + width - 2,
		y0: y + 1, y1: y + len(playerCommands),
	}
	return frame, list
}

// commandAt はコマンドモーダルのどの項目を指しているかを返す。1 行 1 項目
// なので添字は行番号の差から出る。
func (model *Model) commandAt(x, y int) (int, bool) {
	_, list := model.commandHitboxes()
	if !list.contains(x, y) {
		return 0, false
	}
	return y - list.y0, true
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
	// コマンド一覧の上ではカーソルそのものを動かす。選んでも副作用が無いので
	// メニューのような専用のホバー用フィールドは持たない。
	if model.commandModalOpen() {
		// 閉じたあと選択モードへ戻ったときに、ポインタの無いパネルへホバー枠が
		// 残らないよう、モーダル中も位置は捨てておく。
		model.hovering = false
		if index, ok := model.commandAt(message.X, message.Y); ok {
			model.commandCursor = index
		}
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
	// モーダルは 2 つとも同じ扱い。ボタンや項目を押したときはキーボードと
	// 同じ経路へ流し、外しても中なら何もせず、外を押したときだけ閉じる。
	if model.confirmOpen {
		if button, ok := model.confirmButtonAt(message.X, message.Y); ok {
			model.confirmCursor = button
			return model.handleConfirmKey(tea.Key{Code: tea.KeyEnter})
		}
		if !model.confirmBox.contains(message.X, message.Y) {
			model.confirmOpen = false
		}
		return model, nil
	}
	if model.quitMenuOpen {
		if item, ok := model.quitMenuItemAt(message.X, message.Y); ok {
			return model.activateQuitMenu(item)
		}
		// 項目を外しても、モーダルの中なら何もしない。字形の左右の余白を
		// 押しただけでメニューごと閉じると、狙いを外すたびに開き直す。
		if !model.quitMenuBox.contains(message.X, message.Y) {
			model.quitMenuOpen = false
		}
		return model, nil
	}
	if model.mouseDiscarded() {
		return model, nil
	}
	// 項目を押せば実行。枠の中を外しただけなら何もしない。枠の外を押したら
	// 一覧へ戻したうえで、そのクリックを背後の処理へ渡す。捨てると、行の
	// ダブルクリックが「開いてすぐ閉じる」になる。
	if model.commandModalOpen() {
		// 開いた行そのものへのクリックは捨てる。画面が低いとモーダルが上へ
		// 押し戻されてその行を覆うので、ダブルクリックの 2 回目が下の項目に
		// 当たってしまう。上にも下にも出せない高さがあり、位置の調整では
		// 塞げない。
		if index, ok := model.playerAt(message.X, message.Y); ok &&
			index == model.playerCursor {
			return model, nil
		}
		if index, ok := model.commandAt(message.X, message.Y); ok {
			model.commandCursor = index
			model.applyPlayerCommand(index)
			return model, nil
		}
		if frame, _ := model.commandHitboxes(); frame.contains(message.X, message.Y) {
			return model, nil
		}
		model.playerStage = playerStagePlayers
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
	// モーダル中に playerCursor を動かすと、モーダルの表示位置ごと飛ぶ。
	if model.commandModalOpen() {
		model.commandCursor = clamp(model.commandCursor-delta, 0, len(playerCommands)-1)
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
