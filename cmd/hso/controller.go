package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/config"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/process"
	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
	"github.com/hijoushoku7/hijo-server-ops/internal/ui"
)

const (
	logQueueSize     = 16
	gracefulStopWait = 60 * time.Second
)

type stoppableServer interface {
	Done() <-chan struct{}
	Send(string) error
	Signal(os.Signal) error
}

type outputPipe struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

func newOutputPipe() outputPipe {
	reader, writer := io.Pipe()
	return outputPipe{reader: reader, writer: writer}
}

func (pipe outputPipe) close() {
	_ = pipe.reader.Close()
	_ = pipe.writer.Close()
}

type serverRuntime struct {
	server     *process.Process
	generation uint64
	startedAt  time.Time
	javaFound  atomic.Bool
	// stopCommandSent と shutdownNoticed は、意図された停止を確認した経路を
	// 分けて持つ。送信失敗時に取り消すのは前者だけにし、ログ側の確認を
	// 消さない。
	stopCommandSent atomic.Bool
	shutdownNoticed atomic.Bool
	// crashNoticed はクラッシュの標識をログで見たか。整然と畳まれること
	// 自体はクラッシュでも同じなので、これで区別する。
	crashNoticed atomic.Bool
	cancel       context.CancelFunc
	// monitorCancel はメトリクス・GC・アドレス解決だけを止める。ログの
	// 排出は cancel まで生かし、終了直前の行を落とさない。
	monitorCancel context.CancelFunc
	stdout        outputPipe
	stderr        outputPipe
	// logsDone は pumpLogs が止まったことを知らせる。終了を UI へ告げる
	// 前に、残っているログを送り切るために待つ。
	logsDone chan struct{}
}

// noteShutdown は畳まれ方をログから見て、整然と止まったなら意図された終了
// として印を付ける。クラッシュの標識はシャットダウンの告知より先に出るので、
// 先に見えていたらこの停止はクラッシュの後始末とみなし、印を付けない。
//
// 誰が止めたかは見ない。/stop のコマンドフィードバックを見る手もあるが、
// プレイヤーがワールドで実行した場合は gamerule logAdminCommands に依存し、
// 切られている環境では拾えない。
//
// 印を付けるだけで hso は畳まない。実際にプロセスが終わったときに
// waitForServer がこれを読み、終了コードが 0 ならクラッシュ扱いを外す。
// 印が立っていても終了コードが 0 でなければクラッシュのままなので、
// SIGTERM で畳まれた場合（143）は自動再起動の対象に残る。
//
// 標識が告知より後に届いても結論は変わらない。expectedExit がクラッシュの
// 標識を最後に引くので、ここで告知の印を立てた後に標識が立っても覆る。
// stdout と stderr で読み手が分かれており順序を保証できないため、順序に
// 頼らない形にしてある。
func (runtime *serverRuntime) noteShutdown(entry serverlog.Entry) {
	switch {
	case serverlog.IsCrashNotice(entry):
		runtime.crashNoticed.Store(true)
	case serverlog.IsShutdownStart(entry) && !runtime.crashNoticed.Load():
		runtime.shutdownNoticed.Store(true)
	}
}

// expectedExit は、クラッシュの標識がなく、停止コマンドの送信またはログで
// 整然とした停止を確認できた場合にだけ true を返す。クラッシュの標識は
// 確定状態なので、後から停止コマンドやシャットダウン告知が来ても覆さない。
func (runtime *serverRuntime) expectedExit() bool {
	expected := runtime.stopCommandSent.Load() || runtime.shutdownNoticed.Load()
	return expected && !runtime.crashNoticed.Load()
}

// sendCommand は停止コマンドの送信状態だけを管理する。印を立てるのは送信が
// 成功した後で、一度立てたら降ろさない。送信の前に立てて失敗したら降ろす、
// という書き方だと 2 つの穴ができる。確定前に waitForServer が読むと失敗した
// 送信が正常停止に見え、2 回目の送信失敗が 1 回目の成功を消す。
//
// 送信が成功してから印が立つまでの隙間に終了判定が走ると、意図した停止を
// 取りこぼす。ただし今書いた stop が効くまでには JVM の停止処理が要るので、
// その間にプロセスが消えてログも流し切っていることは実際には起こらない。
// 起きたとしてもログ側の shutdownNoticed が同じ停止を拾う。
func (runtime *serverRuntime) sendCommand(
	command string,
	send func(string) error,
) error {
	err := send(command)
	if err == nil && isStopCommand(command) {
		runtime.stopCommandSent.Store(true)
	}
	return err
}

