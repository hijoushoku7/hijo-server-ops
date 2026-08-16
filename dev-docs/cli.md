# CLI ツール化 設計

> 管理 issue: hijoushoku7/hijo-server-ops#53（#51 を吸収）

`hso` をサーバーディレクトリに置くバイナリから、どこからでも呼べるコマンドへ広げる。
全体の仕様と位置づけは [spec.md](spec.md)、ビルド手順は [build.md](build.md)、
シェル補完は [completion.md](completion.md)。

## 目的

いまの配布形態は「アーカイブを展開して `./hso` を叩く」。サーバーが 1 台なら過不足ないが、
2 台目からは台数分のバイナリのコピーができ、更新はその全部を手で置き換えることになる。
自分のバージョンを知る手段も、どこにサーバーを置いたかを思い出す手段もない。

ここで足すのは次の 4 つ。

- **インストールスクリプト**による導入（`curl | sh` で PATH の通った場所へ置く）
- サーバー一覧の登録と、名前で選んで起動すること
- バージョンの確認と自己更新
- そのサーバーが今動いているかの確認

**やらないこと**: 複数サーバーの同時管理・集中パネル化。**1 サーバー = 1 hso プロセス**は
変えない（spec.md の「コンソールの代替であって管理パネルではない」を崩さない）。
`hso start` は選んだ 1 台をいまの端末で開くだけで、常駐もデーモンも持たない。

---

## 確定仕様

| 項目 | 決定 | 理由 |
|---|---|---|
| バイナリ構成 | **1 本のまま。サブコマンドで分岐する** | ラッパーを別バイナリにしてサーバーごとにコピーすると、8〜15MB の複製が台数分増え、`hso update` が「全コピーを追う」問題を抱える |
| 導入 | **`curl -fsSL … \| sh` の install.sh** を用意し、Releases から最新バイナリを取って PATH の通った場所へ置く | 「展開して `./hso`」はサーバー 1 台向けの手順で、コマンドとして使う形に合わない |
| インストール先 | **ユーザーが叩き方で選ぶ。** `--system` で `/usr/local/bin`、既定は `~/.local/bin` | 自動判定にしない。`/usr/local/bin` は FHS でパッケージ管理外のソフト用と決まっており、**既定の PATH に入っている**（`/usr/bin` はディストリのパッケージ領域で衝突しうる） |
| 権限昇格 | **書き込みの一手だけ**を `sudo` / `doas` に通す。パイプ全体を `sudo` で走らせない | 取得・展開・照合を root でやる理由がない。`hso update` も同じ形にする |
| 既存の呼び方 | `-config` は**現行のまま**動く。引数なしの `hso` は**ヘルプに変えた**（→「ヘルプ」） | systemd unit と、サーバーディレクトリで直に叩く経路を壊さない。CLI 化は追加であって置き換えではない（**README のクイックスタートは差し替える**） |
| グローバル設定 | `~/.config/hso/config.toml` に**サーバー一覧だけ** | `command` / `workdir` を二重に持たない。設定の真実の在処は各 `hso.toml` のまま |
| per-server 設定 | `hso.toml` は**無変更** | `internal/config` に手を入れずに済む |
| サーバー名 | ASCII の英数字と `-` `_` `.`、**先頭は英数字**、1〜30 バイト | pidfile のファイル名になり、`list` の表に並ぶ（→「サーバー名の規則」） |
| 起動中の判定 | pidfile ＋ `/proc` の起動時刻照合 | PID の存在だけでは再利用で誤検知する |
| pidfile の場所 | `$XDG_RUNTIME_DIR/hso/`、無ければ `/tmp/hso-<uid>/`（0700・所有者を検査） | `/tmp` は 10 日で掃除される環境があるので、生きている間は mtime を更新する（→「置き場所」） |
| バージョン | `-ldflags -X` で埋め込む | `update` が「上げる必要があるか」を判断する前提 |
| 更新 | GitHub Releases から取得し、チェックサム照合後に rename で自己置換 | 配布物がすでに arch × lang の tar.gz ＋ `checksums.txt` で確定している |
| 最新タグの取得 | **GitHub API** の `/repos/:owner/:repo/releases/latest`。install.sh と `hso update` で同じ | 未認証 60 req/h は手で叩くコマンドでは当たらない。アセットの有無を落とす前に言える |
| install.sh のシェル | **POSIX sh だけで書き切る**。bash 依存の記法を入れない | `curl \| sh` と書く以上、走る先は dash や BusyBox ash になる |
| 削除 | **`hso uninstall`。** 権限が足りなければ**昇格せずエラー**で `sudo hso uninstall` を案内する | 入れる手順がコマンド 1 本なら消す手順も 1 本にする。`unlink` 1 回しかない操作を分割して昇格しても何も守れない |
| 常駐 | **持たない** | デーモンを足すと 1 サーバー = 1 プロセスの単純さと、hso が死んだらサーバーも畳むという前提が崩れる |

### バイナリを分けない理由

issue の当初案は「コマンド用バイナリ」と「ラッパー本体（`hso_wrapper.example` としてサーバー
ディレクトリへコピー）」の 2 本立てだった。これを採らない。

- サーバーごとに実体が増える。更新のたびに全ディレクトリのコピーを追う必要が出る
- サーバーディレクトリに置くものは `hso.toml` だけで済むほうが、移動・バックアップが楽
- **すでに 1 バイナリが役割で分岐する構造になっている。** hso は自分自身を
  `__hso_supervise` 引数付きで再実行して supervisor になる（`internal/process/supervisor.go`）。
  サブコマンドはこの分岐が 1 段増えるだけで、新しい仕組みではない

ja / en で 2 本という配布形態はそのまま。言語で実行ファイル名を分岐させない方針も変えない。

---

## 引数の解析

**順番が効く。**

1. `process.SupervisorCommand(os.Args)` — `__hso_supervise` は最優先。サブコマンドの
   解析より**前**に見る（現行の `main` がすでにこの順）。ここを後ろに回すと、supervisor の
   再実行が未知のサブコマンドとして弾かれる
2. 引数なし、または `help` / `-h` / `-help` / `--help` ならヘルプへ
3. `-v` / `--v` / `-version` / `--version` なら `hso version` と同じ出力へ
4. 第 1 引数がサブコマンド名なら、そのコマンドへ
5. それ以外（`-config path`）は TUI 経路へ

サブコマンド名は `-` で始まらないので、4 と 5 は第 1 引数の 1 文字目で分けられる。
ヘルプとバージョンだけはこの区別より先に見るので、`-` 付きの書き方も同じところへ着く。

| コマンド | 動作 |
|---|---|
| `hso` / `hso help` | コマンド一覧を表示するだけ。何も起動しない（→「ヘルプ」） |
| `hso -config path` | 指定した設定で起動。設定が無く端末なら対話セットアップ → 起動 |
| `hso setup` | ウィザードで `hso.toml` を作り、名前を付けて一覧へ登録する |
| `hso start [name]` | 名前で起動。省略時は一覧から選ばせる |
| `hso cd [name]` | 登録済みサーバーのディレクトリで新しいシェルを開く |
| `hso list` (alias): 'hso ls' | 登録済みサーバーの名前 / 状態 / 設定パス |
| `hso java` | Java 関連コマンドの使い方を表示 |
| `hso java change [name]` | 登録済みサーバーが使う Java を変更 |
| `hso java list` | 自動検出した JVM と利用中サーバーを表示 |
| `hso completion <shell>` | bash / zsh / fish の補完スクリプトを標準出力へ出す |
| `hso version` | バージョン・言語・アーキテクチャ（`-v` / `--v` / `-version` / `--version` も同じ） |
| `hso update` | 最新リリースへ自己更新 |
| `hso uninstall` | 自分自身のバイナリを消す。`--purge` で設定と pidfile も |

---

## グローバル設定

`$XDG_CONFIG_HOME/hso/config.toml`、未設定なら `~/.config/hso/config.toml`。

```toml
[[servers]]
name = "survival"
config = "/srv/minecraft/hso.toml"

[[servers]]
name = "creative"
config = "/srv/creative/hso.toml"
```

- **持つのは名前と `hso.toml` のパスだけ。** `command` や `workdir` を写すと、片方だけ
  書き換えたときにどちらが本物か分からなくなる
- 未知キーはエラーにする（`internal/config` と同じ方針。書き間違いを黙って無視しない）
- 保存は一時ファイル ＋ rename。途中で失敗しても一覧を壊さない
- **`internal/config` には混ぜず、別パッケージに置く**（`internal/registry` を想定）。
  混ぜると「設定が読めない」というエラーが、サーバーの設定の話なのか一覧の話なのか
  読み手に分からなくなる
- 設定ファイルが消えているエントリは `list` で「見つからない」と出すだけで、**勝手に
  消さない**。外付けディスクやマウント待ちで一時的に見えないことがある

---

## サーバー名の規則

名前は「一覧のキー」であると同時に **pidfile のファイル名の一部**（`<name>.pid`）になり、
`list` の表に並ぶ。この 3 つを同時に満たす、いちばん狭い範囲を採る。

