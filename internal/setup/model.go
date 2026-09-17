package setup

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hijoushoku7/hijo-server-ops/internal/config"
	"github.com/hijoushoku7/hijo-server-ops/internal/javaenv"
	"github.com/hijoushoku7/hijo-server-ops/internal/mcversions"
	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
	"github.com/hijoushoku7/hijo-server-ops/internal/registry"
)

const (
	maxInputRunes = 512
	listViewport  = 10
)

type step uint8

const (
	stepWorkDir step = iota
	stepName
	stepCommand
	stepCommandInput
	stepInstallKind
	stepInstallVersion
	stepInstallVersionInput
	stepInstallLoader
	stepInstallConfirm
	stepInstalling
	stepInstallJava
	stepConfirm
	stepRegisterNotice
)

type model struct {
	configPath    string
	configDir     string
	step          step
	input         []rune
	workDir       string
	name          string
	servers       registry.Registry
	candidates    []candidate
	cursor        int
	command       string // 設定に書く形の起動スクリプト
	commandAbs    string // 権限確認と chmod に使う絶対パス
	fromInput     bool   // 起動スクリプトを一覧ではなく手入力で決めたか
	needsChmod    bool   // 起動スクリプトに実行権限がないか
	grantChmod    bool   // 実行権限を付けてよいという同意
	message       string
	created       bool
	canceled      bool
	err           error
	register      bool
	client        *mcversions.Client
	httpClient    *http.Client
	cacheDir      string
	installKind   string
	versions      []mcversions.Version
	loaders       []mcversions.Loader
	minecraft     string
	loader        string
	showSnapshots bool
	fetchedAt     time.Time
	downloaded    atomic.Int64
	downloadTotal atomic.Int64
	java          string
	installedJava func(string) ([]javaenv.Installation, error)
	installations []javaenv.Installation
	javaHome      string
	cancelInstall context.CancelFunc
	outputMu      sync.Mutex
	installOutput []string
}

type versionsMsg struct {
	kind      string
	versions  []mcversions.Version
	fetchedAt time.Time
	err       error
}
type loadersMsg struct {
	kind      string
	minecraft string
	loaders   []mcversions.Loader
	fallback  []mcversions.Loader
	fetchedAt time.Time
	err       error
}
type installedMsg struct{ err error }
type tickMsg time.Time

func newRegisterModel(configPath string, cfg config.Config, servers registry.Registry) *model {
	return &model{
		configPath: configPath,
		configDir:  filepath.Dir(configPath),
		step:       stepRegisterNotice,
		workDir:    cfg.Server.WorkDir,
		command:    cfg.Server.Command,
		servers:    servers,
		register:   true,
	}
}

func newModel(configPath string, servers registry.Registry) *model {
	return newModelWithVersion(configPath, servers, "dev", filepath.Dir(configPath))
}

func newModelWithVersion(configPath string, servers registry.Registry, version, cacheDir string) *model {
	configDir := filepath.Dir(configPath)
	// 設定ファイルの置き場所をそのままサーバーディレクトリの初期値にする。
	// 大半のケースで同じディレクトリになる。
	return &model{
		configPath:    configPath,
		configDir:     configDir,
		input:         []rune(configDir),
		servers:       servers,
		httpClient:    http.DefaultClient,
		client:        mcversions.NewClient(http.DefaultClient, version),
		cacheDir:      cacheDir,
		java:          "java",
		installedJava: javaenv.Installed,
	}
}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case versionsMsg:
		return m.receiveVersions(message)
	case loadersMsg:
		return m.receiveLoaders(message)
	case installedMsg:
		m.cancelInstall = nil
		if message.err != nil {
			m.message = message.err.Error()
			m.step = stepInstallConfirm
			return m, nil
		}
		m.command = "./run.sh"
		m.commandAbs = filepath.Join(m.workDir, "run.sh")
		m.needsChmod = false
		m.fromInput = false
		installations, err := m.installedJava("/usr/lib/jvm")
		if err != nil || len(installations) == 0 {
			m.step = stepConfirm
			return m, nil
		}
		m.installations = installations
		m.cursor = 0
		m.step = stepInstallJava
		return m, nil
	case tickMsg:
		if m.step == stepInstalling {
			return m, installTick()
		}
		return m, nil
	}
	key, ok := message.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if key.String() == "ctrl+c" {
		// 走っているインストーラを道連れにする。親が終わっても子は残るため。
		if m.cancelInstall != nil {
			m.cancelInstall()
		}
		m.canceled = true
		return m, tea.Quit
	}

	m.message = ""
	switch m.step {
	case stepRegisterNotice:
		return m.updateRegisterNotice(key.Key())
	case stepWorkDir:
		return m.updateWorkDir(key.Key())
	case stepName:
		return m.updateName(key.Key())
	case stepCommand:
		return m.updateCommand(key.Key())
	case stepCommandInput:
		return m.updateCommandInput(key.Key())
	case stepInstallKind:
		return m.updateInstallKind(key.Key())
	case stepInstallVersion:
		return m.updateInstallVersion(key.Key())
	case stepInstallVersionInput:
		return m.updateInstallVersionInput(key.Key())
	case stepInstallLoader:
		return m.updateInstallLoader(key.Key())
	case stepInstallConfirm:
		return m.updateInstallConfirm(key.Key())
	case stepInstalling:
		return m, nil
	case stepInstallJava:
		return m.updateInstallJava(key.Key())
	default:
		return m.updateConfirm(key.Key())
	}
}

