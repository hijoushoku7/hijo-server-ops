<img src="https://img.shields.io/badge/Go-1.25.12-00ADD8?logo=go&logoColor=white"> <img src="https://img.shields.io/badge/platform-Linux-333">

![hijo Server Ops](hso-animation.gif)

## hijo Server Ops

Linux で使える Minecraft サーバー用の TUI 画面ソフトウェアです。サーバーのラッパーとして動くので、いま使っている起動スクリプトはそのままで構いません。

## クイックスタート

[Releases](https://github.com/hijoushoku7/hijo-server-ops/releases) からアーカイブを取得して展開します。

```bash
tar xzf hso_v0.1.1_linux_amd64_ja.tar.gz
cd hso_v0.1.1_linux_amd64_ja
./hso
```

初回は設定ウィザードが開きます。サーバーのディレクトリを入力し、起動スクリプトを一覧から選ぶだけで、そのままサーバーが立ち上がります。

arm64 なら `arm64`、英語表示がよければ `_en` のアーカイブを選んでください。

## 機能

- Heap（Java が確保したメモリ）と RSS（実際の使用メモリ）を別々に表示
- その差分も表示。`-Xmx` を積んだのに OOM Killer に殺される、の原因が見える
- メモリ推移をグラフ表示
- GC の回数と平均停止時間
- 接続中のプレイヤー数と一覧
- プレイヤーを選んでコマンドを実行
- ログからチャットだけを抜き出して表示

サーバー側にプラグインや MOD を入れる必要はありません。

## ドキュメント

- [ビルド手順](docs/build.md)
- [仕様・技術調査](docs/spec.md)

## 作者

hijoushoku https://github.com/hijoushoku7
A Student Engineer from Japan🗾