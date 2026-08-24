package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
)

func (model *Model) handleKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := message.Key()
	if message.String() == "ctrl+c" {
		return model, model.requestQuit()
	}
	// 停止待ちの間は ^C 以外を受けない。再起動やコマンドを重ねられると、
	// サーバーが動いているのに quitting が立ったままになり、次の ^C が
	// 保存を待たずに殺す元の挙動へ戻る。
	if model.quitting && model.exit == nil {
		return model, nil
	}
	if model.exit != nil {
		return model.handleExitKey(message)
	}
	// g/G は既存どおり設定専用にする。vim の gg/G を追加すると既存キーを
	// 奪うため、先頭・末尾への移動には使わない。
	if model.timeModal == nil && !model.settingsOpen &&
		(message.String() == "g" || message.String() == "G") &&
		!model.editingConsole() {
		model.settingsOpen = true
		model.settingCursor = 0
		return model, nil
	}
	if !model.editingConsole() {
		key = hjklArrowKey(key)
	}
	if model.timeModal != nil {
		return model.handleTimeModalKey(key)
	}
	if model.settingsOpen {
		return model.handleSettingsKey(key)
	}
	if model.mode == modeSelect {
		return model.handleSelectKey(key)
	}
	if model.panel == panelConsole {
		return model.handleConsoleKey(key)
	}
	if model.panel == panelPlayers {
		return model.handlePlayersKey(key)
	}
	return model.handleBufferKey(key)
}

func (model *Model) handleExitKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := hjklArrowKey(message.Key())
	// 立ち直った後は Enter で閉じるだけ。裏で復旧していても、落ちた事実を
	// 読んで消す操作を挟ませる。表示側（exitModal・keybar）も restarted を
	// 最初に見るので、判定の順番を合わせる。
	if model.exit.restarted {
		if key.Code == tea.KeyEnter || key.Code == tea.KeyKpEnter {
			model.exit = nil
		}
		return model, nil
	}
	// 自動再起動の最中は、やめる操作だけを受ける。裏で立ち直る途中に
	// 再起動や終了を重ねられると、どちらが効いたのか分からなくなる。
	if model.exit.autoRestart {
		if key.Code == tea.KeyEscape {
			model.cancelAutoRestart()
		}
		return model, nil
	}
	if model.exit.closed {
		switch message.String() {
		case "r", "R":
			return model, model.requestRestart()
		case "q", "Q":
			return model, tea.Quit
		}
		switch key.Code {
		case tea.KeyEnter, tea.KeyKpEnter:
			return model, tea.Quit
		case tea.KeyUp, tea.KeyDown, tea.KeyPgUp, tea.KeyPgDown,
			tea.KeyHome, tea.KeyEnd:
			return model.handleBufferKey(key)
		default:
			return model, nil
		}
	}

	// 正常終了の自動終了は、モーダルに対するどのキー操作でも解除する。
	model.exit.autoQuitAt = time.Time{}
	switch key.Code {
	case tea.KeyLeft:
		model.exit.button = (model.exit.button + 2) % 3
	case tea.KeyRight, tea.KeyTab:
		model.exit.button = (model.exit.button + 1) % 3
	case tea.KeyEscape:
		model.closeExitModal()
	case tea.KeyEnter, tea.KeyKpEnter:
		switch model.exit.button {
		case 0:
			model.closeExitModal()
		case 1:
			return model, model.requestRestart()
		case 2:
			return model, tea.Quit
		}
	}
	return model, nil
}

// hjklArrowKey は修飾なしの vim 風移動キーだけを矢印へ読み替える。
// Text は文字入力として二重に扱われないよう、変換時に捨てる。
func hjklArrowKey(key tea.Key) tea.Key {
	if key.Mod != 0 {
		return key
	}
	switch key.Code {
	case 'h':
		key.Code = tea.KeyLeft
	case 'j':
		key.Code = tea.KeyDown
	case 'k':
		key.Code = tea.KeyUp
	case 'l':
		key.Code = tea.KeyRight
	default:
		return key
	}
	key.Text = ""
	return key
}

func (model *Model) editingConsole() bool {
	return model.mode == modeFocus && model.panel == panelConsole &&
		model.consoleFocus == consoleInput
}

