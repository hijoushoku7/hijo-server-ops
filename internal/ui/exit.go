package ui

import (
	"errors"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/gclog"
	"github.com/hijoushoku7/hijo-server-ops/internal/hsperfdata"
	"github.com/hijoushoku7/hijo-server-ops/internal/javaenv"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/procstats"
)

const (
	exitModalWidth     = 64
	exitModalHeight    = 17
	normalExitWait     = 3 * time.Second
	unknownExitCode    = -1
	exitErrorLineLimit = 3

	// autoRestartWait は自動再起動までの待ち。落ちた直後に叩き直すと、
	// ポートやワールドのロックが外れる前に次が始まる。
	autoRestartWait = 5 * time.Second
	// shortRunLimit より短く終わった起動を短命とみなし、それが
	// shortRunGiveUp 回続いたらクラッシュループとして自動再起動をやめる。
	shortRunLimit  = 60 * time.Second
	shortRunGiveUp = 3
)

type exitSnapshot struct {
	heap hsperfdata.Memory
	rss  procstats.Number
	gc   gclog.Stats
}

type exitState struct {
	crashed    bool
	exitCode   int
	exitedAt   time.Time
	uptime     hsperfdata.Duration
	errorLines []string
	snapshot   exitSnapshot
	button     int
	closed     bool
	// notice は再起動を試みて失敗したときの理由。モーダルには status 行が
	// ないので、握り潰さずここへ出す。
	notice string

	autoQuitAt time.Time

	// autoRestart が立っている間は、背景をダッシュボードのまま保ち、
	// 三択のボタンを出さない。裏で立ち直る想定なので、ログ全面に
	// 切り替えて操作を促す画面にはしない。
	autoRestart bool
	// autoRestartAt はバックオフの明け時刻。0 なら待ちは終わっている。
	autoRestartAt time.Time
	// restarted は自動再起動で新しい世代が立ち上がった状態。落ちたことを
	// 見落とさないよう、モーダルは勝手に消さず Enter を待つ。
	restarted    bool
	javaMismatch bool
	// fatal は hso 自身の失敗で開いたモーダル。findJava の失敗のように
	// hso が SIGTERM を送る経路では、後から同じ世代の終了通知が届いて
	// 状態を作り直すので、自動再起動の対象外であることを引き継ぐ。
	fatal bool
}

type restartTracker struct {
	startedAt time.Time
	// logMark は現世代が始まった時点の累計ログ件数。エラー行の抜粋を
	// この世代に限り、前回のクラッシュの残骸を原因として出さない。
	logMark  uint64
	attempts []restartAttempt
	// recorded は現世代の終了をもう書き留めたか。手動 restart は
	// controller が runtime を切り離すので終了通知が届かない。
	recorded bool
}

type restartAttempt struct {
	startedAt time.Time
	exitedAt  time.Time
}

// record は 1 世代分の稼働を書き留める。shortRunLimit 以上もった世代は
// 立ち直ったとみなして履歴を捨てる。残すと、何日も動いた後の 1 回目の
// クラッシュが過去の失敗と合算されて打ち切られる。
//
// 履歴を捨てる判定は終了の種類を見ない。正常停止でも、長くもった世代が
// 挟まれば立ち直っている。数えるのは短命な異常終了だけで、短時間で
// 止められた正常停止は増やしも減らしもしない。
func (tracker *restartTracker) record(startedAt, exitedAt time.Time, crashed bool) {
	tracker.recorded = true
	// 稼働時間が分からない世代は、短命とも立ち直りとも言えないので触らない。
	if startedAt.IsZero() || exitedAt.Before(startedAt) {
		return
	}
	if exitedAt.Sub(startedAt) >= shortRunLimit {
		tracker.attempts = nil
		return
	}
	if crashed {
		tracker.attempts = append(tracker.attempts, restartAttempt{
			startedAt: startedAt,
			exitedAt:  exitedAt,
		})
	}
}

// recordStop は終了通知が来ないまま畳まれる世代を書き留める。手動 restart
// では controller が runtime を切り離すため ProcessExitedMsg が届かず、
// ここで拾わないと長くもった世代でも履歴が残る。
func (tracker *restartTracker) recordStop(stoppedAt time.Time) {
	if tracker.recorded {
		return
	}
	tracker.record(tracker.startedAt, stoppedAt, false)
}

func (tracker *restartTracker) crashLoop() bool {
	return len(tracker.attempts) >= shortRunGiveUp
}

type exitCountdownMsg struct {
	state *exitState
}

