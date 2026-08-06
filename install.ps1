# SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

[CmdletBinding()]
param(
    [string]$Version = $(if ($env:NVFLEETINT_VERSION) { $env:NVFLEETINT_VERSION } else { "latest" }),
    [string]$InstallDir = $(if ($env:NVFLEETINT_INSTALL_DIR) {
        $env:NVFLEETINT_INSTALL_DIR
    } else {
        Join-Path $env:LOCALAPPDATA "Programs\nvfleetint\bin"
    }),
    [switch]$NoModifyPath,

    # Download resilience. Every request carries an explicit timeout and a
    # bounded number of attempts, so a dropped or throttled network fails the
    # install instead of hanging a provisioning pipeline indefinitely.
    [ValidateRange(1, 3600)]
    [int]$TimeoutSeconds = $(if ($env:NVFLEETINT_MAX_TIME) { $env:NVFLEETINT_MAX_TIME } else { 120 }),
    [ValidateRange(1, 100)]
    [int]$RetryAttempts = $(if ($env:NVFLEETINT_RETRY_ATTEMPTS) { $env:NVFLEETINT_RETRY_ATTEMPTS } else { 4 }),
    [ValidateRange(1, 3600)]
    [int]$RetryDelaySeconds = $(if ($env:NVFLEETINT_RETRY_DELAY) { $env:NVFLEETINT_RETRY_DELAY } else { 2 }),
    [ValidateRange(1, 3600)]
    [int]$RetryMaxDelaySeconds = $(if ($env:NVFLEETINT_RETRY_MAX_DELAY) { $env:NVFLEETINT_RETRY_MAX_DELAY } else { 30 }),

    # Fallback sources. BaseUrl replaces the default download root, assets are
    # read from <root>/<tag>/<asset>; FallbackBaseUrl is tried only after the
    # primary is exhausted; CacheDir is consulted before the network and
    # populated after a successful checksum verification.
    [string]$BaseUrl = $(if ($env:NVFLEETINT_BASE_URL) { $env:NVFLEETINT_BASE_URL } else { "" }),
    [string]$FallbackBaseUrl = $(if ($env:NVFLEETINT_FALLBACK_BASE_URL) { $env:NVFLEETINT_FALLBACK_BASE_URL } else { "" }),
    [string]$CacheDir = $(if ($env:NVFLEETINT_CACHE_DIR) { $env:NVFLEETINT_CACHE_DIR } else { "" })
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
$repository = "NVIDIA/fleet-intelligence-client"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

if (-not $BaseUrl) {
    $BaseUrl = "https://github.com/$repository/releases/download"
}

# Keeps a caller-supplied mirror from downgrading the transport to plaintext.
# Plain http is accepted only for loopback, matching the rule the SDK applies to
# its own base URL (nvfleetint/baseurl.go) so local mock servers keep working.
function Assert-SecureUrl {
    param([string]$Name, [string]$Value)

    $uri = $null
    if (-not [Uri]::TryCreate($Value, [UriKind]::Absolute, [ref]$uri)) {
        throw "$Name must be an absolute URL, got: $Value"
    }
    if ($uri.Scheme -eq "https") { return }
    if ($uri.Scheme -eq "http" -and $uri.IsLoopback) { return }
    throw "$Name must be an https:// URL (plain http is allowed only for localhost), got: $Value"
}

# Extracts the HTTP status from a failed request, or 0 when the request never
# got a response at all (DNS failure, refused connection, timeout).
function Get-HttpStatusCode {
    param($ErrorRecord)

    $response = $ErrorRecord.Exception.Response
    if (-not $response) { return 0 }
    try {
        return [int]$response.StatusCode
    } catch {
        return 0
    }
}

# Reports whether a failure is worth another attempt. A transport-level failure
# has no status and is always transient enough to retry; a 404 means the release
# or asset does not exist, so retrying only delays a certain failure.
function Test-RetryableFailure {
    param($ErrorRecord)

    $code = Get-HttpStatusCode $ErrorRecord
    if ($code -eq 0) { return $true }
    return @(408, 425, 429, 500, 502, 503, 504) -contains $code
}

# Runs a request with bounded retries and exponential backoff, throwing a clear
# message once the attempts are exhausted or the failure is deterministic.
function Invoke-WithRetry {
    param([string]$Description, [scriptblock]$Action)

    $attempt = 1
    $delay = $RetryDelaySeconds
    while ($true) {
        try {
            return & $Action
        } catch {
            $record = $_
            $code = Get-HttpStatusCode $record
            $reason = if ($code -ne 0) { "HTTP $code" } else { $record.Exception.Message }

            if (-not (Test-RetryableFailure $record)) {
                throw "$Description failed ($reason); not retryable."
            }
            if ($attempt -ge $RetryAttempts) {
                throw "$Description failed after $RetryAttempts attempts ($reason)."
            }

            Write-Warning "$Description failed ($reason); retrying in ${delay}s (attempt $($attempt + 1)/$RetryAttempts)."
            Start-Sleep -Seconds $delay
            $attempt++
            $delay = [Math]::Min($delay * 2, $RetryMaxDelaySeconds)
        }
    }
}

Assert-SecureUrl -Name "BaseUrl" -Value $BaseUrl
$BaseUrl = $BaseUrl.TrimEnd("/")
if ($FallbackBaseUrl) {
    Assert-SecureUrl -Name "FallbackBaseUrl" -Value $FallbackBaseUrl
    $FallbackBaseUrl = $FallbackBaseUrl.TrimEnd("/")
}

if ($Version -eq "latest") {
    $release = Invoke-WithRetry -Description "latest release lookup" -Action {
        Invoke-RestMethod -Uri "https://api.github.com/repos/$repository/releases/latest" `
            -TimeoutSec $TimeoutSeconds
    }
    $Version = $release.tag_name
}

$tag = if ($Version.StartsWith("v")) { $Version } else { "v$Version" }
if ($tag -notmatch '^v[0-9][0-9A-Za-z.+-]*$') {
    throw "Invalid release version: $tag"
}
$releaseVersion = $tag.TrimStart("v")
$machineArchitecture = if ($env:PROCESSOR_ARCHITEW6432) {
    $env:PROCESSOR_ARCHITEW6432
} else {
    $env:PROCESSOR_ARCHITECTURE
}

if (-not $machineArchitecture) {
    throw "Could not determine the Windows architecture"
}

$architecture = switch ($machineArchitecture.ToUpperInvariant()) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default { throw "Unsupported Windows architecture: $machineArchitecture" }
}

$asset = "nvfleetint_${releaseVersion}_windows_${architecture}.zip"
$workDir = Join-Path ([System.IO.Path]::GetTempPath()) ("nvfleetint-install-" + [guid]::NewGuid())
$archive = Join-Path $workDir $asset
$checksumPath = Join-Path $workDir "checksums.txt"
$extractDir = Join-Path $workDir "extract"

# Resolves one release file into the work directory: the cache first, then each
# configured download root in turn. Every source is fully retried before the
# next is tried.
function Get-ReleaseFile {
    param([string]$Name, [string]$Destination)

    if ($CacheDir) {
        $cached = Join-Path (Join-Path $CacheDir $tag) $Name
        if (Test-Path -LiteralPath $cached) {
            Write-Host "Using cached $Name from $(Join-Path $CacheDir $tag)"
            Copy-Item -LiteralPath $cached -Destination $Destination -Force
            return
        }
    }

    $roots = @($BaseUrl)
    if ($FallbackBaseUrl) { $roots += $FallbackBaseUrl }

    foreach ($root in $roots) {
        try {
            Invoke-WithRetry -Description "download of $Name from $root" -Action {
                Invoke-WebRequest -Uri "$root/$tag/$Name" -OutFile $Destination `
                    -UseBasicParsing -TimeoutSec $TimeoutSeconds
            }
            return
        } catch {
            Write-Warning "Giving up on $root for ${Name}: $($_.Exception.Message)"
        }
    }

    throw "Could not obtain $Name from any configured source."
}