| 項目 | 決定 |
|---|---|
| 使える文字 | ASCII の英数字（`A-Z` `a-z` `0-9`）と `-` `_` `.` の 3 記号だけ |
| 先頭の文字 | **英数字のみ。** `-` と `.` で始められない |
| 長さ | **1〜30 バイト**（ASCII なのでバイト数 ＝ 文字数） |
| 重複 | **大文字小文字を区別せずに**弾く。`survival` があるとき `Survival` は登録できない |

### なぜこの範囲か

- **`/` と `..` を弾く、では足りない。** 名前はパスの一部になるので、通す文字を列挙する
  ホワイトリストにする。「危ないものを挙げて弾く」形にすると、挙げ忘れた 1 文字が
  そのままパスへ入る
- **先頭の `-` を弾く。** `hso start -x` のような名前は、この先どのコマンドの引数解析でも
  フラグと区別が付かない。`--` を要求する運用にするより、名前の側で作らせない
- **先頭の `.` を弾く。** `.` と `..` そのものが名前になる経路を、この 1 行で同時に塞げる。
  隠しファイルとして pidfile が見えなくなるのも避けられる
- **空白を弾く。** `list` の表が崩れ、シェルで `hso start my server` と打った人が
  引数 2 つを渡すことになる
- **非 ASCII を弾く。** 日本語の名前を許すと、`が` を NFC で打った登録と NFD で打った検索が
  一致しない（見た目が同じで一致しない名前ができる）。ファイル名のエンコーディングと
  端末の表示幅計算にも引きずられる。**名前は識別子であって表示名ではない**ので、
  読みやすさは `hso.toml` 側の設定（MOTD など）が担う
- **30 バイト**は `list` の 1 行が端末幅で折り返さない範囲、かつ `NAME_MAX`（255）に
  対して十分な余裕を残す長さとして採る。上限が要るのは、長さの検査を省くと
  「登録はできたが pidfile の作成で失敗する」名前が作れてしまうため
- **大文字小文字の重複を弾く**のは、`list` に `Survival` と `survival` が並んだときに
  どちらがどれか読み手に分からないため。Linux のファイル名としては別物なので技術的には
  共存できるが、共存させる利点がない

### 実装は正規表現を使わない

**バイト単位のループで書く。** `regexp` を使わない理由:

- パターンを 1 文字直すと通る範囲が変わり、しかもその変化がテストを書かないと見えない。
  `.` を素で書いた、`^`/`$` が複数行で行頭行末に化ける、といった事故がそのまま
  「パスへ通してはいけない文字を通す」に直結する
- Go の `regexp` は入力を UTF-8 としてデコードするので、不正なバイト列が `.` に
  どう当たるかを別途考えることになる。ASCII だけを通したいのに、UTF-8 の話が挟まる
- 通す文字が 3 記号 ＋ 英数字しかない以上、ループのほうが短く、読めば分かる

```go
// ValidateName は一覧に登録できる名前かどうかを返す。
// 名前は pidfile のファイル名になるので、通す文字を列挙して検査する。
func ValidateName(name string) error {
	if len(name) == 0 || len(name) > 30 {
		return errNameLength
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			continue
		case c == '-' || c == '_' || c == '.':
			if i == 0 {
				return errNameLeading // 先頭は英数字だけ
			}
		default:
			return errNameChar
		}
	}
	return nil
}
```

`for i := 0; i < len(name); i++` と**バイトで回す**（`for _, r := range name` にしない）。
range で回すと非 ASCII が 1 つの `rune` として現れ、`default` へ落ちるので結果は同じだが、
「ここで見ているのはバイトである」ことがコードから読めなくなる。

### どこで検査するか

- `hso setup` の名前入力 — その場で弾き、理由を出して打ち直させる
- **`registry.Load` — 手で書いた `config.toml` も検査する。** 未知キーをエラーにするのと
  同じ理由で、通らない名前を黙って読み込まない。`start` / `list` がパスを組み立てる前に
  落ちるほうが、`/tmp/hso-1000/../../etc/x.pid` のような文字列を作ってから気付くより良い
- エラー文はユーザー向けなので `internal/msg` に置く。「使える文字は英数字と `-` `_` `.`、
  先頭は英数字、30 文字まで」と**規則そのものを出す**（「不正な名前です」だけにしない）

---

## 各コマンド

### `hso setup`

現行のウィザード（ディレクトリ入力 → 起動スクリプト選択 → TOML プレビュー）に
**名前の入力**を足し、作成後に一覧へ登録する。名前が既にあれば上書きせずエラー。
名前の初期値はサーバーのディレクトリ名とする

名前に使える文字と長さは「サーバー名の規則」で決めたとおり（ASCII の英数字と `-` `_` `.`、
先頭は英数字、30 バイトまで）。ディレクトリ名を初期値にするので、**そのままでは通らない
ディレクトリ名がある**（日本語のディレクトリ名、空白入り）。その場合は初期値を空にして
入力させる。ディレクトリ名を機械的に変換して埋める（空白を `_` にするなど）ことはしない。
勝手に変えた名前を、後から `hso start` で打つのはユーザーだから。

「設定ファイルが無く、標準入出力が端末なら自動でウィザードへ入る」経路は
**`-config` を付けたときだけ残す**。引数なしの `./hso` はヘルプへ変えた（→「ヘルプ」）。

### `hso help`

**引数なしの `hso` と `hso help` は、コマンド一覧を出すだけで何も起動しない。**
`-h` / `-help` / `--help` も同じところへ着く。`flag` パッケージの既定の使い方表示
（`-config` の 1 行だけ）より、コマンド一覧を出すほうが探しているものに近い。
同じ理由で `-v` / `--v` / `-version` / `--version` は `hso version` と同じ出力にする。
`--help` が効くなら `--version` も効くと思うのが自然なので、ここで flag のエラーに
落とさない。

**後ろに何が付いていてもヘルプとバージョンは同じものを出す。** `hso help start` の
ような打ち方をエラーにしても、読みたいものは変わらない。

引数なしでウィザードが始まる形をやめたのは、**打ち間違いの行き先としてセットアップが重い**
ため。`hso start` のつもりで `hso` と打った人が、いまいるディレクトリに `hso.toml` を作る
ウィザードへ入ってしまう。ヘルプなら、そこから `hso setup` にも `hso start` にも進める。

- 一覧の文言は `internal/msg` の `CommandHelp` に置く。ja / en が同じ識別子を宣言する契約に
  乗るので、片方への書き忘れはコンパイルで落ちる（→ [i18n.md](../.claude/docs/i18n.md)）
- **オプションは主要なものだけ**を載せ、それぞれの細かい説明は
  [commands.md](commands.md) に置く。ヘルプが 1 画面に収まらないと読まれない
- **存在しないコマンドは、コマンド名と `hso help` の案内を出して終了コード 1。**
  ヘルプ全文は出さない（エラーが流れて読めなくなる）。利用できるコマンドの一覧を
  エラー文の中に持たないので、コマンドが増えたときに直す場所は `CommandHelp` だけになる

### `hso start [name]`

- 名前あり → 一覧から `hso.toml` のパスを引いて起動する
- 名前なし → 一覧を出して選ばせる。端末でなければエラー（選ばせられない）
- **すでに起動中のサーバーを選んだらエラー。** 1 サーバー = 1 hso を保つ。走っている hso に
  後からアタッチする機能は作らない（stdin を握れるプロセスは 1 つだけで、2 つ目を許すと
  コマンドの送り先が不定になる）。起動中のものも一覧には出す。**選べないのではなく、
  選ぶと「すでに起動中」と言われる。** 一覧から消すと「登録したはずのものが無い」に見える

**一覧は Bubble Tea で出す。** セットアップウィザードが既に Bubble Tea なので、矢印で選んで
Enter という操作がツール全体で揃う。番号を打たせるプロンプトだと、この後に起動する TUI との
間で操作の作法が変わる。状態（起動中 / 停止 / 設定が見つからない）を行に添えて色を付けられる
のも、`list` の表示とそのまま共有できる。

起動そのものは現行の TUI 経路をそのまま呼ぶ。`start` は「どの `hso.toml` を渡すか」を
決めるだけの層に留める。

### `hso list`

| 列 | 中身 |
|---|---|
| 名前 | 登録名 |
| 状態 | 起動中（PID）/ 停止 / 設定が見つからない |
| パス | `hso.toml` の場所 |

### `hso java`

引数なしでは対話 UI を開かず、端末かどうかに関係なく `change` と `list` の使い方を stdout へ表示する。配下のサブコマンドはこの 2 つだけとし、`change` には登録済みサーバー名を 1 つまで指定できる。

`hso java change [name]` は registry の登録済みサーバーだけを対象にし、サーバーと `/usr/lib/jvm` から自動検出した JVM を Bubble Tea で選択して、`config.SetJava` で `hso.toml` に保存する。現在値を初期選択にして印を付け、起動中なら変更が次回起動から反映されることを表示する。キャンセル、設定エラー、検出 0 件では変更しない。対話できる端末がなければエラーにする。

`hso java list` は JVM をバージョン降順、同一ならパスの辞書順で表示し、正規化した JAVA_HOME が一致する登録済みサーバーを並べる。設定無し、ファイル無し、設定エラーは stderr に警告して紐付けず、一覧表示は続ける。両コマンドでは、自動検出が `/usr/lib/jvm` だけであり、SDKMAN、asdf、`/opt` などは対象外で、手動指定には `hso.toml` の `[server] java` へ JAVA_HOME の絶対パスを書くことを案内する。

