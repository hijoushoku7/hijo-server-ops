# hijo-server-ops 仕様書

> 管理 issue: hijoushoku7/life#24

## 概要

**hijo-server-ops**（バイナリ名 `hso`）— Minecraft サーバー用のラッパー型 TUI コンソール。

既存の起動スクリプトを hso 経由で起動すると、サーバーのログを表示しつつ、メモリ・プレイヤー・稼働時間・ラグイベントなどをターミナル上のパネルに常時表示する。

**解決する課題**
- Linux サーバー上では Swing GUI（`nogui` を付けないと出るあれ）が見られない
- ヒープ使用量・プレイヤー一覧などの情報取得が個別に面倒
- `htop` は RSS しか見えず、ヒープの状況が分からない

**位置づけ**: 複数サーバーの集中管理パネル（Pterodactyl 等）ではなく、**1サーバーのコンソールそのものを代替する**もの。1サーバー = 1 hso プロセス。

**ターゲット**: Linux でサーバーをホストしている人。Windows でのホスト経験がある人が Linux に移ってきたケースも想定するが、**動作対象は Linux 限定**（そもそも「Swing GUI が見られない」という課題が Windows では成立しない）。CLI と設定ファイルで完結する前提でよく、初心者向けの GUI インストーラ等は不要。

## 期限・見積もり

期限: 未定 / 見積: v1 で 20〜30 時間程度（hsperfdata パーサとログパーサが山）

---

## 確定仕様

| 項目 | 決定 |
|---|---|
| 名前 | `hijo-server-ops`（バイナリ `hso`） |
| 言語 | **Go**（TUI は Bubble Tea + Lipgloss） |
| 対応 OS | **Linux 専用**。中核の `/proc` と cgroup が Linux 依存のため |
| 形態 | **ラッパー型**（サーバープロセスを自分で起動して包む）。既存プロセスへのアタッチは対象外 |
| 対象台数 | 1サーバー専用 |
| 対象サーバー | v1 はバニラ / Forge / NeoForge（ログ形式が共通のもの） |
| 起動 | **起動スクリプトのパスを設定ファイルで明示指定**。環境ごとに `run.sh` / `ServerStart.sh` / 手書き `start.sh` とバラバラなため自動検出には頼らない |
| JVM 引数 | **hso は管理しない**。ユーザーの `user_jvm_args.txt` / 起動スクリプトの記述をそのまま使う。`-Xmx` の実効値は hsperfdata から読み取って表示するだけ |
| フラグ注入 | `JAVA_TOOL_OPTIONS` 環境変数経由（起動スクリプトを一切書き換えない） |
| メモリ | **ヒープ（hsperfdata）と RSS（/proc）を両方取得して並べて表示** |
| TPS | v1 は対象外（ラグイベント検知のみ）。v2 で Fabric mod により対応 |
| プレイヤー一覧 | ログの `joined the game` / `left the game` を追跡 |
| 操作 | ほぼ表示専用。**矢印でパネル選択 / Enter でフォーカス**、restart / stop、コマンド送信 |
| ログ | **チャット / コマンド使用履歴 / その他** の3系統に分類して別ペインに表示 |
| 将来 | 表示要素・レイアウトのカスタマイズ |

---

## アーキテクチャ

### 起動フロー

```
hso 起動
 └─ 設定ファイルの起動スクリプトを子プロセスとして実行
     ├─ env: JAVA_TOOL_OPTIONS="-Xlog:gc:file=..." を注入
     ├─ stdin/stdout/stderr をパイプで接続
     └─ 起動後、/proc を辿って実際の java プロセスの PID を特定
```

**`JAVA_TOOL_OPTIONS` を使う理由**: 起動スクリプトの中身は任意の bash（変数展開・条件分岐・`exec`）であり、静的パースは必ず破綻する。この環境変数は JVM が起動時に必ず読み込むため、**スクリプトの中身を一切知らずに、ユーザーのファイルを一切書き換えずに** GC ログを有効化できる。`user_jvm_args.txt` に追記するような破壊的操作を避けられる点も重要。

