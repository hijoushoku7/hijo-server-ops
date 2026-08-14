package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func preserveUninstallGlobals(t *testing.T) {
	t.Helper()
	previousExecutablePath := executablePath
	previousEUID := uninstallEUID
	t.Cleanup(func() {
		executablePath = previousExecutablePath
		uninstallEUID = previousEUID
	})
}

func TestParseUninstallOptions(t *testing.T) {
	for _, test := range []struct {
		name  string
		args  []string
		purge bool
		yes   bool
	}{
		{name: "default"},
		{name: "purge", args: []string{"--purge"}, purge: true},
		{name: "short yes", args: []string{"-y"}, yes: true},
		{name: "long yes", args: []string{"--yes"}, yes: true},
		{name: "combined", args: []string{"--purge", "-y"}, purge: true, yes: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseUninstallOptions(test.args)
			if err != nil {
				t.Fatal(err)
			}
			if got.purge != test.purge || got.yes != test.yes {
				t.Fatalf("options = %#v", got)
			}
		})
	}

	for _, args := range [][]string{{"--unknown"}, {"extra"}} {
		if _, err := parseUninstallOptions(args); err == nil {
			t.Errorf("args=%q がエラーにならなかった", args)
		}
	}
}

func TestValidateUninstallRootRejectsOnlyPurge(t *testing.T) {
	if err := validateUninstallRoot(false, 0); err != nil {
		t.Fatalf("root の通常アンインストール: %v", err)
	}
	err := validateUninstallRoot(true, 0)
	if err == nil {
		t.Fatal("root の --purge が通った")
	}
	for _, want := range []string{"must not be run as root", "hso uninstall --purge"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q に %q がない", err, want)
		}
	}
	if err := validateUninstallRoot(true, 1000); err != nil {
		t.Fatalf("通常ユーザーの --purge: %v", err)
	}
}

func TestRunUninstallRejectsRootPurgeBeforeResolvingPaths(t *testing.T) {
	preserveUninstallGlobals(t)
	uninstallEUID = func() int { return 0 }
	executablePath = func() (string, error) {
		t.Fatal("root の --purge で実行ファイルのパスを調べた")
		return "", nil
	}

	err := runUninstall([]string{"--purge", "-y"}, strings.NewReader(""), &bytes.Buffer{}, false)
	if err == nil || !strings.Contains(err.Error(), "must not be run as root") {
		t.Fatalf("err = %v", err)
	}
}

func TestTargetDirectoryAccessChecksParentDirectory(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "read-only-binary")
	if err := os.WriteFile(target, []byte("hso"), 0o400); err != nil {
		t.Fatal(err)
	}
	if err := targetDirectoryAccess(target); err != nil {
		t.Fatalf("ファイル自身が読み取り専用でも親を書ければ削除可能: %v", err)
	}

	if os.Geteuid() == 0 {
		t.Skip("root では access(2) の権限不足を再現できない")
	}
	if err := os.Chmod(directory, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(directory, 0o700) })
	if err := targetDirectoryAccess(target); !errors.Is(err, unix.EACCES) {
		t.Fatalf("親ディレクトリを書けないときの error = %v", err)
	}
}

func TestPrepareUninstallStopsAtPermissionCheckBeforeUserPaths(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root では access(2) の権限不足を再現できない")
	}
	preserveUninstallGlobals(t)
	directory := t.TempDir()
	executable := filepath.Join(directory, "hso")
	if err := os.WriteFile(executable, []byte("hso"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(directory, 0o700) })
	executablePath = func() (string, error) { return executable, nil }
	uninstallEUID = func() int { return 1000 }
	// 相対パスなら registry.Path はエラーになる。そこへ進まず、まず
	// バイナリの権限不足だけを返す計画になることを確かめる。
	t.Setenv("XDG_CONFIG_HOME", "relative-config-home")

	plan, err := prepareUninstall(uninstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.binaryWritable {
		t.Fatal("書き込めない親ディレクトリを削除可能と判定した")
	}
	if plan.configFile != "" || plan.pidDir != "" {
		t.Fatalf("権限エラーの前にユーザーパスを解決した: %#v", plan)
	}
}

