# hijo-server-ops

Minecraft サーバー用のラッパー型 TUI コンソール。**Linux 専用。**

既存の起動スクリプトを `hso` 経由で起動すると、サーバーのログを表示しつつ、メモリ・プレイヤー・稼働時間・ラグイベントをターミナル上のパネルに常時表示する。

**ステータス: v1 実装中。**

## 解決する課題

- Linux サーバー上では Swing GUI（`nogui` を付けないと出るあれ）が見られない
- ヒープ使用量・プレイヤー一覧などの情報取得が個別に面倒
- `htop` は RSS しか見えず、ヒープの状況が分からない

複数サーバーの集中管理パネル（Pterodactyl 等）ではなく、**1サーバーのコンソールそのものを代替する**もの。

## 画面イメージ

```
┌─ hijo-server-ops · uptime 3d 04:12:33 ─┐┌─ Meters · RSS/cgroup ─┐┌─ Players 3 ──┐
│ Heap 2.1G / 3.2G committed (max 4.0G)  ││ CPU  ███░░░░░░░  82%  ││ alice        │
│ RSS  5.4G / 8.0G limit  Δ +3.3G        ││ Heap █████▏░░░░  52%  ││ bob          │
│ GC   142 collections  last 12.3ms      ││ RSS  ███████░░░  67%  ││ carol        │
│ Players 3  Lag events: 2  CPU 82%      ││                       ││              │
│ Heap ⡀⡠⡴⡿⠋⢀⡠⡴⡿⠋                        ││                       ││              │
│ RSS  ⣀⣀⣤⣤⣶⣶⣿⣿                          ││                       ││              │
└────────────────────────────────────────┘└───────────────────────┘└──────────────┘
┌─ Chat ─────────────────┐┌─ Log ─────────────────────────────────┐
│                        ││                                       │
└────────────────────────┘│                                       │
┌─ Commands ─────────────┐│                                       │
│                        ││                                       │
└────────────────────────┘└───────────────────────────────────────┘
┌─ Console ──────────────────────────────────────────────────────┐
│ > _                                        [restart] [stop]    │
└────────────────────────────────────────────────────────────────┘
 Esc  select   Tab  input/restart/stop   Enter  execute   ^C  exit
```

Meters は CPU / Heap / RSS を横棒でも出す。満目はそれぞれ **コア数 × 100%**、
**heap max**、**cgroup 制限（なければホスト総メモリ）**。どの分母を使ったかは
`Meters · RSS/cgroup` のようにタイトルへ出し、分母が取れないときはメーターを
描かず `n/a` と表示する。パーセントの数値表示は従来どおり併記する。

`RSS - Heap committed` の差分（Δ）を明示するのが既存ツールにない点。「`-Xmx` を積んだのに OOM Killer に殺される」の原因（Direct ByteBuffer / Metaspace / JIT Code Cache / スレッドスタック）がそのまま可視化される。

## 仕組み

```
hso
 └─ 設定した起動スクリプトを子プロセスとして実行
     ├─ env JAVA_TOOL_OPTIONS で -Xlog:gc を注入（スクリプトを書き換えない）
     ├─ /proc を辿って実際の java プロセスの PID を特定
     ├─ hsperfdata を mmap 直読み        → ヒープ / 世代別 / GC 統計
     ├─ /proc/<pid>/status + cgroup      → RSS / メモリ上限
     ├─ GC ログを tail                   → GC 後の谷の値、停止時間
     └─ stdout をパース                  → チャット / コマンド / 参加退出 / ラグ
```

サーバー側にプラグインも MOD も不要（TPS 表示のみ将来 Fabric mod を使用）。JVM 引数は管理せず、ユーザーの `user_jvm_args.txt` / 起動スクリプトの記述をそのまま尊重する。

## 設定

テンプレートをコピーして `hso.toml` を作る。

```bash
cp hso.toml.example hso.toml
```

`hso.toml`

```toml
[server]
command = "./run.sh"      # 起動スクリプト。必須・明示指定
workdir = "/srv/minecraft"

[ui]
panes = ["stats", "chat", "commands", "log"]
```

