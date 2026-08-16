package main

import (
	"errors"
	"fmt"
	"io/fs"
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
	// 権限が足りないだけのときに「見つからない」と言うと、登録パスを疑って
	// 直せない。list の inspectServer と同じく、無いことだけを特別扱いする。
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", msg.CheckServerDirectoryFailed(err, dir)
	}
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

// shellEnvironment は開くシェルへ渡す環境を作る。入れ子で開いたときに前の
// サーバー名が残らないよう、既にある HSO_SERVER は残さず置き換える。
func shellEnvironment(environment []string, name string) []string {
	serverVariable := "HSO_SERVER=" + name
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
	return environment
}

func openShell(server registry.Server, dir string) error {
	shell := shellPath()
	if err := os.Chdir(dir); err != nil {
		return msg.OpenShellFailed(err, shell)
	}
	fmt.Println(msg.CdOpeningShell(server.Name, dir))
	if err := syscall.Exec(shell, []string{shell}, shellEnvironment(os.Environ(), server.Name)); err != nil {
		return msg.OpenShellFailed(err, shell)
	}
	return nil
}
