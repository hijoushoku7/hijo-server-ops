# Java バージョンの検出・案内とサーバー別切替 設計

> 管理 issue: hijoushoku7/hijo-server-ops#64

Minecraft サーバーが要求する Java と実際に起動した Java が食い違ったとき、
**原因を終了モーダルで示し、`hso java change` で登録済みサーバーごとに使う JVM を選べるようにする**。
全体仕様は [spec.md](spec.md)、サーバー一覧と CLI は [cli.md](cli.md)、設定の現状は
[config.md](../.claude/docs/config.md) を参照。

## 目的

Java の食い違いで JVM が `UnsupportedClassVersionError` を出しても、いまの hso は
「起動スクリプトが java を立てずに終わった」としか言えない。また、要求 Java が違う
サーバーが同じマシンに同居するため、マシン全体の既定を変える案内では解決にならない。

そこで各 `hso.toml` に JAVA_HOME を持たせ、そのサーバーの起動時だけ `PATH` の先頭へ
`<JAVA_HOME>/bin` を加える。責務は次のように分ける。

- 検出経路: ログから要求 Java と実際の Java を検出し、終了モーダルで案内する
- `hso java change`: 登録済みサーバーと導入済み JVM を選び、`hso.toml` へ設定する
- 起動経路: `[server] java` を読み、子プロセスへ渡す `PATH` だけを組み立てる

**やらないこと**:

| やらないこと | 理由 |
|---|---|
| Java の自動インストール | 取得・更新・削除まで hso の責任にしない |
| ディストリ判別とパッケージ名の提示 | 検証できない定数表を持たない |
| システム既定の切替 | alternatives 類は他サーバーにも影響する。per-server 注入で不要 |
| `JAVA_HOME` の注入 | Forge/NeoForge や ATM の代表的スクリプトは参照しない。副作用に対する利点がない |
| 起動スクリプトの書き換え | hso が管理するのは `hso.toml` と子プロセス環境だけ |
| セットアップウィザードへの追加 | ウィザードには一切触れない |
| 未登録サーバーの操作 | `hso java change` は registry を入口にする。直指定と systemd 専用構成は対象外 |

## 確定仕様

| 項目 | 決定 | 理由 |
|---|---|---|
| 判定材料 | `UnsupportedClassVersionError` | JVM 自身の形式でローダーに依存しない |
| バージョン逆算 | class file major − 44 | 52→8、61→17、65→21、69→25 |
| 実際の Java | 同じエラーの `up to N` | JVM の自己申告なので絶対パス起動でも正しい |
| JVM 列挙 | `/usr/lib/jvm/*/release` | fork 不要で既定でない JVM も見える |
| 設定値 | JAVA_HOME のディレクトリ | release の位置と一致し、注入時は `/bin` を足すだけ |
| 手書き値 | `.../bin/java` も受けて正規化 | よくある指定を設定エラーにしない |
| symlink | 選んだパスを保存し、解決は比較にだけ使う | Fedora の versioned 実体は Java 更新のたびに消える |
| 設定の破損 | `Load` は成功させ、起動時に再スキャンして続行 | 補助機能が主機能を止めない。存在確認を `Load` に置くと `hso java change` 自身も動かなくなる |
| 注入 | 子プロセスの `PATH` 先頭へ追加 | 素の `java` をサーバー別に解決する |
| 親コマンド | `hso java` | Java 関連コマンドの一覧と使い方を表示する |
| 設定コマンド | `hso java change [name]` | 名前省略時は登録済みサーバーから選ぶ |
| 一覧コマンド | `hso java list` | JVM と選択状況を確認できる |
| 設定保存 | Java 専用の局所更新 | `config.Save` のコメント消失を CLI 操作へ持ち込まない |
| 案内場所 | 終了モーダルだけ | ウィザードには触れない |
| 自動再起動 | Java 不一致は対象外 | 再起動しても直らない |

## `hso.toml`

`[server]` に `java` を追加する。