起動スクリプトは実行可能ファイルとして用意する。`workdir` を省略した
場合は `hso.toml` のあるディレクトリを使う。TUI は 72 列 × 21 行以上の
端末で表示する。ただし 72 列では上段左の Stats の行末が切れる（Δ・post-GC・
Braille グラフは残る）。全項目が読めるのは **94 列**から。ログは各ペイン
ごとに 500 行、メモリ推移は Braille グラフの横幅に入るサンプル数だけを
保持する。

```bash
hso -config /path/to/hso.toml
```

### 操作

画面は**選択モード**と**フォーカスモード**の 2 状態で動く。現在のキー割り当ては
常に最下行に表示される。

| モード | キー | 動作 |
|---|---|---|
| 選択 | ← ↑ ↓ → | パネルを選ぶ（枠がシアンの太線になる） |
| 選択 | Enter | 選んだパネルにフォーカスする（枠が黄色の太線になる） |
| フォーカス | Esc | フォーカスを外して選択モードへ戻る（スクロール位置は最新へ戻る） |
| Console | 文字 / Enter | コマンドを入力して Minecraft サーバーへ送信 |
| Console | Tab | 入力欄 → `restart` → `stop` を巡回、Enter で実行 |
| Chat / Commands / Log | ↑ ↓ | 1 行スクロール |
| Chat / Commands / Log | PgUp / PgDn | 1 画面スクロール |
| Chat / Commands / Log | End | 最新行へ戻る |
| Players | ↑ ↓ | プレイヤーを選ぶ |
| Players | Enter | 選んだプレイヤーへのコマンド一覧を開く |
| Players（コマンドモーダル） | Enter | コマンドを Console 入力欄に組み立てる |
| Players（コマンドモーダル） | Esc | モーダルを閉じてプレイヤー一覧へ戻る |
| 常時 | Ctrl+C | 終了 |

フォーカスできるのは Players / Chat / Commands / Log / Console の 5 つ。
Stats と Meters は表示専用なので選択対象に含めない。

### プレイヤーへのコマンド

Players パネルでプレイヤーを選んで Enter を押すと、そのプレイヤーに対する
コマンド一覧が**モーダル**で開く。選んだ行の左下を起点に、フォーカス枠と
同じ色の細枠で重なって出るので、背後のプレイヤー一覧は見えたまま残る。

`tell` `kick` `ban` `op` `deop` `whitelist add` `whitelist rm`
`gm survival` `gm creative` `gm adventure` `gm spectator` `kill`

選んだコマンドは**即実行せず、Console 入力欄に組み立てて置く**。`ban` や
`kick` の誤操作が Enter のもう一押しで止まり、`tell` の本文や `kick` の理由を
そのまま書き足せる。送信されるのは `whitelist remove` のように省略しない
完全な形で、一覧のラベルだけを枠幅に合わせて短くしてある。

起動直後は Console にフォーカスした状態なので、そのままコマンドを打てる。
Console にフォーカス中は ← → を入力に使うため、`restart` / `stop` の選択は
Tab で行う。最新行から遡っている間はパネルのタイトルに `↑N` と遡り行数が出て、
新着ログが届いても表示位置は動かない。

入力は 512 文字を上限とし、操作キューも固定長にしてメモリ使用量を制限する。

`restart` は Minecraft の `stop` コマンドによる正常終了を待ってから同じ
起動スクリプトを再実行する。Java プロセスをまだ特定できていない起動途中
では、安全に停止できないため `restart` を受け付けない。

## インストール

[Releases](https://github.com/hijoushoku7/hijo-server-ops/releases) から
アーカイブを取得する。

```bash
tar xzf hso_v0.1_linux_amd64.tar.gz
cd hso_v0.1_linux_amd64
```

`hso`（実行ファイル）と `hso.toml`（設定テンプレート）が展開される。arm64
環境では `linux_arm64` のアーカイブを使う。

## ビルド

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o hso ./cmd/hso
```

`CGO_ENABLED=0` で完全静的リンク。実行時依存はゼロ。配布対象は `linux/amd64` と `linux/arm64`。

## ドキュメント

- [ビルド手順](docs/build.md)
- [仕様・技術調査](docs/spec.md)