func (model *Model) handlePlayersKey(key tea.Key) (tea.Model, tea.Cmd) {
	if model.playerStage == playerStageCommands {
		return model.handlePlayerCommandKey(key)
	}

	cursor := moveCursor(key, model.playerCursor, len(model.playerList),
		model.layout.playerLines())
	switch key.Code {
	case tea.KeyEscape:
		model.playerCursor = 0
		model.mode = modeSelect
		model.selected = true
	case tea.KeyEnter, tea.KeyKpEnter:
		if model.playerCursor < len(model.playerList) {
			model.playerTarget = model.playerList[model.playerCursor]
			model.playerStage = playerStageCommands
			model.commandCursor = 0
		}
	default:
		model.playerCursor = cursor
	}
	return model, nil
}

func (model *Model) handlePlayerCommandKey(key tea.Key) (tea.Model, tea.Cmd) {
	switch key.Code {
	case tea.KeyEscape:
		model.playerStage = playerStagePlayers
	case tea.KeyEnter, tea.KeyKpEnter:
		model.applyPlayerCommand(model.commandCursor)
	default:
		if index, ok := playerCommandIndex(key.Code); ok {
			model.applyPlayerCommand(index)
		} else {
			model.commandCursor = moveCursor(key, model.commandCursor,
				len(playerCommands), model.layout.playerLines())
		}
	}
	return model, nil
}

func (model *Model) applyPlayerCommand(index int) {
	if index < 0 || index >= len(playerCommands) {
		return
	}
	command := playerCommands[index]
	// 実行前に Console へ置くことで、破壊的なコマンドにも確認を挟む。
	model.input = []rune(fmt.Sprintf(command.template, model.playerTarget))
	model.playerStage = playerStagePlayers
	model.panel = panelConsole
	model.consoleFocus = consoleInput
}

const playerCommandAccelerators = "123456789qwertyuio"

func playerCommandIndex(code rune) (int, bool) {
	for index := range playerCommands {
		if code == rune(playerCommandAccelerators[index]) {
			return index, true
		}
	}
	return 0, false
}

func commandAccelerator(index int) string {
	if index < 0 || index >= len(playerCommandAccelerators) {
		return ""
	}
	return "( " + string(playerCommandAccelerators[index]) + " )"
}

func moveCursor(key tea.Key, cursor, count, viewport int) int {
	switch key.Code {
	case tea.KeyUp:
		cursor--
	case tea.KeyDown:
		cursor++
	case tea.KeyPgUp:
		cursor -= viewport
	case tea.KeyPgDown:
		cursor += viewport
	case tea.KeyHome:
		cursor = 0
	case tea.KeyEnd:
		cursor = count - 1
	}
	return clamp(cursor, 0, max(0, count-1))
}

func windowStart(cursor, count, viewport int) int {
	if viewport <= 0 || count <= viewport {
		return 0
	}
	return clamp(cursor-viewport/2, 0, count-viewport)
}

func (model *Model) handleSelectKey(key tea.Key) (tea.Model, tea.Cmd) {
	move := neighbors[model.panel]
	switch key.Code {
	case tea.KeyUp:
		model.selected = true
		model.panel = move.up
	case tea.KeyDown:
		model.selected = true
		model.panel = move.down
	case tea.KeyLeft:
		model.selected = true
		model.panel = move.left
	case tea.KeyRight:
		model.selected = true
		model.panel = move.right
	case tea.KeyEnter, tea.KeyKpEnter:
		if !model.selected {
			return model, nil
		}
		model.mode = modeFocus
		model.consoleFocus = consoleInput
	case tea.KeyEscape:
		if buffer, _ := model.bufferFor(model.panel); buffer != nil && !buffer.Following() {
			buffer.ScrollToEnd()
		} else if model.panel == panelChat || model.panel == panelLog {
			model.selected = false
		}
	}
	return model, nil
}

