package ui

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
	panelLog
	panelConsole
)

type playerStage uint8

const (
	playerStagePlayers playerStage = iota
	playerStageCommands
)

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
//	┌─ Graph ─┐┌──────────────────┐
//	┌─ Chat ──┐│       Log        │
//	└─────────┘└──────────────────┘
//	┌─────────── Console ───────────┐
//
// Stats / Meters / Graph は表示専用で選択対象でないため、Chat から上へ
// 抜けると Graph を飛ばして Players に着く。
type neighbor struct {
	up    panel
	down  panel
	left  panel
	right panel
}

var neighbors = [...]neighbor{
	panelPlayers: {
		up:    panelPlayers,
		down:  panelLog,
		left:  panelPlayers,
		right: panelPlayers,
	},
	panelChat: {
		up:    panelPlayers,
		down:  panelConsole,
		left:  panelChat,
		right: panelLog,
	},
	panelLog: {
		up:    panelPlayers,
		down:  panelConsole,
		left:  panelChat,
		right: panelLog,
	},
	panelConsole: {
		up:    panelChat,
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
	case panelLog:
		return "Log"
	default:
		return "Console"
	}
}