### `hso version`

バージョン・表示言語・アーキテクチャを出す。`update` が何を落としてくるかが
これで読めるようにする
新バージョンがあればアップデートを促す

### `hso update`

1. 最新リリースのタグを取る
2. 自分と同じ arch・言語の `hso_<version>_linux_<arch>_<lang>.tar.gz` を落とす
3. `checksums.txt` と照合する
4. 一時ディレクトリへ展開し、実行中のバイナリと同じ場所へ rename で置き換える

決めごと:

- **チェックサム照合は必須。** 落としたものをそのまま実行可能な場所へ置かない
- **バイナリは自分の言語を知る必要がある。** `msg` に `Lang`（`"ja"` / `"en"`）を足す。
  `msg_ja.go` / `msg_en.go` が同じ識別子を宣言する契約に乗るので、片方の書き忘れは
  コンパイルで落ちる（→ [i18n.md](../.claude/docs/i18n.md) の方針どおり）
- Linux では実行中のバイナリでも rename で差し替えられる（元の inode が生きたまま残る）。
  走っている hso は影響を受けず、次の起動から新しいものになる
- 取得の失敗で既存のバイナリを壊さない。置換は最後の 1 手だけにする
- **rename の前に置く一時ファイルは、置き場のディレクトリに一意名で作る**（`os.CreateTemp`
  / `mktemp`）。固定名にすると、そこに既存バイナリへの symlink を置かれたときに
  書き込みが symlink をたどり、**「最後の 1 手」より前に現在のバイナリを壊せる**。
  一意名かつ `O_EXCL` なら symlink をたどらない。install.sh の設置も同じにする
- **`sudo hso update` を案内で済ませない。** install.sh と同じ問題で、それだと取得・展開・
  照合まで全部 root で走る。`update` も**置換の一手だけを昇格させる**（`sudo` / `doas` を
  `exec` して `cp` → `chmod` → `mv` を通す）。→「特権が要るのは書き込みだけ」
- 昇格の手段が無ければ、そこで初めて「root で実行し直してください」と出して終わる。
  ツールが勝手に権限昇格の可否を決めない

#### 最新バージョンの取得先: GitHub API（決定）

**`https://api.github.com/repos/hijoushoku7/hijo-server-ops/releases/latest` を叩き、
`tag_name` を読む。install.sh と `hso update` の両方でこれに揃える。**
以下は比較の記録。

「最新のタグ名は何か」を知る手段が 2 つある。ダウンロード自体はどちらでも
`.../releases/download/<tag>/<asset>` を叩くので、**違うのはタグ名の引き方だけ**。

| | GitHub API (`/repos/:owner/:repo/releases/latest`) | リダイレクト (`/releases/latest` の `Location`) |
|---|---|---|
| 返るもの | JSON。タグ名・アセット一覧・URL・公開日時・プレリリース種別 | HTTP 302 の `Location` ヘッダに `/releases/tag/v1.2.3` |
| 認証 | **不要**（公開リポジトリなら API キーなしで叩ける） | 不要 |
| レート制限 | **未認証で 60 req/h・送信元 IP 単位**。GitHub REST API 全体で共有 | 実質なし（通常の HTTP） |
| 依存 | `encoding/json` のみ（標準ライブラリ） | なし。ヘッダを 1 本読むだけ |
| 実装量 | 構造体定義 ＋ アセット名の照合 | タグ名を切り出す 1 関数 |
| 壊れやすさ | JSON スキーマは安定。GitHub の公開 API 契約に乗る | **URL の形に依存する。** 仕様として保証されたものではない |
| 取れる情報 | リリースノート・公開日・アセットのサイズ / ダウンロード数まで | タグ名だけ |
| プレリリース | `latest` は除外して返す。含めたければ一覧 API へ | 同じく除外される |
| 資産の有無 | **叩く前に分かる**（アセット一覧が入っている） | 落としに行って 404 で初めて分かる |
| 企業 NW | API ドメインが塞がれている環境がある | `github.com` だけで済む |

**レート制限が現実の問題になるか**が判断の分かれ目。`hso update` は人が手で叩くコマンドで、
CI が回すものではない。共有ホストや NAT の裏でも 1 IP から 1 時間に 60 回叩く状況は想定しにくく、
超えたときも「時間をおいて」と出せば済む（バイナリは壊れていない）。

一方、**リダイレクト方式が省くのは JSON 構造体の 10 行程度**で、代わりに
「アセットが揃っているか」をダウンロードの 404 で知ることになる。arch × lang の 4 本のうち
1 本だけアップロードに失敗したリリースが出たとき、API なら「あなたの環境向けの資産が無い」と
先に言えるが、リダイレクトでは「ダウンロードに失敗した」としか言えない。

→ **API 方式に決定。** 制限は現実的に当たらず、当たっても壊れず、エラーメッセージの質で勝てる。
リダイレクト方式が効くのは、API ドメインが塞がれた環境を積極的に支えたい場合に限る。
`api.github.com` が塞がれているという報告が実際に出てきたら、そのときリダイレクトへの
フォールバックを足す（**タグ名を引く関数を 1 つ差し替えるだけ**の形にしておけば、
後から足しても他に波及しない）。

#### API を使うときの実測メモ

`api.github.com` に未認証で叩いて確認した挙動。

- **API キーは要らない。** 公開リポジトリの `releases/latest` は未認証で 200 が返る。
  返却ヘッダは `x-ratelimit-limit: 60` / `x-ratelimit-remaining` / `x-ratelimit-reset`
  （UNIX 時刻）/ `x-ratelimit-resource: core`
- **制限は「IP 単位」だが「このエンドポイント単位」ではない。** `core` という
  1 つの枠を REST API 全体で共有するので、同じ IP の別のツールが GitHub API を叩いていれば
  そのぶん減る。CGNAT や共有 VPS では他人と枠を分け合う可能性がある
- **`User-Agent` ヘッダが無いと 403 で弾かれる**（レート制限とは無関係に、常に）。
  curl は既定で付けるが、Go の `net/http` も既定の `Go-http-client/1.1` を付けるので実害はない。
  それでも `hso/<version>` を明示するほうが、GitHub 側から見て何が叩いているか分かる
- **アセットのダウンロードは枠を消費しない。** 実体は `objects.githubusercontent.com` への
  リダイレクトで、API ではない。つまり `update` 1 回で使う枠は**タグ取得の 1 回だけ**
- 超過時は 403 か 429 が返り、`retry-after` または `x-ratelimit-reset` が付く。
  **`x-ratelimit-remaining: 0` を見て「あと N 分で回復する」と出せる**
- **逃げ道**: `GITHUB_TOKEN` が環境変数にあれば `Authorization` に載せる（5,000 req/h）。
  install.sh にトークンを埋め込むことは**しない**（公開スクリプトに書けば即漏洩する）。
  あくまで、共有 IP で詰まった人が自分のトークンを渡せる口として用意する

#### install.sh 側で JSON をどう読むか

`hso update` は `encoding/json` で読める。問題は install.sh のほうで、**`jq` を要求しない**
（最小構成のサーバーに入っていない）。`tag_name` は英数字と `.` `-` だけの単純な文字列なので、
`sed` の 1 行で足りる。

```sh
# GitHub の JSON は 2 スペース整形で "tag_name": "v1.2.3" の形（実測）。
# 詰まった形 ("tag_name":"v1.2.3") でも通るよう [[:space:]]* を挟む。
tag=$(printf '%s\n' "$body" |
    sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
    head -n 1)
```

- **切り出したタグは、URL に入れる前に検査する。** 通すのは英数字と `.` `-` `_` だけ、
  かつ空でないこと。取ってきた文字列がそのままダウンロード URL と一時ファイル名に
  入るので、ここは名前の検査（→「サーバー名の規則」）と同じ姿勢で扱う
- **アセットの有無も同じ本文から見る。** JSON をいったん一時ファイルに落とし、
  `grep '"name"[[:space:]]*:[[:space:]]*"hso_<tag>_linux_<arch>_<lang>.tar.gz"'` で
  存在を確かめてからダウンロードへ進む。これで「API 方式を選んだ利点（落とす前に
  資産の欠けが分かる）」が install.sh 側にも乗る
- `head -n 1` を必ず付ける。`tag_name` は 1 リリース分の JSON に 1 つしか出ないが、
  将来 API の返却が変わっても複数行が変数へ入らないようにする
- `User-Agent` は curl が既定で付けるが、`-H "User-Agent: hso-install"` を明示する。
  GitHub 側から見て何が叩いているか分かるようにする（→ 実測メモ）
- `GITHUB_TOKEN` が環境にあれば `-H "Authorization: Bearer $GITHUB_TOKEN"` を足す。
  **スクリプトにトークンを埋め込むことはしない**

#### 前提: リポジトリを公開する必要がある

