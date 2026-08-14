//go:build en

// Package msg collects every string shown to the user. Japanese is the
// default build; the English build is produced with `-tags en`. The ja and en
// files declare the same identifiers, so a missing translation fails to
// compile.
//
// Only strings a normal user reads belong here: the settings modal, the setup
// wizard, config validation errors. Developer-facing diagnostics such as
// hsperfdata or /proc parse errors stay as English literals in their own
// packages, identical in both builds.
//
// Anything with an embedded value is a function; a const format string would
// let the verbs drift between ja and en unnoticed. Anything wrapped with %w
// returns an error.
package msg

import (
	"errors"
	"fmt"
)

// Settings modal section headings.
const (
	SectionPreferences = "Preferences"
	SectionAdvanced    = "Advanced"
)

// Settings modal item names.
const (
	LabelFrame     = "border"
	LabelGraphLine = "graph line"
	LabelMeterBar  = "meter bar"
	LabelTitle     = "title"
	LabelSelection = "selection"
	LabelLog       = "log"

	LabelAutoRestart = "auto restart"
	LabelTimezone    = "time zone"
	LabelCurrentTime = "current time"
	LabelTimeDrift   = "offset"
)

const (
	OptSystemTime     = "system time"
	TimeSettingButton = "set"
	TimeModalTitle    = "Set current time"
)

// Color preset names.
const (
	OptDefault = "default"
	OptMono    = "mono"
	OptNeon    = "neon"
	OptOcean   = "ocean"
	OptForest  = "forest"
	OptWarm    = "warm"
	OptCool    = "cool"
	OptSafe    = "simple"
	OptSignal  = "signal"
	OptFlat    = "flat"
	OptCyan    = "cyan"
	OptWhite   = "white"
	OptAmber   = "amber"
	OptViolet  = "violet"
	OptQuiet   = "quiet"
)

// On/off names.
const (
	OptOn  = "on"
	OptOff = "off"
)

// Status line.
const StatusIdle = "idle"

func SaveSettingsFailed(err error) string {
	return "failed to save settings: " + err.Error()
}

func ActionFailed(err error) string {
	return "operation failed: " + err.Error()
}

// Server exit modal and stopped log screen.
const (
	ExitTitleCrashed  = "Server crashed"
	ExitTitleStopped  = "Server stopped"
	ExitErrorLines    = "Error lines"
	ExitStateCrashed  = "The server crashed. Read the log or restart it."
	ExitStateStopped  = "The server stopped normally."
	ExitButtonLogs    = "Read logs"
	ExitButtonRestart = "Restart"
	ExitButtonQuit    = "Quit"

	ExitAutoRestartHint         = "Esc: stop auto restart"
	ExitAutoRestartCanceled     = "Auto restart stopped."
	ExitAutoRestartStopped      = "Auto restart gave up: the server keeps dying at startup."
	ExitAutoRestartRejected     = "Could not request the auto restart."
	ExitAutoRestartSkipped      = "No auto restart: the start script never started java."
	ExitAutoRestartFatal        = "No auto restart: hso itself failed."
	ExitAutoRestartJavaMismatch = "No auto restart: the Java version does not match."
	ExitAutoRestartDone         = "Auto restart done. The server is running."
	ExitAutoRestartDoneHint     = "Enter: close"
)

func JavaVersionMismatch(required, actual int) string {
	if actual == 0 {
		return fmt.Sprintf("This server requires Java %d, but the current Java version could not be determined.", required)
	}
	return fmt.Sprintf("This server requires Java %d, but it was started with Java %d.", required, actual)
}

func JavaVersionChange(required int) string {
	return fmt.Sprintf("Run hso java change and switch to Java %d.", required)
}

func JavaVersionInstall(required int) string {
	return fmt.Sprintf("Install Java %d, then run hso java change to switch to it.", required)
}

func StoppedLogTitle(code string) string {
	return "Log · stopped (exit " + code + ")"
}

func ExitSummary(code, exitedAt, uptime string) string {
	return fmt.Sprintf("exit %s · stopped %s · uptime %s", code, exitedAt, uptime)
}

