package setup

import (
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/registry"
)

const (
	maxInputRunes = 512
	listViewport  = 10
)

type step uint8

const (
	stepWorkDir step = iota
	stepName
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
	name       string
	servers    registry.Registry
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

func newModel(configPath string, servers registry.Registry) *model {
	configDir := filepath.Dir(configPath)
	// 設定ファイルの置き場所をそのままサーバーディレクトリの初期値にする。
	// 大半のケースで同じディレクトリになる。
	return &model{
		configPath: configPath,
		configDir:  configDir,
		input:      []rune(configDir),
		servers:    servers,
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
	case stepName:
		return m.updateName(key.Key())
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
		m.input = []rune(defaultServerName(workDir))
		m.step = stepName
	default:
		m.editInput(key)
	}
	return m, nil
}

func (m *model) updateName(key tea.Key) (tea.Model, tea.Cmd) {
	switch key.Code {
	case tea.KeyEscape:
		m.step = stepWorkDir
		m.input = []rune(m.workDir)
	case tea.KeyEnter, tea.KeyKpEnter:
		name := string(m.input)
		if err := registry.ValidateName(name); err != nil {
			m.message = err.Error()
			return m, nil
		}
		if _, found := m.servers.Find(name); found {
			m.message = msg.DuplicateServerName(name).Error()
			return m, nil
		}
		m.name = name
		m.candidates = scanCommands(m.workDir)
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
		m.step = stepName
		m.input = []rune(m.name)
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
			m.step = stepName
			m.input = []rune(m.name)
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

func defaultServerName(workDir string) string {
	name := filepath.Base(filepath.Clean(workDir))
	if registry.ValidateName(name) != nil {
		return ""
	}
	return name
}