**現状このリポジトリは private で、未認証では API も raw もアセットも全部 404 になる。**
`curl -fsSL … | sh` は「誰でも認証なしで取れる」ことが前提なので、install.sh を出す前に
**public にする**（あるいは公開用のミラーを別途持つ）。private のままで配る方法は、
実質トークンをユーザーに用意させることになり、`curl | sh` の手軽さと両立しない。

これは取得先を API にするかリダイレクトにするかとは**独立した前提**で、どちらを選んでも要る。

### `hso uninstall`

**入れたものを消すコマンドを持つ。** `curl | sh` で入れた人に「消し方は README を読んで
`rm` を打て」と言わせない。入れる手順がコマンド 1 本なら、消す手順もコマンド 1 本にする。

やることは次の 4 つだけ。

1. 自分自身のパス（`os.Executable()`）を確かめ、消してよいか確認を取る
2. そのファイルを `unlink` する
3. `--purge` が付いていれば、設定ディレクトリと pidfile も消す
4. install.sh が設置したシェル補完ファイルを消す（`--purge` の有無にかかわらない）

#### 権限が無いときは、昇格せずエラーで止める

`/usr/local/bin/hso` を一般ユーザーが `hso uninstall` すると失敗する。ここで
**`sudo` を `exec` しない。エラーを出して終わる。**

```
Error: cannot remove /usr/local/bin/hso: permission denied.

hso is installed in a system directory, so removing it requires root.
Run it again with sudo:

    sudo hso uninstall

If sudo is not available on this machine, run it as root.
```

- **`install.sh` や `hso update` が昇格するのは、昇格しない部分があるから。** あちらは
  ダウンロード・展開・照合を一般ユーザーで走らせ、**書き込みの一手だけ**を root に渡すことに
  意味がある。`uninstall` は実体が `unlink` 1 回しかない。**分ける対象が無いものを分けても
  何も守れず**、消す操作のためにツールが勝手に `sudo` を起動する挙動だけが残る
- `sudo hso uninstall` は**そのまま通る**。`/usr/local/bin` は `ENV_SUPATH` に入っている
  ので、root の PATH からも `hso` が引ける（→「PATH をどう扱うか」）
- **これは「hso 本体を `sudo` で動かす想定にしない」の唯一の例外。** `sudo hso start` が
  駄目なのは、root の `~/.config` を見にいってサーバー一覧が見つからないから。
  `uninstall` の既定はバイナリを消すだけで**一覧も pidfile も読まない**ので、
  root で走っても行き先が変わらない（`--purge` だけは別扱い → 下記）
- 判定は EUID ではなく**実際に消せるか**で行う。`unlink` に要るのは親ディレクトリの
  書き込み ＋ 実行権限で、ファイル自身の権限ではない。ACL・`root_squash` の NFS・
  `chattr +i` など EUID から読めない事情があるので、**まず `unix.Access(dir, W_OK|X_OK)` で
  先に見て**確認プロンプトの前に落とし、それでも `unlink` が `EACCES` / `EPERM` を返したら
  同じ文面を出す。どちらの経路でも同じことが起きたと読めるようにする
- 終了コードは非 0。スクリプトから叩かれたときに成功と区別できるようにする

> **文面は英語**（この節のとおり）。`internal/msg` を通さず、`uninstall` の実装に
> 英語リテラルで置く。ja バイナリでもここだけ英語になるので、CLAUDE.md の
> 「ユーザー向け文字列は日本語」から外れる**意図的な例外**として扱う。
> 揃えたくなったら `msg` に移すだけで済む形にしておく（→ 実装メモ）。

#### 消す対象

| | 既定 | `--purge` |
|---|---|---|
| バイナリ（自分自身） | 消す | 消す |
| bash / zsh / fish の補完ファイル | **消す** | **消す** |
| `~/.config/hso/config.toml`（サーバー一覧） | **残す** | 消す |
| pidfile のディレクトリ | **残す** | 消す |
| サーバーディレクトリの `hso.toml` / ワールド | **触れない** | **触れない** |

- **既定で設定を消さない。** 入れ直す人のほうが多く、消えて困るのは一覧のほうだから。
  「消したはずのものが残っている」より「入れ直したら一覧が消えていた」のほうが痛い
- **サーバーディレクトリには何があっても触れない。** ワールドと `hso.toml` は
  ユーザーの資産で、アンインストーラが消してよいものではない（spec.md の
  「hso がユーザーのファイルに触れるのは起動スクリプトへの実行権限付与だけ」）
- **`--purge` はバイナリも消す。** 「設定だけ消してバイナリは残す」形（`--keep-binary`）は
  作らない。**消したのに `hso` と打てる状態が残るのがいちばん分かりにくい**（一覧が
  無いだけの hso が PATH にいて、`hso list` が空で返る）。`--purge` は
  「この機械から hso を消す」1 つの意味だけを持つ
- **`--purge` は root で走らせない。** `sudo hso uninstall --purge` は root の
  `~/.config/hso` を見にいくので、消したい一覧が消えずに終わる（`sudo` は既定で
  `HOME` を root のものに差し替える）。EUID が 0 のときに `--purge` が付いていたら
  **エラーで止める**:

```
Error: --purge must not be run as root.

As root, hso would look for the config in root's home directory, not yours.
Run it as your normal user first:

    hso uninstall --purge

If that reports the binary itself needs root, finish with:

    sudo hso uninstall
```

`SUDO_UID` から呼び出し元のホームを推測して消しにいく、という実装はしない。
**消す対象を環境変数から推測するのは、消す操作でやってよい推測ではない。**

#### `/usr/local/bin` に入っているときの `--purge`

一般ユーザーの `hso uninstall --purge` は、設定は消せるがバイナリは消せない。
**この食い違いは、消し始める前に分かる**（→ 「権限が無いときは、昇格せずエラーで止める」の
`Access(dir, W_OK|X_OK)`）ので、**確認プロンプトの時点で先に言う**。設定を消してから
「バイナリは消せませんでした」と言うのが、いちばん腹の立つ順番になる。

```
hso is installed at /usr/local/bin/hso, which requires root to remove.
Your config can be removed now, without root.

  Will remove now:   ~/.config/hso, /run/user/1000/hso
  Needs root:        /usr/local/bin/hso  ->  sudo hso uninstall

Continue? [y/N]:
```

進めた場合は、設定と pidfile を消し切ってから最後に `sudo hso uninstall` を再掲し、
**終了コードは非 0**（全部は終わっていないため）。`~/.local/bin` に入っている
通常のユーザーインストールでは、この分岐そのものが出ずに 1 コマンドで終わる。

#### 実行中のサーバーがあるとき

**一度だけ注意を出し、そのうえで消す。止めることも、止めるまで待つこともしない。**
Linux では実行中のバイナリを `unlink` しても、開いている inode は生き続ける。走っている
hso とその配下の Minecraft サーバーは何の影響も受けず、消えるのはパス（名前）だけ。
だから「先にサーバーを止めてください」と要求する理由がない。**要求すると、
消したいだけの人がサーバーを畳まされる。**

```
hso will be removed from /home/alice/.local/bin/hso

  !! 2 servers are running right now: survival, creative.

     They keep running. Only the command is removed, so `hso list`
     will no longer show them -- go back to the terminal each one
     is running in to stop it.

Remove? [y/N]:
```

決めごと:

- **注意は 1 回だけ。** 上のプロンプトで `y` を打った後は、実行中かどうかを二度と見ずに
  最後まで消し切る。「本当にいいですか」を重ねると、読まずに `y` を押す癖が付くだけで、
  1 回目の注意まで効かなくなる
- **生死の確認は、何かを消す前に済ませる。** `--purge` は pidfile を消すので、順番を
  間違えると「消した後にもう一度数える」ことができない。**起動中の一覧を先に作り、
  それを持ったまま削除へ進む**
- 走っているサーバーが `hso list` から見えなくなることを、この場で言い切る。
  `--purge` の実害はほぼここ 1 点で、プロセスが壊れることではなく**追跡できなくなること**
- **一覧が読めなくても `uninstall` は通す。** `config.toml` が壊れていたり未知のキーを
  持っていたりして `registry.Load` が落ちても、そこで `uninstall` を止めない。一覧を
  読んでいるのは**この注意を出すためだけ**で、`--purge` はディレクトリごと消すので
  中身を必要としない。`uninstall` は「もう使わないので消す」ための最後の逃げ道であり、
  **設定が壊れているときこそ通らないといけない**。読めなかったときは黙って飲み込まず、
  「一覧が読めなかったので実行中のサーバーは確認できていない」と 1 行出してから進む
- 走っている hso 側は、自分の pidfile が消えていても落ちない作りにする。終了時の
  `unlink` は「無ければ何もしない」、1 時間ごとの `utime`（→「置き場所」）も
  失敗を無視する。**pidfile は起動時の排他と状態表示に使うが、起動後の hso の動作の
  前提にはしない**

#### 確認と、消してよいパスかどうか

- **既定で確認を取る。** `curl | sh` と違って stdin は端末なので、`[y/N]` を出せる。
  `-y` / `--yes` で省略できる。**端末でないとき（スクリプトから叩かれたとき）は
  `-y` が無ければエラー**にする。無人環境で黙ってファイルを消さない
