package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"

	"github.com/hijoushoku7/hijo-server-ops/internal/config"
	"github.com/hijoushoku7/hijo-server-ops/internal/process"
	"github.com/hijoushoku7/hijo-server-ops/internal/setup"
)

func main() {
	if command, ok := process.SupervisorCommand(os.Args); ok {
		os.Exit(process.RunSupervisor(command))
	}
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "hso: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "hso.toml", "設定ファイルのパス")
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
			fmt.Fprintln(os.Stderr, "hso: 中止しました")
			return nil
		}
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	return runTUI(cfg)
}

func missingConfig(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, fs.ErrNotExist)
}

// interactive は対話的なウィザードを出せる端末かどうか。パイプや
// systemd 配下では出せないので、その場合は普通のエラーで終わらせる。
func interactive() bool {
	return isTerminal(os.Stdin) && isTerminal(os.Stdout)
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
