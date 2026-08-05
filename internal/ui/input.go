package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
)

func (model *Model) handleKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := message.Key()
	if message.String() == "ctrl+c" {
		return model, tea.Quit
	}
	if model.settingsOpen {
		return model.handleSettingsKey(key)
	}
	if (message.String() == "g" || message.String() == "G") &&
		!model.editingConsole() {
		model.settingsOpen = true
		model.settingCursor = 0
		return model, nil
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
		command := playerCommands[model.commandCursor]
		// 実行前に Console へ置くことで、破壊的なコマンドにも確認を挟む。
		model.input = []rune(fmt.Sprintf(command.template, model.playerTarget))
		model.playerStage = playerStagePlayers
		model.panel = panelConsole
		model.consoleFocus = consoleInput
	default:
		model.commandCursor = moveCursor(key, model.commandCursor,
			len(playerCommands), model.layout.playerLines())
	}
	return model, nil
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
		model.panel = move.up
	case tea.KeyDown:
		model.panel = move.down
	case tea.KeyLeft:
		model.panel = move.left
	case tea.KeyRight:
		model.panel = move.right
	case tea.KeyEnter, tea.KeyKpEnter:
		model.mode = modeFocus
		model.consoleFocus = consoleInput
	}
	return model, nil
}

func (model *Model) handleConsoleKey(key tea.Key) (tea.Model, tea.Cmd) {
	switch key.Code {
	case tea.KeyEscape:
		model.mode = modeSelect
	case tea.KeyTab:
		model.consoleFocus = (model.consoleFocus + 1) % consoleFocusCount
	case tea.KeyBackspace:
		if model.consoleFocus == consoleInput && len(model.input) > 0 {
			model.input = model.input[:len(model.input)-1]
		}
	case tea.KeyEnter, tea.KeyKpEnter:
		switch model.consoleFocus {
		case consoleStop:
			return model, tea.Quit
		case consoleRestart:
			model.offer(Action{Kind: ActionRestart})
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
		// フォーカスを外した後の新着を見落とさないよう最新へ戻す。
		buffer.ScrollToEnd()
		model.mode = modeSelect
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
	switch model.panel {
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
	command := strings.TrimSpace(string(model.input))
	if command == "" {
		return
	}
	if model.offer(Action{Kind: ActionSendCommand, Command: command}) {
		model.input = model.input[:0]
	}
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