func ExitMemory(rss, heapUsed, heapCommitted, delta string) string {
	return fmt.Sprintf(
		"final memory  RSS %s · heap %s/%s · Δ %s",
		rss, heapUsed, heapCommitted, delta,
	)
}

func ExitGC(collections uint64, available bool, last string) string {
	count := "n/a"
	if available {
		count = fmt.Sprintf("%d collections", collections)
	}
	return fmt.Sprintf("GC  %s · last pause %s", count, last)
}

func ExitStateRestarting(dots string) string {
	return "restarting" + dots
}

func ExitAutoRestartIn(seconds int) string {
	return fmt.Sprintf("restarting automatically in %d seconds", seconds)
}

func ExitAutoQuit(seconds int) string {
	return fmt.Sprintf("hso exits in %d seconds (press a key to stay)", seconds)
}

// Key bar (the hint line at the bottom of the screen).
const (
	BarItem         = "item"
	BarValue        = "value"
	BarClose        = "close"
	BarExit         = "exit"
	BarSelectPanel  = "select"
	BarFocus        = "focus"
	BarSettings     = "settings"
	BarBackToSelect = "select"
	BarConsoleTab   = "input/restart/stop"
	BarExecute      = "execute"
	BarBack         = "back"
	BarCommand      = "command"
	BarPutInConsole = "put in console"
	BarPlayer       = "player"
	BarCommandList  = "commands"
	BarScroll       = "scroll"
	BarPage         = "page"
	BarLatest       = "latest"
	BarExitButton   = "button"
	BarConfirm      = "confirm"
	BarReadLogs     = "logs"
	BarEnds         = "first/last"
	BarRestart      = "restart"
	BarStopAuto     = "stop auto restart"
	BarTimeField    = "hour/minute"
	BarTimeAdjust   = "change"
	BarCancel       = "cancel"
)

// Setup wizard screens.
const (
	SetupTitle            = "hijo-server-ops setup"
	SetupStepWorkDir      = "1/4 Minecraft server directory"
	SetupStepName         = "2/4 Server name"
	SetupStepCommand      = "3/4 Choose the start script"
	SetupStepCommandInput = "3/4 Start script path"
	SetupStepConfirm      = "4/4 Create with these contents"
	SetupManualEntry      = "enter a path directly"
	SetupNotExecutable    = "(not executable)"
	SetupNoCandidates     = "no start script candidates found"
	SetupChmodGrant       = "[x] add execute permission (only for those who can already read it)"
	SetupChmodDeny        = "[ ] leave execute permission off (hso cannot start as is)"
)

func SetupTarget(path string) string {
	return "creating: " + path
}

func SetupRelativeHint(dir string) string {
	return "a path relative to " + dir + " also works"
}

func SetupServerName(name string) string {
	return "registered as: " + name
}

// Key bar descriptions.
const (
	KeyNext        = "next"
	KeyAbort       = "abort"
	KeySelect      = "select"
	KeyConfirm     = "confirm"
	KeyBack        = "back"
	KeyCreate      = "create"
	KeyToggleChmod = "toggle execute permission"
)

// Setup input validation.
func EnterCommand() error {
	return errors.New("enter a start script")
}

func EnterDirectory() error {
	return errors.New("enter a directory")
}

func FileNotFound(path string) error {
	return fmt.Errorf("no such file: %s", path)
}

func NotRegularFile(path string) error {
	return fmt.Errorf("not a regular file: %s", path)
}

func DirectoryNotFound(path string) error {
	return fmt.Errorf("no such directory: %s", path)
}

func NotDirectory(path string) error {
	return fmt.Errorf("not a directory: %s", path)
}

func AbsPathFailed(err error) error {
	return fmt.Errorf("resolve absolute path: %w", err)
}

// Reading and writing the config file.
func CommandRequired() error {
	return errors.New("server.command is required")
}

func JavaHomeInvalid(err error) error {
	return fmt.Errorf("server.java is not a valid JAVA_HOME: %w", err)
}

