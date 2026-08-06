package ui

import (
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/gclog"
	"github.com/hijoushoku7/hijo-server-ops/internal/hsperfdata"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/procstats"
)

const (
	exitModalWidth     = 64
	exitModalHeight    = 17
	normalExitWait     = 3 * time.Second
	unknownExitCode    = -1
	exitErrorLineLimit = 3
)

type exitSnapshot struct {
	heap hsperfdata.Memory
	rss  procstats.Number
	gc   gclog.Stats
}

type exitState struct {
	crashed     bool
	err         error
	exitCode    int
	exitedAt    time.Time
	uptime      time.Duration
	uptimeKnown bool
	errorLines  []string
	snapshot    exitSnapshot
	button      int
	closed      bool
	// notice は再起動を試みて失敗したときの理由。モーダルには status 行が
	// ないので、握り潰さずここへ出す。
	notice string

	autoQuit   bool
	autoQuitAt time.Time
}

type restartTracker struct {
	startedAt time.Time
	// logMark は現世代が始まった時点の累計ログ件数。エラー行の抜粋を
	// この世代に限り、前回のクラッシュの残骸を原因として出さない。
	logMark  uint64
	attempts []restartAttempt
}

// restartAttempt はフェーズ 2 の短命再起動判定で記録する 1 回分の枠だけを
// 先に定義する。今回は記録ロジックを持たせない。
type restartAttempt struct {
	startedAt time.Time
	exitedAt  time.Time
}

type exitCountdownMsg struct {
	state *exitState
}

func (model *Model) setProcessExit(message ProcessExitedMsg) tea.Cmd {
	model.busy = false
	model.endRestart()
	exitedAt := message.ExitedAt
	if exitedAt.IsZero() {
		exitedAt = time.Now()
	}
	startedAt := message.StartedAt
	if startedAt.IsZero() {
		startedAt = model.restart.startedAt
	}
	state := model.newExitState(message.Err != nil || message.ExitCode != 0,
		message.Err, message.ExitCode,
		startedAt, exitedAt)
	model.exit = state
	if state.crashed {
		return nil
	}
	state.autoQuit = true
	state.autoQuitAt = time.Now().Add(normalExitWait)
	return model.exitCountdownCmd(state)
}

// setFatalExit は hso 側の失敗（起動できなかった等）でモーダルを出す。
// 前世代の稼働時間やメモリは引き継がない。停止後にログを読んでいた時間まで
// uptime に足され、死んだサーバーの RSS が今の値として出てしまう。
func (model *Model) setFatalExit(err error) {
	model.busy = false
	model.endRestart()
	state := model.newExitState(true, err, unknownExitCode, time.Time{}, time.Now())
	state.snapshot = exitSnapshot{}
	model.exit = state
}

func (model *Model) newExitState(
	crashed bool,
	err error,
	exitCode int,
	startedAt time.Time,
	exitedAt time.Time,
) *exitState {
	uptime := time.Duration(0)
	known := !startedAt.IsZero() && !exitedAt.Before(startedAt)
	if known {
		uptime = exitedAt.Sub(startedAt)
	}
	return &exitState{
		crashed:     crashed,
		err:         err,
		exitCode:    exitCode,
		exitedAt:    exitedAt,
		uptime:      uptime,
		uptimeKnown: known,
		errorLines:  model.exitErrorLines(err),
		snapshot: exitSnapshot{
			heap: model.metrics.Heap,
			rss:  model.memory.RSS,
			gc:   model.gcStats,
		},
	}
}

func (model *Model) exitErrorLines(err error) []string {
	lines := make([]string, 0, exitErrorLineLimit+1)
	if err != nil {
		lines = append(lines, err.Error())
	}

	oldest := model.generationLogStart()
	matches := make([]string, 0, exitErrorLineLimit)
	for index := model.logs.Len() - 1; index >= oldest && len(matches) < exitErrorLineLimit; index-- {
		line := model.logs.At(index).line()
		if strings.Contains(line, "ERROR") || strings.Contains(line, "FATAL") ||
			strings.Contains(line, "Exception") || strings.Contains(line, "Caused by") {
			matches = append(matches, line)
		}
	}
	if len(matches) == 0 {
		for index := model.logs.Len() - 1; index >= oldest && len(matches) < exitErrorLineLimit; index-- {
			matches = append(matches, model.logs.At(index).line())
		}
	}
	for index := len(matches) - 1; index >= 0; index-- {
		lines = append(lines, matches[index])
	}
	return lines
}

