package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/hijoushoku7/hijo-server-ops/internal/config"
	"github.com/hijoushoku7/hijo-server-ops/internal/process"
)

func main() {
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

	server, err := process.Start(process.Options{
		Command: cfg.Server.Command,
		WorkDir: cfg.Server.WorkDir,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	})
	if err != nil {
		return err
	}

	go copyInput(server, os.Stdin)

	findCtx, cancelFind := context.WithCancel(context.Background())
	defer cancelFind()

	javaResult := make(chan findResult, 1)
	javaFound := false
	go func() {
		pid, findErr := (process.JavaFinder{}).Wait(findCtx, server.PID())
		javaResult <- findResult{pid: pid, err: findErr}
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

	for {
		select {
		case <-server.Done():
			cancelFind()
			waitErr := server.Wait()
			if !javaFound {
				if waitErr != nil {
					return waitErr
				}
				return errors.New("起動スクリプトがjavaプロセスを開始せずに終了しました")
			}
			return waitErr
		case result := <-javaResult:
			if result.err != nil {
				if errors.Is(result.err, context.Canceled) {
					javaResult = nil
					continue
				}
				_ = server.Signal(syscall.SIGTERM)
				<-server.Done()
				return fmt.Errorf("javaプロセスの特定: %w", result.err)
			}
			fmt.Fprintf(os.Stderr, "hso: java pid %d\n", result.pid)
			javaFound = true
			javaResult = nil
		case sig := <-signals:
			if err := server.Signal(sig); err != nil {
				return fmt.Errorf("シグナル転送: %w", err)
			}
		}
	}
}

type findResult struct {
	pid int
	err error
}

func copyInput(server *process.Process, input io.Reader) {
	_, _ = io.Copy(server, input)
}
