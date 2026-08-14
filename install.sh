#!/bin/sh

set -eu

usage() {
    cat <<'EOF'
Usage: install.sh [--system] [--lang ja|en] [--help]

Options:
  --system       Install to /usr/local/bin (requires sudo, doas, or root)
  --lang ja|en   Select the installed language (default: en)
  --help         Show this help and exit

Environment variables:
  HSO_INSTALL_DIR  Override the installation directory
  HSO_LANG         Select ja or en; --lang takes precedence
  GITHUB_TOKEN     Authenticate GitHub API requests
EOF
}

die() {
    printf 'Error: %s\n' "$*" >&2
    exit 1
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || die "$1 is required"
}

# 対象の sh がすべて対応しているため、設計上の例外として local を使う。
# shellcheck disable=SC3043
download() {
    local url="$1"
    local output="$2"

    if [ -n "${GITHUB_TOKEN:-}" ]; then
        curl -fsSL \
            -H 'User-Agent: hso-install' \
            -H "Authorization: Bearer $GITHUB_TOKEN" \
            -o "$output" "$url"
    else
        curl -fsSL \
            -H 'User-Agent: hso-install' \
            -o "$output" "$url"
    fi
}

# shellcheck disable=SC3043
detect_shell() {
    local name

    # $SHELL はログインシェルとは限らないため、呼び出し元のプロセスを先に見る。
    name=$(cat "/proc/${PPID:-}/comm" 2>/dev/null || printf '')
    case "$name" in
        bash|zsh|fish|ksh|dash|sh)
            printf '%s\n' "$name"
            return
            ;;
    esac
    basename "${SHELL:-sh}"
}

# shellcheck disable=SC3043
print_get_started() {
    local tag="$1"
    local destination="$2"

    printf '==> hso %s installed to %s\n\n' "$tag" "$destination"
    printf '   Get started:\n\n'
    printf '       cd /path/to/your/minecraft/server\n'
    printf '       hso setup\n'
}

# 表示する $HOME と $PATH は、インストーラでは展開しない。
# shellcheck disable=SC3043,SC2016
print_debian_path_guidance() {
    local tag="$1"
    local destination="$2"
    local dir="$3"

    printf '==> hso %s installed to %s\n\n' "$tag" "$destination"
    printf '!! %s is not in your PATH yet.\n' "$dir"
    printf '   Your ~/.profile already adds it at login, but it was skipped this time\n'
    printf '   because the directory did not exist until now.\n\n'
    printf '   Log out and back in, or run:\n\n'
    printf '       %s\n' 'export PATH="$HOME/.local/bin:$PATH"'
    printf '       hash -r\n'
}

# PATH へ追記する行の $HOME は、案内を実行したときに展開させる。
# shellcheck disable=SC3043,SC2016
print_path_guidance() {
    local tag="$1"
    local destination="$2"
    local dir="$3"
    local shell_name rc_file add_line now_line path_entry home_marker='~'

    case ":${PATH:-}:" in
        *":$dir:"*)
            print_get_started "$tag" "$destination"
            return
            ;;
    esac

    if [ -n "${HOME:-}" ] && \
        [ "$dir" = "$HOME/.local/bin" ] && \
        [ -f "$HOME/.profile" ] && \
        grep -q '\.local/bin' "$HOME/.profile"; then
        print_debian_path_guidance "$tag" "$destination" "$dir"
        return
    fi

    if [ -n "${HOME:-}" ] && [ "$dir" = "$HOME/.local/bin" ]; then
        path_entry='$HOME/.local/bin'
    else
        path_entry=$dir
    fi

    shell_name=$(detect_shell)
    case "$shell_name" in
        bash)
            rc_file=$home_marker/.bashrc
            add_line="export PATH=\"$path_entry:\$PATH\""
            now_line="$add_line
       hash -r"
            ;;
        zsh)
            rc_file=$home_marker/.zshrc
            add_line="export PATH=\"$path_entry:\$PATH\""
            now_line="$add_line
       hash -r"
            ;;
        fish)
            rc_file=$home_marker/.config/fish/config.fish
            add_line="fish_add_path \"$path_entry\""
            now_line="set -gx PATH \"$path_entry\" \$PATH"
            ;;
        *)
            rc_file=$home_marker/.profile
            add_line="export PATH=\"$path_entry:\$PATH\""
            now_line=$add_line
            ;;
    esac

    printf '==> hso %s installed to %s\n\n' "$tag" "$destination"
    printf "!! %s is not in your PATH, so \`hso\` will not be found yet.\n\n" "$dir"
    printf '   To use it in this shell right now:\n\n'
    printf '       %s\n\n' "$now_line"
    printf '   To keep it after you log out, add this line to %s:\n\n' "$rc_file"
    printf '       %s\n\n' "$add_line"
    printf '   Or install system-wide instead, where no PATH setup is needed:\n\n'
    printf '       curl -fsSL https://raw.githubusercontent.com/hijoushoku7/hijo-server-ops/main/install.sh | sh -s -- --system\n'
}

