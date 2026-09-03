[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$Version,

    [string]$InstallDir = $env:COUPANGCTL_INSTALL_DIR
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$script:FailureCode = 'install_failed'
$script:FailureMessage = 'installation failed without changing the existing binary'

function Stop-Installer {
    param(
        [Parameter(Mandatory = $true)][string]$Code,
        [Parameter(Mandatory = $true)][string]$Message
    )
    $script:FailureCode = $Code
    $script:FailureMessage = $Message
    throw [System.InvalidOperationException]::new('controlled installer failure')
}

$workDir = $null
$stagingPath = $null
try {
    if ($Version -cnotmatch '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$') {
        Stop-Installer 'invalid_version' 'version must be an immutable semantic release tag such as v0.1.0'
    }

    $goarch = $env:COUPANGCTL_INSTALL_GOARCH
    if ([string]::IsNullOrWhiteSpace($goarch)) {
        switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()) {
            'X64' { $goarch = 'amd64' }
            'Arm64' { $goarch = 'arm64' }
            default { Stop-Installer 'unsupported_architecture' 'supported Windows architectures are amd64 and arm64' }
        }
    }
    if ($goarch -notin @('amd64', 'arm64')) {
        Stop-Installer 'unsupported_architecture' 'supported Windows architectures are amd64 and arm64'
    }

    $baseUrl = $env:COUPANGCTL_INSTALL_BASE_URL
    if ([string]::IsNullOrWhiteSpace($baseUrl)) {
        $baseUrl = 'https://github.com/JungHoonGhae/coupang-ctl/releases/download'
    }
    $baseUri = $null
    if (-not [System.Uri]::TryCreate($baseUrl, [System.UriKind]::Absolute, [ref]$baseUri)) {
        Stop-Installer 'invalid_release_source' 'release base URL must be an absolute HTTPS URL'
    }
    $allowedSource = $baseUri.Scheme -ceq 'https' -or ($baseUri.Scheme -ceq 'http' -and $baseUri.IsLoopback)
    if (-not $allowedSource) {
        Stop-Installer 'invalid_release_source' 'release base URL must use HTTPS; loopback HTTP is test-only'
    }

    if ([string]::IsNullOrWhiteSpace($InstallDir)) {
        if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
            Stop-Installer 'install_directory_unavailable' 'LOCALAPPDATA is required when InstallDir is omitted'
        }
        $InstallDir = [System.IO.Path]::Combine($env:LOCALAPPDATA, 'Programs', 'coupangctl')
    }

    $assetVersion = $Version.Substring(1)
    $assetName = "coupangctl_${assetVersion}_windows_${goarch}.zip"
    $workDir = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), "coupangctl-install-$([System.Guid]::NewGuid().ToString('N'))")
    [System.IO.Directory]::CreateDirectory($workDir) | Out-Null
    $archivePath = [System.IO.Path]::Combine($workDir, $assetName)
    $checksumsPath = [System.IO.Path]::Combine($workDir, 'checksums.txt')
    $releaseRoot = $baseUrl.TrimEnd('/') + '/' + $Version

    try {
        Invoke-WebRequest -Uri "$releaseRoot/checksums.txt" -OutFile $checksumsPath -MaximumRedirection 5
        Invoke-WebRequest -Uri "$releaseRoot/$assetName" -OutFile $archivePath -MaximumRedirection 5
    }
    catch {
        Stop-Installer 'download_failed' 'could not download the requested release and checksum manifest'
    }

    $checksumMatches = @()
    foreach ($line in [System.IO.File]::ReadLines($checksumsPath)) {
        if ($line -cmatch '^([0-9A-Fa-f]{64})\s+\*?(\S+)$' -and $Matches[2] -ceq $assetName) {
            $checksumMatches += $Matches[1].ToLowerInvariant()
        }
    }
    if ($checksumMatches.Count -ne 1) {
        Stop-Installer 'invalid_checksum_manifest' 'release checksum entry is missing or duplicated'
    }
    $actualDigest = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualDigest -cne $checksumMatches[0]) {
        Stop-Installer 'checksum_mismatch' 'archive checksum does not match the release manifest'
    }

    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $archive = [System.IO.Compression.ZipFile]::OpenRead($archivePath)
    try {
        $expectedEntries = @('coupangctl.exe', 'LICENSE', 'README.md', 'BROWSER_BRIDGE.md')
        $archiveEntries = @($archive.Entries | ForEach-Object { $_.FullName })
        if ($archiveEntries.Count -ne $expectedEntries.Count) {
            Stop-Installer 'unexpected_archive_content' 'release archive contains unexpected entries'
        }
        foreach ($entryName in $archiveEntries) {
            if ($entryName -cnotin $expectedEntries) {
                Stop-Installer 'unexpected_archive_content' 'release archive contains unexpected entries'
            }
        }
        foreach ($expectedName in $expectedEntries) {
            if (@($archiveEntries | Where-Object { $_ -ceq $expectedName }).Count -ne 1) {
                Stop-Installer 'unexpected_archive_content' 'release archive contains unexpected entries'
            }
        }
        $binaryEntry = $archive.Entries | Where-Object { $_.FullName -ceq 'coupangctl.exe' }
        $candidate = [System.IO.Path]::Combine($workDir, 'coupangctl.exe')
        [System.IO.Compression.ZipFileExtensions]::ExtractToFile($binaryEntry, $candidate, $false)
    }
    finally {
        $archive.Dispose()
    }

    try {
        $versionOutput = & $candidate version | Out-String
        if ($LASTEXITCODE -ne 0) {
            Stop-Installer 'invalid_executable' 'downloaded executable did not report its version'
        }
        $reportedVersion = $versionOutput | ConvertFrom-Json
    }
    catch {
        if ($script:FailureCode -eq 'invalid_executable') { throw }
        Stop-Installer 'invalid_executable' 'downloaded executable did not return valid version JSON'
    }
    if ($reportedVersion.name -cne 'coupangctl' -or $reportedVersion.version -cne $assetVersion) {
        Stop-Installer 'version_mismatch' 'downloaded executable identity or version does not match the requested release'
    }

    if (Test-Path -LiteralPath $InstallDir) {
        $installDirectoryItem = Get-Item -LiteralPath $InstallDir -Force
        if (-not $installDirectoryItem.PSIsContainer -or ($installDirectoryItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint)) {
            Stop-Installer 'unsafe_install_directory' 'install directory must be a real directory, not a link'
        }
    }
    else {
        [System.IO.Directory]::CreateDirectory($InstallDir) | Out-Null
    }

    $destination = [System.IO.Path]::Combine($InstallDir, 'coupangctl.exe')
    if (Test-Path -LiteralPath $destination) {
        $destinationItem = Get-Item -LiteralPath $destination -Force
        if ($destinationItem.PSIsContainer -or ($destinationItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint)) {
            Stop-Installer 'unsafe_destination' 'refusing to replace a linked or non-file destination'
        }
    }

    $stagingPath = [System.IO.Path]::Combine($InstallDir, ".coupangctl.new.$([System.Guid]::NewGuid().ToString('N'))")
    [System.IO.File]::Copy($candidate, $stagingPath, $false)
    [System.IO.File]::Move($stagingPath, $destination, $true)
    $stagingPath = $null

    [ordered]@{
        name = 'coupangctl'
        version = $assetVersion
        status = 'installed'
    } | ConvertTo-Json -Compress
}
catch {
    $failure = [ordered]@{
        error = [ordered]@{
            code = $script:FailureCode
            message = $script:FailureMessage
        }
    } | ConvertTo-Json -Compress
    [Console]::Error.WriteLine($failure)
    exit 1
}
finally {
    if (-not [string]::IsNullOrWhiteSpace($stagingPath) -and [System.IO.File]::Exists($stagingPath)) {
        [System.IO.File]::Delete($stagingPath)
    }
    if (-not [string]::IsNullOrWhiteSpace($workDir) -and [System.IO.Directory]::Exists($workDir)) {
        [System.IO.Directory]::Delete($workDir, $true)
    }
}