```toml
[server]
command = "./run.sh"
java = "/usr/lib/jvm/java-21-openjdk-amd64"
```

値は JAVA_HOME、すなわち `bin/java` を配下に持つディレクトリとする。相対パスは受けない。
`command` や `workdir` と違い、サーバーディレクトリではなくシステム側を指す値のため。
手書きで `.../bin/java` が指定されていれば親の親を取り、`filepath.Clean` で整える。
`release` は列挙に使うが、手動導入 JVM も許すため必須条件にしない。

**`Load` は `java` の存在を確認しない。形を整えるだけにする。** Java の指定は
サーバーを動かすための補助であって、`command` のような必須項目ではない。存在確認を
`Load` に置くと、Java を消しただけで設定ファイル全体が読めなくなり、**サーバーが
一切起動できなくなる**。さらに `SetJava` は先頭で `Load` を呼ぶので、
**壊れた設定を直すための `hso java change` 自身が動かなくなる**。値が空でも、
形が不正でも、指す先が無くても `Load` は成功させ、判断は起動時へ回す。

**`EvalSymlinks` の結果は保存しない。** Fedora / RHEL の `/usr/lib/jvm/java-21-openjdk`
は symlink で、実体 `java-21-openjdk-21.0.5.11-1.fc41.x86_64` の名前はパッチ更新のたびに
変わる。実体を書くと `dnf update` しただけでパスが消え、後述の起動前検査に掛かって
サーバーが起動しなくなる。保存するのは**ユーザーが選んだパス**とし、`EvalSymlinks` は
重複排除と `list` の突き合わせにだけ使う。

設定が無ければ親の `PATH` をそのまま使う。既存の `hso.toml`、`hso -config`、
systemd の起動方法は変わらない。

### コメントを残す保存

既存の `config.Save` は TOML 全体を再生成し、コメントやコメントアウトした設定を残さない。
設定モーダルでは既知の仕様だが、`hso java change` で全コメントが消えるのは避ける。

`hso java change` は `config.Save` を呼ばず、`internal/config` に Java 専用の局所更新 API
（仮に `SetJava(path, javaHome)`）を置く。

1. 更新前を `config.Load` で検証する
2. TOML 構文を追い、`[server]` 内の `java` だけを置換する
3. 無ければ `[server]` の末尾へ追加する
4. 一時ファイルへ書き、元と同じ権限にする
5. 一時ファイルを再度 Load し、Java と既存項目を確認して rename する

行頭の文字列一致だけで編集してはならない。引用符内の `#`、dotted key、複数行文字列、
別テーブルの同名キーを誤認しない。コメント、空行、キー順、改行コード、末尾改行は残す。
コメント保持できる既存ライブラリが適合するなら独自走査より優先する。

通常の設定モーダルが `config.Save` で全体を再生成する仕様はこの issue では変えない。
ただし `Config.Server.Java` と `render` へ `java` を加え、モーダル保存後も値は残す。

## `hso java`

```text
$ hso java
Java の設定と確認:
  hso java change [name]  サーバーが使う Java を変更
  hso java list           自動検出した Java と利用中サーバーを表示

$ hso java change
サーバーを選ぶ → JVM を選ぶ → 保存

$ hso java change survival
登録名を指定 → JVM を選ぶ → 保存
```

引数なしの `hso java` は対話 UI を開かず、端末かどうかに関係なくコマンド一覧を stdout
へ表示する。`java` の後に指定できるサブコマンドは `change` と `list` だけ。
`change` の後には登録済みサーバー名を 1 つまで指定できる。この形なら `list` という
既存サーバーも `hso java change list` で指定でき、registry の予約名追加は不要になる。
JVM パスを直接設定する非対話フラグはこの issue では設けない。

設定フローは registry からサーバーを選択し、その `hso.toml` を Load、導入済み JVM を
列挙・選択して `SetJava` で保存する。設定ファイルが無い、読めない、JVM が無い場合は
変更しない。起動中にも設定できるが、**反映は次回起動から**と表示する。JVM はバージョン降順、
同一ならパスの辞書順。現在値には印と初期選択を付け、キャンセルは変更なしで終える。

