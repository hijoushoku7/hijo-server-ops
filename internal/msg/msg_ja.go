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
)

// 設定モーダルの見出し。
const (
	SectionPreferences = "プリファレンス"
	SectionAdvanced    = "詳細設定"
)

// 設定モーダルの項目名。
const (
	LabelFrame     = "枠"
	LabelGraphLine = "グラフの線"
	LabelMeterBar  = "メーターの棒"
	LabelTitle     = "タイトル"
	LabelSelection = "選択行"
	LabelLog       = "ログ"

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
	OptDefault = "既定"
	OptMono    = "モノクロ"
	OptNeon    = "ネオン"
	OptOcean   = "海"
	OptForest  = "森"
	OptWarm    = "暖色"
	OptCool    = "寒色"
	OptSafe    = "シンプル"
	OptSignal  = "信号"
	OptFlat    = "単色"
	OptCyan    = "シアン"
	OptWhite   = "白"
	OptAmber   = "黄"
	OptViolet  = "紫"
	OptQuiet   = "控えめ"
)

// 入切の表示名。
const (
	OptOn  = "有効"
	OptOff = "無効"
)

// ステータス行。
const StatusIdle = "操作待ち"

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

	ExitAutoRestartHint     = "Esc: 自動再起動をやめる"
	ExitAutoRestartCanceled = "自動再起動をやめました。"
	ExitAutoRestartStopped  = "短時間での終了が続いたため、自動再起動を打ち切りました。"
	ExitAutoRestartSkipped  = "起動スクリプトがjavaを起動しないため、自動再起動しません。"
	ExitAutoRestartRejected = "自動再起動を要求できませんでした。"
	ExitAutoRestartFatal    = "hso 側の失敗のため、自動再起動しません。"
	ExitAutoRestartDone     = "自動再起動しました。サーバーは動いています。"
	ExitAutoRestartDoneHint = "Enter: 閉じる"
)

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
	BarClose        = "閉じる"
	BarExit         = "終了"
	BarSelectPanel  = "選ぶ"
	BarFocus        = "開く"
	BarSettings     = "設定"
	BarBackToSelect = "選択"
	BarConsoleTab   = "入力/再起動/停止"
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

func SetupRelativeHint(dir string) string {
	return dir + " からの相対パスも書ける"
}

func SetupServerName(name string) string {
	return "登録名: " + name
}

// キーバーの説明。
const (
	KeyNext        = "次へ"
	KeyAbort       = "中止"
	KeySelect      = "選ぶ"
	KeyConfirm     = "決定"
	KeyBack        = "戻る"
	KeyCreate      = "作成"
	KeyToggleChmod = "実行権限の付与を切替"
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

func ReadConfigFailed(err error, path string) error {
	return fmt.Errorf("設定ファイルを読む: %w%s", err, reinitialize(path))
}

func UnknownConfigKeys(keys, path string) error {
	return fmt.Errorf("不明な設定項目: %s%s", keys, reinitialize(path))
}

// reinitialize は設定ファイルが読めないときの直し方を添える。古い形式の
// 項目が残っているのが主な原因で、消して起動し直せばセットアップから
// 作り直せる、というところまで書かないと何をすればいいか分からない。
func reinitialize(path string) string {
	return fmt.Sprintf(
		"\nhso.toml を初期化してください: %s を削除して hso を起動すると"+
			"セットアップから作り直せます",
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
	Lang             = "ja"
	ConfigFlagUsage  = "設定ファイルのパス"
	Aborted          = "中止しました"
	ListNameHeader   = "名前"
	ListStatusHeader = "状態"
	ListPathHeader   = "パス"
	ServerStopped    = "停止"
	ConfigNotFound   = "設定が見つからない"
	EmptyServerList  = "登録済みのサーバーはありません。hso setup で登録してください。"
	StartTitle       = "起動するサーバーを選ぶ"
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

func ServerNotRegistered(name string) error {
	return fmt.Errorf("サーバーが一覧にありません: %s", name)
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

func UnknownCommand(command, available string) error {
	return fmt.Errorf(
		"未知のサブコマンドです: %s\n利用できるサブコマンド: %s",
		command, available,
	)
}
