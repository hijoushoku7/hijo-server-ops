package completion

import _ "embed"

//go:embed scripts/hso.bash
var bashScript string

//go:embed scripts/_hso
var zshScript string

//go:embed scripts/hso.fish
var fishScript string

// Script は指定されたシェルの補完スクリプトを返す。
func Script(shell string) (string, bool) {
	switch shell {
	case "bash":
		return bashScript, true
	case "zsh":
		return zshScript, true
	case "fish":
		return fishScript, true
	default:
		return "", false
	}
}
