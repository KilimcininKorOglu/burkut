# Burkut PowerShell completion script
# Add to $PROFILE: . /path/to/burkut.ps1

Register-ArgumentCompleter -Native -CommandName burkut -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)

    $options = @(
        @{ Name = '-o'; Tooltip = 'Output filename' }
        @{ Name = '--output'; Tooltip = 'Output filename' }
        @{ Name = '-P'; Tooltip = 'Output directory' }
        @{ Name = '--output-dir'; Tooltip = 'Output directory' }
        @{ Name = '-c'; Tooltip = 'Resume download' }
        @{ Name = '--continue'; Tooltip = 'Resume download' }
        @{ Name = '-n'; Tooltip = 'Parallel connections' }
        @{ Name = '--connections'; Tooltip = 'Parallel connections' }
        @{ Name = '-T'; Tooltip = 'Timeout' }
        @{ Name = '--timeout'; Tooltip = 'Timeout' }
        @{ Name = '-q'; Tooltip = 'Quiet mode' }
        @{ Name = '--quiet'; Tooltip = 'Quiet mode' }
        @{ Name = '-v'; Tooltip = 'Verbose output' }
        @{ Name = '--verbose'; Tooltip = 'Verbose output' }
        @{ Name = '--progress'; Tooltip = 'Progress style (bar, minimal, json, none)' }
        @{ Name = '--no-color'; Tooltip = 'Disable colors' }
        @{ Name = '-h'; Tooltip = 'Show help' }
        @{ Name = '--help'; Tooltip = 'Show help' }
        @{ Name = '-V'; Tooltip = 'Show version' }
        @{ Name = '--version'; Tooltip = 'Show version' }
        @{ Name = '--limit-rate'; Tooltip = 'Speed limit (e.g., 10M)' }
        @{ Name = '--checksum'; Tooltip = 'Verify checksum' }
        @{ Name = '--proxy'; Tooltip = 'Proxy URL' }
        @{ Name = '--no-check-certificate'; Tooltip = 'Skip TLS verification' }
        @{ Name = '--config'; Tooltip = 'Config file' }
        @{ Name = '--profile'; Tooltip = 'Named profile' }
        @{ Name = '--init-config'; Tooltip = 'Generate config' }
        @{ Name = '-i'; Tooltip = 'URL list file' }
        @{ Name = '--input-file'; Tooltip = 'URL list file' }
        @{ Name = '--on-complete'; Tooltip = 'Success command' }
        @{ Name = '--on-error'; Tooltip = 'Error command' }
        @{ Name = '--webhook'; Tooltip = 'Webhook URL' }
        @{ Name = '--mirrors'; Tooltip = 'Mirror URLs' }
        @{ Name = '--http3'; Tooltip = 'Use HTTP/3' }
        @{ Name = '--netrc'; Tooltip = 'Use netrc' }
        @{ Name = '-u'; Tooltip = 'Basic auth' }
        @{ Name = '--user'; Tooltip = 'Basic auth' }
        @{ Name = '-H'; Tooltip = 'Custom header' }
        @{ Name = '--header'; Tooltip = 'Custom header' }
    )

    # Get previous token
    $tokens = $commandAst.CommandElements
    $prevToken = if ($tokens.Count -gt 1) { $tokens[-2].Extent.Text } else { '' }

    # Context-aware completions
    switch -Regex ($prevToken) {
        '^(-n|--connections)$' {
            @('1', '2', '4', '8', '16', '32') | Where-Object { $_ -like "$wordToComplete*" } |
                ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) }
            return
        }
        '^(-T|--timeout)$' {
            @('10s', '30s', '60s', '120s', '5m', '10m') | Where-Object { $_ -like "$wordToComplete*" } |
                ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) }
            return
        }
        '^--progress$' {
            @('bar', 'minimal', 'json', 'none') | Where-Object { $_ -like "$wordToComplete*" } |
                ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) }
            return
        }
        '^--limit-rate$' {
            @('100K', '500K', '1M', '5M', '10M', '50M', '100M') | Where-Object { $_ -like "$wordToComplete*" } |
                ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) }
            return
        }
        '^--checksum$' {
            @('md5:', 'sha1:', 'sha256:', 'sha512:', 'blake3:') | Where-Object { $_ -like "$wordToComplete*" } |
                ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) }
            return
        }
        '^--profile$' {
            @('fast', 'slow', 'tor') | Where-Object { $_ -like "$wordToComplete*" } |
                ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) }
            return
        }
        '^(-H|--header)$' {
            @('Authorization:', 'Content-Type:', 'Accept:', 'X-API-Key:', 'User-Agent:') |
                Where-Object { $_ -like "$wordToComplete*" } |
                ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) }
            return
        }
    }

    # Complete options
    if ($wordToComplete -match '^-') {
        $options | Where-Object { $_.Name -like "$wordToComplete*" } |
            ForEach-Object {
                [System.Management.Automation.CompletionResult]::new(
                    $_.Name,
                    $_.Name,
                    'ParameterName',
                    $_.Tooltip
                )
            }
    }
}
