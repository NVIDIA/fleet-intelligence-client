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
    [switch]$NoModifyPath
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
$repository = "NVIDIA/fleet-intelligence-client"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

# Every network call is bounded. Without these a stalled connection leaves the
# installer -- and any CI job running it -- hanging indefinitely. Windows
# PowerShell 5.1 has no -MaximumRetryCount, so retries are done here.
$MetadataTimeoutSeconds = 30
$DownloadTimeoutSeconds = 300
$RetryAttempts = 3

function Invoke-WithRetry {
    param(
        [Parameter(Mandatory = $true)][scriptblock]$Action,
        [Parameter(Mandatory = $true)][string]$Description
    )

    # $RetryAttempts is the number of *retries*, matching curl's --retry in
    # install.sh, so the initial attempt is on top of it.
    $MaxAttempts = $RetryAttempts + 1

    for ($attempt = 1; $attempt -le $MaxAttempts; $attempt++) {
        try {
            return & $Action
        } catch {
            # Retry only what may succeed later: transport failures and
            # timeouts (no status code), 5xx, 408, and 429. A 404 for a
            # nonexistent release must fail immediately.
            $status = $null
            try { $status = [int]$_.Exception.Response.StatusCode } catch { $status = $null }
            $retryable = (-not $status) -or ($status -ge 500) -or ($status -eq 408) -or ($status -eq 429)

            if (-not $retryable -or $attempt -eq $MaxAttempts) {
                throw "$Description failed: $($_.Exception.Message)"
            }

            # Exponential backoff: 1s, 2s, 4s ...
            $delay = [int][math]::Pow(2, $attempt - 1)
            Write-Host "$Description failed (attempt $attempt of $MaxAttempts): $($_.Exception.Message). Retrying in $delay second(s)."
            Start-Sleep -Seconds $delay
        }
    }
}

if ($Version -eq "latest") {
    # Resolve the version from the releases/latest permalink, which redirects to
    # the newest release's own page, rather than from api.github.com. The REST
    # API allows 60 unauthenticated requests per hour per IP -- a budget shared
    # with every other tool behind the same address, which a CI runner or a
    # corporate NAT can exhaust -- while the website imposes no such quota.
    # install.sh resolves the version the same way.
    $response = Invoke-WithRetry -Description "Resolving the latest release" -Action {
        Invoke-WebRequest -Uri "https://github.com/$repository/releases/latest" `
            -Method Head -TimeoutSec $MetadataTimeoutSeconds -UseBasicParsing
    }

    # Windows PowerShell 5.1 reports the address the redirects landed on as
    # ResponseUri; PowerShell 7 as RequestMessage.RequestUri.
    $resolvedUri = $null
    if ($response.BaseResponse.ResponseUri) {
        $resolvedUri = $response.BaseResponse.ResponseUri.AbsoluteUri
    } elseif ($response.BaseResponse.RequestMessage) {
        $resolvedUri = $response.BaseResponse.RequestMessage.RequestUri.AbsoluteUri
    }

    # A repository with no published release redirects to its release index
    # instead of a tag, which is where the REST API would have returned a 404.
    if (-not ($resolvedUri -match '/releases/tag/(.+)$')) {
        throw "Could not determine the latest release version"
    }
    $Version = [uri]::UnescapeDataString($Matches[1])
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
$baseUrl = "https://github.com/$repository/releases/download/$tag"
$workDir = Join-Path ([System.IO.Path]::GetTempPath()) ("nvfleetint-install-" + [guid]::NewGuid())
$archive = Join-Path $workDir $asset
$checksumPath = Join-Path $workDir "checksums.txt"
$extractDir = Join-Path $workDir "extract"

try {
    New-Item -ItemType Directory -Path $workDir | Out-Null
    Write-Host "Downloading nvfleetint $tag for windows/$architecture"
    Invoke-WithRetry -Description "Downloading $asset" -Action {
        Invoke-WebRequest -Uri "$baseUrl/$asset" -OutFile $archive `
            -TimeoutSec $DownloadTimeoutSeconds -UseBasicParsing
    }
    Invoke-WithRetry -Description "Downloading checksums.txt" -Action {
        Invoke-WebRequest -Uri "$baseUrl/checksums.txt" -OutFile $checksumPath `
            -TimeoutSec $DownloadTimeoutSeconds -UseBasicParsing
    }

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
            Write-Host "Added $InstallDir to your user PATH."
        }

        # Persisting the user PATH only affects new processes. Update this
        # PowerShell process too so the command is available immediately when
        # the installer is invoked through `irm ... | iex`.
        $processEntries = @($env:Path -split ';' | Where-Object { $_ })
        if ($processEntries -notcontains $InstallDir) {
            $env:Path = (@($processEntries) + $InstallDir) -join ';'
            Write-Host "Added $InstallDir to the current PowerShell session PATH."
        }
    }

    Write-Host "Installed nvfleetint to $destination"
    & $destination version
} finally {
    if (Test-Path -LiteralPath $workDir) {
        Remove-Item -LiteralPath $workDir -Recurse -Force
    }
}
