//go:build !en

// Package msg はユーザーに見せる文字列を 1 箇所に集める。日本語版が既定で、
// 英語版は `-tags en` でビルドする。ja と en は同じ識別子を宣言するので、
// 片方で書き忘れるとコンパイルエラーになる。
//
// ここに入れるのは設定モーダル・セットアップウィザード・設定ファイルの
// 検証エラーなど、普通のユーザーが読む文字列だけ。hsperfdata や /proc の
// パースエラーのような開発者向けの診断メッセージは、両言語で英語リテラル
// のまま各パッケージに置く。
//
// 埋め込む値があるものは関数にする。const の書式文字列にすると、ja と en で
// verb がずれても気付けない。%w で包むものは error を返す。
package msg

import (
	"errors"
	"fmt"
	"io/fs"
)

// 設定モーダルの見出し。
const (
	SectionPreferences = "プリファレンス"
	SectionAdvanced    = "詳細設定"
)

// 設定モーダルの項目名。
const (
	LabelTheme      = "テーマ"
	LabelFrame      = "枠"
	LabelBackground = "背景"
	LabelGraphLine  = "グラフの線"
	LabelMeterBar   = "メーターの棒"
	LabelTitle      = "タイトル"
	LabelSelection  = "選択行"
	LabelLog        = "ログ"

	LabelAutoRestart = "自動再起動"
	LabelTimezone    = "タイムゾーン"
	LabelCurrentTime = "現在時刻"
	LabelTimeDrift   = "ずれ"
)

const (
	OptSystemTime     = "システム時刻"
	TimeSettingButton = "設定"
	TimeModalTitle    = "現在時刻の設定"
)

// 配色プリセットの表示名。
const (
	OptDefault  = "既定"
	OptCustom   = "カスタム"
	OptMono     = "モノクロ"
	OptNeon     = "ネオン"
	OptOcean    = "海"
	OptForest   = "森"
	OptWarm     = "暖色"
	OptCool     = "寒色"
	OptSafe     = "シンプル"
	OptSignal   = "信号"
	OptFlat     = "単色"
	OptCyan     = "シアン"
	OptWhite    = "白"
	OptAmber    = "黄"
	OptViolet   = "紫"
	OptQuiet    = "控えめ"
	OptSunset   = "夕焼け"
	OptSakura   = "桜"
	OptNord     = "ノルド"
	OptTerminal = "端末"
	OptDark     = "ダーク"
	OptNight    = "夜"
	OptDeep     = "深黒"
	OptCharcoal = "炭色"
)

// 入切の表示名。
const (
	OptOn  = "有効"
	OptOff = "無効"
)

// ステータス行。
const (
	StatusIdle = "操作待ち"
	// StatusStopping は ^C でサーバーの終了を待っている間の表示。
	StatusStopping = "停止中 保存を待っています"
)

func SaveSettingsFailed(err error) string {
	return "設定の保存に失敗: " + err.Error()
}

func ActionFailed(err error) string {
	return "操作失敗: " + err.Error()
}

// サーバー終了後のモーダルとログ画面。
const (
	ExitTitleCrashed  = "サーバー異常終了"
	ExitTitleStopped  = "サーバー停止"
	ExitErrorLines    = "エラー行"
	ExitStateCrashed  = "異常終了しました。ログを確認するか再起動してください。"
	ExitStateStopped  = "サーバーは正常に停止しました。"
	ExitButtonLogs    = "ログを読む"
	ExitButtonRestart = "再起動"
	ExitButtonQuit    = "終了"

	ExitAutoRestartHint         = "Esc: 自動再起動をやめる"
	ExitAutoRestartCanceled     = "自動再起動をやめました。"
	ExitAutoRestartStopped      = "短時間での終了が続いたため、自動再起動を打ち切りました。"
	ExitAutoRestartSkipped      = "起動スクリプトがjavaを起動しないため、自動再起動しません。"
	ExitAutoRestartRejected     = "自動再起動を要求できませんでした。"
	ExitAutoRestartFatal        = "hso 側の失敗のため、自動再起動しません。"
	ExitAutoRestartJavaMismatch = "Java のバージョンが合わないため、自動再起動しません。"
	ExitAutoRestartDone         = "自動再起動しました。サーバーは動いています。"
	ExitAutoRestartDoneHint     = "Enter: 閉じる"
)

