_hso() {
    local IFS=$'\n' out line values=''
    out=$(hso __complete "${COMP_WORDS[@]:0:COMP_CWORD+1}" 2>/dev/null)
    [[ $out == ':files' ]] && return 1
    while IFS=$'\t' read -r line _; do
        [[ -n $line ]] && values+="$line"$'\n'
    done <<< "$out"
    COMPREPLY=($(compgen -W "$values" -- "${COMP_WORDS[COMP_CWORD]}"))
}
complete -o default -F _hso hso
