package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/pidfile"
	"github.com/hijoushoku7/hijo-server-ops/internal/registry"
)

type deleteOptions struct {
	name string
	yes  bool
}

type registrySaver func(registry.Registry) error
type registryLoader func() (registry.Registry, error)

func runDelete(args []string, input io.Reader, output io.Writer, terminal bool) error {
	options, err := parseDeleteOptions(args)
	if err != nil {
		return err
	}
	path, err := registry.Path()
	if err != nil {
		return err
	}
	choose := func(servers registry.Registry) (registry.Server, bool, error) {
		return chooseServer(servers, pidfile.Running, msg.DeleteTitle)
	}
	return deleteFromRegistry(options, func() (registry.Registry, error) { return registry.Load(path) }, terminal, input, output, choose, pidfile.Running,
		func(servers registry.Registry) error { return registry.Save(path, servers) })
}

func parseDeleteOptions(args []string) (deleteOptions, error) {
	var options deleteOptions
	for _, argument := range args {
		switch argument {
		case "-y", "--yes":
			options.yes = true
		default:
			if options.name != "" || strings.HasPrefix(argument, "-") {
				return deleteOptions{}, msg.DeleteArgumentsNotAllowed()
			}
			options.name = argument
		}
	}
	return options, nil
}

func deleteFromRegistry(
	options deleteOptions,
	load registryLoader,
	terminal bool,
	input io.Reader,
	output io.Writer,
	choose serverChooser,
	running func(string) (int, bool, error),
	save registrySaver,
) error {
	servers, err := load()
	if err != nil {
		return err
	}
	if len(servers.Servers) == 0 {
		return msg.NoRegisteredServers()
	}
	if options.name == "" && !terminal {
		return msg.DeleteRequiresTerminal()
	}

	var server registry.Server
	if options.name == "" {
		var chosen bool
		var err error
		server, chosen, err = choose(servers)
		if err != nil || !chosen {
			return err
		}
	} else {
		if err := registry.ValidateName(options.name); err != nil {
			return err
		}
		var found bool
		server, found = servers.Find(options.name)
		if !found {
			return msg.ServerNotRegistered(options.name)
		}
	}

	if err := ensureStopped(server.Name, running); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(output, "%s\n\n", msg.DeleteTarget(server.Name, server.Config)); err != nil {
		return err
	}
	confirmed, err := confirmDelete(input, output, terminal, options.yes)
	if err != nil {
		return err
	}
	if !confirmed {
		_, err := fmt.Fprintln(output, msg.Aborted)
		return err
	}
	// 確認は人間が考えている間ずっと開いている。その間に別の端末が一覧を
	// 変えている（setup で足す・別の delete で消す・start で起動する）ことが
	// あるので、読み直してから消す。ロックは入れない。単一ユーザー向けの
	// ツールで、他の書き込み経路も同じ方式のため、複雑さに見合わない。
	// 読み直しから保存までのわずかな窓は残る。
	targetName := server.Name
	servers, err = load()
	if err != nil {
		return err
	}
	server, found := servers.Find(targetName)
	if !found {
		return msg.ServerNotRegistered(targetName)
	}
	if err := ensureStopped(server.Name, running); err != nil {
		return err
	}
	servers.Remove(server.Name)
	if err := save(servers); err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, msg.ServerDeleted(server.Name))
	return err
}

// ensureStopped は起動中のサーバーを削除させないための検査。設定ファイルが
// 見つからない登録でも起動中なら削除させたくないので、設定ファイルの有無を
// 先に見る inspectServer は使わない。
func ensureStopped(name string, running func(string) (int, bool, error)) error {
	pid, alive, err := running(name)
	if err != nil {
		return err
	}
	if alive {
		return msg.CannotDeleteRunningServer(name, pid)
	}
	return nil
}

func confirmDelete(input io.Reader, output io.Writer, terminal, yes bool) (bool, error) {
	if yes {
		return true, nil
	}
	if !terminal {
		return false, msg.DeleteRequiresConfirmation()
	}
	if _, err := io.WriteString(output, msg.DeleteConfirmPrompt); err != nil {
		return false, err
	}
	line, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(line), "y"), nil
}
