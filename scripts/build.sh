#!/bin/sh

set -eu

go_version=1.25.12
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

if command -v go >/dev/null 2>&1; then
	go_command=go
elif [ -x "$HOME/.local/share/go-$go_version/bin/go" ]; then
	go_command="$HOME/.local/share/go-$go_version/bin/go"
else
	echo "go not found. Install Go $go_version by following docs/build.md." >&2
	exit 1
fi

cd "$project_dir"

"$go_command" version
echo "build started: $project_dir"

# 日本語版はタグなし、英語版は -tags en。テストも両方のタグで回す。片方の
# 言語ファイルでキーを書き忘れると、そちらのタグでだけコンパイルが落ちる。
"$go_command" test ./...
"$go_command" test -tags en ./...

CGO_ENABLED=0 "$go_command" build \
	-ldflags="-s -w" \
	-o hso_ja \
	./cmd/hso
CGO_ENABLED=0 "$go_command" build \
	-tags en \
	-ldflags="-s -w" \
	-o hso_en \
	./cmd/hso

echo "dependency versions:"
"$go_command" version -m ./hso_ja
echo "build finished: $project_dir/hso_ja, $project_dir/hso_en"
