# サーバーインストーラのフェーズ計画

`hso` のセットアップウィザードから Vanilla / Fabric / Paper / Forge / NeoForge を
インストールできるようにする。1 フェーズ 1 PR。各フェーズ単体で main に入れて壊れない
ことを条件に切ってある。

## 決まっていること

### ウィザードの流れ

```
1. 種別の選択        vanilla / fabric / paper / forge / neoforge
2. MC バージョン選択  ← 種別ごとの API から取る（Mojang の一覧は使わない）
3. ローダー選択       vanilla は飛ばす
4. ディレクトリ確認
5. EULA 同意 + 内容確認 → インストール実行
```

既存の作成ウィザード（workDir → name → command → confirm）の `command` を選ぶ手前に
「新規インストール」の枝を足す形にする。既存サーバーの登録経路は触らない。

### バージョン一覧の取得元

| 種別 | step 2（MC 一覧） | step 3（ローダー一覧） |
|---|---|---|
| Vanilla | `piston-meta.mojang.com/mc/game/version_manifest_v2.json` の `versions[]` | なし |
| Fabric | `meta.fabricmc.net/v2/versions/game` | `/v2/versions/loader/{mc}` |
| Paper | `fill.papermc.io/v3/projects/paper` の `versions` のキー | ビルドは実行時に解決（下記） |
| Forge | `files.minecraftforge.net/.../promotions_slim.json` のキーを分解 | 同 JSON の recommended / latest |
| NeoForge | `maven.neoforged.net/api/maven/versions/releases/net/neoforged/neoforge` から導出 | 同じ一覧を MC で絞る |

- **Paper は User-Agent 必須。** `hso/<version> (https://github.com/hijoushoku7/hijo-server-ops)`
  のように名乗る。generic な UA は弾かれる。
- **NeoForge は MC バージョンを持たない。** `21.1.209` → MC `1.21.1` の規則で逆算する。
  規則から外れる版（`0.25w14craftmine.3-beta` 等）は捨てる。
- **Forge は recommended が無い MC がある。** その場合は latest だけを出す。
- **ダウンロード URL と sha は一覧に含めない。** 実行時に上流から取り直す。Paper のビルドは
  1 時間おきに出るので、キャッシュに焼くと古いビルドを掴む。

### スナップショットのトグル

既定は正式リリースのみ。Vanilla（`type != "release"`）と Fabric（`stable != true`）で
同じキーを使い、同じ挙動にする。別実装にしない。

### Forge / NeoForge の全ビルド取得

step 3 は既定で recommended / latest の 2 択。**別キー**を押したときだけ
`maven.minecraftforge.net/net/minecraftforge/forge/maven-metadata.xml`（211KB / 5046 件）を
引いて、選択中の MC に前方一致する版だけを一覧にする。既定の経路では引かない。

### キャッシュ

- 置き場: サーバー一覧（`config.toml`）と同じディレクトリ。ファイル名は別（`versions.json`）
- 形: 1 ファイル 1 JSON。`{取得時刻, 種別ごとの一覧}`
- TTL: 12 時間。切れていたら取り直し、その間はローディングを出して待つ
- **裏での非同期更新はしない。** キャッシュがあればそれを出し、切れていたら待つだけ
- Fabric の `loader/{mc}` は 590KB あるので、**直近に選んだ MC の 1 件だけ**保持する
- ロックは取らない。壊れても消して取り直せばいいだけなので、一時ファイル → rename のみ
- 取得に失敗しても**画面には何も出さない**。オフラインで毎回赤字を出さないため。
  キャッシュの取得時刻だけは常時小さく表示する

### ローディング表示

`internal/setup` の `Update` は現在キー入力しか見ていない（`model.go:80`）。
取得中を描くために、キー以外の msg を先に捌いてから既存の `updateXxx` へ流す形に変える。
既存ステップの関数シグネチャは変えない。3 秒で諦めてバージョン手入力へ落とす。

### Java

- **選択の前に Java の有無で弾かない。** Java 設定は主機能を止めない（issue #64 の原則）
- Forge / NeoForge のインストーラ実行が Java で失敗したときは、**エラーをそのまま画面に出す**
- インストール後、`javaenv.Installed` で見つかった JDK から使うものを選べるようにし、
  選んだら `hso.toml` の `[server] java` に書く。MC のバージョンによって動く Java が
  違うため、インストール直後に決められる必要がある

## フェーズ

### Phase 1: バージョン取得とキャッシュ（UI なし）

`internal/mcversions`（仮）を新規追加する。UI からは呼ばない。ユーザーから見た変化はない。

- 5 種類の API クライアント。`net/http` + `encoding/json` のみ。**依存追加なし**
- NeoForge の版番号 → MC バージョンの逆算
- Forge の promotions のキー分解（recommended 欠落の扱いを含む）
- キャッシュの読み書きと TTL 判定
- 各 API のレスポンスを固定した `httptest` ベースのテスト

Forge の maven-metadata.xml は Phase 3 で足す。

### Phase 2: ウィザードへの組み込みと Vanilla / Fabric / Paper のインストール

ここで初めてユーザーから見える。ダウンロードだけで完結する 3 種類を通す。

- ウィザードに種別 → MC → ローダーの 3 ステップを追加
- `Update` にキー以外の msg の口を開ける。ローディング表示
- スナップショットのトグル
- ダウンロードと進捗表示。Vanilla は sha1、Paper は sha256 を検証する
- EULA 同意画面と `eula.txt` の生成
- `run.sh` の生成と実行権の付与。既存の「起動スクリプトに実行権を付ける箇所」と同じ扱いに寄せる
- 生成した `run.sh` を `command` として `hso.toml` を書き、そのまま起動へ繋ぐ

### Phase 3: Forge / NeoForge

インストーラの実行が要るのでここだけ性質が違う。

- インストーラ jar のダウンロードと `java -jar ... --installServer` の実行
- 実行中の出力を画面に流す。失敗時はエラーをそのまま出す（握り潰さない）
- **出力レイアウトが MC バージョンで変わる**
  - 1.17+ : `run.sh` + `user_jvm_args.txt` が生成される → それを `command` にする
  - 1.16 以前 : `forge-{mc}-{forge}.jar` 単体 → `run.sh` をこちらで書く
- 別キーでの全ビルド取得（maven-metadata.xml）

### Phase 4: Java の選択

- インストール完了後に `javaenv.Installed` の一覧から使う JDK を選ぶ
- 選択結果は `hso.toml` を新規作成するときにそのまま `[server] java` として書く。
  `config.SetJava` は既存の設定を書き換えるためのもので、ここでは作りたての設定を
  読み直して書き換えることになるので使わない
- 選ばずに飛ばせること。飛ばしても起動はできる

## やらないこと

- modpack（CurseForge / Modrinth）。API も認証も別物
- ダウンロードの再開・ミラー
- バージョン一覧の事前集約（GitHub Actions で JSON を作って配る案）。
  実測で最も遅い NeoForge でも 0.8 秒、raw.githubusercontent.com 自体が 0.29 秒なので
  速度の理由にならない。Paper の UA ポリシーで弾かれた場合や、上流の一覧取得が
  実際に失敗した報告が出た場合に再検討する
- 既存サーバーのバージョンアップ。インストールが通ってから考える