// JavaInlineTableUnsupported は server = { ... } 形式の設定を断る。この形は
// hso が生成しないので、書き換えに対応するより手で直してもらう方が安全。
func JavaInlineTableUnsupported() error {
	return errors.New(
		"hso java change cannot edit a config written as server = { ... }. " +
			"Add java = \"path to JAVA_HOME\" under [server] in hso.toml instead")
}

func JavaHomeReplaced(configured, actual string) string {
	return fmt.Sprintf("The configured Java (%s) is unavailable; using %s found by rescanning. Run hso java change to choose Java again.", configured, actual)
}

func JavaHomeNotInjected(configured string) string {
	return fmt.Sprintf("The configured Java (%s) is unavailable and no replacement was found; starting without adding Java to PATH. Run hso java change to choose Java again.", configured)
}

func ReadConfigFailed(err error, path string) error {
	return fmt.Errorf("read config file: %w%s", err, reinitialize(path))
}

func ValidateJavaConfigFailed(err error, path string) error {
	return fmt.Errorf("validate the updated Java config: %s: %w", path, err)
}

func UnknownConfigKeys(keys, path string) error {
	return fmt.Errorf("unknown config keys: %s%s", keys, reinitialize(path))
}

// reinitialize explains how to recover from an unreadable config file. Stale
// keys from an older format are the usual cause, and without spelling out
// that deleting the file brings the setup wizard back, there is no way to
// know what to do.
func reinitialize(path string) string {
	return fmt.Sprintf(
		"\nreinitialize hso.toml: delete %s and start hso to build it again"+
			" from the setup wizard",
		path,
	)
}

func ConfigAbsPathFailed(err error) error {
	return fmt.Errorf("resolve absolute path of config file: %w", err)
}

func ConfigAlreadyExists(path string) error {
	return fmt.Errorf("config file already exists: %s", path)
}

func CreateConfigFailed(err error) error {
	return fmt.Errorf("create config file: %w", err)
}

func WriteConfigFailed(err error) error {
	return fmt.Errorf("write config file: %w", err)
}

func ConfigPermissionFailed(err error) error {
	return fmt.Errorf("match config file permission: %w", err)
}

func ReplaceConfigFailed(err error) error {
	return fmt.Errorf("replace config file: %w", err)
}

func RemoveConfigAfterRegistrationFailed(registrationErr, removeErr error) error {
	return fmt.Errorf(
		"failed to register the server and could not remove the newly created config: %v: %w",
		registrationErr, removeErr,
	)
}

// Server registry.
func RegistryPathFailed(err error) error {
	return fmt.Errorf("resolve server registry path: %w", err)
}

func InvalidServerName(name string) error {
	return fmt.Errorf(
		"invalid server name %q (use 1-30 bytes of ASCII letters, digits, -, _, or .; start with a letter or digit)",
		name,
	)
}

func DuplicateServerName(name string) error {
	return fmt.Errorf("duplicate server name ignoring case: %s", name)
}

func DuplicateServerConfig(name, path string) error {
	return fmt.Errorf("config file is already registered as server %s: %s", name, path)
}

func ConfigAlreadyRegistered(name, path string) error {
	return fmt.Errorf("config file is already registered as server %s: %s\nrun hso delete %s to remove it from the list", name, path, name)
}

func ReadRegistryFailed(err error, path string) error {
	return fmt.Errorf("read server registry (%s): %w", path, err)
}

func UnknownRegistryKeys(keys, path string) error {
	return fmt.Errorf("unknown server registry keys (%s): %s", path, keys)
}

func EncodeRegistryFailed(err error) error {
	return fmt.Errorf("encode server registry as TOML: %w", err)
}

func CreateRegistryDirectoryFailed(err error) error {
	return fmt.Errorf("create server registry directory: %w", err)
}

func WriteRegistryFailed(err error) error {
	return fmt.Errorf("write server registry: %w", err)
}

func RegistryPermissionFailed(err error) error {
	return fmt.Errorf("match server registry permission: %w", err)
}

func ReplaceRegistryFailed(err error) error {
	return fmt.Errorf("replace server registry: %w", err)
}

