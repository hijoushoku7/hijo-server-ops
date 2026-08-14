package main

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/pidfile"
	"github.com/hijoushoku7/hijo-server-ops/internal/registry"
)

func runList(output io.Writer) error {
	path, err := registry.Path()
	if err != nil {
		return err
	}
	servers, err := registry.Load(path)
	if err != nil {
		return err
	}
	return printServerList(output, servers, pidfile.Running)
}

func printServerList(
	output io.Writer,
	servers registry.Registry,
	running func(string) (int, bool, error),
) error {
	if len(servers.Servers) == 0 {
		_, err := io.WriteString(output, msg.EmptyServerList+"\n")
		if err != nil {
			return msg.WriteServerListFailed(err)
		}
		return nil
	}

	type row struct {
		name   string
		status string
		path   string
	}
	rows := []row{{msg.ListNameHeader, msg.ListStatusHeader, msg.ListPathHeader}}
	nameWidth := ansi.StringWidth(msg.ListNameHeader)
	statusWidth := ansi.StringWidth(msg.ListStatusHeader)
	for _, server := range servers.Servers {
		status, err := serverStatus(server, running)
		if err != nil {
			return err
		}
		rows = append(rows, row{server.Name, status, server.Config})
		nameWidth = max(nameWidth, ansi.StringWidth(server.Name))
		statusWidth = max(statusWidth, ansi.StringWidth(status))
	}
	for _, row := range rows {
		line := row.name + strings.Repeat(" ", nameWidth-ansi.StringWidth(row.name)+2) +
			row.status + strings.Repeat(" ", statusWidth-ansi.StringWidth(row.status)+2) +
			row.path + "\n"
		if _, err := io.WriteString(output, line); err != nil {
			return msg.WriteServerListFailed(err)
		}
	}
	return nil
}

func serverStatus(
	server registry.Server,
	running func(string) (int, bool, error),
) (string, error) {
	if _, err := os.Stat(server.Config); errors.Is(err, fs.ErrNotExist) {
		return msg.ConfigNotFound, nil
	} else if err != nil {
		return "", msg.CheckRegisteredConfigFailed(err, server.Config)
	}
	pid, alive, err := running(server.Name)
	if err != nil {
		return "", err
	}
	if alive {
		return msg.ServerRunning(pid), nil
	}
	return msg.ServerStopped, nil
}
