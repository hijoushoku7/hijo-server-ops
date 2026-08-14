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

const availableSubcommands = "setup, start, list (ls), version, update, uninstall"

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
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
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
		return true, msg.UnknownCommand(args[0], availableSubcommands)
	}
}

func run() error {
	configPath := flag.String("config", "hso.toml", msg.ConfigFlagUsage)
	flag.Parse()

	// 設定ファイルがない初回はセットアップへ回し、作成できたらそのまま
	// サーバーを起動する。端末がないときはウィザードを出せないので、
	// 従来どおり config.Load のエラーを返す。
	if missingConfig(*configPath) && interactive() {
		created, err := setup.Run(*configPath)
		if err != nil {
			return err
		}
		if created == "" {
			fmt.Fprintln(os.Stderr, "hso: "+msg.Aborted)
			return nil
		}
	}

	return launchConfig(*configPath)
}

func runSetup(output io.Writer) error {
	if !interactive() {
		return msg.SetupRequiresTerminal()
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
	return runTrackedTUI(configPath, cfg, trackRegisteredServer, runTUI)
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