// pidfile.
func AlreadyRunning() error {
	return errors.New("server is already running")
}

func UnsafePIDDirectory() error {
	return errors.New("could not verify pidfile directory safety")
}

func CreatePIDDirectoryFailed(err error) error {
	return fmt.Errorf("create pidfile directory: %w", err)
}

func CheckPIDDirectoryFailed(err error) error {
	return fmt.Errorf("check pidfile directory: %w", err)
}

func PIDDirectoryIsSymlink(path string) error {
	return fmt.Errorf("pidfile directory is a symbolic link: %s", path)
}

func PIDDirectoryWrongOwner(path string) error {
	return fmt.Errorf("pidfile directory is not owned by the current user: %s", path)
}

func PIDDirectoryWrongMode(path string, mode uint32) error {
	return fmt.Errorf("pidfile directory permission is not 0700: %s (%04o)", path, mode)
}

func ReadPIDStartTimeFailed(err error) error {
	return fmt.Errorf("read hso start time: %w", err)
}

func WritePIDFileFailed(err error, path string) error {
	return fmt.Errorf("write pidfile (%s): %w", path, err)
}

func LockPIDFileFailed(err error, path string) error {
	return fmt.Errorf("lock pidfile (%s): %w", path, err)
}

func PIDFileChangedTooOften(path string) error {
	return fmt.Errorf("pidfile path changed repeatedly while acquiring its lock (%s)", path)
}

func MalformedPIDFile() error {
	return errors.New("malformed pidfile")
}

func ReadPIDFileFailed(err error, path string) error {
	return fmt.Errorf("read pidfile (%s): %w", path, err)
}

func CheckProcessFailed(err error, pid int) error {
	return fmt.Errorf("check process %d: %w", pid, err)
}

func RemoveStalePIDFileFailed(err error, path string) error {
	return fmt.Errorf("remove stale pidfile (%s): %w", path, err)
}

func CheckRegisteredConfigFailed(err error, path string) error {
	return fmt.Errorf("check registered config file (%s): %w", path, err)
}

func WriteServerListFailed(err error) error {
	return fmt.Errorf("display server list: %w", err)
}

func WorkDirCheckFailed(err error) error {
	return fmt.Errorf("check server.workdir: %w", err)
}

func WorkDirNotDirectory(path string) error {
	return fmt.Errorf("server.workdir is not a directory: %s", path)
}

// Start script.
func ScriptAbsPathFailed(err error) error {
	return fmt.Errorf("resolve absolute path of start script: %w", err)
}

func ScriptStatFailed(err error) error {
	return fmt.Errorf("check start script: %w", err)
}

func ScriptIsDirectory(path string) error {
	return fmt.Errorf("start script is a directory: %s", path)
}

func ScriptNotExecutable(path string) error {
	return fmt.Errorf("start script is not executable: %s", path)
}

func ChmodFailed(err error) error {
	return fmt.Errorf("add execute permission: %w", err)
}

// Server operations.
var (
	ErrHeapCountersUnavailable = errors.New("hsperfdata has no heap usage counters")
	ErrRSSUnavailable          = errors.New("/proc status has no VmRSS")
	ErrServerStopped           = errors.New("server is not running")
	ErrRestartBeforeJava       = errors.New("restart is available once the java process has started")
	ErrScriptExitedWithoutJava = errors.New("start script exited without starting a java process")
	ErrServerExited            = errors.New("Minecraft server exited unexpectedly")
)

func RestartFailed(err error) error {
	return fmt.Errorf("restart server: %w", err)
}

func StopFailed(err error) error {
	return fmt.Errorf("stop server: %w", err)
}

func FindJavaFailed(err error) error {
	return fmt.Errorf("locate java process: %w", err)
}

// Command line.
const (
	Lang                = "en"
	ConfigFlagUsage     = "path to the config file"
	Aborted             = "aborted"
	ListNameHeader      = "Name"
	ListStatusHeader    = "Status"
	ListPathHeader      = "Path"
	ServerStopped       = "stopped"
	ConfigNotFound      = "config not found"
	EmptyServerList     = "No servers are registered. Run hso setup to register one."
	StartTitle          = "Choose a server to start"
	DeleteTitle         = "Choose a server to delete"
	DeleteConfirmPrompt = "Only this registration will be removed. hso.toml and the server directory contents will not be deleted.\nDelete it? [y/N]: "
)

