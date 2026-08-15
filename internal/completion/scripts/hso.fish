function __hso_complete
    set -l words (commandline -opc)
    # 打ちかけの単語は空のこともある。素の (commandline -ct) だと空のときに
    # 要素が 1 つも増えず、位置が 1 つずれる。
    set -a words (commandline -ct | string collect --allow-empty)
    set -l out (hso __complete $words 2>/dev/null)
    if test "$out" = :files
        __fish_complete_path (commandline -ct)
    else
        printf '%s\n' $out
    end
end

complete -f -c hso -a '(__hso_complete)'
