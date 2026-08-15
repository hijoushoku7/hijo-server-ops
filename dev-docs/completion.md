# シェル補完 設計

`hso` のサブコマンド・フラグ・**登録済みサーバー名**を bash / zsh / fish で Tab 補完できるようにする。
コマンド体系は [cli.md](cli.md)、配布とインストールも同じく cli.md の「配布とインストール」を前提にする。

## 目的

`hso start` の引数は登録名で、いま何を登録したかは `hso list` を叩かないと分からない。
打ち間違えれば「そのサーバーは登録されていません」で止まる。**候補はツールが持っているのだから、
Tab で出す。** サブコマンドとフラグの補完はそのついでに付いてくる。

**やらないこと**:

| やらないこと | 理由 |
|---|---|
| cobra / urfave-cli の導入 | 補完のためだけに引数解析を全面書き換えることになる。いまの `dispatchCommand` は 60 行で読み切れる |
| rc ファイル（`.bashrc` / `.zshrc` / `config.fish`）の自動編集 | install.sh が `.profile` を書き換えず「追記すべき 1 行」を出すのと同じ姿勢を通す |
| 候補のキャッシュ・常駐 | 読むのは TOML 1 ファイル。キャッシュを持つと「消したサーバーが候補に残る」を作るだけ |
| `-config` のパス補完を自力で書く | ファイル名の補完はシェルが自前で持っている。`_files` / `compgen -f` に渡す |
| 起動中サーバーの候補からの除外 | cli.md の「一覧から消すと『登録したはずのものが無い』に見える」と同じ。**選べて、選んだら「すでに起動中」と言われる**ほうが読める |

## 確定仕様

| 項目 | 決定 | 理由 |
|---|---|---|
| 候補の決定 | **Go 側に 1 箇所**。シェルスクリプトは候補を持たない | コマンドが増えたとき直す場所を 3 本のスクリプトに散らさない |
| 問い合わせ口 | 隠しサブコマンド **`hso __complete <words...>`** | すでに `__hso_supervise` で自分自身を再実行する構造がある。分岐が 1 段増えるだけ |
| スクリプトの配布 | **`hso completion <shell>`** が `go:embed` したスクリプトを stdout へ出す | 配布物はバイナリ 1 本のまま。スクリプトの版とバイナリの版がずれない |
| スクリプトの中身 | **薄い委譲だけ**。単語列を `__complete` へ渡して結果を並べる | `hso update` でコマンドが増えても、設置済みスクリプトを置き換えずに追随する（→「なぜ委譲するか」） |
| 絞り込み | **シェル側に任せる。** `__complete` は位置に対する候補を全部返す | 同じ前方一致の規則を Go とシェルの 2 箇所に持たない |
| 出力形式 | 1 行 1 候補、`候補<TAB>説明`。説明は省略可 | 3 つのシェルが揃って解釈できる最小の形 |
| エラー | **候補ゼロ・終了コード 0。** stdout には何も出さない | プロンプトの下に Go のエラー文が噴き出す事故を作らない |
| 副作用 | registry を読むだけ。TUI・ネットワーク・pidfile・`/proc` に触らない | Tab のたびに走る。生死判定は `list` の仕事 |
| 設置 | install.sh が置く。`hso uninstall` が消す | 入れる手順がコマンド 1 本なら、消えるのも 1 本で消える（cli.md「uninstall」） |
| 説明文 | `internal/msg` に置いて ja / en 契約に乗せる | zsh と fish は説明を表示する。ユーザー向け文字列なので msg の担当 |

### なぜ委譲するか（静的生成にしない理由）

補完スクリプトに候補を焼き込む形（`hso completion zsh` がコマンド一覧を展開した完成品を出す）でも
動くが、**`hso update` でバイナリだけ新しくなり、設置済みスクリプトが古いまま残る**。
補完に出ないコマンドは「無い」と読まれるので、これは黙って壊れる壊れ方になる。

薄いスクリプトが持つのは「`__complete` を呼んで結果を並べる」手順だけで、コマンドが増減しても
中身が変わらない。置き換えが要るのは出力形式そのものを変えるときだけで、それは滅多に起きない。

代償は Tab ごとに 1 回 exec が増えること。静的リンクの hso は起動して TOML を 1 つ読むだけなので
数 ms で、体感に出ない。

## 補完する位置

| 打っているもの | 候補 |
|---|---|
| `hso <TAB>` | `setup` `start` `list` `ls` `delete` `java` `completion` `version` `update` `uninstall` `help` `-config` |
| `hso start <TAB>` | 登録済みサーバー名 |
| `hso delete <TAB>` | 登録済みサーバー名 ＋ `-y` `--yes` |
| `hso java <TAB>` | `change` `list` |
| `hso java change <TAB>` | 登録済みサーバー名 |
| `hso uninstall <TAB>` | `--purge` `-y` `--yes` |
| `hso completion <TAB>` | `bash` `zsh` `fish` |
| `hso -config <TAB>` | **シェルのファイル補完**（`__complete` は候補を返さず、その旨を伝える） |
| 上記以外（`setup` `list` `version` `update` `help` の後ろなど） | 無し |

サーバー名を取る位置は 3 つ（`start` / `delete` / `java change`）で、どれも同じ候補集合を返す。

## 問い合わせのプロトコル

```
$ hso __complete hso start ''
survival	/srv/minecraft/hso.toml
creative	/srv/creative/hso.toml

$ hso __complete hso java ''
change	サーバーが使う Java を変更する
list	自動検出した Java と、それを使っているサーバーを表示する
```

- 渡すのは**いま打たれている単語列そのまま**。末尾はまだ打ちかけの単語（空文字もある）
- サーバー名の説明には `hso.toml` のパスを添える。同じ名前で迷うことはないが、
  どこのサーバーかがその場で読める
