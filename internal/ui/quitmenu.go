package ui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
)

const (
	quitMenuOptions = iota
	quitMenuRestart
	quitMenuQuit
	quitMenuItemCount
)

// 罫線素片で組んだ高さ 3 行の字形。行の末尾の空白は幅を揃えるためのもので、
// 詰めてはいけない。
//
// カーソルの当たっている項目だけ太線にする。dim / 選択色 / アクセント色の
// 3 段だけで示すと、単色寄りの配色プリセットで差が読めなくなるため、線の
// 重さも一緒に変える。
var bigWords = [quitMenuItemCount][3]string{
	quitMenuOptions: {
		"╭─╮╭─╮╭┬╮┬╭─╮╭╮╷╭─╮",
		"│ │├─╯ │ ││ ││││╰─╮",
		"╰─╯╵   ┴ ┴╰─╯╵╰╯╰─╯",
	},
	quitMenuRestart: {
		"╭─╮╭─╴╭─╮╭┬╮╭─╮╭─╮╭┬╮",
		"├┬╯├─╴╰─╮ │ ├─┤├┬╯ │ ",
		"╵╰╴╰─╴╰─╯ ┴ ╵ ╵╵╰╴ ┴ ",
	},
	quitMenuQuit: {
		"╭─╮ ┬ ┬ ┬ ╭┬╮",
		"│─┼╮│ │ │  │ ",
		"╰─╯╰╰─╯ ┴  ┴ ",
	},
}

// 細線と太線は素片が 1 対 1 に対応しているので、太線の字形は持たずに置き換え
// で作る。字形を 2 組持つと、片方だけ直して桁がずれる。丸角には太線が無い
// ので、そこだけ角ばる。
var heavyStrokes = strings.NewReplacer(
	"─", "━", "│", "┃",
	"╭", "┏", "╮", "┓", "╰", "┗", "╯", "┛",
	"├", "┣", "┤", "┫", "┬", "┳", "┴", "┻", "┼", "╋",
	"╴", "╸", "╵", "╹", "╶", "╺", "╷", "╻",
)

// ブロック要素の本体と、右下へ 1 セルずらした影。影は本体より暗い色で描く。
var bigLogo = [8]string{
	"██   ██  ███████  ███████ ",
	"██░  ██░ ██░░░░░░ ██░░░██░",
	"██░  ██░ ██░      ██░  ██░",
	"███████░ ███████  ██░  ██░",
	"██░░░██░  ░░░░██░ ██░  ██░",
	"██░  ██░      ██░ ██░  ██░",
	"██░  ██░ ███████░ ███████░",
	" ░░   ░░  ░░░░░░░  ░░░░░░░",
}

const logoSubtitle = "hijo server ops"

// 左右の余白。枠に文字が貼り付かないよう広めに取る。
const quitMenuPad = 4

// ロゴを出すのに要る画面の大きさ。これを下回る端末ではロゴを落とし、
// OPTIONS / QUIT だけ出す。
const (
	quitMenuLogoHeight = 26
	quitMenuLogoWidth  = 26 + quitMenuPad*2 + 2
)

// styleLogo は本体（█）にアクセント色を、影（░）に dim を当てる。同じ文字が
// 続く区間をまとめて 1 回で描き、セルごとにエスケープを吐かない。
func styleLogo(line string) string {
	var result strings.Builder
	runes := []rune(line)
	for start := 0; start < len(runes); {
		end := start
		for end < len(runes) && runes[end] == runes[start] {
			end++
		}
		segment := string(runes[start:end])
		switch runes[start] {
		case '█':
			result.WriteString(titleStyle.Render(segment))
		case '░':
			result.WriteString(dimStyle.Render(segment))
		default:
			result.WriteString(segment)
		}
		start = end
	}
	return result.String()
}

