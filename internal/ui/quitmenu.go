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

// glyph4 は幅可変・高さ4の粗いビットマップフォント。'#' が点灯、それ以外は
// 消灯。OPTIONS / QUIT の描画にしか使わないので、必要な文字だけ持つ。
var glyph4 = map[rune][4]string{
	'O': {".##.", "#..#", "#..#", ".##."},
	'P': {"###.", "#.#.", "###.", "#..."},
	'T': {"####", ".#..", ".#..", ".#.."},
	'I': {"###", ".#.", ".#.", "###"},
	'N': {"#..#", "##.#", "#.##", "#..#"},
	'S': {".###", "##..", "..##", "###."},
	'Q': {".##.", "#..#", "#.#.", ".###"},
	'U': {"#..#", "#..#", "#..#", ".##."},
}

// bigText は word を half-block 2 行のアスキーアートに描く。4 行分のビット
// マップを、1 セルに縦 2 ピクセルを詰める半角ブロック文字で 2 行に圧縮する。
func bigText(word string) [2]string {
	var rows [4]strings.Builder
	for index, letter := range word {
		glyph, ok := glyph4[letter]
		if !ok {
			continue
		}
		if index > 0 {
			for row := range rows {
				rows[row].WriteByte(' ')
			}
		}
		for row, line := range glyph {
			rows[row].WriteString(line)
		}
	}
	return [2]string{
		halfBlockLine(rows[0].String(), rows[1].String()),
		halfBlockLine(rows[2].String(), rows[3].String()),
	}
}

func halfBlockLine(top, bottom string) string {
	topRunes := []rune(top)
	bottomRunes := []rune(bottom)
	width := max(len(topRunes), len(bottomRunes))
	var line strings.Builder
	for index := 0; index < width; index++ {
		on := index < len(topRunes) && topRunes[index] == '#'
		off := index < len(bottomRunes) && bottomRunes[index] == '#'
		switch {
		case on && off:
			line.WriteRune('█')
		case on:
			line.WriteRune('▀')
		case off:
			line.WriteRune('▄')
		default:
			line.WriteRune(' ')
		}
	}
	return line.String()
}

func (model *Model) quitMenuModal() (string, int, int) {
	options := bigText("OPTIONS")
	quit := bigText("QUIT")

	optionsStyle, quitStyle := dimStyle, dimStyle
	if model.quitMenuCursor == quitMenuOptions {
		optionsStyle = selectedStyle
	} else {
		quitStyle = selectedStyle
	}

	lines := []string{
		optionsStyle.Render(options[0]),
		optionsStyle.Render(options[1]),
		"",
		quitStyle.Render(quit[0]),
		quitStyle.Render(quit[1]),
	}

	width := stringWidth(msg.QuitMenuTitle) + 4
	for _, line := range lines {
		width = max(width, stringWidth(line)+4)
	}
	width = min(width, model.layout.width)
	height := len(lines) + 2
	x := max(0, (model.layout.width-width)/2)
	y := max(0, (model.layout.height-height)/2)
	return renderPanel(msg.QuitMenuTitle, lines, width, height, false, modalFrame), x, y
}