func JavaVersionMismatch(required, actual int) string {
	if actual == 0 {
		return fmt.Sprintf("このサーバーは Java %d が必要ですが、現在の Java バージョンは確認できませんでした。", required)
	}
	return fmt.Sprintf("このサーバーは Java %d が必要ですが、Java %d で起動しました。", required, actual)
}

func JavaVersionChange(required int) string {
	return fmt.Sprintf("hso java change で Java %d に切り替えてください。", required)
}

func JavaVersionInstall(required int) string {
	return fmt.Sprintf("Java %d をインストールしてから、hso java change で切り替えてください。", required)
}

func StoppedLogTitle(code string) string {
	return "Log · 停止済み (exit " + code + ")"
}

func ExitSummary(code, exitedAt, uptime string) string {
	return fmt.Sprintf("終了コード %s · 停止 %s · 稼働 %s", code, exitedAt, uptime)
}

func ExitMemory(rss, heapUsed, heapCommitted, delta string) string {
	return fmt.Sprintf(
		"最終メモリ  RSS %s · heap %s/%s · Δ %s",
		rss, heapUsed, heapCommitted, delta,
	)
}

func ExitGC(collections uint64, available bool, last string) string {
	count := "n/a"
	if available {
		count = fmt.Sprintf("%d 回", collections)
	}
	return fmt.Sprintf("GC  %s · 最終停止 %s", count, last)
}

func ExitStateRestarting(dots string) string {
	return "再起動中" + dots
}

func ExitAutoRestartIn(seconds int) string {
	return fmt.Sprintf("%d 秒後に自動で再起動します", seconds)
}

func ExitAutoQuit(seconds int) string {
	return fmt.Sprintf("%d 秒後に hso を終了します（キー入力で解除）", seconds)
}

// キーバー（画面下部のキー説明）。日本語は全角なので、最小端末幅 72 桁に
// 収まるよう短くしている。
const (
	BarItem         = "項目"
	BarValue        = "値"
	BarCandidate    = "候補"
	BarClose        = "閉じる"
	BarExit         = "終了"
	BarQuitNow      = "即終了"
	BarSelectPanel  = "選ぶ"
	BarFocus        = "開く"
	BarSettings     = "設定"
	BarBackToSelect = "選択"
	BarConsoleTab   = "入力/再起動/停止"
	BarComplete     = "補完"
	BarExecute      = "実行"
	BarBack         = "戻る"
	BarCommand      = "コマンド"
	BarPutInConsole = "入力欄へ"
	BarPlayer       = "プレイヤー"
	BarCommandList  = "コマンド一覧"
	BarScroll       = "スクロール"
	BarPage         = "ページ"
	BarLatest       = "最新"
	BarExitButton   = "ボタン"
	BarConfirm      = "決定"
	BarReadLogs     = "ログ"
	BarEnds         = "先頭/末尾"
	BarRestart      = "再起動"
	BarStopAuto     = "自動再起動をやめる"
	BarTimeField    = "時/分"
	BarTimeAdjust   = "変更"
	BarCancel       = "取消"
)

// セットアップウィザードの画面。
const (
	SetupTitle            = "hijo-server-ops セットアップ"
	SetupRegisterTitle    = "既存の hso.toml をサーバー一覧に追加"
	SetupRegisterNotice   = "hso.toml が既にあります。この設定をサーバー一覧に追加しますか？"
	SetupRegisterStepName = "一覧に表示するサーバー名を入力してください"
	SetupStepWorkDir      = "1/4 Minecraft サーバーのディレクトリ"
	SetupStepName         = "2/4 サーバー名"
	SetupStepCommand      = "3/4 起動スクリプトを選ぶ"
	SetupStepCommandInput = "3/4 起動スクリプトのパス"
	SetupStepConfirm      = "4/4 この内容で作成する"
	SetupManualEntry      = "パスを直接入力する"
	SetupNotExecutable    = "(実行権限なし)"
	SetupNoCandidates     = "起動スクリプトの候補が見つかりません"
	SetupChmodGrant       = "[x] 実行権限を付ける（読める相手にだけ実行を許す）"
	SetupChmodDeny        = "[ ] 実行権限を付けない（このままでは hso は起動できない）"
)

