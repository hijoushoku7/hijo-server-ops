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

		baseWidth := stringWidth(lines[row])
		if x < 0 || x >= baseWidth {
			continue
		}
		// 下地からはみ出す部分は捨て、行が伸びないようにする。
		boxLine = truncate(boxLine, baseWidth-x)

		// 切断位置が全角文字にかかると、左は文字が落ちて 1 セル狭くなり、
		// 右は文字が丸ごと残って 1 セル広くなる。どちらも幅を測って
		// 空白で詰め直し、行全体の桁数を変えない。
		cut := x + stringWidth(boxLine)
		want := max(0, baseWidth-cut)

		left := ansi.Truncate(lines[row], x, "")
		left += strings.Repeat(" ", max(0, x-stringWidth(left)))
		right := tail(ansi.TruncateLeft(lines[row], cut, ""), want)
		right = strings.Repeat(" ", max(0, want-stringWidth(right))) + right

		// 切った箇所で下地の色が漏れないよう、前後で属性を戻す。
		lines[row] = left + ansi.ResetStyle + boxLine + ansi.ResetStyle + right
	}
	return strings.Join(lines, "\n")
}