// autoRestartMsg はバックオフの残りを数える。待ちを controller に持たせず
// UI 側の tick にすることで、待っている間も画面が更新され、やめられる。
type autoRestartMsg struct {
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
	err := message.Err
	crashed := err != nil || message.ExitCode != 0
	// 先に出ていた原因を残す。java を見つけられず hso が SIGTERM を送った
	// ような場合、後から届く「exit status 143」に置き換えると本当の理由が
	// モーダルから消える。
	if model.runErr != nil {
		err = model.runErr
		crashed = true
	}
	state := model.newExitState(crashed, err, message.ExitCode, startedAt, exitedAt)
	// hso 自身の失敗で開いていたモーダルを、その後始末で届く終了通知が
	// 普通のクラッシュに見せかけないようにする。
	state.fatal = model.exit != nil && model.exit.fatal
	model.exit = state
	model.restart.record(startedAt, exitedAt, state.crashed && !state.fatal)
	if state.crashed {
		return model.startAutoRestart(state, err)
	}
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
	state.fatal = true
	model.exit = state
}

func (model *Model) newExitState(
	crashed bool,
	err error,
	exitCode int,
	startedAt time.Time,
	exitedAt time.Time,
) *exitState {
	uptime := hsperfdata.Duration{}
	if !startedAt.IsZero() && !exitedAt.Before(startedAt) {
		uptime = hsperfdata.Duration{
			Value:     exitedAt.Sub(startedAt),
			Available: true,
		}
	}
	mismatch, javaMismatch := model.javaVersionMismatch()
	guidance := []string(nil)
	if javaMismatch {
		guidance = javaMismatchLines(mismatch)
	}
	return &exitState{
		crashed:      crashed,
		exitCode:     exitCode,
		exitedAt:     exitedAt,
		uptime:       uptime,
		errorLines:   model.exitErrorLines(err, crashed, guidance),
		javaMismatch: javaMismatch,
		snapshot: exitSnapshot{
			heap: model.lastHeap,
			rss:  model.lastRSS,
			gc:   model.gcStats,
		},
	}
}

// exitErrorLines は原因として読ませる行を集める。異常終了で 1 本も
// 拾えなかったときだけ末尾で埋める。正常停止でこれをやると、保存完了の
// INFO 行が「エラー行」の見出しで並ぶ。
func (model *Model) exitErrorLines(err error, crashed bool, guidance []string) []string {
	lines := append([]string(nil), guidance...)
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
	if len(matches) == 0 && crashed {
		for index := model.logs.Len() - 1; index >= oldest && len(matches) < exitErrorLineLimit; index-- {
			matches = append(matches, model.logs.At(index).line())
		}
	}
	for index := len(matches) - 1; index >= 0; index-- {
		lines = append(lines, matches[index])
	}
	return lines
}

func (model *Model) javaVersionMismatch() (javaenv.ClassVersionError, bool) {
	oldest := model.generationLogStart()
	lines := make([]string, 0, model.logs.Len()-oldest)
	for index := oldest; index < model.logs.Len(); index++ {
		lines = append(lines, model.logs.At(index).line())
	}
	return javaenv.ParseUnsupportedClassVersion(strings.Join(lines, "\n"))
}

func javaMismatchLines(mismatch javaenv.ClassVersionError) []string {
	installations, _ := javaenv.Installed("/usr/lib/jvm")
	return javaMismatchGuidance(mismatch, installations)
}

func javaMismatchGuidance(
	mismatch javaenv.ClassVersionError,
	installations []javaenv.Installation,
) []string {
	lines := []string{msg.JavaVersionMismatch(mismatch.Required, mismatch.Actual)}
	for _, installation := range installations {
		if installation.Major == mismatch.Required {
			return append(lines, msg.JavaVersionChange(mismatch.Required))
		}
	}
	return append(lines, msg.JavaVersionInstall(mismatch.Required))
}

// generationLogStart は現世代の最初のログが今どの位置にいるかを返す。
// バッファは再起動で消さないので、位置は古い行が押し出されるたびにずれる。
func (model *Model) generationLogStart() int {
	discarded := model.logs.nextNumber - uint64(model.logs.Len())
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
	if model.exit != message.state || message.state.autoQuitAt.IsZero() {
		return model, nil
	}
	if !time.Now().Before(message.state.autoQuitAt) {
		return model, tea.Quit
	}
	return model, model.exitCountdownCmd(message.state)
}

func (model *Model) exitCountdownSeconds() int {
	if model.exit == nil || model.exit.autoQuitAt.IsZero() {
		return 0
	}
	remaining := time.Until(model.exit.autoQuitAt)
	if remaining <= 0 {
		return 0
	}
	return int((remaining + time.Second - 1) / time.Second)
}

