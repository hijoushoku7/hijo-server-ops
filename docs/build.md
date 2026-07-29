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

テストを実行する。

```bash
go test ./...
```

Linux向けの静的バイナリをビルドする。

```bash
CGO_ENABLED=0 go build \
  -ldflags="-s -w" \
  -o hso \
  ./cmd/hso
```

このコマンドは、リポジトリ直下の既存`hso`を最新ソースからビルドした
バイナリで置き換える。

ビルド情報を確認する。

```bash
go version -m ./hso
```

## 起動

設定ファイルを指定して起動する。

```bash
./hso -config /path/to/hso.toml
```

リポジトリ内の検証用Minecraftサーバーを使用する場合は、次のように起動する。

```bash
./hso -config mc-server-test/hso.toml
```