func SetupTarget(path string) string {
	return "作成先: " + path
}

// SetupRegisterTarget は登録ウィザードの見出し。こちらは hso.toml を作らない
// ので「作成先」とは書けない。
func SetupRegisterTarget(path string) string {
	return "対象: " + path
}

func SetupRelativeHint(dir string) string {
	return dir + " からの相対パスも書ける"
}

func SetupServerName(name string) string {
	return "登録名: " + name
}

// キーバーの説明。
const (
	KeyNext           = "次へ"
	KeyAbort          = "中止"
	KeySelect         = "選ぶ"
	KeyConfirm        = "決定"
	KeyBack           = "戻る"
	KeyCreate         = "作成"
	KeyToggleChmod    = "実行権限の付与を切替"
	KeyRegister       = "登録"
	KeyAddConfig      = "追加する"
	KeyDoNotAddConfig = "追加しない"
)

// セットアップの入力検証。
func EnterCommand() error {
	return errors.New("起動スクリプトを入力してください")
}

func EnterDirectory() error {
	return errors.New("ディレクトリを入力してください")
}

func FileNotFound(path string) error {
	return fmt.Errorf("ファイルがありません: %s", path)
}

func NotRegularFile(path string) error {
	return fmt.Errorf("通常のファイルではありません: %s", path)
}

func DirectoryNotFound(path string) error {
	return fmt.Errorf("ディレクトリがありません: %s", path)
}

func NotDirectory(path string) error {
	return fmt.Errorf("ディレクトリではありません: %s", path)
}

func AbsPathFailed(err error) error {
	return fmt.Errorf("絶対パスを求める: %w", err)
}

// 設定ファイルの読み書き。
func CommandRequired() error {
	return errors.New("server.command は必須です")
}

func JavaHomeInvalid(err error) error {
	return fmt.Errorf("server.java が有効な JAVA_HOME ではありません: %w", err)
}

// JavaInlineTableUnsupported は server = { ... } 形式の設定を断る。この形は
// hso が生成しないので、書き換えに対応するより手で直してもらう方が安全。
func JavaInlineTableUnsupported() error {
	return errors.New(
		"server = { ... } の形で書かれた設定は hso java change では変更できません。" +
			"hso.toml の [server] に java = \"JAVA_HOME のパス\" を直接書いてください")
}

func JavaHomeReplaced(configured, actual string) string {
	return fmt.Sprintf("設定された Java (%s) は使用できないため、再スキャンで見つけた %s を使用します。hso java change で Java を選び直してください。", configured, actual)
}

func JavaHomeNotInjected(configured string) string {
	return fmt.Sprintf("設定された Java (%s) は使用できず、代わりの Java も見つからなかったため、Java の PATH を注入せずに起動します。hso java change で Java を選び直してください。", configured)
}

func ReadConfigFailed(err error, path string) error {
	// まだファイルが無いだけのときに「削除してください」とは言えないので、
	// 作り方の案内へ分ける。
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("設定ファイルを読む: %w%s", err, createConfig(path))
	}
	return fmt.Errorf("設定ファイルを読む: %w%s", err, reinitialize(path))
}

func ValidateJavaConfigFailed(err error, path string) error {
	return fmt.Errorf("Java 設定の更新内容を検証する: %s: %w", path, err)
}

func UnknownConfigKeys(keys, path string) error {
	return fmt.Errorf("不明な設定項目: %s%s", keys, reinitialize(path))
}

// reinitialize は設定ファイルが読めないときの直し方を添える。古い形式の
// 項目が残っているのが主な原因で、消して hso setup をやり直せば作り直せる、
// というところまで書かないと何をすればいいか分からない。引数なしの hso は
// ヘルプを出すだけなので、案内先は必ず hso setup にする。
func reinitialize(path string) string {
	return fmt.Sprintf(
		"\nhso.toml を初期化してください: %s を削除してから、"+
			"Minecraft サーバーのディレクトリで hso setup を実行すると作り直せます",
		path,
	)
}