func (model *Model) quitMenuModal() (string, int, int) {
	// 選択は背景の反転ではなく、字形と文字色の変化で示す。カーソルは太線＋
	// アクセント色、マウスのホバーは細線のまま選択色、それ以外は dim。
	drawFor := func(item int) ([3]string, lipgloss.Style) {
		word := bigWords[item]
		switch {
		case model.quitMenuCursor == item:
			for index, line := range word {
				word[index] = heavyStrokes.Replace(line)
			}
			return word, titleStyle
		case model.quitMenuHover == item:
			return word, selectionTextStyle
		default:
			return word, dimStyle
		}
	}

	showLogo := model.layout.height >= quitMenuLogoHeight &&
		model.layout.width >= quitMenuLogoWidth
	inner := 0
	for _, word := range bigWords {
		inner = max(inner, stringWidth(word[0]))
	}
	if showLogo {
		inner = max(inner, stringWidth(bigLogo[0]))
	}
	center := func(line string) string {
		return strings.Repeat(" ", quitMenuPad+(inner-stringWidth(line))/2) + line
	}

	var lines []string
	lines = append(lines, "")
	if showLogo {
		for _, line := range bigLogo {
			lines = append(lines, styleLogo(center(line)))
		}
		lines = append(lines,
			"",
			dimStyle.Italic(true).Render(center(logoSubtitle)),
			"",
		)
	}
	// 項目の行番号を控えておき、描き終えてから画面座標へ直してマウスの
	// 当たり判定にする。描画のたびに作り直すので、端末の大きさが変わっても
	// 次の描画で追いつく。
	var rows [quitMenuItemCount]int
	for item := range bigWords {
		word, style := drawFor(item)
		rows[item] = len(lines)
		for _, line := range word {
			lines = append(lines, style.Render(center(line)))
		}
		lines = append(lines, "")
	}

	width := max(stringWidth(msg.QuitMenuTitle)+4, inner+quitMenuPad*2+2)
	width = min(width, model.layout.width)
	height := len(lines) + 2
	x := max(0, (model.layout.width-width)/2)
	y := max(0, (model.layout.height-height)/2)
	model.quitMenuBox = hitbox{
		x0: x, x1: x + width - 1, y0: y, y1: y + height - 1,
	}
	for item, row := range rows {
		// 枠の 1 行ぶん下から字形の 3 行。横は中央寄せした字形の実幅だけを
		// 取る。行いっぱいにすると、離れた余白のクリックでも項目が動く。
		wordWidth := stringWidth(bigWords[item][0])
		left := x + 1 + quitMenuPad + (inner-wordWidth)/2
		model.quitMenuHits[item] = hitbox{
			x0: left, x1: left + wordWidth - 1,
			y0: y + 1 + row, y1: y + 1 + row + 2,
		}
	}
	return renderPanel(msg.QuitMenuTitle, lines, width, height, false, modalFrame), x, y
}

type hitbox struct {
	x0, y0, x1, y1 int
}

func (box hitbox) contains(x, y int) bool {
	return x >= box.x0 && x <= box.x1 && y >= box.y0 && y <= box.y1
}

// confirmButtonAt は最後に描いた確認モーダルのどのボタンを指しているかを返す。
func (model *Model) confirmButtonAt(x, y int) (int, bool) {
	for button, box := range model.confirmHits {
		if box.contains(x, y) {
			return button, true
		}
	}
	return 0, false
}

// quitMenuItemAt は最後に描いたメニューのどの項目を指しているかを返す。
func (model *Model) quitMenuItemAt(x, y int) (int, bool) {
	for item, box := range model.quitMenuHits {
		if box.contains(x, y) {
			return item, true
		}
	}
	return 0, false
}

// サーバーを止める操作の確認。誤って選んでもプレイヤーを落とさないよう、
// RESTART と QUIT からは必ずここを通す。既定は OK に置く。
const (
	confirmWidth       = 52
	confirmOK          = 0
	confirmCancel      = 1
	confirmButtonCount = 2
	confirmButtonGap   = "  "
)

// confirmText は確認中の項目に出す見出しと本文を返す。
func confirmText(item int) (string, string) {
	if item == quitMenuQuit {
		return msg.QuitConfirmTitle, msg.QuitConfirmBody
	}
	return msg.RestartConfirmTitle, msg.RestartConfirmBody
}

func (model *Model) confirmModal() (string, int, int) {
	// メニューと同じく、選択は反転ではなく文字色で示す。
	labels := [confirmButtonCount]string{"[ OK ]", "[ CANCEL ]"}
	buttons := labels
	for index := range buttons {
		if index == model.confirmCursor {
			buttons[index] = titleStyle.Render(buttons[index])
		} else {
			buttons[index] = dimStyle.Render(buttons[index])
		}
	}
	line := strings.Join(buttons[:], confirmButtonGap)

	title, body := confirmText(model.confirmItem)
	width := min(confirmWidth, max(2, model.layout.width-4))
	pad := func(text string) int {
		return max(0, (width-2-stringWidth(text))/2)
	}
	center := func(text string) string {
		return strings.Repeat(" ", pad(text)) + text
	}
	lines := []string{"", center(body), "", center(line), ""}
	height := len(lines) + 2
	x := max(0, (model.layout.width-width)/2)
	y := max(0, (model.layout.height-1-height)/2)

	// ボタンの当たり判定。行の中での桁は素のラベルから積み上げる。
	left := x + 1 + pad(strings.Join(labels[:], confirmButtonGap))
	buttonRow := y + 1 + 3
	for index, label := range labels {
		labelWidth := stringWidth(label)
		model.confirmHits[index] = hitbox{
			x0: left, x1: left + labelWidth - 1,
			y0: buttonRow, y1: buttonRow,
		}
		left += labelWidth + len(confirmButtonGap)
	}
	return renderPanel(title, lines, width, height, false, modalFrame), x, y
}
