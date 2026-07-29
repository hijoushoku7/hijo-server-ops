package ui

// mode は選択モード（矢印でパネルを選ぶ）とフォーカスモード
// （選んだパネルを操作する）の 2 状態を表す。
type mode uint8

const (
	modeSelect mode = iota
	modeFocus
)

// panel は選択・フォーカスの対象になるパネル。
// Stats と Meters は表示専用で操作対象がないため含めない。
type panel uint8

const (
	panelPlayers panel = iota
	panelChat
	panelCommands
	panelLog
	panelConsole
)

// consoleFocus は Console パネル内のサブフォーカス。
// Console にフォーカス中は左右キーを入力に使うため、Tab で巡回する。
type consoleFocus uint8

const (
	consoleInput consoleFocus = iota
	consoleRestart
	consoleStop
)

const consoleFocusCount = 3

// playerStage は Players パネルにフォーカス中の段階。
// プレイヤーを選ぶ段階と、そのプレイヤーへのコマンドを選ぶ段階に分ける。
type playerStage uint8

const (
	playerStagePlayers playerStage = iota
	playerStageCommands
)

// playerCommand は選択したプレイヤーに対して実行できるコマンド。
// template の %s にプレイヤー名が入る。末尾が空白のものは引数を続けて
// 入力してもらう前提（tell の本文、kick / ban の理由）。
type playerCommand struct {
	label    string
	template string
}

// バニラに存在し、オンラインのプレイヤーに対して意味があるものだけを置く。
// pardon は対象がオンライン一覧に出ないため、tp は誰を誰に送るか決まらない
// ため入れない。
var playerCommands = []playerCommand{
	{label: "tell", template: "tell %s "},
	{label: "kick", template: "kick %s "},
	{label: "ban", template: "ban %s "},
	{label: "op", template: "op %s"},
	{label: "deop", template: "deop %s"},
	// ラベルは Players パネルの幅（16 列）に収まるよう短くする。
	// 送るコマンドは省略しない完全な形。
	{label: "whitelist add", template: "whitelist add %s"},
	{label: "whitelist rm", template: "whitelist remove %s"},
	{label: "gm survival", template: "gamemode survival %s"},
	{label: "gm creative", template: "gamemode creative %s"},
	{label: "gm adventure", template: "gamemode adventure %s"},
	{label: "gm spectator", template: "gamemode spectator %s"},
	{label: "kill", template: "kill %s"},
}

// neighbors はレイアウト固定を前提にした遷移表。
// 座標計算より、どのパネルからどこへ動くかが直接読める。
//
//	┌─ Stats ─┐┌─ Meters ─┐┌─ Players ─┐
//	└─────────┘└──────────┘└───────────┘
//	┌─ Chat ─────┐┌───────────────┐
//	┌─ Commands ─┐│      Log      │
//	└────────────┘└───────────────┘
//	┌─────────── Console ───────────┐
//
// Stats と Meters は選択対象でないため、上へ抜けると Players に着く。
type neighbor struct {
	up    panel
	down  panel
	left  panel
	right panel
}

var neighbors = map[panel]neighbor{
	panelPlayers: {
		up:    panelPlayers,
		down:  panelLog,
		left:  panelPlayers,
		right: panelPlayers,
	},
	panelChat: {
		up:    panelPlayers,
		down:  panelCommands,
		left:  panelChat,
		right: panelLog,
	},
	panelCommands: {
		up:    panelChat,
		down:  panelConsole,
		left:  panelCommands,
		right: panelLog,
	},
	panelLog: {
		up:    panelPlayers,
		down:  panelConsole,
		left:  panelChat,
		right: panelLog,
	},
	panelConsole: {
		up:    panelCommands,
		down:  panelConsole,
		left:  panelConsole,
		right: panelConsole,
	},
}

func (current panel) title() string {
	switch current {
	case panelPlayers:
		return "Players"
	case panelChat:
		return "Chat"
	case panelCommands:
		return "Commands"
	case panelLog:
		return "Log"
	default:
		return "Console"
	}
}
