package msg

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// このテストはタグを付けないので、日本語版と英語版の両方のビルドで走る。
// ja と en に同じ識別子があることはコンパイラが保証するが、それは
// ビルドしたタグ側の言語ファイルだけの話なので、両ファイルの突き合わせは
// ソースを読んで確認する。

// TestLanguageFilesDeclareTheSameIdentifiers は msg_ja.go と msg_en.go が
// 同じ識別子を同じ形（関数なら引数と返り値まで）で宣言しているかを見る。
// 片方に足し忘れると、そのタグでビルドするまで気付けないため。
func TestLanguageFilesDeclareTheSameIdentifiers(t *testing.T) {
	ja := declarations(t, "msg_ja.go")
	en := declarations(t, "msg_en.go")

	for name, signature := range ja {
		switch other, ok := en[name]; {
		case !ok:
			t.Errorf("%s が msg_en.go にない", name)
		case other != signature:
			t.Errorf("%s の形が違う: ja=%s en=%s", name, signature, other)
		}
	}
	for name := range en {
		if _, ok := ja[name]; !ok {
			t.Errorf("%s が msg_ja.go にない", name)
		}
	}
}

// declarations はファイル内のトップレベル宣言を「名前 → 形」で返す。
// 関数は引数と返り値の型、const と var は種別だけを形とする。
func declarations(t *testing.T, path string) map[string]string {
	t.Helper()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	found := map[string]string{}
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			if declaration.Recv != nil {
				continue
			}
			found[declaration.Name.Name] = "func" + types(declaration.Type.Params) +
				types(declaration.Type.Results)
		case *ast.GenDecl:
			for _, spec := range declaration.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, name := range value.Names {
					found[name.Name] = declaration.Tok.String()
				}
			}
		}
	}
	return found
}

func types(fields *ast.FieldList) string {
	if fields == nil {
		return "()"
	}
	names := make([]string, 0, len(fields.List))
	for _, field := range fields.List {
		name, ok := field.Type.(*ast.Ident)
		if !ok {
			names = append(names, "?")
			continue
		}
		// 同じ型をまとめて書いた引数（a, b string）は個数どおりに展開する。
		count := max(len(field.Names), 1)
		for range count {
			names = append(names, name.Name)
		}
	}
	return "(" + strings.Join(names, ", ") + ")"
}

// TestMessagesAreNotEmpty は全メッセージが空でないかを見る。訳を書き忘れた
// まま識別子だけ足すと、画面から文字が消えるだけで気付きにくい。
func TestMessagesAreNotEmpty(t *testing.T) {
	sample := errors.New("boom")

	messages := map[string]string{
		"Lang":                  Lang,
		"SectionPreferences":    SectionPreferences,
		"SectionAdvanced":       SectionAdvanced,
		"LabelFrame":            LabelFrame,
		"LabelGraphLine":        LabelGraphLine,
		"LabelMeterBar":         LabelMeterBar,
		"LabelTitle":            LabelTitle,
		"LabelSelection":        LabelSelection,
		"LabelTimezone":         LabelTimezone,
		"LabelCurrentTime":      LabelCurrentTime,
		"LabelTimeDrift":        LabelTimeDrift,
		"OptSystemTime":         OptSystemTime,
		"TimeSettingButton":     TimeSettingButton,
		"TimeModalTitle":        TimeModalTitle,
		"BarTimeField":          BarTimeField,
		"BarTimeAdjust":         BarTimeAdjust,
		"BarCancel":             BarCancel,
		"OptDefault":            OptDefault,
		"OptMono":               OptMono,
		"OptNeon":               OptNeon,
		"OptOcean":              OptOcean,
		"OptForest":             OptForest,
		"OptWarm":               OptWarm,
		"OptCool":               OptCool,
		"OptSafe":               OptSafe,
		"OptSignal":             OptSignal,
		"OptFlat":               OptFlat,
		"OptCyan":               OptCyan,
		"OptWhite":              OptWhite,
		"OptAmber":              OptAmber,
		"OptViolet":             OptViolet,
		"OptQuiet":              OptQuiet,
		"StatusIdle":            StatusIdle,
		"BarItem":               BarItem,
		"BarValue":              BarValue,
		"BarClose":              BarClose,
		"BarExit":               BarExit,
		"BarSelectPanel":        BarSelectPanel,
		"BarFocus":              BarFocus,
		"BarSettings":           BarSettings,
		"BarBackToSelect":       BarBackToSelect,
		"BarConsoleTab":         BarConsoleTab,
		"BarExecute":            BarExecute,
		"BarBack":               BarBack,
		"BarCommand":            BarCommand,
		"BarPutInConsole":       BarPutInConsole,
		"BarPlayer":             BarPlayer,
		"BarCommandList":        BarCommandList,
		"BarScroll":             BarScroll,
		"BarPage":               BarPage,
		"BarLatest":             BarLatest,
		"SetupTitle":            SetupTitle,
		"SetupStepWorkDir":      SetupStepWorkDir,
		"SetupStepCommand":      SetupStepCommand,
		"SetupStepCommandInput": SetupStepCommandInput,
		"SetupStepConfirm":      SetupStepConfirm,
		"SetupManualEntry":      SetupManualEntry,
		"SetupNotExecutable":    SetupNotExecutable,
		"SetupNoCandidates":     SetupNoCandidates,
		"SetupChmodGrant":       SetupChmodGrant,
		"SetupChmodDeny":        SetupChmodDeny,
		"KeyNext":               KeyNext,
		"KeyAbort":              KeyAbort,
		"KeySelect":             KeySelect,
		"KeyConfirm":            KeyConfirm,
		"KeyBack":               KeyBack,
		"KeyCreate":             KeyCreate,
		"KeyToggleChmod":        KeyToggleChmod,
		"ConfigFlagUsage":       ConfigFlagUsage,
		"Aborted":               Aborted,

		"SaveSettingsFailed": SaveSettingsFailed(sample),
		"ActionFailed":       ActionFailed(sample),
		"SetupTarget":        SetupTarget("/srv/hso.toml"),
		"SetupRelativeHint":  SetupRelativeHint("/srv"),
		"VersionOutput":      VersionOutput("v1.2.3", "ja", "amd64"),
		"VersionArguments":   VersionArgumentsNotAllowed().Error(),
		"UnknownCommand":     UnknownCommand("unknown", "version").Error(),

		"ErrHeapCountersUnavailable": ErrHeapCountersUnavailable.Error(),
		"ErrRSSUnavailable":          ErrRSSUnavailable.Error(),
		"ErrServerStopped":           ErrServerStopped.Error(),
		"ErrRestartBeforeJava":       ErrRestartBeforeJava.Error(),
		"ErrScriptExitedWithoutJava": ErrScriptExitedWithoutJava.Error(),
		"ErrServerExited":            ErrServerExited.Error(),

		"EnterCommand":    EnterCommand().Error(),
		"EnterDirectory":  EnterDirectory().Error(),
		"CommandRequired": CommandRequired().Error(),
	}

	for name, message := range messages {
		if strings.TrimSpace(message) == "" {
			t.Errorf("%s が空", name)
		}
	}
}

