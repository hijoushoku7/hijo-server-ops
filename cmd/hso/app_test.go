package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/config"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/process"
	"github.com/hijoushoku7/hijo-server-ops/internal/procstats"
	"github.com/hijoushoku7/hijo-server-ops/internal/serverlog"
	"github.com/hijoushoku7/hijo-server-ops/internal/ui"
)

func TestMain(tests *testing.M) {
	if command, ok := process.SupervisorCommand(os.Args); ok {
		os.Exit(process.RunSupervisor(command))
	}
	if dir := os.Getenv("HSO_FAKE_JAVA_DIR"); dir != "" {
		runFakeJava(dir)
		os.Exit(0)
	}
	os.Exit(tests.Run())
}

func TestReadServerOutputParsesLines(t *testing.T) {
	logs := make(chan serverlog.Entry, 4)
	readServerOutput(strings.NewReader(
		"[12:00:00] [Server thread/INFO]: <alice> hello\n"+
			"[12:00:01] [Server thread/INFO]: alice joined the game\n",
	), logs, nil)

	first := <-logs
	second := <-logs
	if first.Kind != serverlog.KindChat || first.Chat != "hello" {
		t.Fatalf("first = %#v", first)
	}
	if second.Kind != serverlog.KindPlayerJoin || second.Player != "alice" {
		t.Fatalf("second = %#v", second)
	}
	if first.Raw != "" || second.Raw != "" {
		t.Fatalf("raw log was retained")
	}
	if first.TimestampSource != serverlog.TimestampLog ||
		first.Timestamp.Format("15:04:05") != "12:00:00" {
		t.Fatalf("first timestamp = %v (%d)", first.Timestamp, first.TimestampSource)
	}
}

