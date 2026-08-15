_hso() {
    local IFS=$'\n' out line values=''
    out=$(hso __complete "${COMP_WORDS[@]:0:COMP_CWORD+1}" 2>/dev/null)
    # ファイル補完へ回すのは :files を返した位置だけ。complete に -o default を
    # 付けると候補ゼロの位置でも既定のファイル補完が出てしまうため、ここで
    # その位置に限って有効にする。
    if [[ $out == ':files' ]]; then
        compopt -o default
        return 0
    fi
    while IFS=$'\t' read -r line _; do
        [[ -n $line ]] && values+="$line"$'\n'
    done <<< "$out"
    COMPREPLY=($(compgen -W "$values" -- "${COMP_WORDS[COMP_CWORD]}"))
}
complete -F _hso hso
