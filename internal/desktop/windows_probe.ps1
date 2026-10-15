$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
$items = @()
$issues = @()
$processes = @()
try {
    $processes = @(Get-Process -Name 'ChatGPT','Codex','Claude','OpenCode','Antigravity' -ErrorAction SilentlyContinue | ForEach-Object {
        try { if ($_.Path) { $_.Path } } catch {}
    })
} catch {
    $issues += 'Unable to enumerate running application paths.'
}
try {
    Get-AppxPackage | Where-Object {
        $_.Name -eq 'OpenAI.Codex' -or $_.Name -eq 'Claude' -or $_.Name -like 'Anthropic.Claude*'
    } | ForEach-Object {
        $items += @{
            name = $_.Name; version = $_.Version.ToString()
            publisher = $_.Publisher; package = $_.Name
            family = $_.PackageFamilyName
            path = $_.InstallLocation; scope = 'user'
            arch = $_.Architecture.ToString(); source = 'msix'
        }
    }
} catch {
    $issues += 'Unable to query MSIX packages using Windows PowerShell.'
}
$roots = @(
    @{ path = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*'; scope = 'user' },
    @{ path = 'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*'; scope = 'system' },
    @{ path = 'HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*'; scope = 'system' }
)
foreach ($root in $roots) {
    Get-ItemProperty -Path $root.path -ErrorAction SilentlyContinue | Where-Object {
        $_.DisplayName -match '^(ChatGPT|Codex|Claude|OpenCode|Antigravity)( \d[\d.]*)?$'
    } | ForEach-Object {
        $path = $_.InstallLocation
        if (-not $path -and $_.DisplayIcon) {
            $path = ($_.DisplayIcon -replace ',[-]?\d+$', '').Trim('"')
        }
        if ($path -and (Test-Path -LiteralPath $path)) {
            $items += @{
                name = $_.DisplayName; version = $_.DisplayVersion
                publisher = $_.Publisher; package = $_.PSChildName
                path = $path; scope = $root.scope; arch = 'unknown'; source = 'installer'
            }
        }
    }
}
@{ items = @($items); processes = @($processes); issues = @($issues) } | ConvertTo-Json -Compress -Depth 6
