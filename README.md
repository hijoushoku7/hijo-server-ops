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
┌─ hijo-server-ops ──────────────────────── uptime 3d 04:12:33 ─┐
│ Heap ██████░░░░ 2.1G / 3.2G committed (max 4.0G)  post-GC 512M │
│ RSS  ████████░░ 5.4G / 8.0G limit                 Δ +3.3G      │
│ GC   young 142 (1.8s, 0.9%)  last 12.3ms   CPU 82%            │
│ Players 3/20  [alice bob carol]            Lag events: 2       │
│ Heap ⡀⡠⡴⡿⠋⢀⡠⡴⡿⠋     RSS ⣀⣀⣤⣤⣶⣶⣿⣿                  │
├────────────────────────┬───────────────────────────────────────┤
│ Chat                   │ Log                                   │
├────────────────────────┤                                       │
│ Commands               │                                       │
├────────────────────────┴───────────────────────────────────────┤
│ > _                                        [restart] [stop]    │
└────────────────────────────────────────────────────────────────┘
```

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
場合は `hso.toml` のあるディレクトリを使う。TUI は 72 列 × 20 行以上の
端末で表示する。ログは各ペインに表示できる行数、メモリ推移は Braille
グラフの横幅に入るサンプル数だけを保持する。

```bash
hso -config /path/to/hso.toml
```

コンソール欄でコマンドを入力し、Enter で Minecraft サーバーへ送信する。
入力は 512 文字を上限とし、操作キューも固定長にしてメモリ使用量を制限する。
↓ または Tab で `restart` / `stop` にフォーカスを移し、← → で選択、
Enter で実行する。↑ または Esc で入力欄へ戻る。

`restart` は Minecraft の `stop` コマンドによる正常終了を待ってから同じ
起動スクリプトを再実行する。Java プロセスをまだ特定できていない起動途中
では、安全に停止できないため `restart` を受け付けない。

## ビルド

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o hso ./cmd/hso
```

`CGO_ENABLED=0` で完全静的リンク。実行時依存はゼロ。配布対象は `linux/amd64` と `linux/arm64`。

## ドキュメント

- [ビルド手順](docs/build.md)
- [仕様・技術調査](docs/spec.md)
