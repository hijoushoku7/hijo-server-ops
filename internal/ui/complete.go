package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

var (
	timeCommandCompletions = []string{"set"}
	timeCompletions        = []string{"day", "night", "noon", "midnight"}
	weatherCompletions     = []string{"clear", "rain", "thunder"}
)

// completionScan は打ちかけの入力を解析し、候補の手前に残す文字列と候補を返す。
func (model *Model) completionScan() (string, []string) {
	original := string(model.input)
	input := strings.TrimPrefix(original, "/")
	words := strings.Split(input, " ")
	for len(words) > 0 && words[0] == "" {
		words = words[1:]
	}

	candidatesFor := func(command []string) []string {
		switch strings.Join(command, " ") {
		case "tell":
			return model.playerList
		case "time":
			return timeCommandCompletions
		case "time set":
			return timeCompletions
		case "weather":
			return weatherCompletions
		default:
			return nil
		}
	}

	prefix := ""
	keep := original
	candidates := candidatesFor(words)
	if len(candidates) > 0 {
		if !strings.HasSuffix(keep, " ") {
			keep += " "
		}
	} else if len(words) > 0 {
		prefix = words[len(words)-1]
		candidates = candidatesFor(words[:len(words)-1])
		start := strings.LastIndex(original, " ") + 1
		keep = original[:start]
	}
	if len(candidates) == 0 {
		return "", nil
	}

	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.HasPrefix(strings.ToLower(candidate), strings.ToLower(prefix)) {
			result = append(result, candidate)
		}
	}
	if len(result) == 0 {
		return "", nil
	}
	return keep, result
}

// completions は打ちかけの入力に対する候補を返す。
func (model *Model) completions() []string {
	_, candidates := model.completionScan()
	return candidates
}

// completionHint は入力の続きとして表示する未確定の候補を返す。
func (model *Model) completionHint() string {
	keep, candidates := model.completionScan()
	if len(candidates) == 0 {
		return ""
	}
	cursor := clamp(model.completionCursor, 0, len(candidates)-1)
	completed := keep + candidates[cursor]
	input := string(model.input)
	if !strings.HasPrefix(completed, input) {
		return ""
	}
	return strings.TrimPrefix(completed, input)
}

func (model *Model) handleCompletionKey(key tea.Key) (tea.Model, tea.Cmd) {
	candidates := model.completions()
	if len(candidates) == 0 {
		model.closeCompletion()
		return model.handleConsoleKey(key)
	}

	switch key.Code {
	case tea.KeyTab:
		model.completionCursor = (model.completionCursor + 1) % len(candidates)
	case tea.KeyUp, tea.KeyDown:
		model.completionCursor = moveCursor(
			key, model.completionCursor, len(candidates), min(len(candidates), 6),
		)
	case tea.KeyEnter, tea.KeyKpEnter:
		model.completionCursor = clamp(model.completionCursor, 0, len(candidates)-1)
		model.insertCompletion(candidates[model.completionCursor])
	case tea.KeyEscape:
		model.closeCompletion()
	case tea.KeyBackspace:
		if len(model.input) > 0 {
			model.input = model.input[:len(model.input)-1]
		}
		model.refreshCompletions()
	default:
		if key.Text != "" {
			model.appendInput(key.Text)
			model.refreshCompletions()
		}
	}
	return model, nil
}

func (model *Model) refreshCompletions() {
	candidates := model.completions()
	if len(candidates) == 0 {
		model.closeCompletion()
		return
	}
	model.completionCursor = clamp(model.completionCursor, 0, len(candidates)-1)
}

func (model *Model) insertCompletion(candidate string) {
	keep, _ := model.completionScan()
	completed := []rune(keep + candidate + " ")
	if len(completed) <= maxInputRunes {
		model.input = completed
	}
	model.closeCompletion()
}

func (model *Model) closeCompletion() {
	model.completionOpen = false
	model.completionCursor = 0
}

func (model *Model) completionModal() (string, int, int) {
	candidates := model.completions()
	width := 12
	for _, candidate := range candidates {
		width = max(width, stringWidth(candidate)+4)
	}
	width = min(width, model.layout.width)
	viewport := min(len(candidates), 6)
	height := viewport + 2
	start := windowStart(model.completionCursor, len(candidates), viewport)

	lines := make([]string, 0, viewport)
	for index := 0; index < viewport; index++ {
		position := start + index
		line := fitLine(" "+candidates[position], width-2)
		if position == model.completionCursor {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}

	x := clamp(2, 0, max(0, model.layout.width-width))
	y := model.layout.height - footerHeight - keybarHeight - height
	y = clamp(y, 0, max(0, model.layout.height-height))
	box := renderPanel("Tab", lines, width, height, false, modalFrame)
	return box, x, y
}