// createConfig は設定ファイルがまだ無いときの作り方を添える。
func createConfig(path string) string {
	return fmt.Sprintf(
		"\nMinecraft サーバーのディレクトリで hso setup を実行すると %s を作成できます",
		path,
	)
}

func ConfigAbsPathFailed(err error) error {
	return fmt.Errorf("設定ファイルの絶対パスを求める: %w", err)
}

func ConfigAlreadyExists(path string) error {
	return fmt.Errorf("設定ファイルは既にあります: %s", path)
}

func CreateConfigFailed(err error) error {
	return fmt.Errorf("設定ファイルを作る: %w", err)
}

func WriteConfigFailed(err error) error {
	return fmt.Errorf("設定ファイルを書く: %w", err)
}

func ConfigPermissionFailed(err error) error {
	return fmt.Errorf("設定ファイルの権限を合わせる: %w", err)
}

func ReplaceConfigFailed(err error) error {
	return fmt.Errorf("設定ファイルを置き換える: %w", err)
}

func RemoveConfigAfterRegistrationFailed(registrationErr, removeErr error) error {
	return fmt.Errorf(
		"サーバー一覧への登録に失敗し、作成した設定ファイルも戻せませんでした: %v: %w",
		registrationErr, removeErr,
	)
}

// サーバー一覧。
func RegistryPathFailed(err error) error {
	return fmt.Errorf("サーバー一覧の場所を求める: %w", err)
}

func InvalidServerName(name string) error {
	return fmt.Errorf(
		"使えないサーバー名です: %q（1〜30バイトのASCII英数字と - _ . が使え、先頭は英数字です）",
		name,
	)
}

func DuplicateServerName(name string) error {
	return fmt.Errorf("サーバー名が大文字小文字を区別せず重複しています: %s", name)
}

func DuplicateServerConfig(name, path string) error {
	return fmt.Errorf("設定ファイルはサーバー %s として登録済みです: %s", name, path)
}

func ConfigAlreadyRegistered(name, path string) error {
	return fmt.Errorf("設定ファイルはサーバー %s として登録済みです: %s\n一覧から外すには hso delete %s を実行してください", name, path, name)
}

func ReadRegistryFailed(err error, path string) error {
	return fmt.Errorf("サーバー一覧を読む (%s): %w", path, err)
}

func UnknownRegistryKeys(keys, path string) error {
	return fmt.Errorf("サーバー一覧に不明な設定項目があります (%s): %s", path, keys)
}

func EncodeRegistryFailed(err error) error {
	return fmt.Errorf("サーバー一覧をTOMLに変換する: %w", err)
}

func CreateRegistryDirectoryFailed(err error) error {
	return fmt.Errorf("サーバー一覧のディレクトリを作る: %w", err)
}

func OpenRegistryLockFailed(err error, path string) error {
	return fmt.Errorf("サーバー一覧のロックファイルを開く (%s): %w", path, err)
}

func LockRegistryFailed(err error, path string) error {
	return fmt.Errorf("サーバー一覧をロックする (%s): %w", path, err)
}

func WriteRegistryFailed(err error) error {
	return fmt.Errorf("サーバー一覧を書く: %w", err)
}

func RegistryPermissionFailed(err error) error {
	return fmt.Errorf("サーバー一覧の権限を合わせる: %w", err)
}

func ReplaceRegistryFailed(err error) error {
	return fmt.Errorf("サーバー一覧を置き換える: %w", err)
}

// pidfile。
func AlreadyRunning() error {
	return errors.New("サーバーはすでに起動中です")
}

func UnsafePIDDirectory() error {
	return errors.New("pidfileのディレクトリの安全性を確認できません")
}

func CreatePIDDirectoryFailed(err error) error {
	return fmt.Errorf("pidfileのディレクトリを作る: %w", err)
}

func CheckPIDDirectoryFailed(err error) error {
	return fmt.Errorf("pidfileのディレクトリを確認する: %w", err)
}

func PIDDirectoryIsSymlink(path string) error {
	return fmt.Errorf("pidfileのディレクトリはシンボリックリンクです: %s", path)
}

func PIDDirectoryWrongOwner(path string) error {
	return fmt.Errorf("pidfileのディレクトリの所有者が現在のユーザーではありません: %s", path)
}

