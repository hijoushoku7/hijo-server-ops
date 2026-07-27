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

`hso.toml`

```toml
[server]
command = "./run.sh"      # 起動スクリプト。必須・明示指定
workdir = "/srv/minecraft"

[ui]
panes = ["stats", "chat", "commands", "log"]
```

起動スクリプトは実行可能ファイルとして用意する。`workdir` を省略した
場合は `hso.toml` のあるディレクトリを使う。現時点ではTUI実装前のため、
サーバーの標準入出力をそのまま端末へ接続する。

```bash
hso -config /path/to/hso.toml
```

## ビルド

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o hso ./cmd/hso
```

`CGO_ENABLED=0` で完全静的リンク。実行時依存はゼロ。配布対象は `linux/amd64` と `linux/arm64`。

## ドキュメント

詳細な仕様・技術調査は [docs/spec.md](docs/spec.md) を参照。
