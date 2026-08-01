package setup

import (
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
)

var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8BE9FD")).
			Bold(true)
	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#777777"))
	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#282A36")).
			Background(lipgloss.Color("#F1FA8C")).
			Bold(true)
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555"))
	keyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#282A36")).
			Background(lipgloss.Color("#BBBBBB"))
)

const (
	maxInputRunes = 512
	listViewport  = 10
)

type step uint8

const (
	stepWorkDir step = iota
	stepCommand
	stepCommandInput
	stepConfirm
)

type model struct {
	configPath string
	configDir  string
	step       step
	input      []rune
	workDir    string
	candidates []candidate
	cursor     int
	command    string // 設定に書く形の起動スクリプト
	commandAbs string // 権限確認と chmod に使う絶対パス
	fromInput  bool   // 起動スクリプトを一覧ではなく手入力で決めたか
	needsChmod bool   // 起動スクリプトに実行権限がないか
	grantChmod bool   // 実行権限を付けてよいという同意
	message    string
	created    bool
	err        error
}

func newModel(configPath string) *model {
	configDir := filepath.Dir(configPath)
	// 設定ファイルの置き場所をそのままサーバーディレクトリの初期値にする。
	// 大半のケースで同じディレクトリになる。
	return &model{
		configPath: configPath,
		configDir:  configDir,
		input:      []rune(configDir),
	}
}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if key.String() == "ctrl+c" {
		return m, tea.Quit
	}

	m.message = ""
	switch m.step {
	case stepWorkDir:
		return m.updateWorkDir(key.Key())
	case stepCommand:
		return m.updateCommand(key.Key())
	case stepCommandInput:
		return m.updateCommandInput(key.Key())
	default:
		return m.updateConfirm(key.Key())
	}
}

func (m *model) updateWorkDir(key tea.Key) (tea.Model, tea.Cmd) {
	switch key.Code {
	case tea.KeyEscape:
		return m, tea.Quit
	case tea.KeyEnter, tea.KeyKpEnter:
		workDir, err := resolveWorkDir(string(m.input))
		if err != nil {
			m.message = err.Error()
			return m, nil
		}
		m.workDir = workDir
		m.candidates = scanCommands(workDir)
		m.cursor = 0
		m.step = stepCommand
		if len(m.candidates) == 0 {
			// 候補がないなら選ばせる画面に意味がないので入力へ送る。
			m.message = msg.SetupNoCandidates
			m.input = []rune("./run.sh")
			m.step = stepCommandInput
		}
	default:
		m.editInput(key)
	}
	return m, nil
}

func (m *model) updateCommand(key tea.Key) (tea.Model, tea.Cmd) {
	count := len(m.candidates) + 1 // 末尾は手入力
	switch key.Code {
	case tea.KeyEscape:
		m.step = stepWorkDir
		m.input = []rune(m.workDir)
	case tea.KeyEnter, tea.KeyKpEnter:
		if m.cursor == len(m.candidates) {
			m.step = stepCommandInput
			m.input = []rune("./")
			return m, nil
		}
		m.selectCommand(m.candidates[m.cursor].name, false)
	default:
		m.cursor = moveCursor(key, m.cursor, count)
	}
	return m, nil
}

func (m *model) updateCommandInput(key tea.Key) (tea.Model, tea.Cmd) {
	switch key.Code {
	case tea.KeyEscape:
		if len(m.candidates) == 0 {
			m.step = stepWorkDir
			m.input = []rune(m.workDir)
			return m, nil
		}
		m.step = stepCommand
	case tea.KeyEnter, tea.KeyKpEnter:
		m.selectCommand(string(m.input), true)
	default:
		m.editInput(key)
	}
	return m, nil
}

func (m *model) selectCommand(input string, fromInput bool) {
	command, path, err := resolveCommand(input, m.workDir)
	if err != nil {
		m.message = err.Error()
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		m.message = err.Error()
		return
	}
	m.command = command
	m.commandAbs = path
	m.fromInput = fromInput
	m.needsChmod = info.Mode().Perm()&0o111 == 0
	// 実行権限がなければ hso は起動できないので、付ける側を初期値にする。
	// c で断れる。
	m.grantChmod = m.needsChmod
	m.step = stepConfirm
}