func TestServerDisplayName(t *testing.T) {
	tests := []struct {
		name    string
		workDir string
		lookup  func(string) (string, bool, error)
		want    string
	}{
		{
			name:    "登録名を優先する",
			workDir: "/srv/minecraft/survival",
			want:    "main-server",
			lookup: func(string) (string, bool, error) {
				return "main-server", true, nil
			},
		},
		{
			name:    "未登録なら作業ディレクトリ名を使う",
			workDir: "/srv/minecraft/creative",
			want:    "creative",
			lookup: func(string) (string, bool, error) {
				return "", false, nil
			},
		},
		{
			name:    "一覧の読み取り失敗でも作業ディレクトリ名を使う",
			workDir: "/srv/minecraft/modded",
			want:    "modded",
			lookup: func(string) (string, bool, error) {
				return "", false, errors.New("一覧を読めません")
			},
		},
		{
			name: "作業ディレクトリも空なら空文字を返す",
			lookup: func(string) (string, bool, error) {
				return "", false, nil
			},
		},
		{
			name:    "ディレクトリ名の制御文字を落とす",
			workDir: "/srv/minecraft/ev\x1b]0;il\aserver",
			want:    "ev]0;ilserver",
			lookup: func(string) (string, bool, error) {
				return "", false, nil
			},
		},
		{
			name:    "登録名の制御文字も落とす",
			workDir: "/srv/minecraft/survival",
			want:    "main-server",
			lookup: func(string) (string, bool, error) {
				return "main-\x07server", true, nil
			},
		},
		{
			name:    "長い名前は 30 文字で切る",
			workDir: "/srv/minecraft/" + strings.Repeat("a", 40),
			want:    strings.Repeat("a", 30),
			lookup: func(string) (string, bool, error) {
				return "", false, nil
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const configPath = "/etc/hso.toml"
			lookup := func(path string) (string, bool, error) {
				if path != configPath {
					t.Fatalf("lookup path = %q", path)
				}
				return test.lookup(path)
			}
			got := serverDisplayName(configPath, config.Config{
				Server: config.Server{WorkDir: test.workDir},
			}, lookup)
			if got != test.want {
				t.Fatalf("serverDisplayName() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestReadServerOutputUsesReceiveTimeWithoutLogTimestamp(t *testing.T) {
	logs := make(chan serverlog.Entry, 1)
	readServerOutput(strings.NewReader("plain output\n"), logs, nil)

	entry := <-logs
	if entry.Timestamp.IsZero() ||
		entry.TimestampSource != serverlog.TimestampReceived {
		t.Fatalf("timestamp = %v (%d)", entry.Timestamp, entry.TimestampSource)
	}
}

func TestReadServerOutputBoundsLongLinesAndContinues(t *testing.T) {
	logs := make(chan serverlog.Entry, 4)
	input := bytes.Repeat([]byte("x"), maxOutputLineBytes+100)
	input = append(input, '\n')
	input = append(input, []byte("next\n")...)
	readServerOutput(bytes.NewReader(input), logs, nil)

	first := <-logs
	second := <-logs
	if len(first.Message) > maxOutputLineBytes+len("…") {
		t.Fatalf("line length = %d", len(first.Message))
	}
	if !strings.HasSuffix(first.Message, "…") {
		t.Fatalf("first = %q", first.Message)
	}
	if second.Message != "next" {
		t.Fatalf("second = %q", second.Message)
	}
}

func TestOfferLogDropsOldestWhenQueueIsFull(t *testing.T) {
	logs := make(chan serverlog.Entry, 2)
	offerLog(logs, serverlog.Entry{Message: "one"})
	offerLog(logs, serverlog.Entry{Message: "two"})
	offerLog(logs, serverlog.Entry{Message: "three"})

	first := <-logs
	second := <-logs
	if first.Message != "two" || second.Message != "three" {
		t.Fatalf("logs = %q, %q", first.Message, second.Message)
	}
}

func TestStopServerSendsMinecraftStop(t *testing.T) {
	server := newFakeStoppableServer()
	server.finishOnSend = true

	if err := stopServer(server, true, time.Second); err != nil {
		t.Fatal(err)
	}
	if len(server.commands) != 1 || server.commands[0] != "stop" {
		t.Fatalf("commands = %v", server.commands)
	}
	if len(server.signals) != 0 {
		t.Fatalf("signals = %v", server.signals)
	}
}

func TestStopServerFallsBackToSIGTERMAfterTimeout(t *testing.T) {
	server := newFakeStoppableServer()
	server.finishOnSignal = true

	if err := stopServer(server, true, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if len(server.commands) != 1 || server.commands[0] != "stop" {
		t.Fatalf("commands = %v", server.commands)
	}
	if len(server.signals) != 1 || server.signals[0] != syscall.SIGTERM {
		t.Fatalf("signals = %v", server.signals)
	}
}

func TestStopServerUsesSIGTERMBeforeJavaIsFound(t *testing.T) {
	server := newFakeStoppableServer()
	server.finishOnSignal = true

	if err := stopServer(server, false, time.Second); err != nil {
		t.Fatal(err)
	}
	if len(server.commands) != 0 {
		t.Fatalf("commands = %v", server.commands)
	}
	if len(server.signals) != 1 || server.signals[0] != syscall.SIGTERM {
		t.Fatalf("signals = %v", server.signals)
	}
}

func TestStopServerReturnsSignalError(t *testing.T) {
	server := newFakeStoppableServer()
	server.signalErr = errors.New("signal failed")

	if err := stopServer(server, false, time.Second); err == nil {
		t.Fatal("expected an error")
	}
}

// 停止の目印は logs が詰まっていても拾えないといけない。offerLog は溢れた
// 行を古いものから捨てるので、ここでは告知の行がキューに残らない。それでも
// 印が立つこと＝キューに入った行だけを observe する実装に戻っていないこと
// を見る。
func TestReadServerOutputObservesEveryLineEvenWhenLogsAreFull(t *testing.T) {
	logs := make(chan serverlog.Entry, 1)
	var runtime serverRuntime
	readServerOutput(strings.NewReader(
		"[12:00:00] [Server thread/INFO]: <alice> hello\n"+
			"[12:00:01] [Server thread/INFO]: Stopping server\n"+
			"[12:00:02] [Server thread/INFO]: Saving worlds\n",
	), logs, runtime.noteShutdown)

	if len(logs) != 1 {
		t.Fatalf("queued lines = %d", len(logs))
	}
	if entry := <-logs; entry.Message != "Saving worlds" {
		t.Fatalf("the shutdown notice survived the queue: %#v", entry)
	}
	if !runtime.expectedExit() {
		t.Fatal("shutdown notice was not observed")
	}
}

// プレイヤーがワールドで /stop を実行した場合。gamerule logAdminCommands が
// 切られていて `[名前: Stopping the server]` が出なくても、整然と畳まれた
// ことは分かる。
func TestRuntimeNoteShutdownDetectsPlayerStopWithoutAdminLog(t *testing.T) {
	var runtime serverRuntime
	for _, line := range []string{
		"[12:00:00] [Server thread/INFO]: Stopping server",
		"[12:00:00] [Server thread/INFO]: Saving worlds",
	} {
		runtime.noteShutdown(serverlog.Parse(line))
	}
	if !runtime.expectedExit() {
		t.Fatal("an orderly shutdown was not treated as expected")
	}
}

// クラッシュでもシャットダウン処理は走る。標識が先に出ているので、
// 意図された停止と取り違えない。
func TestRuntimeNoteShutdownKeepsCrashUnexpected(t *testing.T) {
	var runtime serverRuntime
	for _, line := range []string{
		"[12:00:00] [Server thread/ERROR]: Encountered an unexpected exception",
		"[12:00:00] [Server thread/ERROR]: This crash report has been saved to: " +
			"/srv/mc/crash-reports/crash-2026-08-08_12.00.00-server.txt",
		"[12:00:00] [Server thread/INFO]: Stopping server",
	} {
		runtime.noteShutdown(serverlog.Parse(line))
	}
	if runtime.expectedExit() {
		t.Fatal("a crash was treated as an expected exit")
	}
}

// クラッシュ標識は確定状態として扱う。後始末中に hso から stop を正常に
// 送れて、その後にシャットダウン告知が来ても正常停止へ戻してはいけない。
// ErrServerExited は UI 側で異常終了として扱われ、自動再起動の対象になる。
func TestRuntimeStopCommandDoesNotOverrideCrashNotice(t *testing.T) {
	var runtime serverRuntime
	runtime.noteShutdown(serverlog.Parse(
		"[12:00:00] [Server thread/ERROR]: Encountered an unexpected exception",
	))
	if err := runtime.sendCommand("stop", func(command string) error {
		if command != "stop" {
			t.Fatalf("command = %q", command)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	runtime.noteShutdown(serverlog.Parse(
		"[12:00:01] [Server thread/INFO]: Stopping server",
	))

	err := serverExitError(nil, true, runtime.expectedExit())
	if !errors.Is(err, msg.ErrServerExited) {
		t.Fatalf("clean crash error = %v", err)
	}
}

// ログで整然とした停止を確認した後なら、終了済みプロセスへの stop 送信が
// 失敗してもログ側の印を消さず、正常停止のままにする。
func TestRuntimeFailedStopCommandKeepsShutdownNotice(t *testing.T) {
	var runtime serverRuntime
	runtime.noteShutdown(serverlog.Parse(
		"[12:00:00] [Server thread/INFO]: Stopping server",
	))
	sendErr := errors.New("server process has exited")
	err := runtime.sendCommand("stop", func(command string) error {
		if command != "stop" {
			t.Fatalf("command = %q", command)
		}
		return sendErr
	})
	if !errors.Is(err, sendErr) {
		t.Fatalf("send error = %v", err)
	}
	if err := serverExitError(nil, true, runtime.expectedExit()); err != nil {
		t.Fatalf("orderly shutdown error = %v", err)
	}
}

// 送信に失敗した stop で印を立てない。送信の前に立てる書き方だと、
// 失敗が確定する前に終了判定が走ったときクラッシュを正常停止と読み違える。
func TestRuntimeFailedStopCommandLeavesNoMark(t *testing.T) {
	var runtime serverRuntime
	sendErr := errors.New("server process has exited")
	var marked bool
	err := runtime.sendCommand("stop", func(string) error {
		marked = runtime.expectedExit()
		return sendErr
	})
	if !errors.Is(err, sendErr) {
		t.Fatalf("send error = %v", err)
	}
	// 送信中に読まれても印は立っていない。
	if marked {
		t.Fatal("the mark was set before the send finished")
	}
	if err := serverExitError(nil, true, runtime.expectedExit()); !errors.Is(err, msg.ErrServerExited) {
		t.Fatalf("failed stop error = %v", err)
	}
}

// 2 回目の stop が失敗しても 1 回目の成功を消さない。印は単調に立つだけ。
func TestRuntimeFailedStopCommandKeepsEarlierSuccess(t *testing.T) {
	var runtime serverRuntime
	if err := runtime.sendCommand("stop", func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	sendErr := errors.New("server process has exited")
	if err := runtime.sendCommand("/stop", func(string) error {
		return sendErr
	}); !errors.Is(err, sendErr) {
		t.Fatalf("send error = %v", err)
	}
	if err := serverExitError(nil, true, runtime.expectedExit()); err != nil {
		t.Fatalf("expected exit error = %v", err)
	}
}

// 標識が告知より後に届いても結論は変わらない。読み手が stdout と stderr に
// 分かれているので、順序に頼った判定にはしていない。
func TestRuntimeCrashNoticeAfterShutdownNoticeStillCrashes(t *testing.T) {
	var runtime serverRuntime
	for _, line := range []string{
		"[12:00:00] [Server thread/INFO]: Stopping server",
		"[12:00:00] [Server thread/ERROR]: Encountered an unexpected exception",
	} {
		runtime.noteShutdown(serverlog.Parse(line))
	}
	err := serverExitError(nil, true, runtime.expectedExit())
	if !errors.Is(err, msg.ErrServerExited) {
		t.Fatalf("late crash notice error = %v", err)
	}
}

func TestRuntimeNoteShutdownIgnoresChat(t *testing.T) {
	var runtime serverRuntime
	for _, line := range []string{
		"[12:00:00] [Server thread/INFO]: <alice> Encountered an unexpected exception",
		"[12:00:01] [Server thread/INFO]: <alice> Stopping server",
	} {
		runtime.noteShutdown(serverlog.Parse(line))
	}
	if runtime.expectedExit() || runtime.crashNoticed.Load() {
		t.Fatal("chat was treated as a server notice")
	}
}

func TestIsStopCommand(t *testing.T) {
	for _, command := range []string{"stop", "/stop", " /Stop ", "STOP"} {
		if !isStopCommand(command) {
			t.Fatalf("%q was not treated as stop", command)
		}
	}
	for _, command := range []string{"stopall", "say stop", "//stop", "/ stop", ""} {
		if isStopCommand(command) {
			t.Fatalf("%q was treated as stop", command)
		}
	}
}

func TestServerExitErrorRejectsUnexpectedCleanExit(t *testing.T) {
	err := serverExitError(nil, true, false)
	if !errors.Is(err, msg.ErrServerExited) {
		t.Fatalf("err = %v", err)
	}
	if err := serverExitError(nil, true, true); err != nil {
		t.Fatalf("expected exit error = %v", err)
	}
}

func TestProcessExitCode(t *testing.T) {
	if code := processExitCode(nil); code != 0 {
		t.Fatalf("clean exit code = %d", code)
	}
	err := exec.Command("sh", "-c", "exit 7").Run()
	if code := processExitCode(err); code != 7 {
		t.Fatalf("failed exit code = %d, err = %v", code, err)
	}
	if code := processExitCode(errors.New("wait failed")); code != -1 {
		t.Fatalf("unknown exit code = %d", code)
	}
}

func TestCalculateCPU(t *testing.T) {
	previous := procstats.Duration{Value: time.Second, Available: true}
	current := procstats.Duration{Value: 2500 * time.Millisecond, Available: true}

	got, ok := calculateCPU(previous, current, time.Second)
	if !ok || got != 150 {
		t.Fatalf("CPU = %f, available = %t", got, ok)
	}
	if _, ok := calculateCPU(procstats.Duration{}, current, time.Second); ok {
		t.Fatal("first CPU sample was available")
	}
}

func TestSettingsRoundTripTimeOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hso.toml")
	cfg := config.Config{
		Server: config.Server{Command: "./run.sh", WorkDir: dir},
		UI:     config.UI{Time: config.Time{OffsetMinutes: -90}},
	}

	settings := settingsFrom(cfg)
	if settings.TimeOffsetMinutes != -90 {
		t.Fatalf("settings = %#v", settings)
	}
	settings.TimeOffsetMinutes = 120
	if err := saveSettings(path, cfg, settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.UI.Time.OffsetMinutes != 120 {
		t.Fatalf("loaded = %#v", loaded.UI.Time)
	}
}

func TestServerControllerSendsCommandsAndRestarts(t *testing.T) {
	dir := t.TempDir()
	javaPath := filepath.Join(dir, "java")
	if err := os.Link(os.Args[0], javaPath); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "run.sh")
	scriptContent := "#!/bin/sh\n" +
		"export HSO_FAKE_JAVA_DIR='" + dir + "'\n" +
		"exec '" + javaPath + "'\n"
	if err := os.WriteFile(script, []byte(scriptContent), 0o700); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	actions := make(chan ui.Action, actionQueueSize)
	model := ui.New(actions, nil, initialGeneration, ui.DefaultSettings(), ui.ServerInfo{})
	program := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithInput(nil),
		tea.WithoutRenderer(),
	)
	controller := newServerController(ctx, config.Config{
		Server: config.Server{
			Command: script,
			WorkDir: dir,
		},
	}, program)
	if err := controller.start(initialGeneration, false); err != nil {
		t.Fatal(err)
	}
	programDone := make(chan error, 1)
	go func() {
		_, err := program.Run()
		programDone <- err
	}()

	waitForControllerJava(t, controller)
	controller.sendCommand(ui.Action{
		Kind:    ui.ActionSendCommand,
		Command: "say hello",
	})
	waitForFileLines(t, filepath.Join(dir, "commands"), 1)

	controller.restart()
	waitForFileLines(t, filepath.Join(dir, "launches"), 2)
	waitForControllerJava(t, controller)
	if runtime := controller.currentRuntime(); runtime.generation != 2 {
		t.Fatalf("generation = %d", runtime.generation)
	}

	// 自然停止後は runtime が detach 済みでも、最後の世代の次から起動する。
	controller.sendCommand(ui.Action{Kind: ui.ActionSendCommand, Command: "stop"})
	waitForControllerStopped(t, controller)
	controller.restart()
	waitForFileLines(t, filepath.Join(dir, "launches"), 3)
	waitForControllerJava(t, controller)
	if runtime := controller.currentRuntime(); runtime.generation != 3 {
		t.Fatalf("generation after stopped restart = %d", runtime.generation)
	}

	cancel()
	if err := controller.shutdown(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-programDone:
	case <-time.After(time.Second):
		t.Fatal("Bubble Tea did not stop")
	}

	data, err := os.ReadFile(filepath.Join(dir, "commands"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Fields(string(data)); len(got) != 5 ||
		strings.Join(got, " ") != "say hello stop stop stop" {
		t.Fatalf("commands = %q", data)
	}
}

type fakeStoppableServer struct {
	done           chan struct{}
	commands       []string
	signals        []os.Signal
	finishOnSend   bool
	finishOnSignal bool
	signalErr      error
}

func newFakeStoppableServer() *fakeStoppableServer {
	return &fakeStoppableServer{done: make(chan struct{})}
}

func (s *fakeStoppableServer) Done() <-chan struct{} {
	return s.done
}

func (s *fakeStoppableServer) Send(command string) error {
	s.commands = append(s.commands, command)
	if s.finishOnSend {
		close(s.done)
	}
	return nil
}

func (s *fakeStoppableServer) Signal(signal os.Signal) error {
	s.signals = append(s.signals, signal)
	if s.signalErr != nil {
		return s.signalErr
	}
	if s.finishOnSignal {
		close(s.done)
	}
	return nil
}

func TestResolveJavaEnvAllPathsContinueStartup(t *testing.T) {
	root := t.TempDir()
	configured := filepath.Join(t.TempDir(), "jdk-21")
	makeControllerJava(t, configured, "")

	env, warning := resolveJavaEnv(configured, root, "/usr/bin")
	if len(env) != 1 || env[0] != "PATH="+filepath.Join(configured, "bin")+":/usr/bin" || warning != "" {
		t.Fatalf("configured: env=%q warning=%q", env, warning)
	}

	if err := os.Remove(filepath.Join(configured, "bin", "java")); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(root, "jdk-21.0.2")
	makeControllerJava(t, replacement, "JAVA_VERSION=\"21.0.2\"\n")
	env, warning = resolveJavaEnv(configured, root, "/usr/bin")
	if len(env) != 1 || env[0] != "PATH="+filepath.Join(replacement, "bin")+":/usr/bin" ||
		!strings.Contains(warning, configured) || !strings.Contains(warning, replacement) {
		t.Fatalf("replacement: env=%q warning=%q", env, warning)
	}

	if err := os.Remove(filepath.Join(replacement, "bin", "java")); err != nil {
		t.Fatal(err)
	}
	env, warning = resolveJavaEnv(configured, root, "/usr/bin")
	if env != nil || !strings.Contains(warning, configured) ||
		!strings.Contains(warning, "hso java change") {
		t.Fatalf("no injection: env=%q warning=%q", env, warning)
	}
}

// Java の警告は端末へ直接書かない。標準エラーへ書くと、初回は TUI が画面を
// 占有した瞬間に消え、再起動では描画中の画面に割り込んで崩す。警告はログの
// 経路に流し、他のサーバー出力と同じように表示させる。
func TestServerControllerKeepsJavaWarningOffStderr(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 5\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	// 存在しない JAVA_HOME。代わりも見つからないので警告つきで起動する。
	missing := filepath.Join(dir, "no-such-jdk")

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stderr
	os.Stderr = writer

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	actions := make(chan ui.Action, actionQueueSize)
	model := ui.New(actions, nil, initialGeneration, ui.DefaultSettings(), ui.ServerInfo{})
	program := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithInput(nil),
		tea.WithoutRenderer(),
	)
	controller := newServerController(ctx, config.Config{
		Server: config.Server{Command: script, WorkDir: dir, Java: missing},
	}, program)
	// 注入できなくても起動は続く。ここで失敗するなら補助機能が主機能を
	// 止めている。
	startErr := controller.start(initialGeneration, false)

	os.Stderr = original
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	written, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	_ = controller.shutdown()

	if startErr != nil {
		t.Fatalf("start with an unusable java = %v", startErr)
	}
	if len(written) != 0 {
		t.Fatalf("start wrote to stderr: %q", written)
	}
}

func makeControllerJava(t *testing.T, home, release string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "bin", "java"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if release != "" {
		if err := os.WriteFile(filepath.Join(home, "release"), []byte(release), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func runFakeJava(dir string) {
	appendTestLine(filepath.Join(dir, "launches"), "start")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		command := scanner.Text()
		appendTestLine(filepath.Join(dir, "commands"), command)
		if command == "stop" {
			return
		}
	}
}

func appendTestLine(path, line string) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = file.WriteString(line + "\n")
	_ = file.Close()
}

func waitForControllerJava(t *testing.T, controller *serverController) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		runtime := controller.currentRuntime()
		if runtime != nil && runtime.javaFound.Load() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("java process was not found")
}

func waitForControllerStopped(t *testing.T, controller *serverController) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if controller.currentRuntime() == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server runtime was not detached")
}

func waitForFileLines(t *testing.T, path string, count int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && len(strings.Fields(string(data))) >= count {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s did not contain %d lines", path, count)
}

func TestPumpLogsDrainsRemainingOutputBeforeDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	actions := make(chan ui.Action, 1)
	model := ui.New(actions, nil, initialGeneration, ui.DefaultSettings(), ui.ServerInfo{})
	program := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithInput(nil),
		tea.WithoutRenderer(),
	)
	programDone := make(chan struct{})
	go func() {
		_, _ = program.Run()
		close(programDone)
	}()

	// クラッシュ直前のスタックトレースを模して、キューを埋めてから閉じる。
	logs := make(chan serverlog.Entry, logQueueSize)
	for index := 0; index < logQueueSize; index++ {
		logs <- serverlog.Entry{Kind: serverlog.KindOther, Message: "crash tail"}
	}
	close(logs)

	done := make(chan struct{})
	go pumpLogs(ctx, program, logs, initialGeneration, done)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pumpLogs did not finish")
	}
	if remaining := len(logs); remaining != 0 {
		t.Fatalf("dropped %d entries", remaining)
	}

	program.Quit()
	<-programDone
}