func TestInspectUninstallExecutableRejectsSymbolicLink(t *testing.T) {
	preserveUninstallGlobals(t)
	directory := t.TempDir()
	target := filepath.Join(directory, "hso-real")
	link := filepath.Join(directory, "hso")
	if err := os.WriteFile(target, []byte("hso"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	executablePath = func() (string, error) { return link, nil }

	_, _, err := inspectUninstallExecutable()
	if err == nil {
		t.Fatal("シンボリックリンクが削除対象になった")
	}
	for _, want := range []string{link, target, "Nothing was removed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q に %q がない", err, want)
		}
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("link が変更された: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("target が変更された: %v", err)
	}
}

func TestConfirmUninstallBranches(t *testing.T) {
	for _, test := range []struct {
		name     string
		input    string
		terminal bool
		yes      bool
		partial  bool
		want     bool
		wantErr  bool
		prompt   string
	}{
		{name: "yes flag", yes: true, want: true},
		{name: "non terminal", wantErr: true},
		{name: "confirmed", input: "y\n", terminal: true, want: true, prompt: "Remove? [y/N]:"},
		{name: "declined by default", input: "\n", terminal: true, prompt: "Remove? [y/N]:"},
		{name: "partial purge", input: "Y\n", terminal: true, partial: true, want: true, prompt: "Continue? [y/N]:"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			got, err := confirmUninstall(strings.NewReader(test.input), &output,
				test.terminal, test.yes, test.partial)
			if (err != nil) != test.wantErr {
				t.Fatalf("err = %v", err)
			}
			if got != test.want {
				t.Fatalf("confirmed = %t, want %t", got, test.want)
			}
			if test.prompt != "" && !strings.Contains(output.String(), test.prompt) {
				t.Errorf("output = %q に %q がない", output.String(), test.prompt)
			}
			if test.yes && output.Len() != 0 {
				t.Errorf("-y でプロンプトが出た: %q", output.String())
			}
		})
	}
}

func TestPrintUninstallPlanShowsEveryPathAndOneRunningWarning(t *testing.T) {
	root := t.TempDir()
	plan := uninstallPlan{
		executable:     filepath.Join(root, "bin", "hso"),
		configDir:      filepath.Join(root, "config", "hso"),
		pidDir:         filepath.Join(root, "run", "hso"),
		binaryWritable: false,
		running:        []string{"survival", "creative"},
		options:        uninstallOptions{purge: true},
	}
	var output bytes.Buffer
	if err := printUninstallPlan(&output, plan); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{plan.executable, plan.configDir, plan.pidDir} {
		if !strings.Contains(output.String(), path) {
			t.Errorf("output に %s がない: %s", path, &output)
		}
	}
	if got := strings.Count(output.String(), "servers are running right now"); got != 1 {
		t.Fatalf("実行中の注意の回数 = %d, output = %q", got, output.String())
	}
}

func TestRemoveExecutableRefusesChangedTarget(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "hso")
	if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	expected, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(directory, "replacement")
	if err := os.WriteFile(replacement, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}

	result := removeExecutable(path, expected)
	if result.state != removalFailed || !strings.Contains(result.err.Error(), "changed") {
		t.Fatalf("result = %#v", result)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "new" {
		t.Fatalf("置換後のファイルが変更された: %q", contents)
	}
}

func TestRemoveUninstallTargetsContinuesAfterFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root では削除権限エラーを再現できない")
	}
	lockedParent := t.TempDir()
	configDirectory := filepath.Join(lockedParent, "hso")
	pidDirectory := filepath.Join(t.TempDir(), "hso")
	executable := filepath.Join(t.TempDir(), "hso")
	for _, path := range []string{configDirectory, pidDirectory} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(executable, []byte("hso"), 0o755); err != nil {
		t.Fatal(err)
	}
	executableInfo, err := os.Lstat(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(lockedParent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(lockedParent, 0o700) })

	results := removeUninstallTargets(uninstallPlan{
		executable:     executable,
		executableInfo: executableInfo,
		configDir:      configDirectory,
		pidDir:         pidDirectory,
		binaryWritable: true,
		options:        uninstallOptions{purge: true},
	})
	if len(results) != 3 || results[0].state != removalFailed ||
		results[1].state != removalRemoved || results[2].state != removalRemoved {
		t.Fatalf("results = %#v", results)
	}
	for _, path := range []string{pidDirectory, executable} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("先の削除失敗後も消すべき %s が残った: %v", path, err)
		}
	}
}