- 副作用: JVM が `Picked up JAVA_TOOL_OPTIONS: ...` を stderr に出すので、TUI 側で除外する
- `-Xmx` は**注入しない**。設定ファイルの記述ではなく「JVM が実際に適用した値」を hsperfdata の `sun.gc.policy.maxCapacity` から読む方が正確

### java プロセスの PID 特定

掴める PID はシェルのものなので、`/proc/*/stat` の PPID を辿って `comm == "java"` の子孫を探す。RSS も hsperfdata も java 本体の PID が必要。

- `exec java ...` で終わるスクリプトなら PID は同一になるので探索不要
- **`screen` / `tmux` を内部で使うスクリプトは stdin パイプが機能しない**。検出して明示的にエラーを出す（サイレントに壊れるのが最悪）

### メトリクス取得

```
連続値(1s)   : hsperfdata (/tmp/hsperfdata_<user>/<pid>) を mmap して直読み
               → heap used / committed / max、世代別、metaspace、GC回数・累積時間、uptime、スレッド数
イベント     : -Xlog:gc の出力ファイルを tail
               → GC後の谷の値、GC停止時間、GC頻度
OS側(1s)     : /proc/<pid>/status の VmRSS、/proc/<pid>/stat の CPU
               ＋ cgroup memory.current / memory.max（Docker/systemd 下）
フォールバック: hsperfdata が読めない場合は RSS のみ表示し、heap: n/a と正直に出す
```

### ログ処理

stdout を1行ずつパースし、分類してペインに振り分ける。バニラのログ形式:

```
[12:34:56] [Server thread/INFO]: <player> メッセージ
[12:34:56] [Server thread/INFO]: player joined the game
[12:34:56] [Server thread/INFO]: player issued server command: /time set day
[12:34:56] [Server thread/WARN]: Can't keep up! Is the server overloaded? Running 2531ms behind, skipping 50 tick(s)
```

| 分類 | 判定 |
|---|---|
| チャット | `<name> ...` 形式 |
| コマンド履歴 | `issued server command:` ＋ `[実行者: 結果]`（バニラの sendCommandFeedback。プレイヤー実行分はこの形でしか出ないことが多い）＋ hso 自身が送信したコマンド（自分の送信分はログに出ないので自前で記録する） |
| プレイヤー増減 | `joined the game` / `left the game` / `lost connection:` |
| ラグイベント | `Can't keep up!` |
| その他 | 上記以外 |

**要検証**: 分類は正規表現ヒューリスティックなので、バージョン差・MOD による形式変化で壊れうる。ルールを設定ファイルで上書きできる形にしておくべきか。

壊れても劣化で済むようにはなっている。`[実行者: 結果]` の判定は MOD が同じ形のログを出すと誤検出しうるが、`Tracker` が見るのは join / leave / lag だけなのでプレイヤー一覧は汚れず、影響は Commands ペインのノイズに閉じる。判定順は chat の後（`[Not Secure] <name> ...` を守るために必須）、join / leave の前（`[...]` に囲まれないので衝突しない）。

### 画面構成（案）

```
┌─ hijo-server-ops ──────────────────────── uptime 3d 04:12:33 ─┐
│ Heap ██████░░░░ 2.1G / 3.2G committed (max 4.0G)  post-GC 512M │
│ RSS  5.4G / 15.6G total (68%)  limit 8.0G         Δ +3.3G      │
│ GC   young 142 (1.8s, 0.9%)  last 12.3ms   CPU 82%            │
│ Players 3/20  [alice bob carol]            Lag events: 2       │
├────────────────────────┬───────────────────────────────────────┤
│ Chat                   │ Log                                   │
│                        │                                       │
├────────────────────────┤                                       │
│ Commands               │                                       │
│                        │                                       │
├────────────────────────┴───────────────────────────────────────┤
│ > _                                        [restart] [stop]    │
└────────────────────────────────────────────────────────────────┘
```

