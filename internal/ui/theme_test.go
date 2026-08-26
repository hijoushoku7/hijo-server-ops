package ui

import "testing"

// プリセット名を改名すると map 参照が空文字になり、選択画面の色が消える。
func TestStartupColorsFilled(t *testing.T) {
	palette := StartupColors()
	for name, value := range map[string]string{
		"Title": palette.Title, "Foreground": palette.Foreground,
		"Background": palette.Background, "KeyForeground": palette.KeyForeground,
		"KeyBackground": palette.KeyBackground, "Dim": palette.Dim,
	} {
		if value == "" {
			t.Errorf("%s が空", name)
		}
	}
}
