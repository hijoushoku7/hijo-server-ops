package ui

// スマホなどの小さい端末から ssh したとき用の 1 列画面。Stats / Meters /
// Graph / Log は落とし、Players・Chat・Console だけを縦に積む。Options は
// 通常画面と同じく g のメニューから開く（設定モーダルは自分で端末の幅と
// 高さに合わせて縮む）。
func (model *Model) renderCompact() (string, int) {
	players := model.renderPlayersPanel()
	chat := model.renderBufferPanel(
		panelChat,
		&model.chat,
		model.layout.width,
		model.layout.chatHeight,
	)
	consoleText, caretX := model.consoleLine()
	console := renderPanel(
		model.compactConsoleTitle(),
		[]string{consoleText},
		model.layout.width,
		footerHeight,
		false,
		model.frameFor(panelConsole),
	)
	return players + "\n" + chat + "\n" + console + "\n" + model.keybar(), caretX
}

// compactConsoleTitle は Console の枠に運転状況を載せる。Stats を落とした
// ぶん、動いているかどうかだけはどこかに出す必要があるが、1 行を専有させる
// 余裕は無い。
func (model *Model) compactConsoleTitle() string {
	if model.status == "" {
		return panelConsole.title()
	}
	return panelConsole.title() + " · " + model.status
}