**`RSS - Heap committed` の差分（Δ）を明示的に出す**のが既存ツールに対する差別化点。「`-Xmx` を積んだのに OOM Killer に殺される」の原因（Direct ByteBuffer / Metaspace / JIT Code Cache / スレッドスタック）がそのまま可視化される。

情報量が多くなるため、ペイン分割の是非は実装後に要評価。将来的にはレイアウトを設定でカスタマイズ可能にする。

メモリ推移のグラフは Braille 文字で描画する。専用のグラフライブラリは
使わず、TUI の横幅に入るサンプルだけを固定長リングバッファに保持する。
ログは表示行数と切り離して各ペイン 500 行の履歴を持ち、スクロールで
画面外へ流れた行まで遡れるようにする（後述）。端末を縮小して表示できない
サイズになった場合は、保持しているデータへの参照も破棄する。

TUI を終了するときは、Java プロセスを特定済みなら Minecraft の `stop`
コマンドを送り、最大 60 秒待つ。終了しない場合だけ supervisor へ
`SIGTERM` を送り、プロセスツリーを停止する。ワールド保存前の強制終了を
通常の終了経路にしない。

コンソール入力は 512 文字、TUI からアプリ層へ渡す操作キューは 4 件を
上限とする。コマンド送信の完了前に入力が連打されても、キューや goroutine
を無制限に増やさない。

操作は**選択モード**と**フォーカスモード**の 2 状態に分ける。選択モードでは
矢印で Players / Chat / Commands / Log / Console を移動し、Enter でフォーカスする。
Stats と Meters は表示専用で操作対象がないため選択対象に含めない。
フォーカス中は Esc で選択モードへ戻る。Console にフォーカス中は ← → を
文字入力に使うため、`restart` / `stop` の選択は Tab の巡回で行う。
Chat / Commands / Log にフォーカス中は ↑ ↓ / PgUp / PgDn でスクロールし、
End で最新行へ戻る。Esc でフォーカスを外すときも最新行の追従に戻す。
遡ったまま放置して新着ログを見落とすのを防ぐためで、この結果、遡り表示中の
パネルは常に「今フォーカスしているパネル」に一致する。枠線は通常が細線グレー、選択中がシアンの太線、
フォーカス中が黄色の太線で、3 状態とも枠幅は 1 セルで変わらない。
キー割り当ては nano 風に最下行へ常時表示する。

上段は Stats / Meters / Players の 3 列。Meters と Players に最小幅
（20 / 18 列）を確保し、余りをすべて Stats へ回す。Stats だけが行の長い
テキストを持つため。72 列端末では Stats の行末が切れるが、Braille グラフと
Δ の表示は残る。全体が読める幅の目安は 94 列。

Meters は CPU / Heap / RSS を横棒で出す。満目は CPU がマシン全体（全コア）、
Heap が heap max、RSS が cgroup 制限。CPU の収集はコア数ぶんを合計した値
（8 コアなら最大 800%）だが、表示は常にコア数で割って 0..100% に直す。
Stats 行の CPU も同じ値を使う。cgroup 制限がない・`unlimited` の
環境では `/proc/meminfo` の MemTotal に落とし、どちらを使ったかをタイトルへ
`RSS/cgroup` `RSS/host` と明示する。どちらも取れなければメーターを描かず
`n/a` と出す（原則4）。パーセントの数値表示はメーターと併記して残す。

Stats の RSS 行には OS の総メモリ（`/proc/meminfo` の MemTotal）と RSS の
割合を併記する。割合の分母は Meters と同じ規則（cgroup 制限があればそれ、
なければ総メモリ）で、取れなければ `n/a`。

Players はオンラインのプレイヤー一覧専用のパネル。Stats 行からは名前の
列挙を外し、人数だけを他の指標と並べて残す。幅は Minecraft のユーザー名
上限である 16 文字がそのまま入るよう、最小 18 列を確保する。

Players にフォーカス中は 2 段階で動く。プレイヤーを選ぶ段階と、選んだ
プレイヤーへのコマンドを選ぶ段階。Esc は 1 段ずつ戻る。スクロール位置は
状態として持たず、カーソル位置から毎回導出する（`windowStart`）。