func TestRunUninstallPurgesOnlyTemporaryHSOPaths(t *testing.T) {
	preserveUninstallGlobals(t)
	configHome := t.TempDir()
	runtimeHome := t.TempDir()
	installDirectory := t.TempDir()
	serverDirectory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_RUNTIME_DIR", runtimeHome)
	uninstallEUID = func() int { return 1000 }

	executable := filepath.Join(installDirectory, "hso")
	configDirectory := filepath.Join(configHome, "hso")
	configFile := filepath.Join(configDirectory, "config.toml")
	pidDirectory := filepath.Join(runtimeHome, "hso")
	serverConfig := filepath.Join(serverDirectory, "hso.toml")
	for path, contents := range map[string]string{
		executable:   "test binary",
		configFile:   "",
		serverConfig: "[server]\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(pidDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pidDirectory, "stale.pid"), []byte("1 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	executablePath = func() (string, error) { return executable, nil }

	var output bytes.Buffer
	if err := runUninstall([]string{"--purge", "-y"}, strings.NewReader(""), &output, false); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{executable, configDirectory, pidDirectory} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s が残った: %v", path, err)
		}
		if !strings.Contains(output.String(), path) {
			t.Errorf("output に削除対象 %s がない: %s", path, &output)
		}
	}
	if contents, err := os.ReadFile(serverConfig); err != nil || string(contents) != "[server]\n" {
		t.Fatalf("サーバーディレクトリが変更された: contents=%q, err=%v", contents, err)
	}
}

func TestRunUninstallContinuesWhenServerListCannotBeRead(t *testing.T) {
	for _, config := range []struct {
		name     string
		contents string
	}{
		{name: "malformed TOML", contents: "invalid toml"},
		{name: "unknown key", contents: "[[servers]]\nname = \"survival\"\nconfig = \"/tmp/hso.toml\"\nunknown_key = true\n"},
	} {
		for _, mode := range []struct {
			name  string
			args  []string
			purge bool
		}{
			{name: "default", args: []string{"-y"}},
			{name: "purge", args: []string{"--purge", "-y"}, purge: true},
		} {
			t.Run(config.name+"/"+mode.name, func(t *testing.T) {
				preserveUninstallGlobals(t)
				configHome := t.TempDir()
				runtimeHome := t.TempDir()
				installDirectory := t.TempDir()
				t.Setenv("XDG_CONFIG_HOME", configHome)
				t.Setenv("XDG_RUNTIME_DIR", runtimeHome)
				uninstallEUID = func() int { return 1000 }

				executable := filepath.Join(installDirectory, "hso")
				configDirectory := filepath.Join(configHome, "hso")
				configFile := filepath.Join(configDirectory, "config.toml")
				pidDirectory := filepath.Join(runtimeHome, "hso")
				if err := os.WriteFile(executable, []byte("test binary"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(configDirectory, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(configFile, []byte(config.contents), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(pidDirectory, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(pidDirectory, "stale.pid"), []byte("1 1\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				executablePath = func() (string, error) { return executable, nil }

				var output bytes.Buffer
				if err := runUninstall(mode.args, strings.NewReader(""), &output, false); err != nil {
					t.Fatalf("終了コード 1 相当のエラー: %v", err)
				}
				if _, err := os.Lstat(executable); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("binary が残った: %v", err)
				}
				warning := "Could not read server list at " + configFile + "; running servers were not checked."
				if !strings.Contains(output.String(), warning+"\n") {
					t.Fatalf("output = %q に警告 %q がない", output.String(), warning)
				}
				for _, path := range []string{configDirectory, pidDirectory} {
					_, err := os.Lstat(path)
					if mode.purge && !errors.Is(err, os.ErrNotExist) {
						t.Errorf("--purge 後も %s が残った: %v", path, err)
					}
					if !mode.purge && err != nil {
						t.Errorf("通常アンインストールで %s が変更された: %v", path, err)
					}
				}
			})
		}
	}
}

func TestRunUninstallDefaultKeepsTemporaryServerList(t *testing.T) {
	preserveUninstallGlobals(t)
	configHome := t.TempDir()
	installDirectory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	uninstallEUID = func() int { return 1000 }

	executable := filepath.Join(installDirectory, "hso")
	configFile := filepath.Join(configHome, "hso", "config.toml")
	if err := os.WriteFile(executable, []byte("test binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(configFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configFile, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	executablePath = func() (string, error) { return executable, nil }

	var output bytes.Buffer
	if err := runUninstall([]string{"--yes"}, strings.NewReader(""), &output, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(executable); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("binary が残った: %v", err)
	}
	if _, err := os.Stat(configFile); err != nil {
		t.Fatalf("server list が消えた: %v", err)
	}
	want := "Server list kept at " + configFile + " (use --purge to remove it)."
	if !strings.Contains(output.String(), want) {
		t.Fatalf("output = %q に %q がない", output.String(), want)
	}
}