### `hso java list`

```text
JAVA  IMPLEMENTOR        JAVA_HOME                                      SERVERS
21    Eclipse Adoptium   /usr/lib/jvm/temurin-21-jre-amd64              survival
17    Debian             /usr/lib/jvm/java-17-openjdk-amd64             modded, test
```

登録済み設定を読み、正規化後の JAVA_HOME が一致する名前を `SERVERS` に並べる。
設定無し、ファイル無し、設定エラーは推測で紐付けず、警告を stderr に出して一覧は表示する。
`change` の JVM 選択画面と `list` の末尾には、必ず次の注記を出す。

```text
自動検出の対象は /usr/lib/jvm だけです。
SDKMAN、asdf、/opt などにある Java は表示されません。
使用する場合は hso.toml の [server] java に JAVA_HOME の絶対パスを指定してください。
```

英語版も同じ情報を省略せず表示する。JVM が 1 件も見つからない場合も「Java がない」と
断定せず、この注記を伴って「`/usr/lib/jvm` には Java が見つかりませんでした」とする。

## 起動時の PATH 注入

注入境界は**ユーザーの起動コマンドを実行する最初の子プロセス**とし、親環境の `PATH`
だけを `PATH=<JAVA_HOME>/bin:<従来の PATH>` に変える。PATH が無ければ Java の bin
だけにし、同じディレクトリは先頭の 1 回だけ残す。`JAVA_HOME` や他の環境変数は変更しない。

絶対パスで Java を実行するスクリプトは上書きしない。別 JVM で失敗すれば次節の検出が
案内する。

### 設定と環境が食い違ったときの再スキャン

**Java の設定が壊れていてもサーバーは起動する。** 補助機能が主機能を止めるのは本末転倒
なので、起動を中止する経路は作らない。起動直前に `bin/java` を検査し、使えなければ
`javaenv.Resolve` が次の順で決める。

1. 設定値が使える → それを注入する
2. 使えない → `/usr/lib/jvm` を**再スキャン**する。設定値のディレクトリ名から
   メジャーバージョンを読み取れて（`java-21-openjdk-amd64` → 21）、同じ世代の JVM が
   見つかれば**それを注入し、警告を出す**。同じ世代が複数あればバージョン降順の先頭
3. 見つからない → **注入せず、親の `PATH` のまま起動する**。警告を出す

2 と 3 の警告には、設定に書かれたパスと、実際に使ったもの（または注入しなかったこと）、
`hso java change` での再選択を並べる。**設定ファイルは書き換えない。** 直すかどうかは
ユーザーが決める。

2 の推定はディレクトリ名頼みの発見的処理で、外れることがある。外れても 3 に落ちるだけで
起動は妨げないので、確実性より「止めないこと」を優先する。

## Java 不一致の検出と案内

`class file version (\d+)\.\d+` を要求、`class file versions? up to (\d+)\.\d+` を
実際とする。ログが複数行に分かれるため現世代を順序どおり走査する。要求だけ取れた場合も
案内し、実際は「不明」とする。

既存の `exitErrorLines` はこの Error を安定して拾えない。`generationLogStart()` から
末尾までを専用パターンで走査し、前世代を混ぜない。通常は
`ErrScriptExitedWithoutJava` に入るが、判定をそのエラー型に限定しない。

要求を検出したら終了モーダルの原因欄の先頭へ出す。

```text
このサーバーは Java 21 が必要ですが、Java 17 で起動しました。
Java 21 をインストールしてから、hso java change で切り替えてください。
```

要求バージョンが `/usr/lib/jvm` にあればインストールの一文を省き、
「`hso java change` で Java 21 に切り替えてください」とする。実際を取れなければ
「現在の Java バージョンは確認できませんでした」とする。

