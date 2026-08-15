package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/hijoushoku7/hijo-server-ops/internal/pidfile"
	"github.com/hijoushoku7/hijo-server-ops/internal/registry"
)

var uninstallEUID = os.Geteuid
var uninstallHomeDir = os.UserHomeDir

type uninstallOptions struct {
	purge bool
	yes   bool
}

type uninstallPlan struct {
	executable       string
	executableInfo   fs.FileInfo
	configFile       string
	configDir        string
	pidDir           string
	completionPaths  []string
	running          []string
	runningUnchecked bool
	serverListKept   bool
	binaryWritable   bool
	options          uninstallOptions
}

type removalState uint8

const (
	removalRemoved removalState = iota
	removalAbsent
	removalKept
	removalFailed
)

type removalResult struct {
	path  string
	state removalState
	err   error
}

func runUninstall(args []string, input io.Reader, output io.Writer, terminal bool) error {
	options, err := parseUninstallOptions(args)
	if err != nil {
		return err
	}
	if err := validateUninstallRoot(options.purge, uninstallEUID()); err != nil {
		return err
	}

	plan, err := prepareUninstall(options)
	if err != nil {
		return err
	}
	if !plan.binaryWritable && !options.purge {
		return uninstallPermissionError(plan.executable)
	}
	if err := printUninstallPlan(output, plan); err != nil {
		return err
	}

	confirmed, err := confirmUninstall(input, output, terminal, options.yes, !plan.binaryWritable)
	if err != nil {
		return err
	}
	if !confirmed {
		_, err := fmt.Fprintln(output, "Aborted.")
		return err
	}

	results := removeUninstallTargets(plan)
	writeErr := printRemovalSummary(output, results)
	if !options.purge && plan.serverListKept {
		if _, err := fmt.Fprintf(output, "Server list kept at %s (use --purge to remove it).\n", displayPath(plan.configFile)); writeErr == nil {
			writeErr = err
		}
	}
	if writeErr != nil {
		return writeErr
	}

	permissionDenied := !plan.binaryWritable
	failed := false
	for _, result := range results {
		if result.state != removalFailed {
			continue
		}
		if plan.isCompletionPath(result.path) {
			continue
		}
		failed = true
		if result.path == plan.executable &&
			(errors.Is(result.err, syscall.EACCES) || errors.Is(result.err, syscall.EPERM)) {
			permissionDenied = true
		}
	}
	if permissionDenied {
		return uninstallPermissionError(plan.executable)
	}
	if failed {
		return errors.New("uninstall incomplete; see the removal summary above")
	}
	return nil
}

func (plan uninstallPlan) isCompletionPath(path string) bool {
	for _, candidate := range plan.completionPaths {
		if candidate == path {
			return true
		}
	}
	return false
}

func parseUninstallOptions(args []string) (uninstallOptions, error) {
	var options uninstallOptions
	flags := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.BoolVar(&options.purge, "purge", false, "remove config and pidfiles")
	flags.BoolVar(&options.yes, "y", false, "skip confirmation")
	flags.BoolVar(&options.yes, "yes", false, "skip confirmation")
	if err := flags.Parse(args); err != nil {
		return uninstallOptions{}, fmt.Errorf("invalid uninstall options: %w", err)
	}
	if flags.NArg() != 0 {
		return uninstallOptions{}, errors.New("uninstall accepts only --purge, -y, or --yes")
	}
	return options, nil
}

func validateUninstallRoot(purge bool, euid int) error {
	if !purge || euid != 0 {
		return nil
	}
	return errors.New("--purge must not be run as root.\n\n" +
		"As root, hso would look for the config in root's home directory, not yours.\n" +
		"Run it as your normal user first:\n\n" +
		"    hso uninstall --purge\n\n" +
		"If that reports the binary itself needs root, finish with:\n\n" +
		"    sudo hso uninstall")
}

