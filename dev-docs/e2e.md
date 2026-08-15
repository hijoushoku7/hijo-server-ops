# TUI の実機テスト（tmux 経由）

エージェントが人手を借りずに `hso` を起動し、キーを送り、画面を読んで検証するための手順。
Go のテストでは拾えない描画・キー操作・実 JVM のメトリクスを見るために使う。
全体の仕様は [spec.md](spec.md)、CLI の仕様は [cli.md](cli.md)。

## 隔離（先に読む）

開発機には**人間が使っている tmux と、本物の hso の状態**がある。素で叩くと踏む。

| 軸 | 対策 | 踏むとどうなるか |
|---|---|---|
| tmux | `tmux -L hso-e2e -f /dev/null` | 素の `tmux` は Claude Code 自身が動いている親セッションに窓を生やす。`kill-server` はユーザーのセッションごと落とす |
| hso の状態 | 必要なら `XDG_CONFIG_HOME` / `XDG_RUNTIME_DIR` を `/tmp` 配下へ振る | サーバー一覧は `~/.config/hso/config.toml`、pidfile は `$XDG_RUNTIME_DIR/hso`（無ければ `/tmp/hso-<uid>`）。テストの登録が本物の一覧に混ざる |
| ポート | `server.properties` の `server-port` をずらす | 25565 で本物が動いていると起動が衝突する |

`-L` は tmux **サーバープロセスごと**分ける。ユーザーの `tmux ls` には出ず、こちらの
`kill-server` も届かない。`-f /dev/null` はユーザーの `~/.tmux.conf` を読まないため
（キーバインドやステータス行が撮影結果に混ざらない）。

許可設定に足すなら `Bash(tmux -L hso-e2e:*)` の形にする。**自動承認されるのが隔離ソケット
宛だけになり、default ソケットを触るコマンドは必ず確認プロンプトに落ちる。**

## ヘルパー

リポジトリ外（`/tmp/hso-e2e/t.sh`）に置く。

```sh
#!/bin/sh
SOCK=hso-e2e
TMUXCMD="tmux -L $SOCK"
export TMUX=          # 親セッションの入れ子と判定されるのを防ぐ
case "$1" in
new)   $TMUXCMD -f /dev/null new-session -d -s "$2" -x 200 -y 50 -c "$3" "$4" ;;
keys)  session=$2; shift 2; $TMUXCMD send-keys -t "$session" "$@" ;;
cap)   $TMUXCMD capture-pane -p -t "$2" | sed -e :a -e '/^[[:space:]]*$/{$d;N;ba' -e '}' ;;
alive) $TMUXCMD has-session -t "$2" 2>/dev/null && echo alive || echo gone ;;
kill)  $TMUXCMD kill-session -t "$2" 2>/dev/null ;;
esac
```

- `-x 200 -y 50` 固定。端末幅に依存しない再現可能な画面になる。狭いとレイアウトが変わる
- キー名は tmux 記法（`Enter` `Escape` `Tab` `BSpace` `C-c` `C-u`）。文字列はそのまま渡せる
- **送出のたびに `sleep` を挟む。** 目安は起動 1.5s、画面遷移 0.3〜0.6s、TUI の起動 2.5s

## 前提

```bash
go build -o /tmp/hso-e2e/hso_ja ./cmd/hso   # 検証したいブランチから建てる
```

対象サーバーは `mc-server-test/`（gitignore 済み）。`server.jar` と起動スクリプトを置く。

```sh
#!/bin/sh
# mc-server-test/start.sh
# JVM 引数はスクリプト側の責務。GC ログは hso が JAVA_TOOL_OPTIONS で注入するので書かない。
exec java -Xms1G -Xmx2G -jar server.jar nogui
```

**`eula.txt` の `eula=true` は人間が書く。** Mojang の EULA への同意はユーザー名義の意思表示
なので、エージェントは書き換えない。未同意のままでもサーバーは起動途中で落ちるだけなので、
**クラッシュ検知・終了モーダル・ウィザード系の検証はそのまま回せる**（むしろ確実に落ちる
フィクスチャとして使える）。アドレス解決・プレイヤー・停止順序・自動再起動を見るには同意が要る。

## 手順

```bash
T=/tmp/hso-e2e/t.sh; D=<サーバーディレクトリ>

$T new s1 "$D" "/tmp/hso-e2e/hso_ja setup"     # 起動
sleep 1.5; $T cap s1                            # 画面を読む
$T keys s1 Enter; sleep 0.6; $T cap s1          # キーを送って読む
```

終了は TUI の正規手順（終了モーダルで `終了`）を通す。`kill` でも Pdeathsig で java は死ぬが、
停止順序の検証にはならない。CLI サブコマンドの終了コードを見たいときはシェルで包む。

```bash
$T new s2 "$D" "sh -c '/tmp/hso-e2e/hso_ja setup; echo EXIT=\$?; sleep 20'"
```

`hso list` や `hso start` は**別プロセス**（普通の Bash）から叩く。pidfile 越しに起動中の
サーバーが見えるかは、TUI を動かしたまま外から確認するのが唯一の検証方法。

## 後片付け

```bash
$T alive s1                       # gone であること
pgrep -af server.jar              # 孤児が残っていないこと
/tmp/hso-e2e/hso_ja list          # 状態が「停止」に戻っていること
tmux ls                           # ユーザーのセッションが無事であること
```

## 検証の型

分岐表があるものは、表の行をそのまま 1 ケースにする。例（issue #66 の登録ウィザード）:

| 操作 | 期待 |
|---|---|
| 未登録 + `hso setup` | 案内 → 名前入力 → 登録 → 起動 |
| 未登録 + 素の `hso` + Esc | 登録せず起動。一覧は空のまま |
| 未登録 + `hso` / `setup` + Ctrl+C | 「中止しました」exit 0、java を起動しない |
| 登録済み + `hso setup` | 登録済みエラー exit 1 |
| 登録後 | 別プロセスの `hso list` が「起動中（PID）」、`hso start` が二重起動を拒否 |

画面の文言は `internal/msg` の ja / en で変わる。英語版を見るなら `-tags en` で建てる。
