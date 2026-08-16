// Package completion は hso のシェル補完候補を決定する。
package completion

import (
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/registry"
)

type Candidate struct {
	Value       string
	Description string
}

// Candidates は打たれた単語列に対する補完候補を返す。
// words は "hso" から始まり、末尾は打ちかけの単語（空文字もある）。
func Candidates(words []string, servers []registry.Server) []Candidate {
	if len(words) == 2 {
		return commandCandidates()
	}
	if len(words) == 3 {
		switch words[1] {
		case "start", "cd":
			return serverCandidates(servers)
		case "delete":
			return append(serverCandidates(servers),
				Candidate{Value: "-y", Description: msg.CompletionYesDescription},
				Candidate{Value: "--yes", Description: msg.CompletionYesDescription})
		case "java":
			return []Candidate{
				{Value: "change", Description: msg.CompletionJavaChangeDescription},
				{Value: "list", Description: msg.CompletionJavaListDescription},
			}
		case "uninstall":
			return []Candidate{
				{Value: "--purge", Description: msg.CompletionPurgeDescription},
				{Value: "-y", Description: msg.CompletionYesDescription},
				{Value: "--yes", Description: msg.CompletionYesDescription},
			}
		case "completion":
			return []Candidate{{Value: "bash"}, {Value: "zsh"}, {Value: "fish"}}
		case "-config":
			return []Candidate{{Value: ":files"}}
		}
	}
	if len(words) == 4 && words[1] == "java" && words[2] == "change" {
		return serverCandidates(servers)
	}
	return nil
}

func commandCandidates() []Candidate {
	return []Candidate{
		{Value: "setup", Description: msg.CompletionSetupDescription},
		{Value: "start", Description: msg.CompletionStartDescription},
		{Value: "cd", Description: msg.CompletionCdDescription},
		{Value: "list", Description: msg.CompletionListDescription},
		{Value: "ls", Description: msg.CompletionListDescription},
		{Value: "delete", Description: msg.CompletionDeleteDescription},
		{Value: "java", Description: msg.CompletionJavaDescription},
		{Value: "completion", Description: msg.CompletionCommandDescription},
		{Value: "version", Description: msg.CompletionVersionDescription},
		{Value: "update", Description: msg.CompletionUpdateDescription},
		{Value: "uninstall", Description: msg.CompletionUninstallDescription},
		{Value: "help", Description: msg.CompletionHelpDescription},
		{Value: "-config", Description: msg.CompletionConfigDescription},
	}
}

func serverCandidates(servers []registry.Server) []Candidate {
	candidates := make([]Candidate, 0, len(servers))
	for _, server := range servers {
		candidates = append(candidates, Candidate{Value: server.Name, Description: server.Config})
	}
	return candidates
}
