package ui

import (
	"errors"
	"fmt"

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
}

type FatalMsg struct {
	Generation uint64
	Err        error
}

type ServerRestartingMsg struct{}

type ServerStartedMsg struct {
	Generation uint64
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
	layout            layout
	status            string
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
}

func New(
	actions chan<- Action,
	save func(Settings) error,
	generation uint64,
	settings Settings,
) *Model {
	applyTheme(settings)
	return &Model{
		status:     "starting",
		actions:    actions,
		save:       save,
		generation: generation,
		// 起動直後からサーバーコマンドを入力できる状態にする。
		mode:     modeFocus,
		panel:    panelConsole,
		settings: settings,
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
	case ServerStartedMsg:
		model.generation = message.Generation
		model.resetServerState()
		model.status = "starting"
		model.busy = false
	case ServerAddressMsg:
		if !model.accepts(message.Generation) {
			break
		}
		model.updateServerAddress(message)
	case ActionResultMsg:
		model.busy = false
		if message.Err != nil {
			model.status = msg.ActionFailed(message.Err)
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
		return model, tea.Quit
	case FatalMsg:
		if !model.accepts(message.Generation) {
			break
		}
		model.status = "error"
		if model.runErr == nil {
			model.runErr = message.Err
		}
		return model, tea.Quit
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