func PIDDirectoryWrongMode(path string, mode uint32) error {
	return fmt.Errorf("pidfileのディレクトリの権限が0700ではありません: %s (%04o)", path, mode)
}

func ReadPIDStartTimeFailed(err error) error {
	return fmt.Errorf("hsoの起動時刻を読む: %w", err)
}

func WritePIDFileFailed(err error, path string) error {
	return fmt.Errorf("pidfileを書く (%s): %w", path, err)
}

func LockPIDFileFailed(err error, path string) error {
	return fmt.Errorf("pidfileをロックする (%s): %w", path, err)
}

func PIDFileChangedTooOften(path string) error {
	return fmt.Errorf("pidfileのパスがロック取得中に繰り返し変更されました (%s)", path)
}

func MalformedPIDFile() error {
	return errors.New("pidfileの内容が壊れています")
}

func ReadPIDFileFailed(err error, path string) error {
	return fmt.Errorf("pidfileを読む (%s): %w", path, err)
}

func CheckProcessFailed(err error, pid int) error {
	return fmt.Errorf("プロセス%dを確認する: %w", pid, err)
}

func RemoveStalePIDFileFailed(err error, path string) error {
	return fmt.Errorf("古いpidfileを消す (%s): %w", path, err)
}

func CheckRegisteredConfigFailed(err error, path string) error {
	return fmt.Errorf("登録された設定ファイルを確認する (%s): %w", path, err)
}

func CheckServerDirectoryFailed(err error, dir string) error {
	return fmt.Errorf("サーバーのディレクトリを確認する (%s): %w", dir, err)
}

func WriteServerListFailed(err error) error {
	return fmt.Errorf("サーバー一覧を表示する: %w", err)
}

func WorkDirCheckFailed(err error) error {
	return fmt.Errorf("server.workdir を確認する: %w", err)
}

func WorkDirNotDirectory(path string) error {
	return fmt.Errorf("server.workdir はディレクトリではありません: %s", path)
}

// 起動スクリプト。
func ScriptAbsPathFailed(err error) error {
	return fmt.Errorf("起動スクリプトの絶対パスを求める: %w", err)
}

func ScriptStatFailed(err error) error {
	return fmt.Errorf("起動スクリプトを確認する: %w", err)
}

func ScriptIsDirectory(path string) error {
	return fmt.Errorf("起動スクリプトはディレクトリです: %s", path)
}

func ScriptNotExecutable(path string) error {
	return fmt.Errorf("起動スクリプトに実行権限がありません: %s", path)
}

func ChmodFailed(err error) error {
	return fmt.Errorf("実行権限を付ける: %w", err)
}

// サーバー操作。
var (
	ErrHeapCountersUnavailable = errors.New("hsperfdataにヒープ使用量カウンタがありません")
	ErrRSSUnavailable          = errors.New("/procのstatusにVmRSSがありません")
	ErrServerStopped           = errors.New("サーバーは停止しています")
	ErrRestartBeforeJava       = errors.New("javaプロセスの起動完了後に再起動できます")
	ErrScriptExitedWithoutJava = errors.New("起動スクリプトがjavaプロセスを開始せずに終了しました")
	ErrServerExited            = errors.New("Minecraftサーバーが予期せず終了しました")
)

func RestartFailed(err error) error {
	return fmt.Errorf("サーバーを再起動する: %w", err)
}

func StopFailed(err error) error {
	return fmt.Errorf("サーバーを停止する: %w", err)
}

func FindJavaFailed(err error) error {
	return fmt.Errorf("javaプロセスの特定: %w", err)
}

// コマンドライン。
const (
	Lang                = "ja"
	ConfigFlagUsage     = "設定ファイルのパス"
	Aborted             = "中止しました"
	ListNameHeader      = "名前"
	ListStatusHeader    = "状態"
	ListPathHeader      = "パス"
	ServerStopped       = "停止"
	ConfigNotFound      = "設定が見つからない"
	EmptyServerList     = "登録済みのサーバーはありません。hso setup で登録してください。"
	StartTitle          = "起動するサーバーを選ぶ"
	CdTitle             = "ディレクトリを開くサーバーを選ぶ"
	DeleteTitle         = "削除するサーバーを選ぶ"
	DeleteConfirmPrompt = "一覧からこの登録だけを外します。hso.toml とサーバーディレクトリの中身は削除しません。\n削除しますか？ [y/N]: "
)