func prepareUninstall(options uninstallOptions) (uninstallPlan, error) {
	executable, executableInfo, err := inspectUninstallExecutable()
	if err != nil {
		return uninstallPlan{}, err
	}
	plan := uninstallPlan{
		executable:     executable,
		executableInfo: executableInfo,
		binaryWritable: targetDirectoryAccess(executable) == nil,
		options:        options,
	}
	if !plan.binaryWritable && !options.purge {
		return plan, nil
	}
	plan.completionPaths, err = uninstallCompletionPaths(uninstallEUID() == 0)
	if err != nil {
		return uninstallPlan{}, err
	}

	// root の通常アンインストールはバイナリだけを扱い、root のホームにある
	// 一覧や pidfile を参照しない。
	if uninstallEUID() == 0 {
		return plan, nil
	}

	configFile, err := registry.Path()
	if err != nil {
		return uninstallPlan{}, err
	}
	plan.configFile = configFile
	plan.configDir = filepath.Dir(configFile)
	_, inspectErr := os.Lstat(configFile)
	if inspectErr == nil {
		plan.serverListKept = true
	}

	servers, loadErr := registry.Load(configFile)
	if inspectErr != nil && !errors.Is(inspectErr, fs.ErrNotExist) {
		loadErr = inspectErr
	}
	if options.purge || loadErr == nil && len(servers.Servers) > 0 {
		plan.pidDir, err = pidfile.Directory()
		if err != nil {
			return uninstallPlan{}, err
		}
	}
	// 一覧は実行中サーバーの注意にだけ使うため、確認できなくても
	// アンインストール自体は続ける。
	if loadErr != nil {
		plan.runningUnchecked = true
		return plan, nil
	}
	if len(servers.Servers) > 0 {
		checker := pidfile.Checker{Directory: plan.pidDir}
		for _, server := range servers.Servers {
			_, running, err := checker.Running(server.Name)
			if err != nil {
				return uninstallPlan{}, err
			}
			if running {
				plan.running = append(plan.running, server.Name)
			}
		}
	}
	return plan, nil
}

func inspectUninstallExecutable() (string, fs.FileInfo, error) {
	executable, err := executablePath()
	if err != nil {
		return "", nil, fmt.Errorf("cannot determine the hso executable path: %w", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", nil, fmt.Errorf("cannot determine the hso executable path: %w", err)
	}
	info, err := os.Lstat(executable)
	if err != nil {
		return "", nil, fmt.Errorf("cannot inspect hso executable %s: %w", executable, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, targetErr := filepath.EvalSymlinks(executable)
		if targetErr != nil {
			target, targetErr = os.Readlink(executable)
			if targetErr != nil {
				return "", nil, fmt.Errorf("cannot inspect symbolic link %s: %w", executable, targetErr)
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(executable), target)
			}
		}
		return "", nil, fmt.Errorf("hso was started through a symbolic link. Nothing was removed.\n\n"+
			"    Link:   %s\n"+
			"    Target: %s\n\n"+
			"Remove the link and target manually after deciding which one you want to keep", executable, target)
	}
	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("refusing to remove %s because it is not a regular file", executable)
	}
	return executable, info, nil
}

func printUninstallPlan(output io.Writer, plan uninstallPlan) error {
	if !plan.binaryWritable {
		if _, err := fmt.Fprintf(output,
			"hso is installed at %s, which requires root to remove.\n"+
				"Your config can be removed now, without root.\n\n"+
				"  Will remove now:   %s, %s\n"+
				"  Needs root:        %s  ->  sudo hso uninstall\n",
			displayPath(plan.executable), displayPath(plan.configDir), displayPath(plan.pidDir),
			displayPath(plan.executable)); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(output, "hso will be removed from %s\n\n  Will remove:\n", displayPath(plan.executable)); err != nil {
			return err
		}
		for _, path := range plan.removalPaths() {
			if _, err := fmt.Fprintf(output, "    %s\n", displayPath(path)); err != nil {
				return err
			}
		}
	}
	if plan.runningUnchecked {
		if _, err := fmt.Fprintf(output,
			"\nCould not read server list at %s; running servers were not checked.\n",
			displayPath(plan.configFile)); err != nil {
			return err
		}
	}

	if len(plan.running) > 0 {
		noun := "servers are"
		if len(plan.running) == 1 {
			noun = "server is"
		}
		if _, err := fmt.Fprintf(output,
			"\n  !! %d %s running right now: %s.\n\n"+
				"     They keep running. Only the command is removed, so `hso list`\n"+
				"     will no longer show them -- go back to the terminal each one\n"+
				"     is running in to stop it.\n",
			len(plan.running), noun, strings.Join(plan.running, ", ")); err != nil {
			return err
		}
	}
	return nil
}

func (plan uninstallPlan) removalPaths() []string {
	paths := append([]string{plan.executable}, plan.completionPaths...)
	if plan.options.purge {
		paths = append(paths, plan.configDir, plan.pidDir)
	}
	return paths
}

