// Package setup は hso.toml を対話的に生成する。
package setup

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/config"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/registry"
)

// Run は設定ファイルを対話的に作る。作成したら作成先のパスを返す。
// ユーザーが中止したときは空文字列を返す。
func Run(configPath string) (string, error) {
	path, err := filepath.Abs(configPath)
	if err != nil {
		return "", msg.ConfigAbsPathFailed(err)
	}
	if _, err := os.Stat(path); err == nil {
		return "", msg.ConfigAlreadyExists(path)
	}
	registryPath, err := registry.Path()
	if err != nil {
		return "", err
	}
	servers, err := registry.Load(registryPath)
	if err != nil {
		return "", err
	}
	if name, found := servers.NameForConfig(path); found {
		return "", msg.ConfigAlreadyRegistered(name, path)
	}

	model := newModel(path, servers)
	if _, err := tea.NewProgram(model).Run(); err != nil {
		return "", err
	}
	if model.err != nil {
		return "", model.err
	}
	if !model.created {
		return "", nil
	}
	if err := registerServer(registryPath, model.name, path); err != nil {
		// この実行で作った設定だけを戻し、名前を直して再実行できる状態を保つ。
		if removeErr := os.Remove(path); removeErr != nil {
			return "", msg.RemoveConfigAfterRegistrationFailed(err, removeErr)
		}
		return "", err
	}
	return path, nil
}

// Register は既存の hso.toml をサーバー一覧へ登録する。
// Esc で追加を断ったときは name == ""、Ctrl+C で中止したときは canceled == true。
func Register(configPath string, cfg config.Config) (name string, canceled bool, err error) {
	return register(configPath, cfg, func(model *model) error {
		_, err := tea.NewProgram(model).Run()
		return err
	})
}

func register(configPath string, cfg config.Config, runWizard func(*model) error) (name string, canceled bool, err error) {
	path, err := filepath.Abs(configPath)
	if err != nil {
		return "", false, msg.ConfigAbsPathFailed(err)
	}
	registryPath, err := registry.Path()
	if err != nil {
		return "", false, err
	}
	servers, err := registry.Load(registryPath)
	if err != nil {
		return "", false, err
	}
	if name, found := servers.NameForConfig(path); found {
		return "", false, msg.ConfigAlreadyRegistered(name, path)
	}

	model := newRegisterModel(path, cfg, servers)
	if err := runWizard(model); err != nil {
		return "", false, err
	}
	if model.err != nil {
		return "", false, model.err
	}
	if !model.created {
		return "", model.canceled, nil
	}
	if err := registerServer(registryPath, model.name, path); err != nil {
		return "", false, err
	}
	return model.name, false, nil
}

func registerServer(path, name, configPath string) error {
	return registry.Update(path, func(servers *registry.Registry) error {
		return servers.Add(registry.Server{Name: name, Config: configPath})
	})
}

// candidate は起動スクリプトの候補。実行権限がないファイルも候補に出す。
// 権限を落としたまま置かれている run.sh は珍しくなく、除外すると
// 一覧が空になって選ばせる意味がなくなる。
type candidate struct {
	name       string
	executable bool
}

func (item candidate) label() string {
	if item.executable {
		return item.name
	}
	return item.name + "  " + msg.SetupNotExecutable
}

// isCandidate は起動スクリプトになりうるファイルかどうか。.sh は権限に
// 関わらず候補にする。それ以外は実行可能かつ拡張子なしのものだけを拾い、
// 実行ビットの付いた jar や画像がノイズとして並ぶのを防ぐ。
func isCandidate(name string, executable bool) bool {
	if name == "hso" {
		return false
	}
	if strings.HasSuffix(name, ".sh") {
		return true
	}
	return executable && filepath.Ext(name) == ""
}

// scanCommands は起動スクリプトの候補を列挙する。実行可能なものを先に並べる。
func scanCommands(dir string) []candidate {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var candidates []candidate
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		executable := info.Mode().Perm()&0o111 != 0
		if !isCandidate(entry.Name(), executable) {
			continue
		}
		candidates = append(candidates, candidate{
			name:       entry.Name(),
			executable: executable,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].executable != candidates[j].executable {
			return candidates[i].executable
		}
		return candidates[i].name < candidates[j].name
	})
	return candidates
}

// resolveCommand は入力されたパスを検証し、設定に書く形へ整える。
// workDir の下にあるファイルは相対パスにして、ディレクトリごと
// 移動しても設定が壊れないようにする。
func resolveCommand(input, workDir string) (command string, path string, err error) {
	input = strings.TrimSpace(expandHome(input))
	if input == "" {
		return "", "", msg.EnterCommand()
	}

	path = input
	if !filepath.IsAbs(path) {
		path = filepath.Join(workDir, path)
	}
	path = filepath.Clean(path)

	info, err := os.Stat(path)
	if err != nil {
		return "", "", msg.FileNotFound(path)
	}
	if !info.Mode().IsRegular() {
		return "", "", msg.NotRegularFile(path)
	}

	command = path
	if relative, err := filepath.Rel(workDir, path); err == nil &&
		!strings.HasPrefix(relative, "..") {
		command = "./" + relative
	}
	return command, path, nil
}

func resolveWorkDir(input string) (string, error) {
	input = strings.TrimSpace(expandHome(input))
	if input == "" {
		return "", msg.EnterDirectory()
	}
	path, err := filepath.Abs(input)
	if err != nil {
		return "", msg.AbsPathFailed(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", msg.DirectoryNotFound(path)
	}
	if !info.IsDir() {
		return "", msg.NotDirectory(path)
	}
	return path, nil
}

func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}

// render は書き出す TOML を組み立てる。workdir は設定ファイルと同じ
// ディレクトリなら省略する（config.Load の既定値と同じになる）。
func render(command, workDir, configDir string) string {
	var out strings.Builder
	out.WriteString("[server]\n")
	out.WriteString("command = " + quote(command) + "\n")
	if workDir != configDir {
		out.WriteString("workdir = " + quote(workDir) + "\n")
	}
	return out.String()
}

// quote は TOML の基本文字列にする。改行を含むパスは滅多にないが、
// そのまま書くと次の起動で読めない設定ファイルができるのでエスケープする。
func quote(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
	)
	return `"` + replacer.Replace(value) + `"`
}

func writeConfig(path, content string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return msg.CreateConfigFailed(err)
	}
	if _, err := file.WriteString(content); err != nil {
		file.Close()
		return msg.WriteConfigFailed(err)
	}
	return file.Close()
}

// grantExecute は起動スクリプトに実行権限を付ける。読める相手にだけ
// 実行を許し、他人に新しく読み書きを許すことはしない。
func grantExecute(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return msg.ScriptStatFailed(err)
	}
	mode := info.Mode().Perm()
	mode |= (mode & 0o444) >> 2
	if err := os.Chmod(path, mode); err != nil {
		return msg.ChmodFailed(err)
	}
	return nil
}
