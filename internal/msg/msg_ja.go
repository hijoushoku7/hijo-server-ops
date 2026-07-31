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

// 設定モーダルの項目名。
const (
	LabelFrame     = "枠"
	LabelGraphLine = "グラフの線"
	LabelMeterBar  = "メーターの棒"
	LabelTitle     = "タイトル"
	LabelSelection = "選択行"
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

// ステータス行。
const StatusIdle = "操作待ち"

func SaveSettingsFailed(err error) string {
	return "設定の保存に失敗: " + err.Error()
}

func ActionFailed(err error) string {
	return "操作失敗: " + err.Error()
}

// セットアップウィザードの画面。
const (
	SetupTitle            = "hijo-server-ops セットアップ"
	SetupStepWorkDir      = "1/3 Minecraft サーバーのディレクトリ"
	SetupStepCommand      = "2/3 起動スクリプトを選ぶ"
	SetupStepCommandInput = "2/3 起動スクリプトのパス"
	SetupStepConfirm      = "3/3 この内容で作成する"
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
const (
	EnterCommand   = "起動スクリプトを入力してください"
	EnterDirectory = "ディレクトリを入力してください"
)

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
const CommandRequired = "server.command は必須です"

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
	ConfigFlagUsage = "設定ファイルのパス"
	Aborted         = "中止しました"
)