ディストリ、パッケージ名、alternatives のコマンドは出さない。案内は短い `[]string`
として作り、モーダル幅で折り返す。長い JVM パスは `hso java list` に委ねる。
Java 不一致は自動再起動の対象外にする。

## JVM の列挙

`/usr/lib/jvm/*/release` の `JAVA_VERSION`、`IMPLEMENTOR`、`OS_ARCH` を読む。
`JAVA_VERSION` の先頭の整数をメジャーとし、`1.8.0_422` は 8 と読む。release の親を
JAVA_HOME とする。壊れた release や `bin/java` の無いディレクトリは個別に無視する。
走査ルートは差し替え可能にする。

同じ実体を指すものは重複を落とすが、**代表には実体ではなく symlink 側**（バージョン番号を
含む最短の名前）を採る。更新をまたいで残るのはそちらのため。ただし `default-java` と
`default-java-runtime` は候補から除く。システム既定を指すので、選ばせると
`update-alternatives` を叩いた瞬間にサーバー別固定の意味が消える。

ディストリ判別は行わず、`distro.go` とパッケージ名・切替コマンド表は作らない。

## 実装の割り付け

| 場所 | 変更 |
|---|---|
| `internal/javaenv/{parse,installed,path}.go` | ログ解析、JVM 列挙、パス検証と PATH 構築 |
| `cmd/hso/main.go` `java.go` | サブコマンド、選択 UI、`java list`、`availableSubcommands` |
| `internal/config/config.go` `java.go` | `Server.Java`、検証、render、`SetJava` |
| `internal/process` | 子プロセスへの PATH 注入 |
| `internal/ui/exit.go` | 検出、終了案内、自動再起動抑止 |
| `internal/msg/msg_ja.go` `msg_en.go` | 両言語の文言 |
| `README.md` | `hso java` の節。`update` / `uninstall` と同じ粒度で 1 節 |
| config/process/cli の各文書 | 設定項目、注入境界、コマンド表を追記 |

`internal/setup` には手を入れない。

## 実装順

案内文が `hso java change` を指すため順序は固定される。機能ごとに PR を分ける
（CLAUDE.md「複数の機能を横断して PR は出さない」）。

1. `internal/javaenv`（列挙・パース）、`[server] java`、`SetJava`
2. `hso java` / `change` / `list`
3. 起動時の PATH 注入
4. 終了モーダルの検出と案内

## テスト

- ログ: 実例、複数行、要求だけ、無関係、壊れた数字、世代境界
- JVM: release 有無、Java 8 形式、壊れた値、ルート無し、symlink 重複で symlink 側が
  代表になること、`default-java` が候補に出ないこと
- PATH/config: 入力正規化、相対、実行不可、PATH 無し、重複、Load/render、
  **保存値が実体へ解決されないこと**
- 破損時の継続: `java` が空／形が不正／指す先が無い設定でも `Load` が成功すること、
  その状態で `SetJava` が直せること、`Resolve` の 3 経路（そのまま／同世代へ再スキャン／
  注入せず続行）、**どの経路でも起動を妨げないこと**
- `SetJava`: 追加/置換、コメント・空行・順序・CRLF・末尾改行、各種 TOML 構文、失敗時の原状維持
- CLI: 引数なしのコマンド一覧、change の名前あり/なし、未登録、非端末、キャンセル、起動中
- CLI表示: list の並びと警告、検出0件、両言語で `/usr/lib/jvm` 限定の注記が出ること
- process/UI: 環境注入、`JAVA_HOME` 不変、JVM 消失、案内、自動再起動抑止、折り返し
- setup: **既存表示と動作が変わらないこと**

`go test ./...` と `go test -tags en ./...` の両方を通す。

## 未決定・要検証

1. PATH 注入が Forge / NeoForge と ATM の代表的な起動スクリプトで選択 JVM へ到達すること
2. TOML 局所更新器の構文範囲。適合するコメント保持ライブラリがあれば優先する

旧案の未決定事項だった 5 系統のパッケージ名確認と Alpine の alternatives 機構は、
ディストリ別案内を廃止したため削除する。
