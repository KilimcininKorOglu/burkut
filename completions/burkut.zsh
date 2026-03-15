#compdef burkut

# Burkut zsh completion script
# Add to ~/.zshrc: source /path/to/burkut.zsh
# Or copy to site-functions directory

_burkut() {
    local -a opts args

    opts=(
        '(-o --output)'{-o,--output}'[Output filename]:filename:_files'
        '(-P --output-dir)'{-P,--output-dir}'[Output directory]:directory:_directories'
        '(-c --continue)'{-c,--continue}'[Resume partially downloaded file]'
        '(-n --connections)'{-n,--connections}'[Number of parallel connections]:count:(1 2 4 8 16 32)'
        '(-T --timeout)'{-T,--timeout}'[Connection timeout]:duration:(10s 30s 60s 120s 5m 10m)'
        '(-q --quiet)'{-q,--quiet}'[Quiet mode]'
        '(-v --verbose)'{-v,--verbose}'[Verbose output]'
        '--progress[Progress display style]:style:(bar minimal json none)'
        '--no-color[Disable colored output]'
        '(-h --help)'{-h,--help}'[Show help]'
        '(-V --version)'{-V,--version}'[Show version]'
        '--limit-rate[Limit download speed]:rate:(100K 500K 1M 5M 10M 50M 100M)'
        '--checksum[Verify checksum]:checksum:(md5\: sha1\: sha256\: sha512\: blake3\:)'
        '--proxy[Proxy URL]:url:(http\:// https\:// socks5\://)'
        '--no-check-certificate[Skip TLS verification]'
        '--config[Config file]:config:_files -g "*.{yaml,yml}"'
        '--profile[Use named profile]:profile:(fast slow tor)'
        '--init-config[Generate default config]'
        '(-i --input-file)'{-i,--input-file}'[URL list file]:file:_files -g "*.txt"'
        '--on-complete[Command on success]:command:_command_names'
        '--on-error[Command on error]:command:_command_names'
        '--webhook[Webhook URL]:url:'
        '--mirrors[Mirror URLs]:urls:'
        '--http1[Force HTTP/1.1]'
        '--http2[Force HTTP/2]'
        '--http3[Use HTTP/3 (QUIC)]'
        '--verify[Auto-detect and verify checksum]'
        '(-N --timestamping)'{-N,--timestamping}'[Only download if remote file is newer]'
        '--pinnedpubkey[SHA256 public key pin]:pin:'
        '--tui[Use interactive TUI mode]'
        '--metrics-addr[Prometheus metrics endpoint]:addr:'
        '--netrc[Use ~/.netrc for auth]'
        '(-u --user)'{-u,--user}'[Basic auth credentials]:credentials:'
        '*'{-H,--header}'[Custom header]:header:(Authorization\: Content-Type\: Accept\: X-API-Key\:)'
        '(-r --recursive)'{-r,--recursive}'[Enable recursive download]'
        '(-l --level)'{-l,--level}'[Maximum recursion depth]:depth:(1 2 3 5 10)'
        '(-m --mirror)'{-m,--mirror}'[Mirror mode]'
        '--span-hosts[Follow links to other hosts]'
        '(-A --accept)'{-A,--accept}'[Accept patterns]:patterns:'
        '(-R --reject)'{-R,--reject}'[Reject patterns]:patterns:'
        '--accept-ext[Accept extensions]:extensions:'
        '--reject-ext[Reject extensions]:extensions:'
        '(-p --page-requisites)'{-p,--page-requisites}'[Download page requisites]'
        '(-k --convert-links)'{-k,--convert-links}'[Convert links to local paths]'
        '--spider[Spider mode - list URLs only]'
        '(-w --wait)'{-w,--wait}'[Wait time between requests]:time:(1s 2s 5s)'
        '--random-wait[Add random delay to wait time]'
        '(-e --robots-off)'{-e,--robots-off}'[Ignore robots.txt]'
        '*:URL:_urls'
    )

    _arguments -s $opts
}

_burkut "$@"
