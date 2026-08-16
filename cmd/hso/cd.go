package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/pidfile"
	"github.com/hijoushoku7/hijo-server-ops/internal/registry"
)

func runCd(args []string) error {
	if len(args) > 1 {
		return msg.CdArgumentsNotAllowed()
	}
	if !interactive() {
		return msg.CdRequiresTerminal()
	}

	path, err := registry.Path()
	if err != nil {
		return err
	}
	servers, err := registry.Load(path)
	if err != nil {
		return err
	}
	name := ""
	if len(args) == 1 {
		name = args[0]
	}
	// cd は起動中かどうかで動きを変えないが、一覧に出す状態は本当のものを出す。
	// 起動中のサーバーを「停止」と書くと、そこだけ嘘の表になる。
	choose := func(servers registry.Registry) (registry.Server, bool, error) {
		return chooseServer(servers, pidfile.Running, msg.CdTitle)
	}
	return cdFromRegistry(name, servers, choose, openShell)
}

func cdFromRegistry(
	name string,
	servers registry.Registry,
	choose serverChooser,
	open func(server registry.Server, dir string) error,
) error {
	if len(servers.Servers) == 0 {
		return msg.NoRegisteredServers()
	}

	var server registry.Server
	if name == "" {
		var chosen bool
		var err error
		server, chosen, err = choose(servers)
		if err != nil || !chosen {
			return err
		}
	} else {
		if err := registry.ValidateName(name); err != nil {
			return err
		}
		var found bool
		server, found = servers.Find(name)
		if !found {
			return msg.ServerNotRegistered(name)
		}
	}

	dir, err := serverDirectory(server)
	if err != nil {
		return err
	}
	return open(server, dir)
}

func serverDirectory(server registry.Server) (string, error) {
	dir := filepath.Dir(server.Config)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", msg.ServerDirectoryNotFound(server.Name, dir)
	}
	return dir, nil
}

func shellPath() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/sh"
}

func openShell(server registry.Server, dir string) error {
	shell := shellPath()
	if err := os.Chdir(dir); err != nil {
		return msg.OpenShellFailed(err, shell)
	}

	environment := os.Environ()
	serverVariable := "HSO_SERVER=" + server.Name
	replaced := false
	for index, variable := range environment {
		if strings.HasPrefix(variable, "HSO_SERVER=") {
			environment[index] = serverVariable
			replaced = true
		}
	}
	if !replaced {
		environment = append(environment, serverVariable)
	}

	fmt.Println(msg.CdOpeningShell(server.Name, dir))
	if err := syscall.Exec(shell, []string{shell}, environment); err != nil {
		return msg.OpenShellFailed(err, shell)
	}
	return nil
}