- **消すパスを必ず全部出す。** `hso uninstall` は `os.Executable()` を消すので、
  リポジトリでビルドした `./hso_ja` から叩けばそれが消える。パスを出しておけば、
  そのつもりが無かった人が `N` で止められる
- **`os.Executable()` がシンボリックリンクだったら、消さずに終わる。** リンクの実体を
  消せばリンクが宙に浮き、リンクを消せば実体が残る。どちらを望むかは hso には決められない
  （そのリンクを作ったのは hso ではない）。両方のパスを出して、手で消すよう案内する
- **`y` の後は途中で止まらない。** 消す対象のどれかで失敗しても、そこで抜けずに残りを
  消し切り、**何を消して何が残ったかを最後にまとめて出す**。途中で抜けると、
  どこまで進んだか分からない状態で終わる（もう一度叩けば直るが、そのとき何が起きるかを
  利用者が予測できない）
- 消したあとに、一覧に登録が残っていれば 1 行出す:
  `Server list kept at ~/.config/hso/config.toml (use --purge to remove it).`
  **消し残しがあることを、消した本人に黙っていない**

#### `uninstall.sh` は置かない

コマンドがあるので、なおさら要らない。**入っていない状態から叩くものは install.sh だけ**で、
入っている状態の操作は全部 `hso` 自身のサブコマンドにする。

---

## 起動中かどうかの判定

### 何を見るか

**hso のプロセスを見る。java ではない。** java は hso の孫（hso → supervisor → 起動スクリプト
→ java）であり、hso が生きていれば必ずいずれかの状態（起動中・再起動待ち・終了モーダル）に
居る。java を数えると、終了モーダルを出したまま待っている hso が「停止」に見えてしまい、
その状態で `hso start` を通してしまう。

### pidfile ＋ 起動時刻の照合

pidfile を `$XDG_RUNTIME_DIR/hso/<name>.pid` に置く（未設定なら `/tmp/hso-<uid>/`、
パーミッションは 0700 → 「置き場所」）。中身は **hso 自身の PID と、
`/proc/<pid>/stat` の起動時刻**の 2 つ。

判定は次の順。

1. pidfile が無い → 停止
2. pidfile が空、または内容が壊れている → 停止（**pidfile は消さない**）
3. `/proc/<pid>` が無い → 停止（pidfile を消す）
4. `/proc/<pid>/stat` の起動時刻が pidfile の値と違う → 停止（**PID 再利用**。pidfile を消す）
5. 一致 → 起動中

空や壊れた内容は、作成側がロックを取った直後で PID をまだ書いていない窓かもしれない。
読み取り側は一瞬「停止」と表示してよいが、他のプロセスが書いている可能性があるファイルは
消さない。

**PID の存在確認だけでは足りない。** PID は一周して再利用されるので、hso が異常終了して
pidfile が残った後に無関係なプロセスが同じ PID を取ると、それを「起動中」と読んでしまう。
起動時刻（boot からの clock tick 数）まで一致することを要求すれば、この取り違えは起きない。

### 既存コードを再利用する

**この仕組みはすでにこのコードベースにある。** `internal/process/java.go` の
`parseProcStat` が `/proc/<pid>/stat` から ppid / pgrp / **startTime**（`stat` 全体では
22 番目のフィールド）を読んでおり、`JavaFinder.ExpectedStartTime` と突き合わせて PID 再利用を検出し
`ErrRootPIDReused` を返す経路が動いている。**同じ判定を pidfile の検証に使う。**

`stat` のパースには `comm` にスペースや括弧が入りうるという罠があり、既存の実装は
最後の `)` の後ろから数えることでこれを避けている。**自前で `strings.Fields` するのではなく、
このパーサを共有できる形へ切り出す。**

### 置き場所

```
$XDG_RUNTIME_DIR/hso/<name>.pid     # 通常（= /run/user/<uid>/hso/）
/tmp/hso-<uid>/<name>.pid           # $XDG_RUNTIME_DIR が無い / 使えないとき
```

`$XDG_RUNTIME_DIR` が**設定されていて、かつ書けるディレクトリを指している**ときだけそちらを
使い、それ以外は `/tmp` へ落ちる。変数が刺さっているのに使えない（設定だけ残って中身が
消えている）ケースがあるので、有無ではなく**書けるかどうか**で分ける。

#### `/tmp` を使うときの検査

`/tmp` は誰でも書ける。`/tmp/hso-<uid>` を**先に他人が作っていた**場合、`mkdir` は
`EEXIST` で失敗し、そのまま書けばそのディレクトリへ書くことになる。だから `mkdir` の後に
**`lstat` で「シンボリックリンクでない」「所有者が自分」「モードが 0700」の 3 つを確かめ、
1 つでも違えばエラーで止める**（黙って別の場所へ逃がさない。`list` に嘘の状態が出るより、
そこで止まって理由が出るほうが良い）。`$XDG_RUNTIME_DIR` 側は logind が 0700 で作った
自分専用のディレクトリなので、この検査は要らない。

#### pidfile が消える 2 つの経路

どちらも「**動いているのに停止と出る**」に倒れる。壊れはしないが、その状態で
`hso start` を通すと 1 サーバー = 1 hso が破れるので、経路として書いておく。

| 経路 | 何が起きるか |
|---|---|
| `/tmp` の定期掃除 | systemd の `/usr/lib/tmpfiles.d/tmp.conf` が `q /tmp 1777 root root 10d`。`systemd-tmpfiles-clean.timer` が 1 日 1 回走り、**atime / mtime / ctime が全部 10 日より古いものを消す**（Ubuntu 26.04 で確認。`/tmp` は tmpfs、timer は active）。Minecraft サーバーは 10 日以上動きっぱなしが普通にあるので、これは実際に踏む |
| ログアウトで `/run/user/<uid>` が消える | linger が無効なユーザーは、最後のセッションが切れた時点で logind が runtime ディレクトリごと消す。**tmux / screen に入れて ssh を抜ける**という、まさにこのツールの使い方で踏む |

対処は次の 2 つ。

- **生きている間は pidfile の mtime を更新する。** hso は元から常駐している TUI なので、
  1 時間ごとに 1 回 `utime` を打つゴルーチンを `internal/pidfile` に持たせれば足りる
  （UI 側には手を入れない）。10 日の判定は atime / mtime / ctime の**全部**が古いことを
  求めるので、1 時間ごとの更新で掃除の対象から外れる
- **ログアウトのほうはツール側では防げない。** `/run/user/<uid>` を消すのは logind で、
  hso が握っているファイルがあっても消える。README の「tmux で動かす」案内に
  `loginctl enable-linger <user>`、または systemd unit で動かす形を添える。
  hso 自身は**消えた pidfile を「停止」と読むだけ**で、それ以上は何もしない

pidfile が残っているのに `hso start` が書きかけを「停止」と読んで起動へ進んでも、作成時に
pidfile 自身のロックを取れなければ「すでに起動中」で止める。`Running` は一覧表示と起動前の
確認のためのもので、**排他の責任は作成時のロックが持つ**。

一方、動作中に pidfile のパス自体が消えれば、既存のロックは削除された inode に残り、次の
起動は新しい pidfile を作れてしまう。この場合の最後の砦は Minecraft 側の `session.lock` で、
後から起動したサーバーがワールドを掴まずに落ちる。上の 2 つでは、この経路へ入る確率を
下げておく。

### 書き込みと後始末

pidfile は hso の起動時に `O_RDWR|O_CREATE` で開き、**pidfile 自身**へ
`LOCK_EX|LOCK_NB` を掛けて作成を排他にする。ロックを取れなければ、別の hso が同じ pidfile を
握っているので「すでに起動中」として止める。そのファイルから正しい PID と起動時刻を読めた
場合だけ PID も表示し、空・書きかけ・壊れた内容なら PID 無しで止める。

ロック取得直後には、開いた fd と現在のパスのデバイス番号・inode 番号を照合する。一致しない、
またはパスが消えていれば fd を閉じ、上限付きで pidfile を開くところからやり直す。

ロックを取れた側は `truncate(0)` の後に PID と起動時刻を書き、fd を開いたままロックを
握り続ける。異常終了・OOM Kill・`kill -9` ではカーネルが fd を閉じてロックを外すため、次の
起動は残った古い pidfile をロックして上書きできる。

`O_EXCL` と「古い内容を確認して消してから作り直す」の組み合わせは使わない。POSIX には
「内容が一致するときだけ消す」原子的な操作が無く、2 つの起動処理が同じ古い内容を読んだ後、
片方が作った新しい pidfile をもう片方が消せるためである。pidfile 自身の `flock` なら、同時に
進める作成処理は 1 つだけになる。ただし `flock` は advisory lock であり、ロック中のファイルの
`unlink` 自体は止めない。そのため `Running` が古い pidfile を消すときも、対象を読み取り専用で
開いて `LOCK_EX|LOCK_NB` を取得する。取得できなければ削除せず、取得できた場合だけロック中に
内容と、開いた fd に対する現在のパスのデバイス番号・inode 番号を再確認してから消す。

終了時にはロックを握ったまま、PID と起動時刻が自分の内容と一致するときだけ pidfile を消し、
その後で fd を閉じてロックを外す。**消し残っても壊れない**のが上の照合の要点で、
異常終了・OOM Kill・`kill -9` のどれでも、次に `list` が見たときに起動時刻が合わず「停止」と
判定してファイルを掃除する。

