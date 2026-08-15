package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/hijoushoku7/hijo-server-ops/internal/config"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/pidfile"
	"github.com/hijoushoku7/hijo-server-ops/internal/process"
	"github.com/hijoushoku7/hijo-server-ops/internal/setup"
)

var version = "dev"

func main() {
	if command, ok := process.SupervisorCommand(os.Args); ok {
		os.Exit(process.RunSupervisor(command))
	}

	handled, err := dispatchCommand(os.Args[1:], os.Stdout)
	if !handled {
		err = run()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "hso: %v\n", err)
		os.Exit(1)
	}
}

func dispatchCommand(args []string, output io.Writer) (bool, error) {
	// 引数なしの hso は何も起動せずヘルプを出す。設定を読んで起動する経路は
	// start か -config に集約し、打ち間違いでセットアップが始まらないようにする。
	if len(args) == 0 {
		return true, writeHelp(output)
	}
	// ヘルプとバージョンだけは `-` 付きの書き方も受ける。flag の既定の使い方
	// 表示より、コマンド一覧やバージョンを出すほうが探しているものに近い。
	// 後ろに何が付いていても同じものを出す（打ち間違いでエラーにしない）。
	switch args[0] {
	case "help", "-h", "-help", "--help":
		return true, writeHelp(output)
	case "-v", "--v", "-version", "--version":
		return true, runVersion(output)
	}
	if strings.HasPrefix(args[0], "-") {
		return false, nil
	}

	switch args[0] {
	case "setup":
		if len(args) != 1 {
			return true, msg.SetupArgumentsNotAllowed()
		}
		return true, runSetup(output)
	case "start":
		if len(args) > 2 {
			return true, msg.StartArgumentsNotAllowed()
		}
		name := ""
		if len(args) == 2 {
			name = args[1]
		}
		return true, runStart(name)
	case "delete":
		return true, runDelete(args[1:], os.Stdin, output, interactive())
	case "java":
		return true, runJava(args[1:], output, os.Stderr, interactive())
	case "version":
		if len(args) != 1 {
			return true, msg.VersionArgumentsNotAllowed()
		}
		return true, runVersion(output)
	case "update":
		if len(args) != 1 {
			return true, msg.UpdateArgumentsNotAllowed()
		}
		return true, runUpdate(output)
	case "uninstall":
		return true, runUninstall(args[1:], os.Stdin, output, interactive())
	case "list", "ls":
		if len(args) != 1 {
			return true, msg.ListArgumentsNotAllowed()
		}
		return true, runList(output)
	default:
		return true, msg.UnknownCommand(args[0])
	}
}

func writeHelp(output io.Writer) error {
	_, err := io.WriteString(output, msg.CommandHelp+"\n")
	return err
}

func run() error {
	configPath := flag.String("config", "hso.toml", msg.ConfigFlagUsage)
	flag.Parse()

	// ここへ来るのは -config を付けた呼び方だけで、引数なしの hso はヘルプへ
	// 分かれている。指定した設定ファイルがなければセットアップへ回し、作成
	// できたらそのままサーバーを起動する。端末がないときはウィザードを出せ
	// ないので、従来どおり config.Load のエラーを返す。
	if missingConfig(*configPath) && interactive() {
		created, err := setup.Run(*configPath)
		if err != nil {
			return err
		}
		if created == "" {
			fmt.Fprintln(os.Stderr, "hso: "+msg.Aborted)
			return nil
		}
		return launchConfig(created)
	}
	return runExistingConfig(*configPath, false, interactive(), os.Stderr,
		config.Load, registeredName, setup.Register, launchLoaded)
}

func runSetup(output io.Writer) error {
	if !interactive() {
		return msg.SetupRequiresTerminal()
	}
	if !missingConfig("hso.toml") {
		return runExistingConfig("hso.toml", true, true, output,
			config.Load, registeredName, setup.Register, launchLoaded)
	}
	created, err := setup.Run("hso.toml")
	if err != nil {
		return err
	}
	if created == "" {
		_, err := fmt.Fprintln(output, msg.Aborted)
		return err
	}
	return launchConfig(created)
}

// launchConfig は従来の設定読み込みと TUI 起動経路をそのまま使う。
func launchConfig(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	return launchLoaded(configPath, cfg)
}

func launchLoaded(configPath string, cfg config.Config) error {
	return runTrackedTUI(configPath, cfg, trackRegisteredServer, runTUI)
}

func runExistingConfig(
	configPath string,
	setupCommand bool,
	terminal bool,
	output io.Writer,
	load func(string) (config.Config, error),
	lookup func(string) (string, bool, error),
	prompt func(string, config.Config) (string, bool, error),
	launch func(string, config.Config) error,
) error {
	if setupCommand && !terminal {
		return msg.SetupRequiresTerminal()
	}
	cfg, err := load(configPath)
	if err != nil {
		return err
	}
	name, found, err := lookup(configPath)
	if err != nil {
		return err
	}
	if found {
		if setupCommand {
			return msg.ConfigAlreadyRegistered(name, configPath)
		}
		return launch(configPath, cfg)
	}
	if terminal {
		name, canceled, err := prompt(configPath, cfg)
		if err != nil {
			return err
		}
		// Ctrl+C の中止はどちらの経路でも起動しない。Esc の「追加しない」で
		// 起動を止めるのは setup のときだけで、素の hso は起動しに来ている。
		if canceled || (name == "" && setupCommand) {
			_, err := fmt.Fprintln(output, msg.Aborted)
			return err
		}
	}
	return launch(configPath, cfg)
}

func runTrackedTUI(
	configPath string,
	cfg config.Config,
	track func(string) (*pidfile.File, error),
	launch func(string, config.Config) error,
) error {
	tracking, trackingErr := track(configPath)
	if trackingErr != nil {
		return trackingErr
	}
	if tracking != nil {
		defer tracking.Close()
	}
	return launch(configPath, cfg)
}

func missingConfig(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, fs.ErrNotExist)
}

// interactive は対話的なウィザードを出せる端末かどうか。パイプや
// systemd 配下では出せないので、その場合は普通のエラーで終わらせる。
// /dev/null もキャラクタデバイスなので、ファイルの種類ではなく
// termios を引けるかどうかで判定する。
func interactive() bool {
	return isTerminal(os.Stdin) && isTerminal(os.Stdout)
}

func isTerminal(file *os.File) bool {
	return term.IsTerminal(file.Fd())
}