func (runtime *serverRuntime) close() {
	runtime.cancel()
	runtime.stdout.close()
	runtime.stderr.close()
}

type serverController struct {
	ctx     context.Context
	cfg     config.Config
	program *tea.Program

	operation sync.Mutex
	currentMu sync.Mutex
	current   *serverRuntime
	// runtime を手放した後も、次の起動世代を決められるよう保持する。
	lastGeneration uint64
}

func newServerController(
	ctx context.Context,
	cfg config.Config,
	program *tea.Program,
) *serverController {
	return &serverController{ctx: ctx, cfg: cfg, program: program}
}

func (controller *serverController) start(generation uint64, announce bool) error {
	if err := controller.ctx.Err(); err != nil {
		return err
	}
	stdout := newOutputPipe()
	stderr := newOutputPipe()
	server, err := process.Start(process.Options{
		Command: controller.cfg.Server.Command,
		WorkDir: controller.cfg.Server.WorkDir,
		Stdout:  stdout.writer,
		Stderr:  stderr.writer,
	})
	if err != nil {
		stdout.close()
		stderr.close()
		return err
	}

	runtimeCtx, runtimeCancel := context.WithCancel(controller.ctx)
	// 監視はログより先に止める。プロセスが消えた後も /proc を引き続けると、
	// 最終 RSS / heap を「取れない」で上書きしてしまう。
	monitorCtx, monitorCancel := context.WithCancel(runtimeCtx)
	runtime := &serverRuntime{
		server:        server,
		generation:    generation,
		startedAt:     time.Now(),
		cancel:        runtimeCancel,
		monitorCancel: monitorCancel,
		stdout:        stdout,
		stderr:        stderr,
		logsDone:      make(chan struct{}),
	}
	controller.currentMu.Lock()
	controller.current = runtime
	controller.lastGeneration = generation
	controller.currentMu.Unlock()

	if announce {
		controller.program.Send(ui.ServerStartedMsg{
			Generation: generation,
			StartedAt:  runtime.startedAt,
		})
	}

	// 出力の読み取りが両方とも EOF に達したら logs を閉じ、pumpLogs が
	// 残りを送り切ってから logsDone を閉じる。クラッシュ時はスタック
	// トレースが最後に固まって出るので、ここを待たずに終了を告げると
	// いちばん読みたい行が捨てられる。
	logs := make(chan serverlog.Entry, logQueueSize)
	var readers sync.WaitGroup
	readers.Add(2)
	go pumpLogs(runtimeCtx, controller.program, logs, generation, runtime.logsDone)
	go func() {
		defer readers.Done()
		readAndCloseServerOutput(stdout.reader, logs, runtime.noteShutdown)
	}()
	go func() {
		defer readers.Done()
		readAndCloseServerOutput(stderr.reader, logs, runtime.noteShutdown)
	}()
	go func() {
		readers.Wait()
		close(logs)
	}()
	go controller.waitForServer(runtime)
	go findJava(monitorCtx, server, &runtime.javaFound, controller.program, generation)
	go streamGC(monitorCtx, server.GCLogPath(), controller.program, generation)
	go resolveServerAddress(
		monitorCtx,
		controller.cfg.Server.WorkDir,
		controller.program,
		generation,
	)
	return nil
}

func (controller *serverController) handleActions(actions <-chan ui.Action) {
	for {
		select {
		case <-controller.ctx.Done():
			return
		case action := <-actions:
			switch action.Kind {
			case ui.ActionRestart:
				controller.restart()
			case ui.ActionSendCommand:
				controller.sendCommand(action)
			}
		}
	}
}

func (controller *serverController) sendCommand(action ui.Action) {
	controller.operation.Lock()
	defer controller.operation.Unlock()

	runtime := controller.currentRuntime()
	if runtime == nil {
		controller.program.Send(ui.ActionResultMsg{Action: action, Err: msg.ErrServerStopped})
		return
	}
	err := runtime.sendCommand(action.Command, runtime.server.Send)
	controller.program.Send(ui.ActionResultMsg{Action: action, Err: err})
}

// isStopCommand はコンソールに打たれた行が停止コマンドかを見る。サーバー
// コンソールでは `stop` だが、ゲーム内の感覚で `/stop` と打つ人もいて、
// どちらも同じように止まる。片方だけ見ていると、意図した停止をクラッシュと
// 誤認して自動再起動が走る。
func isStopCommand(command string) bool {
	trimmed := strings.TrimSpace(command)
	return strings.EqualFold(strings.TrimPrefix(trimmed, "/"), "stop")
}