func confirmUninstall(
	input io.Reader,
	output io.Writer,
	terminal bool,
	yes bool,
	partial bool,
) (bool, error) {
	if yes {
		return true, nil
	}
	if !terminal {
		return false, errors.New("confirmation requires a terminal; rerun with -y or --yes")
	}
	prompt := "Remove? [y/N]: "
	if partial {
		prompt = "Continue? [y/N]: "
	}
	if _, err := io.WriteString(output, "\n"+prompt); err != nil {
		return false, err
	}
	line, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("cannot read confirmation: %w", err)
	}
	return strings.EqualFold(strings.TrimSpace(line), "y"), nil
}

func removeUninstallTargets(plan uninstallPlan) []removalResult {
	results := make([]removalResult, 0, 6)
	if plan.options.purge {
		results = append(results, removeDirectory(plan.configDir), removeDirectory(plan.pidDir))
	}
	if !plan.binaryWritable {
		results = append(results, removalResult{path: plan.executable, state: removalKept})
		return results
	}
	for _, path := range plan.completionPaths {
		results = append(results, removeCompletion(path))
	}
	results = append(results, removeExecutable(plan.executable, plan.executableInfo))
	return results
}

// uninstallCompletionPaths は install.sh が置いた補完ファイルのうち、実際に
// あるものだけを返す。無いパスまで削除予定として並べると、確認画面が置いて
// いないシェルの分まで長くなる。
func uninstallCompletionPaths(system bool) ([]string, error) {
	candidates := []string{
		"/usr/share/bash-completion/completions/hso",
		"/usr/local/share/zsh/site-functions/_hso",
		"/usr/share/fish/vendor_completions.d/hso.fish",
	}
	if !system {
		home, err := uninstallHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine completion paths: %w", err)
		}
		candidates = []string{
			filepath.Join(home, ".local/share/bash-completion/completions/hso"),
			filepath.Join(home, ".local/share/zsh/site-functions/_hso"),
			filepath.Join(home, ".config/fish/completions/hso.fish"),
		}
	}
	var paths []string
	for _, path := range candidates {
		if _, err := os.Lstat(path); err == nil {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

func removeCompletion(path string) removalResult {
	result := removalResult{path: path}
	if err := os.Remove(path); errors.Is(err, fs.ErrNotExist) {
		result.state = removalAbsent
	} else if err != nil {
		result.state = removalFailed
		result.err = err
	} else {
		result.state = removalRemoved
	}
	return result
}

func removeDirectory(path string) removalResult {
	result := removalResult{path: path}
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		result.state = removalAbsent
		return result
	} else if err != nil {
		result.state = removalFailed
		result.err = err
		return result
	}
	if err := os.RemoveAll(path); err != nil {
		result.state = removalFailed
		result.err = err
		return result
	}
	result.state = removalRemoved
	return result
}

func removeExecutable(path string, expected fs.FileInfo) removalResult {
	result := removalResult{path: path}
	current, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		result.state = removalAbsent
		return result
	}
	if err != nil {
		result.state = removalFailed
		result.err = err
		return result
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.Mode().IsRegular() || !os.SameFile(expected, current) {
		result.state = removalFailed
		result.err = errors.New("executable changed after confirmation; refusing to remove it")
		return result
	}
	if err := os.Remove(path); err != nil {
		result.state = removalFailed
		result.err = err
		return result
	}
	result.state = removalRemoved
	return result
}

func printRemovalSummary(output io.Writer, results []removalResult) error {
	if _, err := io.WriteString(output, "\nRemoval summary:\n"); err != nil {
		return err
	}
	for _, result := range results {
		var line string
		switch result.state {
		case removalRemoved:
			line = fmt.Sprintf("  Removed:        %s\n", displayPath(result.path))
		case removalAbsent:
			line = fmt.Sprintf("  Already absent: %s\n", displayPath(result.path))
		case removalKept:
			line = fmt.Sprintf("  Needs root:     %s  ->  sudo hso uninstall\n", displayPath(result.path))
		default:
			line = fmt.Sprintf("  Failed:         %s: %v\n", displayPath(result.path), result.err)
		}
		if _, err := io.WriteString(output, line); err != nil {
			return err
		}
	}
	return nil
}

func uninstallPermissionError(path string) error {
	return fmt.Errorf("cannot remove %s: permission denied.\n\n"+
		"hso is installed in a system directory, so removing it requires root.\n"+
		"Run it again with sudo:\n\n"+
		"    sudo hso uninstall\n\n"+
		"If sudo is not available on this machine, run it as root", path)
}

func displayPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	prefix := home + string(filepath.Separator)
	if strings.HasPrefix(path, prefix) {
		return "~" + string(filepath.Separator) + strings.TrimPrefix(path, prefix)
	}
	return path
}