ロックを別の `hso.toml` に掛ける案は採らない。pidfile とロックのどちらが真かという説明を
2 つ抱えることになるためである。pidfile 自身に掛ければ、状態表示と排他が同じ 1 つのファイルに
乗り、真実の在処は増えない。

---

## 配布とインストール

**README のクイックスタートを `curl -fsSL … | sh` に差し替える。** 「アーカイブを展開して
`./hso`」はサーバー 1 台をそのディレクトリで動かす手順で、PATH の通ったコマンドとして使う
形と噛み合わない。手動での展開は「手動インストール」として README の下の方へ残す。

### install.sh の置き場

リポジトリのルートに `install.sh` を置き、`main` ブランチの raw を叩かせる。

```
https://raw.githubusercontent.com/hijoushoku7/hijo-server-ops/main/install.sh
```

**URL がリリースごとに変わらないことが要点**なので、リリースアセットには入れない
（アセットの URL にはタグ名が入り、README に書いた瞬間に古くなる）。短い独自ドメインは
今は用意しない。

**この URL が未認証で引けること = リポジトリが public であることが前提**
（→「前提: リポジトリを公開する必要がある」）。

### スクリプトがやること

1. `uname -s` が `Linux` でなければエラー（hso は Linux 専用）
2. `uname -m` を `x86_64`→`amd64`、`aarch64`/`arm64`→`arm64` に写す。それ以外はエラー
3. 言語を決める。**既定は `en`**、`--lang ja` で日本語版（→「言語の選び方」）
4. 最新のタグを取る（**`hso update` と同じ経路を使う** → 未決定の「最新バージョンの取得先」）
5. `hso_<tag>_linux_<arch>_<lang>.tar.gz` と `checksums.txt` を落として **SHA-256 を照合**
6. インストール先を `mkdir -p` して、中の `hso` を `0755` で置く
7. 利用できる bash / zsh / fish の補完スクリプトを、それぞれの標準の場所へ置く
8. インストール先が `PATH` に無ければ、追記すべき 1 行を添えて警告する

補完ファイルの設置に失敗してもインストールは続け、警告と設置できたパスを表示する。
ユーザーインストールで zsh の補完を置いた場合だけ、`compinit` より前に
`fpath=(~/.local/share/zsh/site-functions $fpath)` を加えるよう案内する。rc ファイルは編集しない。

`--system` の有無による権限の検査は 1 より前、**何もダウンロードしないうちに**行う。

### インストール先の決め方

**自動で判定せず、ユーザーが叩き方で選ぶ。** 2 通りだけ用意する。

| | ユーザーインストール（既定） | システムインストール |
|---|---|---|
| 置き場所 | `$HOME/.local/bin` | `/usr/local/bin` |
| 叩き方 | `… \| sh` | `… \| sh -s -- --system` |
| 必要な権限 | なし。**誰でも入れられる** | **書き込みのときだけ** root（スクリプト全体を `sudo` で走らせない） |
| 使える範囲 | そのユーザーだけ | 全ユーザー |
| PATH | **通っていないことがある**（後述） | **既定で通っている** |

`HSO_INSTALL_DIR` を指定すればどちらの経路でも上書きできる（`/opt/hso/bin` など）。

### 言語の選び方

**install.sh の既定は英語版（`_en`）。日本語版は `--lang ja` で明示的に選ぶ。**

| 指定 | 落とす資産 |
|---|---|
| なし（既定） | `hso_<tag>_linux_<arch>_en.tar.gz` |
| `--lang ja` | `hso_<tag>_linux_<arch>_ja.tar.gz` |

- 公開リポジトリの `curl \| sh` を叩く人の母集団は英語話者のほうが広い。**読めない言語で
  出るより、読める言語で出るほうを既定にする。** 日本語話者は README を読んで
  `--lang ja` を足せるが、逆（日本語で起動してしまった英語話者）は何が起きたか分からない
- 環境変数 `HSO_LANG=ja` でも同じことができる。Dockerfile や Ansible からはフラグより
  環境変数のほうが渡しやすいため、両方受ける。**フラグが環境変数より優先**
- `--lang` に `ja` / `en` 以外が来たらエラー。存在しない資産名を組み立てて 404 を踏むより、
  受け取った値をそのまま出して弾く
- **ロケール（`$LANG`）から自動で決めない。** サーバーのロケールは `C` や `en_US.UTF-8` の
  ままであることが多く、日本語話者かどうかの判定に使えない。加えて「叩くたびに落ちてくる
  ものが変わる」挙動は、後から `hso update` したときの結果も読めなくする

**ビルドの既定（タグなし = ja、`-tags en` = en）は変えない。** `internal/msg` の
「ja が既定で en がビルドタグ」という契約はそのまま（→ [i18n.md](../.claude/docs/i18n.md)）。
ここで変えるのは**インストーラがどちらの資産を落とすか**だけで、
リリース資産は今までどおり arch × lang の 4 本を出す。

`hso update` は**自分の言語を引き継ぐ**（`msg.Lang` を見る）ので、インストール時の選択が
そのまま残る。言語を変えたければ install.sh を叩き直す。

`/usr/bin` ではなく **`/usr/local/bin`** にする。FHS では `/usr/bin` はディストリの
パッケージマネージャが管理する領域で、そこへ手で置くと将来同名のパッケージが来たときに
衝突する。`/usr/local/bin` は「パッケージ管理外のソフトを置く場所」として定義されており、
**そのために既定の PATH に入っている**（次節）。

### 特権が要るのは書き込みだけ

**`… | sudo sh` にしない。** それだとスクリプト全体が root で走る。root で走らせる必要が
あるのは `/usr/local/bin` への書き込み 1 手だけで、そこに至るまでの

- `curl` でのタグ取得とアーカイブのダウンロード
- `mktemp -d` と展開
- SHA-256 の照合

は全部一般ユーザーのままでよい。ネットワークから落としてきたものを root 権限で展開する
理由がない。

```sh
# パイプに sudo を噛ませない
curl -fsSL …/install.sh | sh -s -- --system
```

スクリプトは呼び出したユーザーとして走り、**最後の配置だけ昇格する**。

```sh
# 同一ファイルシステム上に置いてから rename する。cp で直接上書きしないのは、
# 実行中の hso を上書きすると ETXTBSY（Text file busy）になるため。
privileged sh -c '
    cp "$1" "$2/.hso.new" &&
    chmod 0755 "$2/.hso.new" &&
    mv -f "$2/.hso.new" "$2/hso"
' _ "$work/hso" "$dir"
```

昇格は 1 回の `sh -c` にまとめる。`cp` / `chmod` / `mv` を個別に `sudo` すると、
権限で走る範囲が散らばるうえパスワードを何度も聞かれうる。

### `privileged` の決め方

| EUID | 手段 |
|---|---|
| 0 | 何も噛ませずそのまま実行する |
| ≠ 0、`sudo` あり | `sudo` |
| ≠ 0、`sudo` 無し・`doas` あり | `doas` |
| ≠ 0、どちらも無い | **エラー。** root で実行し直すよう案内して終了 |

- **パスワードの入力はパイプ越しでも通る。** `sudo` はパスワードを stdin ではなく
  `/dev/tty` から読むので、`curl | sh` で stdin がパイプに塞がれていても端末から入力できる
  （stdin から読ませるのは `-S` を付けたときだけ）
- **昇格の確認はダウンロードの前に済ませる。** 先に `sudo -v` を通しておけば、パスワードを
  聞かれるのが最初の一瞬で済む。15MB 落とし終えてから「あなたは sudoers にいません」と
  言われるのが一番いらつく
- 端末が無い（cron や CI から叩かれた）場合は `sudo` がパスワードを読めずに落ちる。
  `sudo -n true` で試して、無理なら**その場で分かるエラー**にする

これは万能の対策ではない。**スクリプト自身が「何を root で置くか」を決めている以上、
スクリプトを信用していることに変わりはない。** 減らせるのは、展開や一時ファイルの扱い、
`umask`、環境変数まわりが root 権限で動くことによる事故のほうで、そこが減るだけでも
やる価値がある、という位置づけ。

### インストール先と権限の食い違い

| 選択 | EUID | 動作 |
|---|---|---|
| `--system` | ≠ 0 | 書き込みのときだけ昇格して `/usr/local/bin` へ（**通常の経路**） |
| `--system` | 0 | 昇格せずそのまま `/usr/local/bin` へ |
| 既定（ユーザー） | ≠ 0 | `$HOME/.local/bin` へ。昇格しない |
| 既定（ユーザー） | 0 | **エラー。** root で叩くと `/root/.local/bin` に入って一般ユーザーから見えない。`--system` を使うか一般ユーザーで叩くよう案内して終了 |

書き込み先の存在と権限は**置く直前ではなく最初に**確かめる。ダウンロードと展開が終わってから
「書けません」と言われるのが、待たされたぶんだけ体感が悪い。

### PATH をどう扱うか

**`/usr/local/bin` は PATH 設定が要らない。** ここが `/usr/bin` を選ばない実利でもある。