func ServerRunning(pid int) string {
	return fmt.Sprintf("起動中（PID %d）", pid)
}

func ListArgumentsNotAllowed() error {
	return errors.New("list サブコマンドに引数は指定できません")
}

func NoRegisteredServers() error {
	return errors.New(EmptyServerList)
}

func SetupArgumentsNotAllowed() error {
	return errors.New("setup サブコマンドに引数は指定できません")
}

func SetupRequiresTerminal() error {
	return errors.New("setup は端末から実行してください")
}

func StartArgumentsNotAllowed() error {
	return errors.New("start サブコマンドに指定できる名前は1つだけです")
}

func StartRequiresTerminal() error {
	return errors.New("サーバー名を省略するには端末から実行してください")
}

func CdArgumentsNotAllowed() error {
	return errors.New("cd サブコマンドに指定できる名前は1つだけです")
}

func CdRequiresTerminal() error {
	return errors.New("cd は端末から実行してください")
}

func CdOpeningShell(name, dir string) string {
	return fmt.Sprintf("%s のディレクトリでシェルを開きます: %s\n戻るには exit を入力してください", name, dir)
}

func DeleteArgumentsNotAllowed() error {
	return errors.New("delete サブコマンドに指定できる名前は1つまでで、フラグは -y または --yes だけです")
}

func DeleteTarget(name, path string) string {
	return fmt.Sprintf("%s: %s\n%s: %s", ListNameHeader, name, ListPathHeader, path)
}

func DeleteRequiresTerminal() error {
	return errors.New("サーバー名を省略するには端末から実行してください")
}

func DeleteRequiresConfirmation() error {
	return errors.New("削除の確認には端末が必要です。-y または --yes を指定してください")
}

func CannotDeleteRunningServer(name string, pid int) error {
	return fmt.Errorf("サーバー %s は起動中です（PID %d）。先に停止してから削除してください", name, pid)
}

func DeleteTargetChanged(name string) error {
	return fmt.Errorf("確認している間にサーバー %s の登録が変わりました。もう一度実行してください", name)
}

func ServerDeleted(name string) string {
	return fmt.Sprintf("サーバー %s を一覧から削除しました", name)
}

func ServerNotRegistered(name string) error {
	return fmt.Errorf("サーバーが一覧にありません: %s", name)
}

func ServerDirectoryNotFound(name, dir string) error {
	return fmt.Errorf("サーバー %s のディレクトリが見つかりません: %s", name, dir)
}

func OpenShellFailed(err error, shell string) error {
	return fmt.Errorf("シェルを起動できませんでした（%s）: %v", shell, err)
}

func ServerAlreadyRunning(name string, pid int) error {
	return fmt.Errorf("サーバー %s はすでに起動中です（PID %d）", name, pid)
}

func ServerAlreadyRunningWithoutPID(name string) error {
	return fmt.Errorf("サーバー %s はすでに起動中です", name)
}

func RegisteredConfigNotFound(name, path string) error {
	return fmt.Errorf("サーバー %s の設定ファイルが見つかりません: %s", name, path)
}

func VersionOutput(version, language, architecture string) string {
	return fmt.Sprintf(
		"バージョン: %s\n表示言語: %s\nアーキテクチャ: %s",
		version, language, architecture,
	)
}

func VersionArgumentsNotAllowed() error {
	return errors.New("version サブコマンドに引数は指定できません")
}

func UpdateAvailable(latest string) string {
	return fmt.Sprintf("新しいバージョン %s があります。hso update で更新できます。", latest)
}

func UpdateArgumentsNotAllowed() error {
	return errors.New("update サブコマンドに引数は指定できません")
}

func AlreadyLatest(current string) string {
	return fmt.Sprintf("hso %s は最新です。", current)
}

func LatestReleaseRequestFailed(err error) error {
	return fmt.Errorf("GitHub から最新リリースを取得する: %w", err)
}

func LatestReleaseStatus(status string) error {
	return fmt.Errorf("GitHub から最新リリースを取得できませんでした: %s", status)
}