func ServerRunning(pid int) string {
	return fmt.Sprintf("running (PID %d)", pid)
}

func ListArgumentsNotAllowed() error {
	return errors.New("the list subcommand does not accept arguments")
}

func NoRegisteredServers() error {
	return errors.New(EmptyServerList)
}

func SetupArgumentsNotAllowed() error {
	return errors.New("the setup subcommand does not accept arguments")
}

func SetupRequiresTerminal() error {
	return errors.New("run setup from a terminal")
}

func StartArgumentsNotAllowed() error {
	return errors.New("the start subcommand accepts at most one server name")
}

func StartRequiresTerminal() error {
	return errors.New("run start from a terminal when omitting the server name")
}

func DeleteArgumentsNotAllowed() error {
	return errors.New("the delete subcommand accepts at most one server name and only the -y or --yes flag")
}

func DeleteTarget(name, path string) string {
	return fmt.Sprintf("%s: %s\n%s: %s", ListNameHeader, name, ListPathHeader, path)
}

func DeleteRequiresTerminal() error {
	return errors.New("run delete from a terminal when omitting the server name")
}

func DeleteRequiresConfirmation() error {
	return errors.New("confirmation requires a terminal; rerun with -y or --yes")
}

func CannotDeleteRunningServer(name string, pid int) error {
	return fmt.Errorf("server %s is running (PID %d); stop it before deleting it", name, pid)
}

func ServerDeleted(name string) string {
	return fmt.Sprintf("removed server %s from the list", name)
}

func ServerNotRegistered(name string) error {
	return fmt.Errorf("server is not registered: %s", name)
}

func ServerAlreadyRunning(name string, pid int) error {
	return fmt.Errorf("server %s is already running (PID %d)", name, pid)
}

func ServerAlreadyRunningWithoutPID(name string) error {
	return fmt.Errorf("server %s is already running", name)
}

func RegisteredConfigNotFound(name, path string) error {
	return fmt.Errorf("config file for server %s was not found: %s", name, path)
}

func VersionOutput(version, language, architecture string) string {
	return fmt.Sprintf(
		"Version: %s\nLanguage: %s\nArchitecture: %s",
		version, language, architecture,
	)
}

func VersionArgumentsNotAllowed() error {
	return errors.New("the version subcommand does not accept arguments")
}

func UpdateAvailable(latest string) string {
	return fmt.Sprintf("A new version, %s, is available. Run hso update to install it.", latest)
}

func UpdateArgumentsNotAllowed() error {
	return errors.New("the update subcommand does not accept arguments")
}

func AlreadyLatest(current string) string {
	return fmt.Sprintf("hso %s is already up to date.", current)
}

func LatestReleaseRequestFailed(err error) error {
	return fmt.Errorf("fetch the latest release from GitHub: %w", err)
}

func LatestReleaseStatus(status string) error {
	return fmt.Errorf("could not fetch the latest release from GitHub: %s", status)
}

func LatestReleaseDecodeFailed(err error) error {
	return fmt.Errorf("read the latest release information from GitHub: %w", err)
}

func GitHubRateLimited(minutes int) error {
	return fmt.Errorf("the GitHub API rate limit was reached; it resets in %d minutes", minutes)
}

func InvalidReleaseTag(tag string) error {
	return fmt.Errorf("GitHub returned an invalid release tag: %s", tag)
}

func ReleaseAssetMissing(name string) error {
	return fmt.Errorf("the latest release does not contain %s", name)
}

func ReleaseAssetURLMissing(name string) error {
	return fmt.Errorf("the latest release asset has no download URL: %s", name)
}

func ExecutablePathFailed(err error) error {
	return fmt.Errorf("find the running hso executable: %w", err)
}

func UpdateTemporaryDirectoryFailed(err error) error {
	return fmt.Errorf("create a temporary directory for the update: %w", err)
}

