# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License.

[CmdletBinding(SupportsShouldProcess = $true)]
param(
    [ValidateSet('Inspect', 'Apply', 'Remove')]
    [string]$Action = 'Inspect',
    [string]$LanAddress,
    [string]$WslAddress,
    [Parameter(Mandatory = $false)]
    [string]$RemoteScope,
    [string]$Distribution
)

$ErrorActionPreference = 'Stop'
$ports = @(8080, 8443)
$rulePrefix = 'g8e Gateway LAN'

function Test-Administrator {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Invoke-WslText([string]$Command) {
    $wslArgs = @()
    if ($Distribution) { $wslArgs += @('-d', $Distribution) }
    $wslArgs += @('--', 'sh', '-lc', $Command)
    $text = & wsl.exe @wslArgs
    if ($LASTEXITCODE -ne 0) { throw "WSL command failed: $Command" }
    return ($text | Out-String).Trim()
}

if (-not $LanAddress) {
    $candidate = Get-NetIPConfiguration | Where-Object {
        $_.NetAdapter.Status -eq 'Up' -and $_.IPv4DefaultGateway -and
        $_.IPv4Address.IPAddress -notlike '169.254.*'
    } | Select-Object -First 1
    if (-not $candidate) { throw 'Could not discover a LAN IPv4 address; pass -LanAddress explicitly.' }
    $LanAddress = $candidate.IPv4Address.IPAddress
}

if (-not $WslAddress) {
    $WslAddress = (Invoke-WslText "hostname -I | awk '{print `$1}'")
}
if ($LanAddress -notmatch '^\d{1,3}(\.\d{1,3}){3}$' -or $WslAddress -notmatch '^\d{1,3}(\.\d{1,3}){3}$') {
    throw "Invalid address selection: LAN=$LanAddress WSL=$WslAddress"
}

Write-Host "LAN address: $LanAddress"
Write-Host "WSL address: $WslAddress"
if ($Action -ne 'Remove') {
    try {
        Write-Host 'Gateway listeners in WSL:'
        Invoke-WslText "ss -ltn | grep -E ':(8080|8443)[[:space:]]'" | Write-Host
        Write-Host 'Gateway health in WSL:'
        Invoke-WslText "curl -fsS --connect-timeout 3 http://127.0.0.1:8080/api/v1/health" | Write-Host
    } catch {
        if ($Action -eq 'Apply') { throw }
        Write-Warning $_
    }
}

Write-Host 'Current matching portproxy rules:'
foreach ($port in $ports) {
    & netsh interface portproxy show v4tov4 listenaddress=$LanAddress listenport=$port
}
Write-Host 'Current matching firewall rules:'
Get-NetFirewallRule -DisplayName "$rulePrefix *" -ErrorAction SilentlyContinue |
    Get-NetFirewallAddressFilter | Format-Table -AutoSize

if ($Action -eq 'Inspect') { return }
if (-not (Test-Administrator)) { throw "$Action requires an elevated PowerShell session." }

if ($Action -eq 'Apply') {
    if (-not $RemoteScope) { throw 'Apply requires -RemoteScope (for example 192.168.1.53 or 192.168.1.0/24).' }
    $escapedLanAddress = [regex]::Escape($LanAddress)
    foreach ($port in $ports) {
        $existing = (& netsh interface portproxy show v4tov4 listenaddress=$LanAddress listenport=$port | Out-String)
        # Some netsh versions ignore the show command's listenport filter and
        # return every v4tov4 row. Match the requested address and port rather
        # than treating any numeric row as a conflict.
        $hasExactForward = $existing -match "(?m)^\s*$escapedLanAddress\s+$port\s+"
        $listener = Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue |
            Where-Object { $_.LocalAddress -in @($LanAddress, '0.0.0.0', '::') }
        $ruleName = "$rulePrefix $port"
        $ownedRule = Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue
        if (($listener -or $hasExactForward) -and -not $ownedRule) {
            $processes = ($listener | Select-Object -ExpandProperty OwningProcess -Unique) -join ', '
            throw "${LanAddress}:$port already has a listener/forward (process ID(s) $processes) without this helper's ownership rule."
        }
        if ($PSCmdlet.ShouldProcess("${LanAddress}:$port", "Forward to ${WslAddress}:$port")) {
            & netsh interface portproxy delete v4tov4 listenaddress=$LanAddress listenport=$port | Out-Null
            & netsh interface portproxy add v4tov4 listenaddress=$LanAddress listenport=$port connectaddress=$WslAddress connectport=$port
            if ($LASTEXITCODE -ne 0) { throw "Failed to add portproxy for port $port" }
            Remove-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue
            New-NetFirewallRule -DisplayName $ruleName -Direction Inbound -Action Allow -Protocol TCP `
                -LocalAddress $LanAddress -LocalPort $port -RemoteAddress $RemoteScope | Out-Null
        }
    }
}

if ($Action -eq 'Remove') {
    foreach ($port in $ports) {
        $ruleName = "$rulePrefix $port"
        $ownedRule = Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue
        if ($ownedRule) {
            if ($PSCmdlet.ShouldProcess("${LanAddress}:$port", 'Remove WSL port forward')) {
                & netsh interface portproxy delete v4tov4 listenaddress=$LanAddress listenport=$port | Out-Null
            }
        } else {
            Write-Warning "Not removing ${LanAddress}:$port because this helper's ownership firewall rule is absent."
        }
        if ($PSCmdlet.ShouldProcess("$rulePrefix $port", 'Remove firewall rule')) {
            Remove-NetFirewallRule -DisplayName "$rulePrefix $port" -ErrorAction SilentlyContinue
        }
    }
}

Write-Host 'Resulting matching portproxy rules:'
foreach ($port in $ports) {
    & netsh interface portproxy show v4tov4 listenaddress=$LanAddress listenport=$port
}