- ファイル補完へ回す位置だけは、候補の代わりに 1 行 `:files` を返す。スクリプトはこれを見て
  シェルのファイル補完に切り替える（候補ゼロと区別が付く形にする）

### `main.go` での分岐位置

**supervisor の判定の直後、`dispatchCommand` の先頭**に置く。

1. `process.SupervisorCommand` — 現行どおり最優先
2. `__complete` — ここ
3. 以降は現行の分岐（ヘルプ / バージョン / サブコマンド / `-config`）

`__complete` はヘルプ（`CommandHelp`）にも `UnknownCommand` の案内にも載せない。
一方 **`completion` は載せる** — 人が叩いて設置し直すコマンドなので、探せないと意味がない。

## Go 側の構造

`internal/completion` に、ディスクに触らない純関数を 1 本置く。

```go
// Candidates は打たれた単語列に対する補完候補を返す。
// words は "hso" から始まり、末尾は打ちかけの単語（空文字もある）。
func Candidates(words []string, servers []registry.Server) []Candidate

type Candidate struct {
	Value       string
	Description string
}
```

- **位置の判定だけを持つ。** registry の読み込みは `cmd/hso` 側でやって渡す。
  これで位置ごとの候補はテーブルテストで全部押さえられる
- `cmd/hso/completion.go` は「`registry.Path` → `registry.Load` → `Candidates` → 印字」だけ。
  `registry.Load` が失敗したら**エラーを返さずコマンド候補だけ**で応答する
  （一覧が壊れていてもサブコマンドの補完は効かせる。java.md の「補助機能が主機能を止めない」と同じ）

補完スクリプト 3 本は `internal/completion/scripts/` に置き、`go:embed` で埋める。
**スクリプトはシェルの構文であって翻訳対象ではない**ので `internal/msg` には入れない
（→ [i18n.md](../.claude/docs/i18n.md) の線引き）。説明文だけが msg 側に立つ。

## シェル別スクリプト

どれも「単語列を渡す → 返ってきた行を並べる」だけ。骨子のみ示す。

**bash** — 説明は捨てて値だけを `COMPREPLY` へ。`:files` なら `-o default` に任せる。

```bash
_hso() {
    local IFS=$'\n' line
    local out=$(hso __complete "${COMP_WORDS[@]:0:COMP_CWORD+1}" 2>/dev/null)
    [[ $out == ':files' ]] && return 1   # シェルのファイル補完へ落とす
    COMPREPLY=($(compgen -W "$(cut -f1 <<<"$out")" -- "${COMP_WORDS[COMP_CWORD]}"))
}
complete -o default -F _hso hso
```

**zsh** — `_describe` がタブ区切りをそのまま説明として扱える。`:files` は `_files` へ。

**fish** — `complete -f -c hso -a '(__hso_complete)'`。fish は候補のタブ以降を説明として表示する
ので、返した行をそのまま流せる。

## 設置・更新・削除

### 置き場所

| shell | システム（`--system`） | ユーザー（既定） |
|---|---|---|
| bash | `/usr/share/bash-completion/completions/hso` | `~/.local/share/bash-completion/completions/hso` |
| zsh | `/usr/local/share/zsh/site-functions/_hso` | `~/.local/share/zsh/site-functions/_hso` |
| fish | `/usr/share/fish/vendor_completions.d/hso.fish` | `~/.config/fish/completions/hso.fish` |

### install.sh

バイナリを置いた後に、**インストール先と同じ権限のまま**、上表のうち置ける場所へ書く。

- **失敗しても致命的にしない。** 補完が無くても hso は動く。警告 1 行を出して続ける
- **置いたパスを出力する。** 後で消すのも読み直すのもユーザーなので、黙って置かない
- rc ファイルは触らない。**ユーザーインストールの zsh だけは例外的に案内が要る**
  （`~/.local/share/zsh/site-functions` は既定の `fpath` に入っていない）。
  PATH の警告と同じ形で、`compinit` より前に足す 1 行を出す:

  ```
  fpath=(~/.local/share/zsh/site-functions $fpath)
  ```

  bash は bash-completion 2.8 以降が `$XDG_DATA_HOME/bash-completion/completions` を
  動的に読むので、fish と同じくファイルを置くだけで効く

### `hso update`

**何もしない。** スクリプトは委譲するだけで版に依存しないので、置き換える必要がない
（→「なぜ委譲するか」）。

### `hso uninstall`

**既定で消す。`--purge` 送りにしない。** バイナリが消えた後に残っても意味のないファイルで、
`~/.config/hso/config.toml`（入れ直す人が失うと困るもの）とは性質が違う。

- 上表のパスを列挙し、存在するものを消す。消したパスは最後のまとめに出す
- 消せなくても `uninstall` を止めない。cli.md の「`y` の後は途中で止まらない」に従う

## テスト

両タグ（ja / en）で回す。

| 対象 | 方法 |
|---|---|
| 位置ごとの候補 | `completion.Candidates` のテーブルテスト。上の「補完する位置」の表を 1 行ずつ |
| 壊れた registry | `Load` 失敗時にコマンド候補だけ返り、エラーが stdout に出ないこと |
| 出力形式 | `hso __complete` の出力が `値<TAB>説明` であること（`cmd/hso` のテスト） |
| スクリプトの構文 | `bash -n` / `zsh -n` / `fish --no-execute`。**そのシェルが無い環境ではスキップ**する |

`__complete` はヘルプに出ないので、`msg_test.go` の識別子照合には現れない。
**説明文を msg に足す以上、ja / en の両方に書く**という契約はそのまま効く。