func (controller *serverController) restart() {
	controller.operation.Lock()
	defer controller.operation.Unlock()

	runtime := controller.currentRuntime()
	if runtime == nil {
		controller.program.Send(ui.ServerRestartingMsg{})
		if err := controller.start(controller.latestGeneration()+1, true); err != nil {
			controller.program.Send(ui.FatalMsg{Err: msg.RestartFailed(err)})
		}
		return
	}
	if !runtime.javaFound.Load() {
		controller.sendRestartResult(msg.ErrRestartBeforeJava)
		return
	}
	if !controller.detach(runtime) {
		return
	}

	runtime.cancel()
	controller.program.Send(ui.ServerRestartingMsg{})
	if err := stopServer(runtime.server, true, gracefulStopWait); err != nil {
		runtime.close()
		controller.program.Send(ui.FatalMsg{Err: err})
		return
	}
	runtime.close()
	if controller.ctx.Err() != nil {
		return
	}

	if err := controller.start(runtime.generation+1, true); err != nil {
		controller.program.Send(ui.FatalMsg{Err: msg.RestartFailed(err)})
	}
}

func (controller *serverController) sendRestartResult(err error) {
	controller.program.Send(ui.ActionResultMsg{
		Action: ui.Action{Kind: ui.ActionRestart},
		Err:    err,
	})
}

func (controller *serverController) shutdown() error {
	controller.operation.Lock()
	defer controller.operation.Unlock()

	runtime := controller.detachCurrent()
	if runtime == nil {
		return nil
	}
	runtime.cancel()
	err := stopServer(runtime.server, runtime.javaFound.Load(), gracefulStopWait)
	runtime.close()
	return err
}

func (controller *serverController) waitForServer(runtime *serverRuntime) {
	waitErr := runtime.server.Wait()
	// 停止時刻はここで確定させる。ログを流し切ってから取ると、その排出に
	// かかった時間まで稼働時間に足される。
	exitedAt := time.Now()
	// 監視はここで止める。ログを流し切るまでの間も /proc を引き続けると、
	// 死んだ PID の結果でモーダルに出す最終 RSS / heap が n/a に化ける。
	runtime.monitorCancel()
	_ = runtime.stdout.writer.Close()
	_ = runtime.stderr.writer.Close()
	// 最後の出力を送り切ってから止める。順番を逆にすると、クラッシュの
	// 原因が書かれた末尾がそのまま消える。
	<-runtime.logsDone
	runtime.cancel()
	if !controller.detach(runtime) {
		return
	}
	controller.program.Send(ui.ProcessExitedMsg{
		Generation: runtime.generation,
		ExitCode:   processExitCode(waitErr),
		StartedAt:  runtime.startedAt,
		ExitedAt:   exitedAt,
		Err: serverExitError(
			waitErr,
			runtime.javaFound.Load(),
			runtime.expectedExit(),
		),
	})
}

func processExitCode(waitErr error) int {
	if waitErr == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func serverExitError(waitErr error, javaFound, expected bool) error {
	if waitErr != nil || expected {
		return waitErr
	}
	if !javaFound {
		return msg.ErrScriptExitedWithoutJava
	}
	return msg.ErrServerExited
}

func (controller *serverController) currentRuntime() *serverRuntime {
	controller.currentMu.Lock()
	defer controller.currentMu.Unlock()
	return controller.current
}

func (controller *serverController) latestGeneration() uint64 {
	controller.currentMu.Lock()
	defer controller.currentMu.Unlock()
	return controller.lastGeneration
}

func (controller *serverController) detachCurrent() *serverRuntime {
	controller.currentMu.Lock()
	defer controller.currentMu.Unlock()
	runtime := controller.current
	controller.current = nil
	return runtime
}

func (controller *serverController) detach(runtime *serverRuntime) bool {
	controller.currentMu.Lock()
	defer controller.currentMu.Unlock()
	if controller.current != runtime {
		return false
	}
	controller.current = nil
	return true
}

func stopServer(server stoppableServer, javaFound bool, wait time.Duration) error {
	select {
	case <-server.Done():
		return nil
	default:
	}

	if javaFound {
		if err := server.Send("stop"); err == nil {
			timer := time.NewTimer(wait)
			defer timer.Stop()
			select {
			case <-server.Done():
				return nil
			case <-timer.C:
			}
		}
	}

	if err := server.Signal(syscall.SIGTERM); err != nil {
		select {
		case <-server.Done():
			return nil
		default:
			return msg.StopFailed(err)
		}
	}
	<-server.Done()
	return nil
}
