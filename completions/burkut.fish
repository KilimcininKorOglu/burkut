# Burkut fish completion script
# Copy to ~/.config/fish/completions/burkut.fish

# Disable file completion by default
complete -c burkut -f

# Basic options
complete -c burkut -s o -l output -d "Output filename" -r
complete -c burkut -s P -l output-dir -d "Output directory" -r -a "(__fish_complete_directories)"
complete -c burkut -s c -l continue -d "Resume partially downloaded file"
complete -c burkut -s n -l connections -d "Number of parallel connections" -x -a "1 2 4 8 16 32"
complete -c burkut -s T -l timeout -d "Connection timeout" -x -a "10s 30s 60s 120s 5m 10m"
complete -c burkut -s q -l quiet -d "Quiet mode"
complete -c burkut -s v -l verbose -d "Verbose output"
complete -c burkut -l progress -d "Progress style" -x -a "bar minimal json none"
complete -c burkut -l no-color -d "Disable colored output"
complete -c burkut -s h -l help -d "Show help"
complete -c burkut -s V -l version -d "Show version"

# Advanced options
complete -c burkut -l limit-rate -d "Limit download speed" -x -a "100K 500K 1M 5M 10M 50M 100M"
complete -c burkut -l checksum -d "Verify checksum" -x -a "md5: sha1: sha256: sha512: blake3:"
complete -c burkut -l proxy -d "Proxy URL" -x -a "http:// https:// socks5://"
complete -c burkut -l no-check-certificate -d "Skip TLS verification"
complete -c burkut -l config -d "Config file" -r -F
complete -c burkut -l profile -d "Use named profile" -x -a "fast slow tor"
complete -c burkut -l init-config -d "Generate default config"

# Authentication
complete -c burkut -l netrc -d "Use ~/.netrc for authentication"
complete -c burkut -s u -l user -d "Basic auth (user:password)" -x
complete -c burkut -s H -l header -d "Custom header" -x -a "Authorization: Content-Type: Accept: X-API-Key: User-Agent:"

# Batch & automation
complete -c burkut -s i -l input-file -d "URL list file" -r -F
complete -c burkut -l on-complete -d "Command on success" -x -a "(__fish_complete_command)"
complete -c burkut -l on-error -d "Command on error" -x -a "(__fish_complete_command)"
complete -c burkut -l webhook -d "Webhook URL" -x
complete -c burkut -l mirrors -d "Mirror URLs" -x
complete -c burkut -l http3 -d "Use HTTP/3 (QUIC)"

# URL argument - allow any input
complete -c burkut -d "URL to download" -a "()"