コマンド一覧は**モーダル**として画面に重ねる。パネルの中身を差し替えると
どのプレイヤーを選んだのかが見えなくなるため。選んだ行の左下を起点に、
フォーカス枠と同じ色の細枠で出す（枠の太さで下地のパネルと区別できる）。
画面からはみ出すときだけ内側へ寄せる。

重ね合わせは `overlay` が行う。下地の各行を重なる位置で左右に切り、間に
モーダルの行を挟む。切った箇所の前後で属性を戻して下地の色が漏れないように
する。切断位置が全角文字にかかると、左は文字が落ちて 1 セル狭くなり、右は
文字が丸ごと残って 1 セル広くなるので、どちらも幅を測って空白で詰め直す。
下地からはみ出す部分は捨てて行を伸ばさない。日本語のチャットやログが
下地になるため、ここは必ず全位置で幅を検証する。

コマンドは `tell` `kick` `ban` `op` `deop` `whitelist add/remove`
`gamemode`（4 種）`kill`。`pardon` は対象がオンライン一覧に出ないため、
`tp` は誰を誰に送るか決まらないため入れない。

**選んだコマンドは即実行せず、Console 入力欄に組み立てて置く。** 実行経路を
新設せず既存の `sendInput` に乗せられ、`ban` / `kick` の誤操作に Enter の
もう一押しという確認が自然に入り、`tell` の本文や理由を続けて書ける。
引数の要るコマンドと要らないコマンドを分岐させずに済むのが決め手。

パネルは表示行数と切り離して各 500 行の履歴を持つ。スクロールで画面外へ
流れた行まで遡れるようにするため。遡っている間は新着行で表示位置を
動かさず、タイトルに `↑N` と遡り行数を出す。

`restart` は Java プロセス特定後のみ受け付ける。Minecraft の `stop` を
送って最大 60 秒待ち、終了しない場合は supervisor の SIGTERM → SIGKILL
へフォールバックしてから、同じ起動スクリプトを再実行する。再起動前の
メトリクスとプレイヤー状態は破棄し、世代番号が古い非同期イベントも
表示へ反映しない。

### 設定ファイル（`hso.toml`）

```toml
[server]
command = "./run.sh"      # 起動スクリプト。必須・明示指定
workdir  = "/srv/minecraft"

[ui]
panes = ["stats", "chat", "commands", "log"]

# ログ分類ルールの上書き（将来）
```

JVM 引数はここには書かせない。責務はユーザーの `user_jvm_args.txt` / 起動スクリプト側に残す。

設定ファイルがない状態で `hso` を実行すると、対話的なセットアップ
ウィザードへ入る（`internal/setup`）。サーバーディレクトリを入力 → その中の
起動スクリプト候補を一覧から選ぶ → 書き出す TOML をプレビューして確定、
の 3 ステップで、作成後はそのままサーバーを起動する。`workdir` が
`hso.toml` と同じディレクトリなら省略し、`command` は `workdir` 配下なら
相対パスにする。書き込みは `O_EXCL` で、既存の設定ファイルは上書きしない。
起動スクリプトに実行権限がない場合だけ、確認画面で同意を取ったうえで
実行権限を付ける（`hso` がユーザーのファイルに触れる唯一の箇所）。

ウィザードへ入る判定は「ファイルが存在しない」かつ「標準入出力が端末」の
両方を満たすときだけで、パイプ越しや systemd 配下では `config.Load` の
エラーを返す。中止した場合はサーバーを起動せず終了コード 0 で終わる。
設定ファイルを作るためだけのサブコマンドやフラグは持たない。

---