func (m *model) updateRegisterNotice(key tea.Key) (tea.Model, tea.Cmd) {
	switch key.Code {
	case tea.KeyEscape:
		return m, tea.Quit
	case tea.KeyEnter, tea.KeyKpEnter:
		m.input = []rune(defaultServerName(m.workDir))
		m.step = stepName
	}
	return m, nil
}

func (m *model) updateWorkDir(key tea.Key) (tea.Model, tea.Cmd) {
	switch key.Code {
	case tea.KeyEscape:
		return m, tea.Quit
	case tea.KeyEnter, tea.KeyKpEnter:
		workDir, err := resolveWorkDir(string(m.input))
		if err != nil {
			m.message = err.Error()
			return m, nil
		}
		m.workDir = workDir
		m.input = []rune(defaultServerName(workDir))
		m.step = stepName
	default:
		m.editInput(key)
	}
	return m, nil
}

func (m *model) updateName(key tea.Key) (tea.Model, tea.Cmd) {
	switch key.Code {
	case tea.KeyEscape:
		if m.register {
			m.step = stepRegisterNotice
			return m, nil
		}
		m.step = stepWorkDir
		m.input = []rune(m.workDir)
	case tea.KeyEnter, tea.KeyKpEnter:
		name := string(m.input)
		if err := registry.ValidateName(name); err != nil {
			m.message = err.Error()
			return m, nil
		}
		if _, found := m.servers.Find(name); found {
			m.message = msg.DuplicateServerName(name).Error()
			return m, nil
		}
		m.name = name
		if m.register {
			m.created = true
			return m, tea.Quit
		}
		m.candidates = scanCommands(m.workDir)
		m.cursor = 0
		m.step = stepCommand
		if len(m.candidates) == 0 {
			m.message = msg.SetupNoCandidates
		}
	default:
		m.editInput(key)
	}
	return m, nil
}

func (m *model) updateCommand(key tea.Key) (tea.Model, tea.Cmd) {
	count := len(m.candidates) + 2 // 末尾は手入力と新規インストール
	switch key.Code {
	case tea.KeyEscape:
		m.step = stepName
		m.input = []rune(m.name)
	case tea.KeyEnter, tea.KeyKpEnter:
		if m.cursor == len(m.candidates) {
			m.step = stepCommandInput
			m.input = []rune("./")
			return m, nil
		}
		if m.cursor == len(m.candidates)+1 {
			m.cursor = 0
			m.step = stepInstallKind
			return m, nil
		}
		m.selectCommand(m.candidates[m.cursor].name, false)
	default:
		m.cursor = moveCursor(key, m.cursor, count)
	}
	return m, nil
}

func (m *model) updateCommandInput(key tea.Key) (tea.Model, tea.Cmd) {
	switch key.Code {
	case tea.KeyEscape:
		m.step = stepCommand
	case tea.KeyEnter, tea.KeyKpEnter:
		m.selectCommand(string(m.input), true)
	default:
		m.editInput(key)
	}
	return m, nil
}

