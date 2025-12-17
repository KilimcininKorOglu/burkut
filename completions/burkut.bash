# Burkut bash completion script
# Add to ~/.bashrc: source /path/to/burkut.bash
# Or copy to /etc/bash_completion.d/burkut

_burkut() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    # All options
    opts="-o --output -P --output-dir -c --continue -n --connections
          -T --timeout -q --quiet -v --verbose --progress --no-color
          -h --help -V --version --limit-rate --checksum --proxy
          --no-check-certificate --config --profile --init-config
          -i --input-file --on-complete --on-error --webhook
          --mirrors --http3 --netrc -u --user -H --header"

    # Handle options that require arguments
    case "${prev}" in
        -o|--output)
            COMPREPLY=( $(compgen -f -- "${cur}") )
            return 0
            ;;
        -P|--output-dir)
            COMPREPLY=( $(compgen -d -- "${cur}") )
            return 0
            ;;
        -n|--connections)
            COMPREPLY=( $(compgen -W "1 2 4 8 16 32" -- "${cur}") )
            return 0
            ;;
        -T|--timeout)
            COMPREPLY=( $(compgen -W "10s 30s 60s 120s 5m 10m" -- "${cur}") )
            return 0
            ;;
        --progress)
            COMPREPLY=( $(compgen -W "bar minimal json none" -- "${cur}") )
            return 0
            ;;
        --limit-rate)
            COMPREPLY=( $(compgen -W "100K 500K 1M 5M 10M 50M 100M" -- "${cur}") )
            return 0
            ;;
        --checksum)
            COMPREPLY=( $(compgen -W "md5: sha1: sha256: sha512: blake3:" -- "${cur}") )
            return 0
            ;;
        --proxy)
            COMPREPLY=( $(compgen -W "http:// https:// socks5://" -- "${cur}") )
            return 0
            ;;
        --config)
            COMPREPLY=( $(compgen -f -X "!*.yaml" -- "${cur}") $(compgen -f -X "!*.yml" -- "${cur}") )
            return 0
            ;;
        --profile)
            # Try to read profiles from config file
            local config_file="${HOME}/.config/burkut/config.yaml"
            if [[ -f "${config_file}" ]]; then
                local profiles=$(grep -E "^  [a-zA-Z0-9_-]+:$" "${config_file}" 2>/dev/null | sed 's/://g' | tr -d ' ')
                COMPREPLY=( $(compgen -W "${profiles}" -- "${cur}") )
            else
                COMPREPLY=( $(compgen -W "fast slow tor" -- "${cur}") )
            fi
            return 0
            ;;
        -i|--input-file)
            COMPREPLY=( $(compgen -f -X "!*.txt" -- "${cur}") $(compgen -f -X "!*.url" -- "${cur}") )
            return 0
            ;;
        --on-complete|--on-error)
            COMPREPLY=( $(compgen -c -- "${cur}") )
            return 0
            ;;
        -u|--user)
            # No completion for credentials
            return 0
            ;;
        -H|--header)
            COMPREPLY=( $(compgen -W "Authorization: Content-Type: Accept: X-API-Key: User-Agent:" -- "${cur}") )
            return 0
            ;;
        *)
            ;;
    esac

    # Complete options or URLs
    if [[ ${cur} == -* ]]; then
        COMPREPLY=( $(compgen -W "${opts}" -- "${cur}") )
    elif [[ ${cur} == http* ]] || [[ ${cur} == ftp* ]] || [[ ${cur} == sftp* ]]; then
        # URL completion - just return what user typed
        COMPREPLY=( "${cur}" )
    fi

    return 0
}

complete -F _burkut burkut
