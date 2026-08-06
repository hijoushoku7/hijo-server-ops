package ui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/gclog"
	"github.com/hijoushoku7/hijo-server-ops/internal/hsperfdata"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/procstats"
	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
)

const (
	maxInputRunes       = 512
	maxMetricErrorRunes = 256
)

type ActionKind uint8

const (
	ActionSendCommand ActionKind = iota
	ActionRestart
)

type Action struct {
	Kind    ActionKind
	Command string
}

type LogMsg struct {
	Generation uint64
	Entry      serverlog.Entry
}

type MetricsMsg struct {
	Generation   uint64
	JVM          hsperfdata.Metrics
	Memory       procstats.Memory
	CPU          float64
	CPUAvailable bool
	JVMError     string
	MemoryError  string
}

type GCMsg struct {
	Generation uint64
	Event      gclog.Event
}

type JavaFoundMsg struct {
	Generation uint64
	PID        int
}

type ProcessExitedMsg struct {
	Generation uint64
	Err        error
	ExitCode   int
	StartedAt  time.Time
	ExitedAt   time.Time
}

type FatalMsg struct {
	Generation uint64
	Err        error
}

type ServerRestartingMsg struct{}

type ServerStartedMsg struct {
	Generation uint64
	StartedAt  time.Time
}

type ServerAddressMsg struct {
	Generation uint64
	IP         string
	Port       uint16
	IPErr      string
	PortErr    string
}

type ActionResultMsg struct {
	Action Action
	Err    error
}

type Model struct {
	layout layout
	status string
	// runErr は現世代で最初に起きた終了原因。後続の終了通知でも上書きせず、
	// ServerStartedMsg で復旧したときだけ消す。
	runErr            error
	actions           chan<- Action
	save              func(Settings) error
	input             []rune
	mode              mode
	panel             panel
	consoleFocus      consoleFocus
	busy              bool
	generation        uint64
	metrics           hsperfdata.Metrics
	memory            procstats.Memory
	cpu               float64
	cpuAvailable      bool
	jvmMetricError    string
	memoryMetricError string
	serverIP          string
	serverPort        uint16
	gcStats           gclog.Stats
	tracker           serverlog.Tracker
	playerList        []string
	playerCursor      int
	playerStage       playerStage
	playerTarget      string
	commandCursor     int
	chat              lineBuffer
	logs              lineBuffer
	samples           sampleBuffer
	settings          Settings
	settingsOpen      bool
	settingCursor     int
	exit              *exitState
	restart           restartTracker
	// 0 は停止中、1..3 は表示する点の数。
	restartPhase int
	// 終了モーダル用に、最後に取れたメモリの値を控える。
	lastHeap hsperfdata.Memory
	lastRSS  procstats.Number
}

// restartTickMsg は再起動待ちの点を進める。停止から起動まで数十秒かかるので、
// 押した操作が生きていることを点の数で見せる。
type restartTickMsg struct{}

const restartTickInterval = 400 * time.Millisecond

func (model *Model) beginRestart() tea.Cmd {
	// すでに動いていれば tick を二重に走らせない。
	if model.restartPhase != 0 {
		model.restartPhase = 1
		return nil
	}
	model.restartPhase = 1
	return restartTick()
}

func (model *Model) endRestart() {
	model.restartPhase = 0
}

func restartTick() tea.Cmd {
	return tea.Tick(restartTickInterval, func(time.Time) tea.Msg {
		return restartTickMsg{}
	})
}

// restartDots は "." → ".." → "..." を返す。幅は常に 3 桁で、隣の表示が
// 点の数で揺れないようにする。
func (model *Model) restartDots() string {
	count := model.restartPhase
	return strings.Repeat(".", count) + strings.Repeat(" ", 3-count)
}

func New(
	actions chan<- Action,
	save func(Settings) error,
	generation uint64,
	settings Settings,
) *Model {
	applyTheme(settings)
	startedAt := time.Now()
	return &Model{
		status:     "starting",
		actions:    actions,
		save:       save,
		generation: generation,
		// 起動直後からサーバーコマンドを入力できる状態にする。
		mode:     modeFocus,
		panel:    panelConsole,
		settings: settings,
		restart:  restartTracker{startedAt: startedAt},
	}
}

func (model *Model) Init() tea.Cmd {
	return nil
}

func (model *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyPressMsg:
		return model.handleKey(message)
	case tea.WindowSizeMsg:
		model.resize(message.Width, message.Height)
	case LogMsg:
		if !model.accepts(message.Generation) {
			break
		}
		model.addLog(message.Entry)
	case MetricsMsg:
		if !model.accepts(message.Generation) {
			break
		}
		model.updateMetrics(message)
	case GCMsg:
		if !model.accepts(message.Generation) {
			break
		}
		model.gcStats.Add(message.Event)
	case JavaFoundMsg:
		if !model.accepts(message.Generation) {
			break
		}
		model.status = fmt.Sprintf("java pid %d", message.PID)
	case ServerRestartingMsg:
		model.status = "restarting"
		model.busy = true
		return model, model.beginRestart()
	case ServerStartedMsg:
		model.generation = message.Generation
		model.resetServerState()
		model.exit = nil
		model.restart.startedAt = message.StartedAt
		if model.restart.startedAt.IsZero() {
			model.restart.startedAt = time.Now()
		}
		model.restart.logMark = model.logs.nextNumber
		// 新しい世代が立ち上がった時点で復旧とみなす。前世代のクラッシュを
		// 抱えたままだと、その後に正常停止しても hso が失敗で終わる。
		model.runErr = nil
		model.status = "starting"
		model.busy = false
		model.endRestart()
	case ServerAddressMsg:
		if !model.accepts(message.Generation) {
			break
		}
		model.updateServerAddress(message)
	case ActionResultMsg:
		model.busy = false
		if message.Err != nil {
			model.endRestart()
			model.status = msg.ActionFailed(message.Err)
			// 終了モーダルには status 行がないので、失敗の理由をここへ移す。
			if model.exit != nil {
				model.exit.notice = model.status
			}
			break
		}
		if message.Action.Kind == ActionSendCommand {
			model.addLog(serverlog.SentCommand(message.Action.Command))
		}
	case ProcessExitedMsg:
		if !model.accepts(message.Generation) {
			break
		}
		model.status = "stopped"
		if model.runErr == nil {
			model.runErr = message.Err
		}
		return model, model.setProcessExit(message)
	case FatalMsg:
		if !model.accepts(message.Generation) {
			break
		}
		model.status = "error"
		if model.runErr == nil {
			model.runErr = message.Err
		}
		model.setFatalExit(message.Err)
	case exitCountdownMsg:
		return model.handleExitCountdown(message)
	case autoRestartMsg:
		return model.handleAutoRestart(message)
	case restartTickMsg:
		if model.restartPhase == 0 {
			break
		}
		model.restartPhase = model.restartPhase%3 + 1
		return model, restartTick()
	}
	return model, nil
}

func (model *Model) accepts(generation uint64) bool {
	return generation == 0 || generation == model.generation
}

func (model *Model) Err() error {
	return model.runErr
}

func IsExpectedExit(err error) bool {
	return err == nil || errors.Is(err, tea.ErrInterrupted)
}