// startAutoRestart は自動再起動が使えるなら待ちを始める。使えないときは
// 理由を notice に残す。設定を有効にしたのに黙って三択が出ると、機能が
// 効いていないのか諦めたのか区別が付かない。
func (model *Model) startAutoRestart(state *exitState, err error) tea.Cmd {
	if !model.settings.AutoRestart {
		return nil
	}
	switch {
	case state.fatal:
		state.notice = msg.ExitAutoRestartFatal
		return nil
	case state.javaMismatch:
		state.notice = msg.ExitAutoRestartJavaMismatch
		return nil
	// 起動スクリプトが java を立てないまま終わるのは設定やスクリプトの
	// 誤りで、待って叩き直しても直らない。1 回で人に渡す。
	case errors.Is(err, msg.ErrScriptExitedWithoutJava):
		state.notice = msg.ExitAutoRestartSkipped
		return nil
	case model.restart.crashLoop():
		state.notice = msg.ExitAutoRestartStopped
		return nil
	}
	state.autoRestart = true
	state.autoRestartAt = time.Now().Add(autoRestartWait)
	return model.autoRestartCmd(state)
}

func (model *Model) autoRestartCmd(state *exitState) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return autoRestartMsg{state: state}
	})
}

func (model *Model) handleAutoRestart(message autoRestartMsg) (tea.Model, tea.Cmd) {
	state := message.state
	if model.exit != state || !state.autoRestart || state.autoRestartAt.IsZero() {
		return model, nil
	}
	if time.Now().Before(state.autoRestartAt) {
		return model, model.autoRestartCmd(state)
	}
	state.autoRestartAt = time.Time{}
	cmd := model.requestRestart()
	if cmd == nil {
		// 要求を出せなかった。待ち続けても勝手には直らないので、
		// 背景をログ全面に戻して三択を出す。
		state.autoRestart = false
		state.notice = msg.ExitAutoRestartRejected
	}
	return model, cmd
}

func (model *Model) autoRestartSeconds() int {
	if model.exit == nil || model.exit.autoRestartAt.IsZero() {
		return 0
	}
	remaining := time.Until(model.exit.autoRestartAt)
	if remaining <= 0 {
		return 0
	}
	return int((remaining + time.Second - 1) / time.Second)
}

// cancelAutoRestart は待ちをやめて手動の三択へ落とす。すでに再起動を
// 頼んだ後には効かない。
func (model *Model) cancelAutoRestart() {
	if model.exit.autoRestartAt.IsZero() {
		return
	}
	model.exit.autoRestart = false
	model.exit.autoRestartAt = time.Time{}
	model.exit.notice = msg.ExitAutoRestartCanceled
}

// onServerStarted は自動再起動で立ち直ったモーダルを残し、それ以外は畳む。
// 人が見ていない間に落ちて戻ったことが画面から消えないよう、残した分は
// Enter を押すまで出したままにする。
func (model *Model) onServerStarted() {
	state := model.exit
	if state == nil {
		return
	}
	if !state.autoRestart {
		model.exit = nil
		return
	}
	state.restarted = true
	state.autoRestartAt = time.Time{}
	state.notice = ""
}

func (model *Model) closeExitModal() {
	model.exit.autoQuitAt = time.Time{}
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
			state.exitedAt.Add(time.Duration(model.settings.TimeOffsetMinutes)*time.Minute).
				Format("2006-01-02 15:04:05"),
			formatUptime(state.uptime),
		),
		msg.ExitMemory(
			formatProcBytes(state.snapshot.rss),
			formatJVMBytes(state.snapshot.heap.Used),
			formatJVMBytes(state.snapshot.heap.Committed),
			formatDelta(state.snapshot.rss, state.snapshot.heap.Committed),
		),
		msg.ExitGC(
			state.snapshot.gc.Collections.Value,
			state.snapshot.gc.Collections.Available,
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
	case state.restarted:
		lines = append(lines, msg.ExitAutoRestartDone)
	case model.restartPhase != 0:
		lines = append(lines, msg.ExitStateRestarting(model.restartDots()))
	case state.autoRestart:
		lines = append(lines,
			msg.ExitStateCrashed,
			msg.ExitAutoRestartIn(model.autoRestartSeconds()),
		)
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
	// 自動再起動の最中はボタンを出さない。押せる操作が無いのに三択を
	// 見せると、選ばないと進まない画面に見える。
	switch {
	case state.restarted:
		lines = append(lines, "", dimStyle.Render(msg.ExitAutoRestartDoneHint))
	case !state.autoRestart:
		lines = append(lines, "", model.exitButtons(width-2))
	case !state.autoRestartAt.IsZero():
		lines = append(lines, "", dimStyle.Render(msg.ExitAutoRestartHint))
	}

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