func (model *Model) handleConsoleKey(key tea.Key) (tea.Model, tea.Cmd) {
	if model.completionOpen {
		return model.handleCompletionKey(key)
	}
	switch key.Code {
	case tea.KeyEscape:
		model.mode = modeSelect
		model.selected = true
	case tea.KeyTab:
		if model.consoleFocus == consoleInput {
			candidates := model.completions()
			if len(candidates) == 1 {
				model.insertCompletion(candidates[0])
				return model, nil
			}
			if len(candidates) > 1 {
				model.completionOpen = true
				model.completionCursor = 0
				return model, nil
			}
		}
		model.consoleFocus = (model.consoleFocus + 1) % consoleFocusCount
	case tea.KeyBackspace:
		if model.consoleFocus == consoleInput && len(model.input) > 0 {
			model.input = model.input[:len(model.input)-1]
		}
	case tea.KeyEnter, tea.KeyKpEnter:
		switch model.consoleFocus {
		case consoleStop:
			return model, model.requestQuit()
		case consoleRestart:
			return model, model.requestRestart()
		default:
			model.sendInput()
		}
	default:
		if model.consoleFocus == consoleInput && key.Text != "" {
			model.appendInput(key.Text)
		}
	}
	return model, nil
}

func (model *Model) handleBufferKey(key tea.Key) (tea.Model, tea.Cmd) {
	buffer, viewport := model.focusedBuffer()
	if buffer == nil {
		return model, nil
	}

	switch key.Code {
	case tea.KeyEscape:
		if !buffer.Following() {
			buffer.ScrollToEnd()
		} else {
			model.mode = modeSelect
			model.selected = true
		}
	case tea.KeyUp:
		buffer.Scroll(1, viewport)
	case tea.KeyDown:
		buffer.Scroll(-1, viewport)
	case tea.KeyPgUp:
		buffer.Scroll(viewport.height, viewport)
	case tea.KeyPgDown:
		buffer.Scroll(-viewport.height, viewport)
	case tea.KeyHome:
		buffer.ScrollToStart(viewport)
	case tea.KeyEnd:
		buffer.ScrollToEnd()
	}
	return model, nil
}

func (model *Model) focusedBuffer() (*lineBuffer, bufferViewport) {
	if model.exit != nil {
		return &model.logs, bufferViewport{
			width:  max(0, model.layout.width-2),
			height: max(0, model.layout.height-3),
		}
	}
	return model.bufferFor(model.panel)
}

func (model *Model) bufferFor(target panel) (*lineBuffer, bufferViewport) {
	switch target {
	case panelChat:
		return &model.chat, bufferViewport{
			width:  model.layout.leftContentWidth(),
			height: model.layout.chatLines(),
		}
	case panelLog:
		return &model.logs, bufferViewport{
			width:  model.layout.rightContentWidth(),
			height: model.layout.logLines(),
		}
	default:
		return nil, bufferViewport{}
	}
}

func (model *Model) appendInput(text string) {
	for _, character := range text {
		if len(model.input) >= maxInputRunes {
			return
		}
		if character >= ' ' && character != '\x7f' {
			model.input = append(model.input, character)
		}
	}
}

func (model *Model) sendInput() {
	defer model.closeCompletion()
	command := strings.TrimSpace(string(model.input))
	if command == "" {
		return
	}
	if model.offer(Action{Kind: ActionSendCommand, Command: command}) {
		model.input = model.input[:0]
	}
}

// requestRestart は再起動を頼み、受け付けられたときだけ点のアニメーションを
// 始める。詰まって落とされた要求で動かすと、動いていないものが動いて見える。
func (model *Model) requestRestart() tea.Cmd {
	if !model.offer(Action{Kind: ActionRestart}) {
		return nil
	}
	return model.beginRestart()
}

// requestQuit は動いているサーバーに stop を送り、自力で終わるのを待つ。
// hso が先に落ちると Pdeathsig 経由で supervisor が短い猶予で殺しにかかり、
// ワールドの保存が終わらない。待っている間にもう一度 ^C を押したときと、
// 待つ相手がいないときは、従来どおりその場で終わる。
func (model *Model) requestQuit() tea.Cmd {
	if model.quitting || model.exit != nil {
		return tea.Quit
	}
	if !model.offer(Action{Kind: ActionSendCommand, Command: "stop"}) {
		return tea.Quit
	}
	model.quitting = true
	model.status = msg.StatusStopping
	return nil
}

func (model *Model) offer(action Action) bool {
	if model.busy || model.actions == nil {
		return false
	}
	select {
	case model.actions <- action:
		model.busy = true
		if action.Kind == ActionRestart {
			model.status = "restarting"
		}
		return true
	default:
		model.status = msg.StatusIdle
		return false
	}
}
