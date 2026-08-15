<img src="https://img.shields.io/badge/Go-1.25.13-00ADD8?logo=go&logoColor=white"> <img src="https://img.shields.io/badge/platform-Linux-333">

[English](README.md) | 日本語

![hijo Server Ops](hso-animation.gif)

## hijo Server Ops

Linux で使える Minecraft サーバー用の TUI 画面ソフトウェアです。サーバーのラッパーとして動くので、いま使っている起動スクリプトはそのままで構いません。
Vanilla,Spigot,Paper,Forge,NeoForge,Fabricなど様々な環境で動作します。

## クイックスタート

### システムにインストールする（推奨）

`/usr/local/bin` に入ります。全ユーザーが使えて、PATH の設定は要りません。日本語版を入れるには次を実行します。

```bash
curl -fsSL https://raw.githubusercontent.com/hijoushoku7/hijo-server-ops/main/install.sh | sh -s -- --system --lang ja
```

`sudo` はパイプに付けないでください。スクリプトは一般ユーザーの権限でダウンロードと検証を行い、`/usr/local/bin` へ置く最後の処理だけ `sudo` を使います。

### 自分のホームにインストールする

root 権限が無い、または使いたくない場合は `~/.local/bin` に入れます。

```bash
curl -fsSL https://raw.githubusercontent.com/hijoushoku7/hijo-server-ops/main/install.sh | sh -s -- --lang ja
```

`~/.local/bin` が PATH に無い環境では、インストール後に追記する 1 行が表示されます。

表示は既定で英語です。上記の `--lang ja` を外すと英語版が入り、環境変数を使う場合は `curl -fsSL https://raw.githubusercontent.com/hijoushoku7/hijo-server-ops/main/install.sh | env HSO_LANG=ja sh -s -- --system` のように指定できます。フラグの指定は環境変数より優先されます。

インストール後に Minecraft サーバーのディレクトリで `hso setup` を実行すると、設定ウィザードが開きます。サーバーのディレクトリを入力し、起動スクリプトを一覧から選ぶだけで、そのままサーバーが立ち上がります。2 回目からは `hso start` で登録済みのサーバーを選んで起動します。hso 本体は `sudo` で実行しないでください。

コマンドの一覧は `hso` または `hso help` で表示できます（→ [コマンド一覧](dev-docs/commands.md)）。

## 機能

- Heap（Java が確保したメモリ）と RSS（実際の使用メモリ）を別々に表示。その差分も出すので、`-Xmx` を積んだのにメモリ不足になるなどの原因が見える
- メモリ推移のグラフと GC の統計
- プレイヤー一覧から選んでコマンドを実行
- ログからチャットだけを抜き出して表示

サーバー側にプラグインや MOD を入れる必要はありません。

## 手動インストール

[Releases](https://github.com/hijoushoku7/hijo-server-ops/releases) から環境に合うアーカイブを取得して展開します。

```bash
tar xzf hso_v0.1.1_linux_amd64_ja.tar.gz
cd hso_v0.1.1_linux_amd64_ja
./hso setup
```

arm64 なら `arm64`、英語表示がよければ `_en` のアーカイブを選んでください。

## Java の切り替えと確認

```bash
hso java change [name]
hso java list
```

`change` は登録済みサーバーが使う Java を、`/usr/lib/jvm` から自動検出した候補から選んで `hso.toml` に保存します。サーバーが起動中でも変更でき、反映は次回起動からです。`list` は検出した JVM と、それぞれを使う登録済みサーバーを表示します。SDKMAN、asdf、`/opt` などの Java は自動検出されないため、使う場合は `hso.toml` の `[server] java` に JAVA_HOME の絶対パスを指定してください。

## 更新

```bash
hso update
```

最新リリースから、いま動いているものと同じアーキテクチャ・同じ表示言語のバイナリを取得し、SHA-256 で照合してから自分自身を置き換えます。すでに最新なら何もしません。

`/usr/local/bin` に入れている場合は、**置き換えの一手だけ** `sudo` / `doas` でパスワードを聞かれます。`sudo hso update` と打つ必要はありません（取得も展開も root で走ってしまいます）。`~/.local/bin` なら昇格なしで通ります。

## アンインストール

```bash
hso uninstall
```

削除するパスを表示して確認を取ってから、いま動いているバイナリを削除します。サーバー一覧は残り、サーバーディレクトリ内の `hso.toml` やワールドには触れません。確認を省略するには `-y` / `--yes` を付けます。

`/usr/local/bin` に入れているバイナリだけを削除する場合は、`sudo hso uninstall` を実行します。アンインストール自身が `sudo` / `doas` を起動することはありません。

サーバー一覧と pidfile も消す場合は、**sudo を付けずに**実行します。

```bash
hso uninstall --purge
```

`/usr/local/bin` に入れている場合は、先に通常ユーザーで設定を消し、残ったバイナリを root で削除します。

```bash
hso uninstall --purge
sudo hso uninstall
```

バイナリが壊れて `uninstall` を実行できない場合は、手動で削除できます。
`HSO_INSTALL_DIR` を指定していた場合は、1 行目をそのディレクトリ内の `hso` に読み替えてください。

```bash
rm "$HOME/.local/bin/hso"                 # または sudo rm /usr/local/bin/hso
rm -rf "${XDG_CONFIG_HOME:-$HOME/.config}/hso"
if [ -n "${XDG_RUNTIME_DIR:-}" ]; then
  rm -rf "$XDG_RUNTIME_DIR/hso"
else
  rm -rf "/tmp/hso-$(id -u)"
fi
```

pidfile は再起動でも消えます。

## ドキュメント

- [コマンド一覧](dev-docs/commands.md)
- [ビルド手順](dev-docs/build.md)
- [仕様・技術調査](dev-docs/spec.md)

## 作者

hijoushoku https://github.com/hijoushoku7
A Student Engineer from Japan🗾