func DownloadAssetFailed(name string, err error) error {
	return fmt.Errorf("download %s: %w", name, err)
}

func DownloadAssetStatus(name, status string) error {
	return fmt.Errorf("could not download %s: %s", name, status)
}

func ReadChecksumsFailed(err error) error {
	return fmt.Errorf("read checksums.txt: %w", err)
}

func ReadArchiveFailed(name string, err error) error {
	return fmt.Errorf("read %s: %w", name, err)
}

func ChecksumNotFound(name string) error {
	return fmt.Errorf("checksums.txt does not contain a valid SHA-256 checksum for %s", name)
}

func CalculateChecksumFailed(name string, err error) error {
	return fmt.Errorf("calculate the SHA-256 checksum for %s: %w", name, err)
}

func ChecksumMismatch(name string) error {
	return fmt.Errorf("the SHA-256 checksum does not match for %s", name)
}

func ExtractArchiveFailed(err error) error {
	return fmt.Errorf("extract hso from the update archive: %w", err)
}

func BinaryMissingFromArchive() error {
	return errors.New("the update archive does not contain hso")
}

func PrivilegeAuthenticationFailed(tool string, err error) error {
	return fmt.Errorf("could not authenticate with %s: %w", tool, err)
}

func PrivilegeToolMissing(path string) error {
	return fmt.Errorf("updating %s requires root, but neither sudo nor doas is available; run hso update again as root", path)
}

func ReplaceExecutableFailed(path string, err error) error {
	return fmt.Errorf("replace hso at %s: %w", path, err)
}

func UpdateComplete(latest, path string) string {
	return fmt.Sprintf("Updated hso to %s: %s", latest, path)
}

func UnknownCommand(command, available string) error {
	return fmt.Errorf(
		"unknown subcommand: %s\navailable subcommands: %s",
		command, available,
	)
}

// Java コマンド。
const (
	JavaCommandHelp = `Java configuration and status:
  hso java change [name]  Change the Java used by a server
  hso java list           Show detected Java installations and servers using them`
	JavaChangeTitle       = "Choose the Java used by the server"
	JavaCurrentMark       = "(current setting)"
	JavaRunningNotice     = "The server is running. This change takes effect the next time it starts."
	JavaDetectionNote     = "Automatic detection only scans /usr/lib/jvm.\nJava installations managed by SDKMAN or asdf, or installed under /opt, are not shown.\nTo use one, set [server] java in hso.toml to the absolute JAVA_HOME path."
	JavaNotFound          = "No Java installation was found under /usr/lib/jvm."
	JavaHeader            = "JAVA"
	JavaImplementorHeader = "IMPLEMENTOR"
	JavaHomeHeader        = "JAVA_HOME"
	JavaServersHeader     = "SERVERS"
)

func JavaChangeArgumentsNotAllowed() error {
	return errors.New("java change accepts at most one server name")
}
func JavaListArgumentsNotAllowed() error { return errors.New("java list does not accept arguments") }
func UnknownJavaCommand(command string) error {
	return fmt.Errorf("unknown java subcommand: %s\navailable java subcommands: change, list", command)
}
func JavaChangeRequiresTerminal() error { return errors.New("run java change from a terminal") }
func JavaScanFailed(err error) error    { return fmt.Errorf("scan /usr/lib/jvm: %w", err) }
func JavaChanged(name, home string) string {
	return fmt.Sprintf("Changed the Java used by server %s: %s", name, home)
}
func JavaConfigWarning(name string, err error) string {
	return fmt.Sprintf("warning: server %s was not associated with Java because its config could not be read: %v", name, err)
}
func JavaNotConfiguredWarning(name string) string {
	return fmt.Sprintf("warning: server %s was not associated with Java because it has no Java setting", name)
}
func JavaConfiguredNotDetectedWarning(name, home string) string {
	return fmt.Sprintf("warning: server %s was not associated with the list because its configured Java was not detected: %s", name, home)
}
func WriteJavaListFailed(err error) error { return fmt.Errorf("write the Java list: %w", err) }