func LatestReleaseDecodeFailed(err error) error {
	return fmt.Errorf("GitHub の最新リリース情報を読む: %w", err)
}

func GitHubRateLimited(minutes int) error {
	return fmt.Errorf("GitHub API のレート制限に達しました。あと %d 分で回復します", minutes)
}

func InvalidReleaseTag(tag string) error {
	return fmt.Errorf("GitHub が不正なリリースタグを返しました: %s", tag)
}

func ReleaseAssetMissing(name string) error {
	return fmt.Errorf("最新リリースにこの資産がありません: %s", name)
}

func ReleaseAssetURLMissing(name string) error {
	return fmt.Errorf("最新リリースの資産にダウンロード先がありません: %s", name)
}

func ExecutablePathFailed(err error) error {
	return fmt.Errorf("実行中の hso のパスを調べる: %w", err)
}

func UpdateTemporaryDirectoryFailed(err error) error {
	return fmt.Errorf("更新用の一時ディレクトリを作る: %w", err)
}

func DownloadAssetFailed(name string, err error) error {
	return fmt.Errorf("%s をダウンロードする: %w", name, err)
}

func DownloadAssetStatus(name, status string) error {
	return fmt.Errorf("%s をダウンロードできませんでした: %s", name, status)
}

func ReadChecksumsFailed(err error) error {
	return fmt.Errorf("checksums.txt を読む: %w", err)
}

func ReadArchiveFailed(name string, err error) error {
	return fmt.Errorf("%s を読む: %w", name, err)
}

func ChecksumNotFound(name string) error {
	return fmt.Errorf("checksums.txt に %s の有効な SHA-256 チェックサムがありません", name)
}

func CalculateChecksumFailed(name string, err error) error {
	return fmt.Errorf("%s の SHA-256 チェックサムを計算する: %w", name, err)
}

func ChecksumMismatch(name string) error {
	return fmt.Errorf("%s の SHA-256 チェックサムが一致しません", name)
}

func ExtractArchiveFailed(err error) error {
	return fmt.Errorf("更新用アーカイブから hso を展開する: %w", err)
}

func BinaryMissingFromArchive() error {
	return errors.New("更新用アーカイブに hso がありません")
}

func PrivilegeExplanation(target, tool string) string {
	return fmt.Sprintf("%s を置き換えるには root 権限が必要なため、%s でパスワードを確認します:", target, tool)
}

func PrivilegeAuthenticationFailed(tool string, err error) error {
	return fmt.Errorf("%s で権限を確認できませんでした: %w", tool, err)
}

func PrivilegeToolMissing(path string) error {
	return fmt.Errorf("%s の更新には root 権限が必要ですが、sudo も doas もありません。root で実行し直してください", path)
}

func ReplaceExecutableFailed(path string, err error) error {
	return fmt.Errorf("hso を置き換える (%s): %w", path, err)
}

func UpdateComplete(latest, path string) string {
	return fmt.Sprintf("hso を %s に更新しました: %s", latest, path)
}

func UnknownCommand(command string) error {
	return fmt.Errorf(
		"未知のコマンドです: %s\nコマンドの一覧は hso help で表示できます",
		command,
	)
}

// シェル補完で候補に添える説明。
const (
	CompletionSetupDescription      = "セットアップを始める"
	CompletionStartDescription      = "登録済みのサーバーを起動する"
	CompletionCdDescription         = "サーバーのディレクトリでシェルを開く"
	CompletionListDescription       = "登録済みのサーバーを表示する"
	CompletionDeleteDescription     = "サーバー一覧から登録を外す"
	CompletionJavaDescription       = "Java の設定と確認"
	CompletionCommandDescription    = "シェル補完スクリプトを出力する"
	CompletionVersionDescription    = "バージョン情報を表示する"
	CompletionUpdateDescription     = "最新リリースへ自己更新する"
	CompletionUninstallDescription  = "hso 自身を削除する"
	CompletionHelpDescription       = "コマンド一覧を表示する"
	CompletionConfigDescription     = "指定した hso.toml で起動する"
	CompletionJavaChangeDescription = "サーバーが使う Java を変更する"
	CompletionJavaListDescription   = "自動検出した Java と利用中サーバーを表示する"
	CompletionPurgeDescription      = "設定と pidfile も削除する"
	CompletionYesDescription        = "確認を省略する"
)