## 配布

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o hso ./cmd/hso
```

- **実行時依存ゼロ**。Go のインストールも不要。バイナリ1個を置いて実行するだけ
- `CGO_ENABLED=0` で完全静的リンク。glibc に依存しないので Alpine（musl）でもそのまま動く。**リリースビルドでは必須**
- サイズは 8〜15MB 程度
- **`linux/amd64` と `linux/arm64` の両方を出す**（Oracle Cloud Ampere / Raspberry Pi でのホストが多いため）
- 配布は GitHub Releases。`v*` タグの push で `.github/workflows/release.yml` が amd64 / arm64 の tar.gz を作って添付する（goreleaser は使わない）
- v2 の Fabric mod jar は `//go:embed` でバイナリに埋め込み、`hso install-mod` で `mods/` に書き出す。**ユーザーから見える成果物はバイナリ1個**に保つ

---

## 技術調査メモ

### バニラ GUI の数字の正体

プロセス内部で `Runtime.getRuntime()` を呼んでいるだけ。

- `used = totalMemory() - freeMemory()`
- `total = totalMemory()` = **committed heap**（`-Xmx` ではない。起動直後は小さく徐々に増える）
- `Avg tick` は `MinecraftServer` 内部の tickTimes 配列の平均

GUI は `DISPLAY` がある環境（Windows でもデスクトップ Linux でも）で出る。ヘッドレスでは `GraphicsEnvironment.isHeadless()` により生成がスキップされる。

