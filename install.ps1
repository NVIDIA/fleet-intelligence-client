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

if ($Version -eq "latest") {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repository/releases/latest"
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
$baseUrl = "https://github.com/$repository/releases/download/$tag"
$workDir = Join-Path ([System.IO.Path]::GetTempPath()) ("nvfleetint-install-" + [guid]::NewGuid())
$archive = Join-Path $workDir $asset
$checksumPath = Join-Path $workDir "checksums.txt"
$extractDir = Join-Path $workDir "extract"

try {
    New-Item -ItemType Directory -Path $workDir | Out-Null
    Write-Host "Downloading nvfleetint $tag for windows/$architecture"
    Invoke-WebRequest -Uri "$baseUrl/$asset" -OutFile $archive -UseBasicParsing
    Invoke-WebRequest -Uri "$baseUrl/checksums.txt" -OutFile $checksumPath -UseBasicParsing

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
            Write-Host "Added $InstallDir to your user PATH. Open a new terminal to use it."
        }
    }

    Write-Host "Installed nvfleetint to $destination"
    & $destination version
} finally {
    if (Test-Path -LiteralPath $workDir) {
        Remove-Item -LiteralPath $workDir -Recurse -Force
    }
}