func CompletionArgumentsInvalid() error {
	return errors.New("completion には bash、zsh、fish のいずれかを指定してください")
}

func UnsupportedCompletionShell(shell string) error {
	return fmt.Errorf("未対応のシェルです: %s（bash、zsh、fish のいずれかを指定してください）", shell)
}

// CommandHelp は hso / hso help で出すコマンド一覧。オプションは主要なものだけを
// 載せ、それぞれの細かい説明は dev-docs/commands.md に置く。
const CommandHelp = `hso — Minecraft サーバー用のラッパー型 TUI コンソール

使い方:
  hso <コマンド> [引数]

コマンド:
  setup                   セットアップを始める。Minecraft サーバーのディレクトリで実行する
  start [name]            登録済みのサーバーを起動する。名前を省くと一覧から選ぶ
  cd [name]               サーバーのディレクトリでシェルを開く。戻るには exit
  list (ls)               登録済みのサーバーの名前・状態・設定ファイルのパスを表示する
  delete [name]           サーバー一覧から登録を外す。hso.toml とワールドは消さない
  java change [name]      サーバーが使う Java を変更する
  java list               自動検出した Java と、それを使っているサーバーを表示する
  completion <shell>      bash / zsh / fish の補完スクリプトを出力する
  version                 バージョン・表示言語・アーキテクチャを表示する
  update                  最新リリースへ自己更新する
  uninstall               hso 自身を削除する。--purge で設定と pidfile も消す
  help                    このヘルプを表示する

オプション:
  -config <path>          指定した hso.toml でサーバーを起動する

各コマンドの詳細:
  https://github.com/hijoushoku7/hijo-server-ops/blob/main/dev-docs/commands.md`

// Java コマンド。
const (
	JavaCommandHelp = `Java の設定と確認:
  hso java change [name]  サーバーが使う Java を変更
  hso java list           自動検出した Java と利用中サーバーを表示`
	JavaChangeTitle       = "サーバーが使う Java を選ぶ"
	JavaCurrentMark       = "（現在の設定）"
	JavaRunningNotice     = "サーバーは起動中です。変更は次回起動から反映されます。"
	JavaDetectionNote     = "自動検出の対象は /usr/lib/jvm だけです。\nSDKMAN、asdf、/opt などにある Java は表示されません。\n使用する場合は hso.toml の [server] java に JAVA_HOME の絶対パスを指定してください。"
	JavaNotFound          = "/usr/lib/jvm には Java が見つかりませんでした。"
	JavaHeader            = "JAVA"
	JavaImplementorHeader = "IMPLEMENTOR"
	JavaHomeHeader        = "JAVA_HOME"
	JavaServersHeader     = "SERVERS"
)

func JavaChangeArgumentsNotAllowed() error {
	return errors.New("java change に指定できるサーバー名は1つだけです")
}
func JavaListArgumentsNotAllowed() error {
	return errors.New("java list に引数は指定できません")
}
func UnknownJavaCommand(command string) error {
	return fmt.Errorf("未知の java サブコマンドです: %s\n利用できるサブコマンド: change, list", command)
}
func JavaChangeRequiresTerminal() error {
	return errors.New("java change は端末から実行してください")
}
func JavaScanFailed(err error) error { return fmt.Errorf("/usr/lib/jvm を調べる: %w", err) }
func JavaChanged(name, home string) string {
	return fmt.Sprintf("サーバー %s が使う Java を変更しました: %s", name, home)
}
func JavaConfigWarning(name string, err error) string {
	return fmt.Sprintf("警告: サーバー %s の設定を読めないため、Java と紐付けません: %v", name, err)
}
func JavaNotConfiguredWarning(name string) string {
	return fmt.Sprintf("警告: サーバー %s には Java の設定がないため、Java と紐付けません", name)
}
func JavaConfiguredNotDetectedWarning(name, home string) string {
	return fmt.Sprintf("警告: サーバー %s に設定された Java は自動検出されなかったため、一覧と紐付けません: %s", name, home)
}
func WriteJavaListFailed(err error) error {
	return fmt.Errorf("Java の一覧を書き出す: %w", err)
}