**GUI 自体は性能ペナルティ源**（ログペインが全行をメモリ保持する等、[MC-135443](https://bugs-legacy.mojang.com/browse/MC-135443)）なので、活用する価値はない。ただし **`tickTimes` の更新自体は GUI の有無と無関係にティックループ内で常に走っている** ため、データはヘッドレスでもプロセス内に存在する。これが v2 の Fabric mod で TPS を取れる根拠。

### メモリ取得方法の比較

| 方式 | 判定 |
|---|---|
| **hsperfdata 直読み** | ★採用。HotSpot が**フラグ不要のデフォルトで**書いている mmap 領域。`jstat` はこれを読んでいるだけ。世代別 used/capacity、metaspace、GC 回数・累積時間、maxCapacity が取れ、バニラ GUI と同じ数字を完全再現できる（かつ情報量は上）。パーサ自作が必要（〜250行）。同一 UID が必要だがラッパーなら自明に満たす。`-XX:-UsePerfData` 指定時は読めないのでフォールバック必須 |
| **`-Xlog:gc` パース** | ★採用。`2048M->512M(4096M) 12.345ms` から GC 前後・総容量・停止時間。**GC 直後の谷 = 真のメモリ逼迫指標**。イベント駆動なので連続値にならず hsperfdata の補完として使う。`file=` 必須（stdout に混ぜるとコンソールが汚れる） |
| **`/proc` RSS + cgroup** | ★採用。依存ゼロで必ず取れる最下層。**OOM Kill を予測できるのはこちらだけ** |
| Attach API 自前実装 | 不採用。SIGQUIT を使うのが行儀悪く、テキスト出力でバージョン差に弱い |
| JMX 注入 | 不採用。クライアントが RMI なので Go から喋るのが非常に面倒 |

**ヒープはノコギリ波**（GC までゴミが溜まる）なので、瞬間値だけ出すと「常に危険に見える」UI になる。GC 後の谷を併記する。

### 既存ツール

| ツール | 評価 |
|---|---|
| [mark2](https://github.com/mark2devel/mark2) | CPU/メモリ/プレイヤー一覧をパネル表示。最も近いが Python/Twisted 製で開発停滞（open PR 0、Collaborators Needed） |
| [mcServerWrapper](https://github.com/ezterry/mcServerWrapper) | curses 製・軽量だが機能が少ない |
| [mcrcon](https://github.com/tiiffi/mcrcon) / [minecraft-tui](https://github.com/rdlu/minecraft-tui) | コマンド送信のみ。監視機能なし |
| Pterodactyl / Crafty Controller / MCSManager | 重量級 Web パネル。CLI ではなく位置づけが違う |
| [minecraft-prometheus-exporter](https://github.com/sladkoff/minecraft-prometheus-exporter) | Bukkit/Paper プラグイン必須で**バニラ不可**。Prometheus + Grafana 前提 |

→ 「バニラで、プラグインなしで、ラッパーとして、ヒープと RSS を両方出す」ものは存在しない。

### TPS と Fabric mod（v2）

TPS / avg tick は**外部から取得する手段が事実上ない**。hsperfdata にも GC ログにも tick 情報はなく、バニラに `/tps` コマンドもない（Paper/Spigot 限定）。

- v1 で取れるのは `Can't keep up!` による**ラグ発生イベントのみ**（連続的な TPS 値にはならない）
- 正確な値には `MinecraftServer.tickTimes` へのアクセスが必要 = プロセス内へのコード注入が必須

**`-javaagent` ではなく Fabric mod を採用**する。両者は別レイヤー:

| | `-javaagent` | Fabric mod |
|---|---|---|
| ロード主体 | JVM 本体 | Fabric Loader |
| 配置 | 任意パス、フラグで明示 | `mods/` に置くだけ |
| 起動コマンド変更 | **必要** | **不要** |
| 難読化対処 | **自前でマッピング取得・追従が必要**（バニラ jar は ProGuard 難読化済み。`tickTimes` → `tickTimesNanos` のように名前も型もバージョンで変わる） | Loom が自動処理、公式 API あり |
| 対象 | バニラ含む何でも | Fabric サーバーのみ |

agent 方式は「マイクラのバージョンアップ追従を永続的に背負う」選択であり、TPS のためだけに払うコストとして見合わない。Fabric mod なら同じことが 50 行程度で書ける。

mod → hso のデータ受け渡しは **stdout に prefix 付きで出力**（`[HSO] tick_ms=4.32`）が最も安く、hso は既に stdout を読んでいるので IPC が不要。汚くなったら Unix ドメインソケットに移行する。

mod が開ける扉は TPS だけではなく、MSPT 分布（p95/最大）・次元ごとの tick 時間・ロード済みチャンク数・エンティティ数・プレイヤーごとの Ping まで届く（= プラグインなしの spark 相当）。

---

## マイルストーン

**v1（バニラ / Forge / NeoForge、mod なし）**
- [x] 起動スクリプトのラップ、stdin/stdout 接続、java PID 特定
- [x] hsperfdata パーサ
- [x] `/proc` RSS + cgroup 取得
- [x] `JAVA_TOOL_OPTIONS` 注入と GC ログ tail
- [x] ログ分類パーサ（チャット / コマンド / 参加退出 / ラグ / その他）
- [x] TUI（統計ヘッダ + 3ペイン + 入力欄）
- [x] コマンド送信、restart / stop の操作
- [x] パネル選択 / フォーカスとスクロール、キー説明行
- [x] CPU / Heap / RSS のメーター表示、プレイヤー一覧パネル
- [x] プレイヤー選択からのコマンド組み立て（モーダル）
- [x] GitHub Actions で amd64 / arm64 リリース

**v2**
- [ ] Fabric mod による TPS / MSPT 取得
- [ ] `hso install-mod`（go:embed した jar の書き出し）

**v3 以降**
- [ ] 表示要素・レイアウトのカスタマイズ
- [ ] バニラ向け `-javaagent` 方式（マッピング取得基盤ごと実装）
- [ ] （検討のみ）Windows 対応。hsperfdata は Windows でも `%TEMP%\hsperfdata_<user>\<pid>` に出るので流用できるが、RSS 取得に Win32 API（`GetProcessMemoryInfo`）が必要で `CGO_ENABLED=0` の静的ビルド方針と要調整。優先度は低い

---

## 未決定・要検証

1. ログ分類の正規表現をバージョン差・MOD 差に対してどこまで堅牢にするか。設定で上書き可能にすべきか
2. `1.18` 以降の bundler 形式サーバーでの PID 特定挙動（bundler が別プロセスを起こすかどうか）の実機確認
3. ペイン3分割が実用的な情報密度か、実装後に評価
4. stdout をパイプに繋いだ際、サーバー側のログ出力形式（色付け等）が変化しないかの確認