// generationLogStart は現世代の最初のログが今どの位置にいるかを返す。
// バッファは再起動で消さないので、位置は古い行が押し出されるたびにずれる。
func (model *Model) generationLogStart() int {
	discarded := model.logsAdded - uint64(model.logs.Len())
	if model.restart.logMark <= discarded {
		return 0
	}
	return int(model.restart.logMark - discarded)
}

func (model *Model) exitCountdownCmd(state *exitState) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return exitCountdownMsg{state: state}
	})
}

func (model *Model) handleExitCountdown(message exitCountdownMsg) (tea.Model, tea.Cmd) {
	if model.exit != message.state || !message.state.autoQuit {
		return model, nil
	}
	if !time.Now().Before(message.state.autoQuitAt) {
		return model, tea.Quit
	}
	return model, model.exitCountdownCmd(message.state)
}

func (model *Model) exitCountdownSeconds() int {
	if model.exit == nil || !model.exit.autoQuit {
		return 0
	}
	remaining := time.Until(model.exit.autoQuitAt)
	if remaining <= 0 {
		return 0
	}
	return int((remaining + time.Second - 1) / time.Second)
}

func (model *Model) closeExitModal() {
	model.exit.autoQuit = false
	model.exit.closed = true
	model.mode = modeFocus
	model.panel = panelLog
}

func (model *Model) exitModal() (string, int, int) {
	state := model.exit
	width := min(exitModalWidth, max(2, model.layout.width-4))
	height := min(exitModalHeight, max(2, model.layout.height-3))
	title := msg.ExitTitleStopped
	boxFrame := focusedFrame
	boxFrame.style = exitStoppedStyle
	if state.crashed {
		title = msg.ExitTitleCrashed
		boxFrame.style = exitCrashStyle
	}

	exitCode := formatExitCode(state.exitCode)
	lines := []string{
		msg.ExitSummary(
			exitCode,
			state.exitedAt.Format("2006-01-02 15:04:05"),
			formatExitUptime(state.uptime, state.uptimeKnown),
		),
		msg.ExitMemory(
			formatProcBytes(state.snapshot.rss),
			formatJVMBytes(state.snapshot.heap.Used),
			formatJVMBytes(state.snapshot.heap.Committed),
			formatDelta(state.snapshot.rss, state.snapshot.heap.Committed),
		),
		msg.ExitGC(
			state.snapshot.gc.Collections,
			formatPause(
				state.snapshot.gc.LastPause.Value,
				state.snapshot.gc.LastPause.Available,
			),
		),
		"",
	}
	if len(state.errorLines) > 0 {
		lines = append(lines, msg.ExitErrorLines)
		for _, line := range state.errorLines {
			lines = append(lines, "  "+line)
		}
		lines = append(lines, "")
	}
	switch {
	case model.restarting:
		lines = append(lines, msg.ExitStateRestarting(model.restartDots()))
	case state.notice != "":
		lines = append(lines, state.notice)
	case state.crashed:
		lines = append(lines, msg.ExitStateCrashed)
	default:
		lines = append(lines, msg.ExitStateStopped)
		if seconds := model.exitCountdownSeconds(); seconds > 0 {
			lines = append(lines, msg.ExitAutoQuit(seconds))
		}
	}
	lines = append(lines, "", model.exitButtons(width-2))

	box := renderPanel(title, lines, width, height, false, boxFrame)
	x := max(0, (model.layout.width-width)/2)
	y := max(0, (model.layout.height-1-height)/2)
	return box, x, y
}

func (model *Model) exitButtons(width int) string {
	buttons := []string{
		"[ " + msg.ExitButtonLogs + " ]",
		"[ " + msg.ExitButtonRestart + " ]",
		"[ " + msg.ExitButtonQuit + " ]",
	}
	for index := range buttons {
		if index == model.exit.button {
			buttons[index] = selectedStyle.Render(buttons[index])
		} else {
			buttons[index] = dimStyle.Render(buttons[index])
		}
	}
	line := strings.Join(buttons, "  ")
	padding := max(0, (width-stringWidth(line))/2)
	return strings.Repeat(" ", padding) + line
}

func formatExitCode(code int) string {
	if code < 0 {
		return "n/a"
	}
	return strconv.Itoa(code)
}

func formatExitUptime(duration time.Duration, known bool) string {
	return formatUptime(hsperfdata.Duration{Value: duration, Available: known})
}