func TestVersionOutputIncludesAssetProperties(t *testing.T) {
	output := VersionOutput("v1.2.3", "test-lang", "test-arch")
	for _, want := range []string{"v1.2.3", "test-lang", "test-arch"} {
		if !strings.Contains(output, want) {
			t.Errorf("version の出力 %q に %q がない", output, want)
		}
	}
}

// TestErrorsIncludeTheirPath は受け取ったパスをメッセージに載せているかを
// 見る。どのファイルの話かが出ないと直しようがない。
func TestErrorsIncludeTheirPath(t *testing.T) {
	const path = "/srv/minecraft/run.sh"

	errorsWithPath := map[string]error{
		"FileNotFound":        FileNotFound(path),
		"NotRegularFile":      NotRegularFile(path),
		"DirectoryNotFound":   DirectoryNotFound(path),
		"NotDirectory":        NotDirectory(path),
		"ConfigAlreadyExists": ConfigAlreadyExists(path),
		"WorkDirNotDirectory": WorkDirNotDirectory(path),
		"ScriptIsDirectory":   ScriptIsDirectory(path),
		"ScriptNotExecutable": ScriptNotExecutable(path),
	}

	for name, err := range errorsWithPath {
		if !strings.Contains(err.Error(), path) {
			t.Errorf("%s にパスがない: %v", name, err)
		}
	}
}

// TestWrappedErrorsUnwrap は %w を落としていないかを見る。ここを取り違えると
// 呼び出し側の errors.Is が静かに false になる。
func TestWrappedErrorsUnwrap(t *testing.T) {
	sample := errors.New("boom")

	wrappers := map[string]func(error) error{
		"AbsPathFailed":          AbsPathFailed,
		"ConfigAbsPathFailed":    ConfigAbsPathFailed,
		"CreateConfigFailed":     CreateConfigFailed,
		"WriteConfigFailed":      WriteConfigFailed,
		"ConfigPermissionFailed": ConfigPermissionFailed,
		"ReplaceConfigFailed":    ReplaceConfigFailed,
		"WorkDirCheckFailed":     WorkDirCheckFailed,
		"ScriptAbsPathFailed":    ScriptAbsPathFailed,
		"ScriptStatFailed":       ScriptStatFailed,
		"ChmodFailed":            ChmodFailed,
		"RestartFailed":          RestartFailed,
		"StopFailed":             StopFailed,
		"FindJavaFailed":         FindJavaFailed,
	}

	for name, wrap := range wrappers {
		err := wrap(sample)
		if !errors.Is(err, sample) {
			t.Errorf("%s が元のエラーを包んでいない: %v", name, err)
		}
		if !strings.Contains(err.Error(), "boom") {
			t.Errorf("%s に元のエラーの文言がない: %v", name, err)
		}
	}
}

// TestConfigErrorsAttachTheHint は設定ファイルが読めないときのエラーに、
// 直し方と対象のパスが付いているかを見る。ここが落ちるとユーザーは
// 何をすればいいか分からないまま起動できなくなる。
func TestConfigErrorsAttachTheHint(t *testing.T) {
	const path = "/srv/minecraft/hso.toml"
	sample := errors.New("boom")

	withHint := map[string]error{
		"ReadConfigFailed":  ReadConfigFailed(sample, path),
		"UnknownConfigKeys": UnknownConfigKeys("server.commnad", path),
	}

	for name, err := range withHint {
		if !strings.Contains(err.Error(), path) {
			t.Errorf("%s に設定ファイルのパスがない: %v", name, err)
		}
		if !strings.Contains(err.Error(), "hso.toml") {
			t.Errorf("%s に直し方のヒントがない: %v", name, err)
		}
	}

	if !errors.Is(ReadConfigFailed(sample, path), sample) {
		t.Error("ReadConfigFailed が元のエラーを包んでいない")
	}
	if !strings.Contains(UnknownConfigKeys("server.commnad", path).Error(), "server.commnad") {
		t.Error("UnknownConfigKeys に項目名がない")
	}
}