```
# /etc/login.defs（Ubuntu 26.04 で確認）
ENV_PATH    PATH=/usr/local/bin:/usr/bin:/bin:/usr/local/games:/usr/games
ENV_SUPATH  PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
```

ログイン時の PATH をここから組み立てるので、**一般ユーザーでも root でも最初から入っている**。
Debian 系・RHEL 系・Arch・Alpine のいずれも既定で `/usr/local/bin` を含む。つまり
システムインストールなら、入れた直後に別の端末を開かずとも `hso` が引ける。

**面倒なのは `~/.local/bin` のほう。** ここは通っている環境と通っていない環境がある。

- Debian / Ubuntu: `/etc/skel/.profile` に
  `if [ -d "$HOME/.local/bin" ] ; then PATH="$HOME/.local/bin:$PATH"; fi` が入っている。
  **`-d` の判定がログイン時に走る**ので、install.sh がその場でディレクトリを作った回は
  PATH に入らない。再ログインするまで `hso: command not found` になる
- Fedora / RHEL: `~/.bash_profile` が無条件に足すので、ディレクトリの有無に関わらず通る
- Arch / Alpine: 既定では通っていない

設計は次のとおり。

- **rc ファイルを勝手に書き換えない。** `~/.bashrc` や `~/.profile` はユーザーのもので、
  アンインストールが `rm` 1 行で済まなくなる。spec.md の「ユーザーのファイルに触れない」方針と
  install.sh でも揃える
- インストール先ディレクトリは**先に作る**（`mkdir -p`）。作らないと上記 Debian の分岐が
  次回以降も永久に効かない
- 置いた後に PATH を確かめる。`case ":$PATH:" in *":$dir:"*)` で見て、入っていなければ
  **そのシェル向けの案内を出す**（次節に文面）
- あわせて「いまの端末ですぐ使うなら」の 1 行も出す。Debian で「再ログインすれば直る」
  ケースでも、この 1 行で今すぐ動く
- bash / zsh はコマンドの場所をキャッシュするので、案内に `hash -r` を添える

### PATH に無いときの案内（文面案）

**この案内だけは install.sh が出す**ので、Go 側の `internal/msg` は通らない。
**文面は英語 1 種類にする** — install.sh は ja / en で 2 本に分かれず、`--lang` は
「どの資産を落とすか」を選ぶだけのフラグだから。ここを言語で出し分けると、
スクリプト内にメッセージの表を持つことになる（→ 見直すならこの前提から）。

#### シェルの見分け方

```sh
# $SHELL は passwd のログインシェルで、いま打っているシェルとは限らない。
# curl … | sh のとき $PPID は呼び出し元の対話シェルなので、そちらを先に見る。
detect_shell() {
    name=$(cat "/proc/$PPID/comm" 2>/dev/null || echo '')
    case "$name" in
        bash|zsh|fish|ksh|dash|sh) printf '%s\n' "$name"; return ;;
    esac
    basename "${SHELL:-sh}"
}
```

| 見分け | 恒久設定の書き先 | 追記する行 | いますぐ使う行 |
|---|---|---|---|
| `bash` | `~/.bashrc` | `export PATH="$HOME/.local/bin:$PATH"` | 同じ行 ＋ `hash -r` |
| `zsh` | `~/.zshrc` | `export PATH="$HOME/.local/bin:$PATH"` | 同じ行 ＋ `hash -r` |
| `fish` | `~/.config/fish/config.fish` | `fish_add_path "$HOME/.local/bin"` | `set -gx PATH "$HOME/.local/bin" $PATH` |
| その他 / 不明 | `~/.profile` | `export PATH="$HOME/.local/bin:$PATH"` | 同じ行 |

**fish は syntax が違う**ので、`export` の行をそのまま出すと貼り付けた瞬間にエラーになる。
出し分けの実利はほぼここに集中している（bash と zsh は同じ行で済む）。

#### 文面（PATH に無いとき）

```
==> hso v1.4.0 installed to /home/alice/.local/bin/hso

!! /home/alice/.local/bin is not in your PATH, so `hso` will not be found yet.

   To use it in this shell right now:

       export PATH="$HOME/.local/bin:$PATH"
       hash -r

   To keep it after you log out, add this line to ~/.bashrc:

       export PATH="$HOME/.local/bin:$PATH"

   Or install system-wide instead, where no PATH setup is needed:

       curl -fsSL https://raw.githubusercontent.com/hijoushoku7/hijo-server-ops/main/install.sh | sh -s -- --system
```

- **「見つからない」を先に言う。** インストールが成功した直後に警告を出すので、
  何が起きていて何が起きていないのかを 1 行目で分ける
- **rc ファイルへの追記はユーザーの手に残す**（→ 方針）。案内は「この行を、このファイルへ」
  まで具体的に言い切って、実行はしない
- **`--system` への逃げ道を最後に置く。** PATH を触りたくない人にとっては、これが
  いちばん短い解決になる

#### 文面（Debian 系で、`~/.profile` に既に入っているとき）

`~/.profile` の `if [ -d "$HOME/.local/bin" ]` は**ログイン時にしか評価されない**ので、
install.sh がその場でディレクトリを作った回は PATH に入らない。この場合は
「rc に足せ」と言うと二重に追記させることになる。`grep -q '\.local/bin' "$HOME/.profile"`
で見て、当たったときは別の文面を出す。

```
==> hso v1.4.0 installed to /home/alice/.local/bin/hso

!! /home/alice/.local/bin is not in your PATH yet.
   Your ~/.profile already adds it at login, but it was skipped this time
   because the directory did not exist until now.

   Log out and back in, or run:

       export PATH="$HOME/.local/bin:$PATH"
       hash -r
```

#### 文面（PATH に有るとき）

```
==> hso v1.4.0 installed to /home/alice/.local/bin/hso

   Get started:

       cd /path/to/your/minecraft/server
       hso setup
```

PATH の話は 1 文字も出さない。**問題が無いときに問題の説明を読ませない。**

**PATH が面倒なのを避けたい人には、README でシステムインストールを先に書く。**
`curl … | sh -s -- --system` は PATH の話が一切出てこない。ユーザーインストールは
「root 権限を取れない / 取りたくない人向け」として二番目に置く。

### sh の範囲（bash に依存しない）

**POSIX sh だけで書き切る。** `#!/bin/sh` で始め、bash 固有の記法は 1 つも入れない。
`curl … | sh` と案内する以上、実際に走るのは Debian / Ubuntu の `dash`、Alpine の
BusyBox `ash`、その他の環境の bash であり、**どれで走るかはこちらが選べない**。

| 使わない | 代わりに |
|---|---|
| `[[ … ]]` | `[ … ]` |
| 配列 `arr=(a b)` | 位置パラメータ（`set -- a b`）か、空白区切りの 1 変数 |
| `==`（test 内） | `=` |
| `echo -e` / `echo -n` | `printf` |
| `$'\n'` | `printf` か実際の改行 |
| `${var,,}` / `${var^^}` | `tr '[:upper:]' '[:lower:]'` |
| `<<<` / `<(…)` | パイプか一時ファイル |
| `function f() {}` | `f() {}` |
| `set -o pipefail` | **無い。** パイプの途中の失敗が要る箇所を作らない（下記） |
| `source` | `.` |
| `which` | `command -v` |

決めごと:

- **`set -eu` だけを使う。** `pipefail` は POSIX に無い。だから
  **「途中で失敗しうるコマンドをパイプの左に置かない」**書き方にする。ダウンロードは
  `curl -fsSL -o file` でファイルへ落とし、その終了コードを見る。
  `curl … | tar xz` のように繋ぐと、`curl` が 404 を踏んでも `tar` の結果しか見えない
- **`local` は使う。** POSIX には無いが dash / BusyBox ash / bash / ksh のすべてが実装して
  いる。ここだけは例外にする。関数の中で変数を切れないほうが、`curl | sh` で走る
  スクリプトとしては事故が大きい（グローバルの取り違えは静かに間違った場所へ入れる）
- **検査を CI に入れる。** `shellcheck -s sh install.sh` を `test.yml` に足す。
  上の表を人間の注意力で守り続けるのは無理で、`[[` を 1 つ書いた時点で落ちるようにする。
  `checkbashisms` は `local` を毎回指摘するので使わない（例外が 1 つあると警告を
  読み飛ばす癖が付く）
- **実機は Alpine（BusyBox ash）で確認する。** dash と ash はどちらも POSIX 寄りだが
  外れる箇所が違う。`docker run --rm -v "$PWD:/x" alpine sh /x/install.sh --help` の形で
  一度は通す

### `curl | sh` の作法

- **全体を関数に包み、最終行で `main "$@"` を呼ぶ。** 回線が途中で切れて半端に落ちてきた
  スクリプトが、そのまま実行されるのを防ぐ。末尾まで届かなければ何も走らない
- `set -eu`。**対話を入れない**（stdin はパイプが占有しているので `read` が使えない）
- 作業は `mktemp -d` の中で行い、`trap` で必ず消す
- **チェックサム照合は必須。** `sha256sum` が無ければ `shasum -a 256` を試し、どちらも
  無ければ照合せずに入れるのではなく**エラーで止める**