func (m *model) updateInstallKind(key tea.Key) (tea.Model, tea.Cmd) {
	kinds := []string{"vanilla", "fabric", "paper", "forge", "neoforge"}
	switch key.Code {
	case tea.KeyEscape:
		m.step = stepCommand
	case tea.KeyEnter, tea.KeyKpEnter:
		m.installKind = kinds[m.cursor]
		m.cursor = 0
		m.showSnapshots = false
		m.versions = nil
		m.loaders = nil
		m.fetchedAt = time.Time{}
		m.step = stepInstallVersion
		return m, m.loadVersions()
	default:
		m.cursor = moveCursor(key, m.cursor, len(kinds))
	}
	return m, nil
}

func (m *model) loadVersions() tea.Cmd {
	kind, client, dir := m.installKind, m.client, m.cacheDir
	return func() tea.Msg {
		cache, _ := mcversions.ReadCache(dir)
		var versions []mcversions.Version
		switch kind {
		case "vanilla":
			versions = cache.Vanilla
		case "fabric":
			versions = cache.Fabric
		case "paper":
			versions = cache.Paper
		case "forge":
			versions = cache.Forge
		case "neoforge":
			versions = cache.NeoForge
		}
		if cache.Fresh(time.Now()) && len(versions) != 0 {
			return versionsMsg{kind: kind, versions: versions, fetchedAt: cache.FetchedAt}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var err error
		switch kind {
		case "vanilla":
			versions, err = client.Vanilla(ctx)
		case "fabric":
			versions, err = client.Fabric(ctx)
		case "paper":
			versions, err = client.Paper(ctx)
		case "forge":
			versions, err = client.Forge(ctx)
		case "neoforge":
			versions, err = client.NeoForge(ctx)
		}
		if err != nil {
			return versionsMsg{kind: kind, err: err}
		}
		// 取得時刻は種別ごとに分けていないので、取り直した種別だけを残す。
		// 他の種別を抱えたままにすると、取得時刻の更新でまとめて延命されてしまう。
		cache = mcversions.Cache{FetchedAt: time.Now(), FabricLoader: cache.FabricLoader}
		switch kind {
		case "vanilla":
			cache.Vanilla = versions
		case "fabric":
			cache.Fabric = versions
		case "paper":
			cache.Paper = versions
		case "forge":
			cache.Forge = versions
		case "neoforge":
			cache.NeoForge = versions
		}
		_ = mcversions.WriteCache(dir, cache)
		return versionsMsg{kind: kind, versions: versions, fetchedAt: cache.FetchedAt}
	}
}

func (m *model) receiveVersions(message versionsMsg) (tea.Model, tea.Cmd) {
	// 取得中に別の種別へ移っていたら、古い応答は捨てる。
	if message.kind != m.installKind || m.step != stepInstallVersion {
		return m, nil
	}
	if message.err != nil || len(message.versions) == 0 {
		if m.installKind == "forge" || m.installKind == "neoforge" {
			// 版とローダーが対で決まるので、版だけ手入力しても選べない。種別選択へ戻す。
			m.step, m.cursor = stepInstallKind, 0
			m.message = msg.SetupVersionsUnavailable
			return m, nil
		}
		m.step = stepInstallVersionInput
		m.input = nil
		return m, nil
	}
	m.versions, m.fetchedAt, m.cursor = message.versions, message.fetchedAt, 0
	return m, nil
}

func (m *model) visibleVersions() []mcversions.Version {
	if m.showSnapshots {
		return m.versions
	}
	versions := make([]mcversions.Version, 0, len(m.versions))
	for _, version := range m.versions {
		if version.Stable {
			versions = append(versions, version)
		}
	}
	return versions
}

func (m *model) updateInstallVersion(key tea.Key) (tea.Model, tea.Cmd) {
	// 取得中でも Esc だけは効かせる。応答が返らない間に詰まらせないため。
	if key.Code == tea.KeyEscape {
		m.step, m.cursor = stepInstallKind, 0
		return m, nil
	}
	if m.versions == nil {
		return m, nil
	}
	versions := m.visibleVersions()
	if key.Text == "s" {
		m.showSnapshots = !m.showSnapshots
		m.cursor = 0
		return m, nil
	}
	switch key.Code {
	case tea.KeyEnter, tea.KeyKpEnter:
		if len(versions) == 0 {
			return m, nil
		}
		selected := versions[m.cursor]
		m.minecraft = selected.Version
		if m.installKind != "fabric" && m.installKind != "forge" && m.installKind != "neoforge" {
			m.step = stepInstallConfirm
			return m, nil
		}
		m.step, m.cursor = stepInstallLoader, 0
		if m.installKind == "forge" || m.installKind == "neoforge" {
			m.loaders = selected.Loaders
			return m, nil
		}
		return m, m.loadLoaders()
	default:
		if len(versions) != 0 {
			m.cursor = moveCursor(key, m.cursor, len(versions))
		}
	}
	return m, nil
}

func (m *model) updateInstallVersionInput(key tea.Key) (tea.Model, tea.Cmd) {
	switch key.Code {
	case tea.KeyEscape:
		m.step, m.cursor = stepInstallKind, 0
	case tea.KeyEnter, tea.KeyKpEnter:
		if len(m.input) == 0 {
			return m, nil
		}
		m.minecraft = string(m.input)
		if m.installKind == "fabric" {
			m.step = stepInstallLoader
			m.cursor = 0
			return m, m.loadLoaders()
		}
		m.step = stepInstallConfirm
	default:
		m.editInput(key)
	}
	return m, nil
}

func (m *model) loadLoaders() tea.Cmd {
	client, dir, minecraft := m.client, m.cacheDir, m.minecraft
	return func() tea.Msg {
		cache, _ := mcversions.ReadCache(dir)
		if cache.FabricLoader != nil && cache.FabricLoader.Fresh(time.Now()) &&
			cache.FabricLoader.Minecraft == minecraft && len(cache.FabricLoader.Loaders) != 0 {
			return loadersMsg{
				kind: "fabric", minecraft: minecraft,
				loaders: cache.FabricLoader.Loaders, fetchedAt: cache.FabricLoader.FetchedAt,
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		loaders, err := client.FabricLoaders(ctx, minecraft)
		if err != nil {
			return loadersMsg{kind: "fabric", minecraft: minecraft, err: err}
		}
		cache.FabricLoader = &mcversions.FabricLoaderCache{
			Minecraft: minecraft, FetchedAt: time.Now(), Loaders: loaders,
		}
		_ = mcversions.WriteCache(dir, cache)
		return loadersMsg{
			kind: "fabric", minecraft: minecraft,
			loaders: loaders, fetchedAt: cache.FabricLoader.FetchedAt,
		}
	}
}

func (m *model) receiveLoaders(message loadersMsg) (tea.Model, tea.Cmd) {
	// 取得中に選び直していたら、古い応答は捨てる。
	if message.kind != m.installKind || message.minecraft != m.minecraft || m.step != stepInstallLoader {
		return m, nil
	}
	if message.err != nil || len(message.loaders) == 0 {
		if len(message.fallback) != 0 {
			m.loaders, m.cursor = message.fallback, 0
			return m, nil
		}
		// Loader を選べないので版の一覧へ黙って戻し、選び直せるようにする。
		m.step = stepInstallVersion
		m.cursor = 0
		return m, nil
	}
	m.loaders, m.fetchedAt, m.cursor = message.loaders, message.fetchedAt, 0
	return m, nil
}

func (m *model) updateInstallLoader(key tea.Key) (tea.Model, tea.Cmd) {
	if key.Code == tea.KeyEscape {
		m.step, m.cursor = stepInstallVersion, 0
		return m, nil
	}
	if m.loaders == nil {
		return m, nil
	}
	if m.installKind == "forge" && key.Text == "a" {
		fallback := m.loaders
		m.loaders = nil
		m.cursor = 0
		client, minecraft := m.client, m.minecraft
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			loaders, err := client.ForgeLoaders(ctx, minecraft)
			return loadersMsg{
				kind: "forge", minecraft: minecraft,
				loaders: loaders, fallback: fallback, err: err,
			}
		}
	}
	switch key.Code {
	case tea.KeyEnter, tea.KeyKpEnter:
		if len(m.loaders) != 0 {
			m.loader = m.loaders[m.cursor].Version
			m.step = stepInstallConfirm
		}
	default:
		if len(m.loaders) != 0 {
			m.cursor = moveCursor(key, m.cursor, len(m.loaders))
		}
	}
	return m, nil
}

func (m *model) updateInstallConfirm(key tea.Key) (tea.Model, tea.Cmd) {
	if key.Code == tea.KeyEscape {
		if m.installKind == "fabric" || m.installKind == "forge" || m.installKind == "neoforge" {
			m.step = stepInstallLoader
		} else {
			m.step = stepInstallVersion
		}
		m.cursor = 0
		return m, nil
	}
	if key.Code != tea.KeyEnter && key.Code != tea.KeyKpEnter {
		return m, nil
	}
	names := []string{"server.jar", "run.sh", "eula.txt"}
	if m.installKind == "forge" || m.installKind == "neoforge" {
		names = []string{"run.sh", "eula.txt"}
	}
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(m.workDir, name)); err == nil {
			m.message = msg.SetupInstallFileExists(name)
			return m, nil
		}
	}
	m.downloaded.Store(0)
	m.downloadTotal.Store(0)
	m.outputMu.Lock()
	m.installOutput = nil
	m.outputMu.Unlock()
	m.step = stepInstalling
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelInstall = cancel
	return m, tea.Batch(m.install(ctx), installTick())
}

