package main

import (
	"flag"
	"fmt"
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
	initialize := flag.Bool("init", false, "設定ファイルを対話的に作成する")
	flag.Parse()

	if *initialize {
		created, err := setup.Run(*configPath)
		if err != nil {
			return err
		}
		if created == "" {
			fmt.Fprintln(os.Stderr, "hso: 中止しました")
			return nil
		}
		fmt.Printf("作成しました: %s\n", created)
		return nil
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	return runTUI(cfg)
}