- 置換は `mv`（rename）で行う。途中で切れた半端なバイナリを `PATH` 上に残さない
- **再実行がそのまま更新になる。** 既に最新版が入っていれば、その旨を出して何もしない

### README のクイックスタート案

````markdown
## クイックスタート

### システムにインストールする（推奨）

`/usr/local/bin` に入る。全ユーザーが使えて、PATH の設定は要らない。

```bash
curl -fsSL https://raw.githubusercontent.com/hijoushoku7/hijo-server-ops/main/install.sh | sh -s -- --system
```

**`sudo` はパイプに付けない。** スクリプトは自分の権限で走り、`/usr/local/bin` へ
ファイルを置く最後の一手だけ `sudo` を使う（途中でパスワードを聞かれる）。

### 自分のホームにインストールする

root 権限が無いときはこちら。`~/.local/bin` に入る。

```bash
curl -fsSL https://raw.githubusercontent.com/hijoushoku7/hijo-server-ops/main/install.sh | sh
```

`~/.local/bin` が PATH に無い環境では、インストール後に追記すべき 1 行が表示される。

### 日本語版を入れる

表示は既定で英語。日本語にするなら `--lang ja` を足す。

```bash
curl -fsSL https://.../install.sh | sh -s -- --system --lang ja
curl -fsSL https://.../install.sh | sh -s -- --lang ja
```

環境変数 `HSO_LANG=ja` でも同じ（`… | env HSO_LANG=ja sh -s -- --system`）。

### 使いはじめる

サーバーのディレクトリで:

```bash
hso setup
```

ディレクトリと起動スクリプトを選ぶと、そのままサーバーが立ち上がる。
2 回目からはどこからでも `hso start`。
````

- **システムインストールを先に書く。** PATH の話が一切出てこないので、読者が詰まらない
- `sh -s -- --system` の `-s` は「スクリプトを stdin から読む」、`--` の後ろが
  スクリプトへの引数になる。`curl | sh` に引数を渡す定石で、rustup などと同じ形。
  フラグは並べられる（`-- --system --lang ja`）
- 環境変数で渡す形も併記する。`env` 経由にしておくと、シェルや `sudo` を挟む書き方に
  変えても同じ形で通る
- **README（日本語）には `--lang ja` を目立つ場所に置く。** 既定が英語なので、日本語で
  読んでいる人が「日本語版はどうやって入れるのか」で詰まらないようにする。英語版 README を
  用意するなら、そちらは既定のコマンドだけでよい

### アンインストール

**`hso uninstall` で消す**（設計は「各コマンド」の → [`hso uninstall`](#hso-uninstall)）。

| インストール先 | 手順 |
|---|---|
| `~/.local/bin` | `hso uninstall` |
| `/usr/local/bin` | `sudo hso uninstall` |

設定まで消すなら、**sudo を付けずに**:

```bash
hso uninstall --purge                 # バイナリ ＋ ~/.config/hso ＋ pidfile
```

`--purge` は**バイナリも一緒に消す**。`~/.local/bin` に入っていればこの 1 行で終わる。

`/usr/local/bin` に入っている場合だけ 2 手になる。`--purge` を root で走らせると
root のホームを見てしまうので、**先に一般ユーザーで**叩く（そのとき「バイナリには root が
要る」と先に出る）。

```bash
hso uninstall --purge                 # 設定と pidfile が消える
sudo hso uninstall                    # 残ったバイナリを消す
```

手で消しても同じことができる。バイナリが壊れて `uninstall` が動かないときはこちら。

```bash
rm ~/.local/bin/hso                        # または sudo rm /usr/local/bin/hso
rm -rf ~/.config/hso                       # サーバー一覧
rm -rf "${XDG_RUNTIME_DIR:-/tmp}"/hso      # pidfile（再起動でも消える）
```

- **サーバーディレクトリの `hso.toml` には触れない。** ユーザーのサーバー設定であり、
  入れ直せばそのまま使える。spec.md の「hso がユーザーのファイルに触れるのは起動スクリプトへの
  実行権限付与だけ」を、インストーラでも崩さない
- **設定の削除を既定の手順にしない。** 入れ直す人のほうが多い。README でも別の段に分ける
- **手で消す手順も README に残す。** `uninstall` はバイナリ自身が動くことが前提で、
  そのバイナリが壊れているときには使えない。`rm` の 3 行は最後の逃げ道として要る

### 権限まわりの注意（README にも 1 行入れる）

- `hso update` は**自分自身のパス**を置き換える。`~/.local/bin` なら昇格なしで通り、
  `/usr/local/bin` なら**置換の一手だけ**パスワードを聞かれる。`sudo hso update` と
  打つ必要はない（打っても動くが、取得から展開まで全部 root で走ることになる）
- **hso 本体を `sudo` で動かす想定にしない。** `sudo hso start` にすると一覧を root の
  `~/.config` から探すのでサーバーが見つからない。root が要るのは**バイナリを置く操作だけ**
- **例外は `sudo hso uninstall` の 1 つ。** これはバイナリを消す操作しかせず、
  一覧も pidfile も読まないので root で走って構わない。ただし `--purge` を付けた
  `sudo hso uninstall --purge` は**エラーで止まる**（root のホームを見てしまうため）
- 昇格が起きるのは**バイナリを置く操作だけ**なので、サーバー一覧も pidfile も
  叩いたユーザーのものが使われる。root の `~/.config` に一覧ができてしまう事故が起きない

### そのほか

- tar.gz の中身（`hso` / `hso.toml` / `README.md`）は変えない。install.sh は `hso` だけ取り出す
- `hso.toml.example` はアーカイブに残す。手動インストールと、引数なしの現行動作のために要る
- `scripts/build.sh` と `release.yml` に `-X main.version=…` を足す。release は
  `github.ref_name`、ローカルビルドは `dev`
- `release.yml` のリリースノート（`notes.md`）のインストール手順も install.sh 方式に直す
- CI（`test.yml` / `deps.yml` / `release.yml`）の構成は変えない。**足すのは
  `test.yml` への `shellcheck -s sh install.sh` 1 ステップだけ**（→「sh の範囲」）

---

## 実装順

各段でリリース可能な形に切る。**1〜5 はバイナリの既存の挙動を一切変えない。**

1. バージョン埋め込みと `hso version` — 最小で、`update` と install.sh の前提
2. `install.sh` と README のクイックスタート差し替え — 既存のリリース資産だけで書ける。
   ただし**リポジトリの public 化が前提**（未認証で取れないと `curl | sh` が成立しない）
3. `internal/registry`（一覧の Load / Save）
4. `hso list` — registry だけで書ける。状態表示は 5 の後で埋める
5. pidfile と生存確認
6. `hso setup` — 名前入力の追加と登録
7. `hso start` — 選択 UI
8. `hso update` — タグ取得と展開は install.sh と同じ手順をなぞる
9. `hso uninstall` — 単体で書けるので順番はどこでもよいが、`--purge` が消す対象
   （一覧・pidfile）が 3〜5 で確定してからにする。**README のアンインストール手順は
   2 の時点では `rm` で書いておき、ここで `hso uninstall` に差し替える**

2 を先に出せる（1 のバージョン埋め込みさえあれば、サブコマンドが 1 つも無くても
「入れて `./hso` と同じことができる」状態になる）。install.sh の取得ロジックを先に固めて
おくと、8 はそれを Go へ移すだけになる。

---

## 他の issue との関係

#51（エイリアス追加）は一覧と `~/.config/hso/config.toml` の設計がまるごと重なるので、
**この設計に吸収して close する。**

#52（入力候補の灰色表示）・#49（hjkl 操作）・#46（`/tell` をチャットへ）・#30（汎用コマンド
の選択）は `internal/ui` と `internal/serverlog` に閉じている。CLI 化との接点は
「`config.Config` を作って TUI を起動する」1 本の呼び出しだけなので、**前後どちらでも
難易度は変わらない**。

---

## 未決定・要検証

かつての未決定 6 件（取得先 / 名前の文字範囲 / pidfile のフォールバック / sh か bash か /
PATH の案内文 / flock の併用）は上の各節で決着した。残っているのは実機確認と、
外部条件が 1 つ。

flock は別のロックファイルや `hso.toml` ではなく pidfile 自身へ掛けることで決着した。
`Running` は表示と事前確認を担い、作成時の `LOCK_EX|LOCK_NB` が排他を担う（→「書き込みと
後始末」）。

1. **リポジトリの public 化**（→「前提: リポジトリを公開する必要がある」）。
   これだけは設計ではなく判断待ちで、**実装順の 2 が丸ごとこれに乗っている**
2. **Alpine（BusyBox ash）での install.sh の実走**。dash では通って ash で落ちる書き方が
   ないことを確認する（→「sh の範囲」）
3. **`/tmp` フォールバック時の mtime 更新が効くこと**の確認。tmpfiles の掃除条件
   （atime / mtime / ctime が全部 10 日より古い）は `systemd-tmpfiles --clean --dry-run` で
   狙って再現できる。1 時間おきの `utime` で対象から外れることを一度は見る
4. **PATH 案内の文面**は英語 1 種類で下書きしただけ（→「PATH に無いときの案内」）。
   語調と行数は要調整