func (m *model) install(ctx context.Context) tea.Cmd {
	client, httpClient, kind, minecraft, loader, dir := m.client, m.httpClient, m.installKind, m.minecraft, m.loader, m.workDir
	return func() tea.Msg {
		var jar mcversions.ServerJar
		var err error
		switch kind {
		case "vanilla":
			jar, err = client.VanillaJar(ctx, minecraft)
		case "fabric":
			jar, err = client.FabricJar(ctx, minecraft, loader)
		case "paper":
			jar, err = client.PaperJar(ctx, minecraft)
		case "forge":
			jar, err = client.ForgeInstaller(ctx, minecraft, loader)
		case "neoforge":
			jar, err = client.NeoForgeInstaller(ctx, loader)
		}
		if err != nil {
			return installedMsg{err: err}
		}
		jarPath := filepath.Join(dir, "server.jar")
		installer := kind == "forge" || kind == "neoforge"
		if installer {
			jarPath = filepath.Join(dir, ".hso-installer.jar")
		}
		if err = mcversions.Download(ctx, httpClient, jar, jarPath, func(done, total int64) { m.downloaded.Store(done); m.downloadTotal.Store(total) }); err != nil {
			return installedMsg{err: err}
		}
		created := []string{jarPath}
		defer func() {
			if err != nil {
				for _, path := range created {
					_ = os.Remove(path)
				}
			}
		}()
		if installer {
			if err = m.runInstaller(ctx, jarPath, dir); err != nil {
				return installedMsg{err: m.installerError(err)}
			}
			if err = os.Remove(jarPath); err != nil {
				return installedMsg{err: err}
			}
			created = created[1:]
			if err = m.prepareInstallerRunScript(dir, minecraft, loader); err != nil {
				return installedMsg{err: err}
			}
		}
		if err = writeExclusive(filepath.Join(dir, "eula.txt"), []byte("eula=true\n"), 0o644); err != nil {
			return installedMsg{err: err}
		}
		created = append(created, filepath.Join(dir, "eula.txt"))
		if !installer {
			script := "#!/bin/sh\n# hso が生成した起動スクリプト。JVM 引数はここで調整する。\nexec java -Xmx2G -jar server.jar nogui\n"
			if err = writeExclusive(filepath.Join(dir, "run.sh"), []byte(script), 0o755); err != nil {
				return installedMsg{err: err}
			}
		}
		return installedMsg{}
	}
}

