# ビルド手順

`hijo-server-ops` は Go 1.25 以上でビルドする。対象OSは Linux のみ。

## Goの導入

ここでは Go 1.25.12 をユーザー領域へインストールする。管理者権限は不要。

### amd64

```bash
curl -fsSLo /tmp/go1.25.12.linux-amd64.tar.gz \
  https://go.dev/dl/go1.25.12.linux-amd64.tar.gz

mkdir -p "$HOME/.local/share/go-1.25.12"

tar -xzf /tmp/go1.25.12.linux-amd64.tar.gz \
  -C "$HOME/.local/share/go-1.25.12" \
  --strip-components=1
```

arm64環境では、ファイル名とURLの`linux-amd64`を`linux-arm64`へ
置き換える。

現在のシェルでGoを使えるようにする。

```bash
export PATH="$HOME/.local/share/go-1.25.12/bin:$PATH"
```

`export`は現在のシェルにしか反映されない。ログイン後も有効にする場合は、
同じ行を`~/.profile`など、使用しているシェルの設定ファイルへ追加する。

導入結果を確認する。

```bash
go version
```

次のように表示されれば導入完了。

```text
go version go1.25.12 linux/amd64
```

ダウンロードしたアーカイブが不要になったら削除できる。

```bash
rm /tmp/go1.25.12.linux-amd64.tar.gz
```

## hsoのビルド

リポジトリ直下へ移動する。

```bash
cd /path/to/hijo-server-ops
```

ビルドスクリプトを実行する。

```bash
./scripts/build.sh
```

スクリプトは次の処理を順番に実行する。

1. PATHまたは`$HOME/.local/share/go-1.25.12/bin/go`からGoを検出
2. `go test ./...`で全テストを実行
3. `CGO_ENABLED=0`でLinux向けの静的バイナリをビルド
4. `go version -m ./hso`でビルド情報を表示

スクリプトを使わず、ビルドだけを直接実行する場合は次のコマンドを使う。

```bash
CGO_ENABLED=0 go build \
  -ldflags="-s -w" \
  -o hso \
  ./cmd/hso
```

どちらの方法も、リポジトリ直下の既存`hso`を最新ソースからビルドした
バイナリで置き換える。

## 起動

設定ファイルを指定して起動する。

```bash
./hso -config /path/to/hso.toml
```

リポジトリ内の検証用Minecraftサーバーを使用する場合は、次のように起動する。

```bash
./hso -config mc-server-test/hso.toml
```
