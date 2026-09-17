#!/usr/bin/env bash
# バイナリにリンクされる依存のライセンスを THIRD_PARTY_LICENSES.md にまとめる。
# 本文が同じライセンスは 1 つに束ね、著作権表示だけをモジュールごとに並べる。
# 依存を足したら実行して差分をコミットする（deps workflow が古さを検出する）。
set -euo pipefail

cd "$(dirname "$0")/.."
out=THIRD_PARTY_LICENSES.md
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# ライセンスファイルは「見出し行と著作権行 → 本文」の順になっている。BSD のように
# 本文の途中にも "copyright notice, ..." で始まる行があるため、両者を分ける境目は
# 本文の 1 行目とし、そこから前を頭、後ろを本文として扱う。
# 見出し行は "MIT License" や "BSD 3-Clause License" のように短くて licen[cs]e を
# 含む行とする。ライセンス名を列挙すると、知らない形式が来たとき頭を本文と誤認して
# 著作権行を落とす（その場合は下の空チェックで落とす）。
head_or_body() {
  awk -v want="$2" '
    !started && (/^[[:space:]]*$/ ||
                 tolower($0) ~ /^[[:space:]]*copyright/ ||
                 tolower($0) ~ /^[[:space:]]*all rights reserved/ ||
                 (length($0) < 60 && tolower($0) ~ /licen[cs]e/)) {
      if (want == "head" && tolower($0) ~ /^[[:space:]]*copyright/) print
      next
    }
    { started = 1; if (want == "body") print }
  ' "$1"
}

body() { head_or_body "$1" body; }

# 権利者が複数ある場合に落とさないよう、頭にある著作権行はすべて並べる。
# 区切りを入れるだけで原文には触らない（著作権行が ";" を含むことがある）。
copyrights() {
  head_or_body "$1" head | awk '{ printf "%s%s", sep, $0; sep = "; " } END { print "" }'
}

# 本文が同一かどうかは空白を潰した文字列で判定する。同じ MIT でも改行位置や
# "The MIT License (MIT)" の有無が違うだけのことが多い。
normalize() { body "$1" | tr -s '[:space:]' ' '; }

# 束に 1 件加える。$1 = 表示名、$2 = ライセンスファイル。
add() {
  local copyright key
  copyright=$(copyrights "$2")
  if [ -z "$copyright" ]; then
    echo "著作権表示を読み取れない: $1 ($2)" >&2
    exit 1
  fi

  key=$(normalize "$2" | md5sum | cut -d' ' -f1)
  mkdir -p "$work/$key"
  # 本文は最初に見つけたものを採用する。著作権行はモジュールごとに集める。
  [ -f "$work/$key/body" ] || cp "$2" "$work/$key/body"
  printf '%s\t%s\n' "$1" "$copyright" >> "$work/$key/mods"
}

# ja は無タグ、en は -tags en でビルドする。タグごとに依存が変わっても拾えるよう
# 両方を流し込む（重複は sort -u で落ちる）。
{
  go list -deps -f '{{if .Module}}{{.Module.Path}}@{{.Module.Version}}	{{.Module.Dir}}{{end}}' ./...
  go list -tags en -deps -f '{{if .Module}}{{.Module.Path}}@{{.Module.Version}}	{{.Module.Dir}}{{end}}' ./...
} | sort -u | while IFS=$'\t' read -r mod dir; do
  [ -n "$dir" ] || continue
  case "$mod" in github.com/hijoushoku7/hijo-server-ops@*) continue ;; esac

  license=$(find "$dir" -maxdepth 1 -iregex '.*/\(LICENSE\|LICENCE\|COPYING\)[^/]*' | sort)
  case $(printf '%s\n' "$license" | grep -c .) in
    0) echo "ライセンスファイルが見つからない: $mod" >&2; exit 1 ;;
    1) ;;
    # 二重ライセンスなど、どれを載せるかは機械的に決められない。人が見る。
    *) echo "ライセンスファイルが複数ある: $mod" >&2; printf '  %s\n' $license >&2; exit 1 ;;
  esac

  add "$mod" "$license"
done

# Go 本体も静的リンクされる。LICENSE は golang.org/x/* と同一の BSD-3-Clause なので
# 同じ束に入って 1 つにまとまる。
add "Go standard library and runtime" "$(go env GOROOT)/LICENSE"

{
  echo "# サードパーティライセンス / Third-party licenses"
  echo
  echo "hso のバイナリには以下の Go モジュールと Go 本体が静的リンクされている。"
  echo "同じ本文のライセンスはまとめ、著作権表示を原文のまま列挙する。"
  echo
  echo "The hso binary statically links the Go modules listed below, plus the Go"
  echo "standard library and runtime. Licenses sharing the same text are grouped."
  echo
  echo "このファイルは \`./scripts/gen-third-party-licenses.sh\` で生成する。"

  # モジュール数の多い束から出す。
  for d in $(for k in "$work"/*/; do printf '%s\t%s\n' "$(wc -l < "$k/mods")" "$k"; done |
               sort -rn | cut -f2); do
    if grep -qiE 'Neither the name of' "$d/body"; then
      name="BSD 3-Clause License"
    elif grep -qiE 'Redistributions of source code' "$d/body"; then
      name="BSD 2-Clause License"
    elif grep -qiE 'Permission is hereby granted, free of charge' "$d/body"; then
      name="MIT License"
    else
      name="License"
    fi

    echo
    echo "## $name"
    echo
    while IFS=$'\t' read -r mod copyright; do
      echo "- \`$mod\` — $copyright"
    done < "$d/mods"
    echo
    echo '```'
    # 見出しと著作権行は上に列挙済みなので本文からは落とす。
    body "$d/body"
    echo '```'
  done
} > "$out"

echo "generated $out" >&2