func (m *model) updateConfirm(key tea.Key) (tea.Model, tea.Cmd) {
	if m.needsChmod && key.Text == "c" {
		m.grantChmod = !m.grantChmod
		return m, nil
	}
	switch key.Code {
	case tea.KeyEscape:
		// 直前にいた画面へ戻す。
		if m.fromInput {
			m.step = stepCommandInput
			break
		}
		m.step = stepCommand
	case tea.KeyEnter, tea.KeyKpEnter:
		if m.needsChmod && m.grantChmod {
			if err := grantExecute(m.commandAbs); err != nil {
				m.message = err.Error()
				return m, nil
			}
			m.needsChmod = false
		}
		if err := writeConfig(m.configPath, m.preview()); err != nil {
			m.err = err
			return m, tea.Quit
		}
		m.created = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *model) editInput(key tea.Key) {
	switch key.Code {
	case tea.KeyBackspace:
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	default:
		if key.Text == "" || len(m.input)+len([]rune(key.Text)) > maxInputRunes {
			return
		}
		m.input = append(m.input, []rune(key.Text)...)
	}
}

func moveCursor(key tea.Key, cursor, count int) int {
	switch key.Code {
	case tea.KeyUp:
		cursor--
	case tea.KeyDown:
		cursor++
	case tea.KeyHome:
		cursor = 0
	case tea.KeyEnd:
		cursor = count - 1
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor > count-1 {
		cursor = count - 1
	}
	return cursor
}

func (m *model) preview() string {
	return render(m.command, m.workDir, m.configDir)
}

func (m *model) View() tea.View {
	lines := []string{
		titleStyle.Render(msg.SetupTitle),
		dimStyle.Render(msg.SetupTarget(m.configPath)),
		"",
	}
	lines = append(lines, m.body()...)
	if m.message != "" {
		lines = append(lines, "", errorStyle.Render(m.message))
	}
	lines = append(lines, "", m.keybar())
	return tea.NewView(strings.Join(lines, "\n") + "\n")
}

func (m *model) body() []string {
	switch m.step {
	case stepWorkDir:
		return []string{
			msg.SetupStepWorkDir,
			"",
			"  " + string(m.input) + "█",
		}
	case stepCommand:
		lines := []string{
			msg.SetupStepCommand,
			"",
		}
		return append(lines, m.candidateLines()...)
	case stepCommandInput:
		return []string{
			msg.SetupStepCommandInput,
			dimStyle.Render("  " + msg.SetupRelativeHint(m.workDir)),
			"",
			"  " + string(m.input) + "█",
		}
	default:
		lines := []string{msg.SetupStepConfirm, ""}
		for _, line := range strings.Split(strings.TrimRight(m.preview(), "\n"), "\n") {
			lines = append(lines, "  "+line)
		}
		if m.needsChmod {
			lines = append(lines, "", "  "+m.chmodLine(), dimStyle.Render(
				"  "+m.commandAbs,
			))
		}
		return lines
	}
}

// chmodLine は実行権限を付けるかどうかの表示。付けないままだと hso が
// 起動できないので、断ったときはその結果も出す。
func (m *model) chmodLine() string {
	if m.grantChmod {
		return msg.SetupChmodGrant
	}
	return errorStyle.Render(msg.SetupChmodDeny)
}

func (m *model) candidateLines() []string {
	labels := make([]string, 0, len(m.candidates)+1)
	for _, item := range m.candidates {
		labels = append(labels, item.label())
	}
	labels = append(labels, msg.SetupManualEntry)

	start := windowStart(m.cursor, len(labels), listViewport)
	end := min(start+listViewport, len(labels))

	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		if index == m.cursor {
			lines = append(lines, "  "+selectedStyle.Render(" "+labels[index]+" "))
			continue
		}
		lines = append(lines, "   "+labels[index])
	}
	if end < len(labels) {
		lines = append(lines, dimStyle.Render("   …"))
	}
	return lines
}

func windowStart(cursor, count, viewport int) int {
	if count <= viewport {
		return 0
	}
	start := cursor - viewport/2
	if start < 0 {
		start = 0
	}
	if start > count-viewport {
		start = count - viewport
	}
	return start
}

func (m *model) keybar() string {
	keys := [][2]string{}
	switch m.step {
	case stepWorkDir:
		// 最初の画面には戻り先がないので、Esc は Ctrl+C と同じく中止。
		return renderKeys([][2]string{
			{"Enter", msg.KeyNext},
			{"Esc / Ctrl+C", msg.KeyAbort},
		})
	case stepCommand:
		keys = append(keys,
			[2]string{"↑↓", msg.KeySelect},
			[2]string{"Enter", msg.KeyConfirm},
			[2]string{"Esc", msg.KeyBack},
		)
	case stepCommandInput:
		keys = append(keys,
			[2]string{"Enter", msg.KeyNext},
			[2]string{"Esc", msg.KeyBack},
		)
	default:
		keys = append(keys, [2]string{"Enter", msg.KeyCreate})
		if m.needsChmod {
			keys = append(keys, [2]string{"c", msg.KeyToggleChmod})
		}
		keys = append(keys, [2]string{"Esc", msg.KeyBack})
	}
	keys = append(keys, [2]string{"Ctrl+C", msg.KeyAbort})
	return renderKeys(keys)
}

func renderKeys(keys [][2]string) string {
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, keyStyle.Render(" "+key[0]+" ")+" "+key[1])
	}
	return strings.Join(parts, "  ")
}
