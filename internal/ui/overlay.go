package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// overlay は base の (x, y) を左上として box を重ねる。
// base の各行を重なる位置で左右に切り、間に box の行を挟む。
// 桁がずれないよう、色付きの行も ansi パッケージ経由で切る。
func overlay(base, box string, x, y int) string {
	lines := strings.Split(base, "\n")
	for index, boxLine := range strings.Split(box, "\n") {
		row := y + index
		if row < 0 || row >= len(lines) {
			continue
		}

		left := ansi.Truncate(lines[row], x, "")
		left += strings.Repeat(" ", max(0, x-stringWidth(left)))
		right := ansi.TruncateLeft(lines[row], x+stringWidth(boxLine), "")
		// 切った箇所で下地の色が漏れないよう、前後で属性を戻す。
		lines[row] = left + ansi.ResetStyle + boxLine + ansi.ResetStyle + right
	}
	return strings.Join(lines, "\n")
}
