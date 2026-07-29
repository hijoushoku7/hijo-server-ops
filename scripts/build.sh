#!/bin/sh

set -eu

go_version=1.25.12
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

if command -v go >/dev/null 2>&1; then
	go_command=go
elif [ -x "$HOME/.local/share/go-$go_version/bin/go" ]; then
	go_command="$HOME/.local/share/go-$go_version/bin/go"
else
	echo "Goが見つかりません。docs/build.mdの手順でGo $go_versionを導入してください。" >&2
	exit 1
fi

cd "$project_dir"

"$go_command" version
"$go_command" test ./...
CGO_ENABLED=0 "$go_command" build \
	-ldflags="-s -w" \
	-o hso \
	./cmd/hso

echo "ビルド完了: $project_dir/hso"
"$go_command" version -m ./hso