func (m *model) runInstaller(ctx context.Context, jarPath, dir string) error {
	command := exec.CommandContext(ctx, m.java, "-jar", filepath.Base(jarPath), "--installServer")
	command.Dir = dir
	output, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	command.Stderr = command.Stdout
	if err := command.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		m.appendInstallOutput(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		_ = command.Wait()
		return err
	}
	return command.Wait()
}

func (m *model) appendInstallOutput(line string) {
	m.outputMu.Lock()
	defer m.outputMu.Unlock()
	m.installOutput = append(m.installOutput, line)
	if len(m.installOutput) > 10 {
		m.installOutput = m.installOutput[len(m.installOutput)-10:]
	}
}

func (m *model) installerOutputLines() []string {
	m.outputMu.Lock()
	defer m.outputMu.Unlock()
	return append([]string(nil), m.installOutput...)
}

func (m *model) installerError(err error) error {
	lines := m.installerOutputLines()
	if len(lines) == 0 {
		return err
	}
	return fmt.Errorf("%w\n%s", err, strings.Join(lines, "\n"))
}

func (m *model) prepareInstallerRunScript(dir, minecraft, loader string) error {
	runPath := filepath.Join(dir, "run.sh")
	if info, err := os.Stat(runPath); err == nil {
		if info.Mode().Perm()&0o111 == 0 {
			return grantExecute(runPath)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	matches, err := filepath.Glob(filepath.Join(dir, "forge-"+minecraft+"-"+loader+"*.jar"))
	if err != nil {
		return err
	}
	for _, match := range matches {
		if strings.HasSuffix(match, "-installer.jar") {
			continue
		}
		script := "#!/bin/sh\n# hso が生成した起動スクリプト。JVM 引数はここで調整する。\nexec java -Xmx2G -jar " + filepath.Base(match) + " nogui\n"
		return writeExclusive(runPath, []byte(script), 0o755)
	}
	return fmt.Errorf("forge server jar not found: %s-%s", minecraft, loader)
}

func writeExclusive(path string, content []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err = file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err = file.Chmod(mode); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	return file.Close()
}

func installTick() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *model) installDescription() string {
	if m.loader == "" {
		return fmt.Sprintf("%s %s", m.installKind, m.minecraft)
	}
	return fmt.Sprintf("%s %s / %s", m.installKind, m.minecraft, m.loader)
}

func (m *model) selectCommand(input string, fromInput bool) {
	command, path, err := resolveCommand(input, m.workDir)
	if err != nil {
		m.message = err.Error()
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		m.message = err.Error()
		return
	}
	m.command = command
	m.commandAbs = path
	m.fromInput = fromInput
	m.needsChmod = info.Mode().Perm()&0o111 == 0
	m.installations = nil
	m.javaHome = ""
	// 実行権限がなければ hso は起動できないので、付ける側を初期値にする。
	// c で断れる。
	m.grantChmod = m.needsChmod
	m.step = stepConfirm
}

func (m *model) updateInstallJava(key tea.Key) (tea.Model, tea.Cmd) {
	count := len(m.installations) + 1
	// インストールは済んでいるので戻れる先がない。Esc は受けない。
	switch key.Code {
	case tea.KeyEnter, tea.KeyKpEnter:
		m.javaHome = ""
		if m.cursor > 0 {
			m.javaHome = m.installations[m.cursor-1].Home
		}
		m.step = stepConfirm
	default:
		m.cursor = moveCursor(key, m.cursor, count)
	}
	return m, nil
}

func (m *model) updateConfirm(key tea.Key) (tea.Model, tea.Cmd) {
	if m.needsChmod && key.Text == "c" {
		m.grantChmod = !m.grantChmod
		return m, nil
	}
	switch key.Code {
	case tea.KeyEscape:
		if len(m.installations) != 0 {
			m.step = stepInstallJava
			break
		}
		// 直前にいた画面へ戻す。
		if m.fromInput {
			m.step = stepCommandInput
			break
		}
		m.step = stepCommand
	case tea.KeyEnter, tea.KeyKpEnter:
		if m.needsChmod && m.grantChmod {
			if err := grantExecute(m.commandAbs); err != nil {
				m.message = err.Error()
				return m, nil
			}
			m.needsChmod = false
		}
		if err := writeConfig(m.configPath, m.preview()); err != nil {
			m.err = err
			return m, tea.Quit
		}
		m.created = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *model) editInput(key tea.Key) {
	switch key.Code {
	case tea.KeyBackspace:
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	default:
		if key.Text == "" || len(m.input)+len([]rune(key.Text)) > maxInputRunes {
			return
		}
		m.input = append(m.input, []rune(key.Text)...)
	}
}

func moveCursor(key tea.Key, cursor, count int) int {
	if len(key.Text) == 1 && key.Text[0] >= '1' && key.Text[0] <= '9' {
		index := int(key.Text[0] - '1')
		if index < count {
			cursor = index
		}
	}
	switch key.Code {
	case tea.KeyUp:
		cursor--
	case tea.KeyDown:
		cursor++
	case tea.KeyHome:
		cursor = 0
	case tea.KeyEnd:
		cursor = count - 1
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor > count-1 {
		cursor = count - 1
	}
	return cursor
}

func (m *model) preview() string {
	if m.register {
		return render(m.command, m.workDir, "", "", "")
	}
	return render(m.command, m.workDir, m.configDir, m.javaHome,
		serverVersion(m.installKind, m.minecraft, m.loader))
}

func defaultServerName(workDir string) string {
	name := filepath.Base(filepath.Clean(workDir))
	if registry.ValidateName(name) != nil {
		return ""
	}
	return name
}
