package ui

import (
	"strings"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
)

const (
	quitMenuOptions = iota
	quitMenuQuit
	quitMenuItemCount
)

// 罫線素片で組んだ高さ 3 行の字形。選択の有無で字形は変えず、色と太字だけを
// 変える。行の末尾の空白は幅を揃えるためのもので、詰めてはいけない
var (
	bigOptions = [3]string{
		"╭─╮╭─╮╭┬╮┬╭─╮╭╮╷╭─╮",
		"│ │├─╯ │ ││ ││││╰─╮",
		"╰─╯╵   ┴ ┴╰─╯╵╰╯╰─╯",
	}
	bigQuit = [3]string{
		"╭─╮ ┬ ┬ ┬ ╭┬╮",
		"│─┼╮│ │ │  │ ",
		"╰─╯╰╰─╯ ┴  ┴ ",
	}
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
	quitMenuLogoHeight = 22
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
	// 選択は背景の反転ではなく、暗い文字から明るいアクセント色＋太字への
	// 変化で示す。どの配色プリセットでも dim とアクセントには明暗差がある。
	optionsStyle, quitStyle := dimStyle, dimStyle
	if model.quitMenuCursor == quitMenuOptions {
		optionsStyle = titleStyle
	} else {
		quitStyle = titleStyle
	}

	showLogo := model.layout.height >= quitMenuLogoHeight &&
		model.layout.width >= quitMenuLogoWidth
	inner := stringWidth(bigOptions[0])
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
	for _, line := range bigOptions {
		lines = append(lines, optionsStyle.Render(center(line)))
	}
	lines = append(lines, "")
	for _, line := range bigQuit {
		lines = append(lines, quitStyle.Render(center(line)))
	}
	lines = append(lines, "")

	width := max(stringWidth(msg.QuitMenuTitle)+4, inner+quitMenuPad*2+2)
	width = min(width, model.layout.width)
	height := len(lines) + 2
	x := max(0, (model.layout.width-width)/2)
	y := max(0, (model.layout.height-height)/2)
	return renderPanel(msg.QuitMenuTitle, lines, width, height, false, modalFrame), x, y
}