# Stores a verified file in the cache. Only called after checksum verification,
# so a later run never reuses an artifact this run could not vouch for.
function Save-CachedFile {
    param([string]$Name, [string]$Path)

    if (-not $CacheDir) { return }
    $target = Join-Path $CacheDir $tag
    New-Item -ItemType Directory -Path $target -Force | Out-Null
    Copy-Item -LiteralPath $Path -Destination (Join-Path $target $Name) -Force
}

try {
    New-Item -ItemType Directory -Path $workDir | Out-Null
    Write-Host "Downloading nvfleetint $tag for windows/$architecture"
    Get-ReleaseFile -Name $asset -Destination $archive
    Get-ReleaseFile -Name "checksums.txt" -Destination $checksumPath

    $escapedAsset = [regex]::Escape($asset)
    $checksumLine = Get-Content $checksumPath | Where-Object {
        $_ -match "^([0-9a-fA-F]{64})\s+\*?$escapedAsset$"
    } | Select-Object -First 1
    if (-not $checksumLine) {
        throw "No valid SHA-256 checksum found for $asset"
    }

    $expectedChecksum = ($checksumLine -split '\s+')[0].ToUpperInvariant()
    $actualChecksum = (Get-FileHash -Algorithm SHA256 -Path $archive).Hash.ToUpperInvariant()
    if ($actualChecksum -ne $expectedChecksum) {
        throw "Checksum verification failed for $asset"
    }

    Save-CachedFile -Name $asset -Path $archive
    Save-CachedFile -Name "checksums.txt" -Path $checksumPath

    New-Item -ItemType Directory -Path $extractDir | Out-Null
    Expand-Archive -Path $archive -DestinationPath $extractDir
    $binary = Get-ChildItem -Path $extractDir -Filter "nvfleetint.exe" -File -Recurse |
        Select-Object -First 1
    if (-not $binary) {
        throw "nvfleetint.exe was not found in $asset"
    }

    $signature = Get-AuthenticodeSignature -FilePath $binary.FullName
    if ($signature.Status -ne "Valid") {
        throw "nvfleetint.exe has an invalid Authenticode signature: $($signature.Status)"
    }
    Write-Host "Authenticode signature valid: $($signature.SignerCertificate.Subject)"

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $destination = Join-Path $InstallDir "nvfleetint.exe"
    Copy-Item -Path $binary.FullName -Destination $destination -Force

    if (-not $NoModifyPath) {
        $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
        $entries = @($userPath -split ';' | Where-Object { $_ })
        if ($entries -notcontains $InstallDir) {
            $newPath = (@($entries) + $InstallDir) -join ';'
            [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
            Write-Host "Added $InstallDir to your user PATH. Open a new terminal to use it."
        }
    }

    Write-Host "Installed nvfleetint to $destination"
    & $destination version
} catch {
    # Fail deterministically: an automated caller sees a non-zero exit code and
    # one clear reason, rather than a partially installed tree and exit 0.
    $Host.UI.WriteErrorLine("Error: $($_.Exception.Message)")
    exit 1
} finally {
    if (Test-Path -LiteralPath $workDir) {
        Remove-Item -LiteralPath $workDir -Recurse -Force
    }
}
