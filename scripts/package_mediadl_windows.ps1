[CmdletBinding()]
param(
    [string]$OutputDir = "build/bin/tools",
    [switch]$Force
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Fail([string]$Message) {
    throw "package-mediadl-windows: $Message"
}

function Get-ExpectedChecksum([string]$SumsUrl, [string]$AssetName) {
    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri $SumsUrl
    }
    catch {
        Fail "could not download checksum manifest $SumsUrl : $($_.Exception.Message)"
    }
    foreach ($line in ($response.Content -split "`r?`n")) {
        if ($line -match '^\s*([0-9a-fA-F]{64})\s+\*?(.+?)\s*$') {
            $name = [System.IO.Path]::GetFileName(($Matches[2] -replace '\\', '/'))
            if ($name -eq $AssetName) {
                return $Matches[1].ToLowerInvariant()
            }
        }
    }
    Fail "checksum manifest $SumsUrl has no entry for $AssetName"
}

function Download-Verified([string]$Url, [string]$AssetName, [string]$Expected, [string]$Destination) {
    try {
        Invoke-WebRequest -UseBasicParsing -Uri $Url -OutFile $Destination
    }
    catch {
        Fail "could not download $AssetName : $($_.Exception.Message)"
    }
    $actual = (Get-FileHash -LiteralPath $Destination -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $Expected) {
        Remove-Item -LiteralPath $Destination -Force -ErrorAction SilentlyContinue
        Fail "$AssetName failed SHA-256 verification (expected $Expected, got $actual)"
    }
}

function Get-VersionLine([string]$Executable, [string[]]$Arguments) {
    $lines = @(& $Executable @Arguments 2>&1 | ForEach-Object { $_.ToString().Trim() } | Where-Object { $_ })
    if ($LASTEXITCODE -ne 0 -or $lines.Count -eq 0) {
        Fail "could not execute packaged helper $Executable"
    }
    return $lines[0]
}

$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$outputPath = if ([System.IO.Path]::IsPathRooted($OutputDir)) {
    [System.IO.Path]::GetFullPath($OutputDir)
} else {
    [System.IO.Path]::GetFullPath((Join-Path $repositoryRoot $OutputDir))
}

if (-not $Force -and (Test-Path -LiteralPath $outputPath -PathType Container) -and (Get-ChildItem -LiteralPath $outputPath -Force | Select-Object -First 1)) {
    Fail "output directory is not empty: $outputPath (pass -Force to replace it)"
}

$ytDlpBase = "https://github.com/yt-dlp/yt-dlp/releases/latest/download"
$ffmpegBase = "https://github.com/yt-dlp/FFmpeg-Builds/releases/latest/download"
$ytDlpAsset = "yt-dlp.exe"
$ytDlpSums = "SHA2-256SUMS"
$ffmpegAsset = "ffmpeg-master-latest-win64-gpl.zip"
$ffmpegSums = "checksums.sha256"
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("tdrive-mediadl-" + [guid]::NewGuid().ToString("N"))

try {
    New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
    New-Item -ItemType Directory -Force -Path $outputPath | Out-Null

    $ytDlpExpected = Get-ExpectedChecksum "$ytDlpBase/$ytDlpSums" $ytDlpAsset
    $ffmpegExpected = Get-ExpectedChecksum "$ffmpegBase/$ffmpegSums" $ffmpegAsset
    $ytDlpArchive = Join-Path $tempRoot $ytDlpAsset
    $ffmpegArchive = Join-Path $tempRoot $ffmpegAsset
    Download-Verified "$ytDlpBase/$ytDlpAsset" $ytDlpAsset $ytDlpExpected $ytDlpArchive
    Download-Verified "$ffmpegBase/$ffmpegAsset" $ffmpegAsset $ffmpegExpected $ffmpegArchive

    $ytDlpPath = Join-Path $outputPath $ytDlpAsset
    Copy-Item -LiteralPath $ytDlpArchive -Destination $ytDlpPath -Force
    $ffmpegExtracted = Join-Path $tempRoot "ffmpeg"
    Expand-Archive -LiteralPath $ffmpegArchive -DestinationPath $ffmpegExtracted -Force
    $ffmpegCandidates = @(Get-ChildItem -LiteralPath $ffmpegExtracted -Recurse -File -Filter "ffmpeg.exe" |
        Where-Object { $_.FullName -match "[\\/]bin[\\/]ffmpeg\.exe$" })
    if ($ffmpegCandidates.Count -ne 1) {
        Fail "FFmpeg archive must contain exactly one bin/ffmpeg.exe (found $($ffmpegCandidates.Count))"
    }
    $ffmpegPath = Join-Path $outputPath "ffmpeg.exe"
    Copy-Item -LiteralPath $ffmpegCandidates[0].FullName -Destination $ffmpegPath -Force

    $ytDlpVersion = Get-VersionLine $ytDlpPath @("--version")
    $ffmpegVersion = Get-VersionLine $ffmpegPath @("-version")
    @(
        "TDrive Windows media downloader runtime",
        "",
        "yt-dlp: $ytDlpVersion",
        "yt-dlp asset: $ytDlpBase/$ytDlpAsset",
        "yt-dlp SHA-256: $ytDlpExpected",
        "",
        "FFmpeg: $ffmpegVersion",
        "FFmpeg asset: $ffmpegBase/$ffmpegAsset",
        "FFmpeg archive SHA-256: $ffmpegExpected",
        "",
        "Each downloaded archive was verified against its upstream SHA-256 manifest before packaging."
    ) | Set-Content -LiteralPath (Join-Path $outputPath "SOURCE.txt") -Encoding utf8
    @(
        "TDrive bundles the following third-party executables for Windows:",
        "",
        "yt-dlp - Unlicense. The executable is downloaded from the official yt-dlp GitHub release.",
        "Source and license: https://github.com/yt-dlp/yt-dlp",
        "",
        "FFmpeg - GNU General Public License version 3 or later (GPL build). The executable is taken from the yt-dlp FFmpeg-Builds release archive.",
        "Source and license: https://github.com/yt-dlp/FFmpeg-Builds",
        "FFmpeg project: https://ffmpeg.org/",
        "",
        "This package does not alter either executable. Refer to the upstream projects for complete license texts and source-code offers."
    ) | Set-Content -LiteralPath (Join-Path $outputPath "THIRD_PARTY_NOTICES.txt") -Encoding utf8

    Write-Host "package-mediadl-windows: verified yt-dlp $ytDlpVersion and $ffmpegVersion"
    Write-Host "package-mediadl-windows: bundled runtime into $outputPath"
}
finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}
