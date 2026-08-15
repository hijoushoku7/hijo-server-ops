package main

import (
	"fmt"
	"io"

	"github.com/hijoushoku7/hijo-server-ops/internal/completion"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/registry"
)

func runComplete(words []string, output io.Writer) {
	var servers []registry.Server
	if path, err := registry.Path(); err == nil {
		if loaded, err := registry.Load(path); err == nil {
			servers = loaded.Servers
		}
	}
	for _, candidate := range completion.Candidates(words, servers) {
		if candidate.Description == "" {
			_, _ = fmt.Fprintln(output, candidate.Value)
		} else {
			_, _ = fmt.Fprintf(output, "%s\t%s\n", candidate.Value, candidate.Description)
		}
	}
}

func runCompletion(args []string, output io.Writer) error {
	if len(args) != 1 {
		return msg.CompletionArgumentsInvalid()
	}
	script, ok := completion.Script(args[0])
	if !ok {
		return msg.UnsupportedCompletionShell(args[0])
	}
	_, err := io.WriteString(output, script)
	return err
}
