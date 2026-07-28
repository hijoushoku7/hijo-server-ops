package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hijoushoku7/hijo-server-ops/internal/config"
	"github.com/hijoushoku7/hijo-server-ops/internal/process"
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

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	return runTUI(cfg)
}