# shellcheck disable=SC3043
verify_checksum() {
    local checksums="$1"
    local archive="$2"
    local asset="$3"
    local expected='' actual='' checksum filename

    while IFS=' ' read -r checksum filename; do
        filename=${filename#\*}
        if [ "$filename" = "$asset" ]; then
            expected=$checksum
            break
        fi
    done < "$checksums"

    if ! printf '%s\n' "$expected" | grep -Eq '^[0-9A-Fa-f]{64}$'; then
        die "no valid SHA-256 checksum was found for $asset"
    fi

    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$archive")
    else
        actual=$(shasum -a 256 "$archive")
    fi
    actual=${actual%% *}

    [ "$actual" = "$expected" ] || die "SHA-256 checksum verification failed for $asset"
}

# shellcheck disable=SC3043
asset_exists() {
    local release_json="$1"
    local asset="$2"

    grep -q "\"name\"[[:space:]]*:[[:space:]]*\"$asset\"" "$release_json"
}

# shellcheck disable=SC3043
install_binary() {
    local privileged="$1"
    local source="$2"
    local dir="$3"

    # $1 and $2 are positional parameters received by the inner shell.
    # shellcheck disable=SC2016
    set -- sh -c '
        mkdir -p "$2" || exit 1
        staging=$(mktemp "$2/.hso-XXXXXX") || exit 1
        trap '\''rm -f "$staging"'\'' 0
        trap '\''exit 1'\'' 1 2 15
        cp "$1" "$staging" || exit $?
        chmod 0755 "$staging" || exit $?
        mv -f "$staging" "$2/hso" || exit $?
        trap - 0 1 2 15
    ' _ "$source" "$dir"

    if [ -n "$privileged" ]; then
        "$privileged" "$@"
    else
        "$@"
    fi
}

# shellcheck disable=SC3043
main() {
    local system=false
    local lang="${HSO_LANG:-en}"
    local lang_from_flag=''
    local dir user_id privileged=''
    local os arch work release_json tag_lines tag
    local asset archive checksums archive_dir binary

    while [ "$#" -gt 0 ]; do
        case "$1" in
            --system)
                system=true
                shift
                ;;
            --lang)
                [ "$#" -ge 2 ] || die "--lang requires ja or en"
                lang_from_flag=$2
                shift 2
                ;;
            --help)
                usage
                return
                ;;
            *)
                die "unknown option: $1"
                ;;
        esac
    done

    if [ -n "$lang_from_flag" ]; then
        lang=$lang_from_flag
    fi
    case "$lang" in
        ja|en) ;;
        *) die "unsupported language: $lang (expected ja or en)" ;;
    esac

    if [ "${HSO_INSTALL_DIR+x}" = x ]; then
        dir=$HSO_INSTALL_DIR
    elif [ "$system" = true ]; then
        dir=/usr/local/bin
    else
        [ -n "${HOME:-}" ] || die "HOME is not set"
        dir=$HOME/.local/bin
    fi
    [ -n "$dir" ] || die "HSO_INSTALL_DIR must not be empty"

    # 権限の食い違いと昇格手段は、OS 判定や通信より前に確認する。
    user_id=$(id -u)
    if [ "$system" = false ] && [ "$user_id" = 0 ]; then
        die "do not run a user install as root; use --system or run as a regular user"
    fi

    if [ "$system" = true ] && [ "$user_id" != 0 ]; then
        if command -v sudo >/dev/null 2>&1; then
            privileged=sudo
            if [ -t 2 ]; then
                sudo -v || die "sudo authentication failed"
            else
                sudo -n true || die "sudo requires a terminal or cached credentials"
            fi
        elif command -v doas >/dev/null 2>&1; then
            privileged=doas
            if [ -t 2 ]; then
                doas true || die "doas authentication failed"
            else
                doas -n true || die "doas requires a terminal or cached credentials"
            fi
        else
            die "--system requires sudo or doas; otherwise run this installer as root"
        fi
    fi

    if [ -e "$dir" ] && [ ! -d "$dir" ]; then
        die "installation directory is not a directory: $dir"
    fi
    if [ -n "$privileged" ]; then
        # $1 は内側の sh が受け取る位置パラメータ。
        # shellcheck disable=SC2016
        "$privileged" sh -c 'mkdir -p "$1"' _ "$dir" || \
            die "could not create installation directory: $dir"
        # shellcheck disable=SC2016
        "$privileged" sh -c '[ -w "$1" ]' _ "$dir" || \
            die "installation directory is not writable: $dir"
    else
        mkdir -p "$dir" || die "could not create installation directory: $dir"
        [ -w "$dir" ] || die "installation directory is not writable: $dir"
    fi

    os=$(uname -s)
    [ "$os" = Linux ] || die "unsupported operating system: $os (Linux is required)"

    case "$(uname -m)" in
        x86_64) arch=amd64 ;;
        aarch64|arm64) arch=arm64 ;;
        *) die "unsupported architecture: $(uname -m)" ;;
    esac

    require_command curl
    require_command tar
    require_command mktemp
    require_command cmp
    if ! command -v sha256sum >/dev/null 2>&1 && \
        ! command -v shasum >/dev/null 2>&1; then
        die "sha256sum or shasum is required"
    fi

    work=$(mktemp -d) || die "could not create a temporary directory"
    trap 'rm -rf "$work"' 0 1 2 15

    release_json=$work/release.json
    if ! download \
        'https://api.github.com/repos/hijoushoku7/hijo-server-ops/releases/latest' \
        "$release_json"; then
        die "could not fetch the latest release from GitHub"
    fi

    tag_lines=$work/tags
    sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
        "$release_json" > "$tag_lines"
    tag=$(head -n 1 "$tag_lines")
    case "$tag" in
        ''|*[!A-Za-z0-9._-]*) die "GitHub returned an invalid release tag: $tag" ;;
    esac

    asset=hso_${tag}_linux_${arch}_${lang}.tar.gz
    if ! asset_exists "$release_json" "$asset"; then
        die "the latest release does not contain $asset"
    fi
    if ! asset_exists "$release_json" checksums.txt; then
        die "the latest release does not contain checksums.txt"
    fi

    archive=$work/$asset
    checksums=$work/checksums.txt
    if ! download \
        "https://github.com/hijoushoku7/hijo-server-ops/releases/download/$tag/$asset" \
        "$archive"; then
        die "could not download $asset"
    fi
    if ! download \
        "https://github.com/hijoushoku7/hijo-server-ops/releases/download/$tag/checksums.txt" \
        "$checksums"; then
        die "could not download checksums.txt"
    fi

    verify_checksum "$checksums" "$archive" "$asset"

    archive_dir=hso_${tag}_linux_${arch}_${lang}
    if ! tar -xzf "$archive" -C "$work" "$archive_dir/hso"; then
        die "could not extract hso from $asset"
    fi
    binary=$work/$archive_dir/hso
    [ -f "$binary" ] || die "$asset does not contain hso"

    if [ -f "$dir/hso" ] && cmp -s "$binary" "$dir/hso"; then
        printf '==> hso %s is already installed at %s/hso\n' "$tag" "$dir"
        rm -rf "$work"
        trap - 0 1 2 15
        return
    fi

    if ! install_binary "$privileged" "$binary" "$dir"; then
        die "could not install hso to $dir"
    fi

    print_path_guidance "$tag" "$dir/hso" "$dir"

    rm -rf "$work"
    trap - 0 1 2 15
}

main "$@"
