param(
    [Parameter(Mandatory)][string]$InstallerPath,
    [Parameter(Mandatory)][string]$Version,
    [string]$InstallDir = '',
    [switch]$AllowOverwrite
)

$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

$resolvedInstaller = (Resolve-Path -LiteralPath $InstallerPath).Path
if (-not (Test-Path -LiteralPath $resolvedInstaller)) {
    throw "Installer not found: $InstallerPath"
}

$ver = $Version.Trim() -replace '^[vV]+', ''
$uninstRegPath = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\{6D9E4A1B-3C82-4F55-9A14-2B7E8C3D5F10}_is1'

if (-not $AllowOverwrite -and (Test-Path -LiteralPath $uninstRegPath)) {
    throw "Existing PureUtils installation found at $uninstRegPath. Pass -AllowOverwrite only in disposable CI/VM environments."
}

function Invoke-WithLogTimeout {
    param(
        [Parameter(Mandatory)][string]$FilePath,
        [Parameter(Mandatory)][string[]]$ArgumentList,
        [Parameter(Mandatory)][string]$LogPath,
        [Parameter(Mandatory)][string]$Label,
        [int]$TimeoutMs = 120000
    )

    if (Test-Path -LiteralPath $LogPath) {
        Remove-Item -Force $LogPath -ErrorAction SilentlyContinue
    }
    $proc = Start-Process -FilePath $FilePath -ArgumentList $ArgumentList -WindowStyle Hidden -PassThru
    $done = $proc.WaitForExit($TimeoutMs)
    if (-not $done) {
        try { Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue } catch {}
        if (Test-Path -LiteralPath $LogPath) {
            Write-Host "--- $Label Log Tail (timeout) ---"
            Get-Content -LiteralPath $LogPath -Tail 80
        }
        throw "$Label timed out after $TimeoutMs ms"
    }
    $proc.Refresh()
    if ($proc.ExitCode -ne 0) {
        if (Test-Path -LiteralPath $LogPath) {
            Write-Host "--- $Label Log Tail (exit $($proc.ExitCode)) ---"
            Get-Content -LiteralPath $LogPath -Tail 80
        }
        throw "$Label exited with code $($proc.ExitCode)"
    }
}

$baseTemp = if (-not [string]::IsNullOrWhiteSpace($env:RUNNER_TEMP)) { $env:RUNNER_TEMP } else { [System.IO.Path]::GetTempPath() }
$targetDir = $InstallDir
if ([string]::IsNullOrWhiteSpace($targetDir)) {
    $targetDir = Join-Path $baseTemp 'pureutils-smoke'
}
$installLog = Join-Path $baseTemp 'pureutils-smoke-install.log'
$uninstallLog = Join-Path $baseTemp 'pureutils-smoke-uninstall.log'

if (Test-Path -LiteralPath $targetDir) {
    Remove-Item -Recurse -Force $targetDir
}

Write-Host "Running silent install -> $targetDir"
Invoke-WithLogTimeout -FilePath $resolvedInstaller -ArgumentList @(
    '/VERYSILENT',
    '/SUPPRESSMSGBOXES',
    '/NORESTART',
    '/NOLAUNCH=1',
    "/LOG=$installLog",
    "/DIR=$targetDir"
) -LogPath $installLog -Label 'Silent installer' -TimeoutMs 120000

foreach ($rel in @(
    'PureUtils.exe',
    'modules.json',
    'libmpv-2.dll',
    'pureimage_codecs.dll',
    'resources\icon.ico',
    'unins000.exe'
)) {
    $full = Join-Path $targetDir $rel
    if (-not (Test-Path -LiteralPath $full)) {
        throw "Expected installed file missing: $full"
    }
}

$manifestRaw = Get-Content -LiteralPath (Join-Path $targetDir 'modules.json') -Raw -Encoding UTF8
$manifest = $manifestRaw | ConvertFrom-Json
foreach ($mod in @('capture', 'history', 'viewer', 'video', 'zip', 'monitor')) {
    if ($manifest.$mod -ne $true) {
        throw "Expected modules.json.$mod = true, got $($manifest.$mod)"
    }
}

if (-not (Test-Path -LiteralPath $uninstRegPath)) {
    throw "Missing uninstall registry key: $uninstRegPath"
}
$regProps = Get-ItemProperty -LiteralPath $uninstRegPath
if ($regProps.DisplayVersion -ne $ver) {
    throw "DisplayVersion mismatch: expected '$ver', got '$($regProps.DisplayVersion)'"
}
if ($regProps.Publisher -ne 'PureUtils') {
    throw "Publisher mismatch: expected 'PureUtils', got '$($regProps.Publisher)'"
}

$uninstaller = Join-Path $targetDir 'unins000.exe'
Write-Host "Running silent uninstall -> $uninstaller"
Invoke-WithLogTimeout -FilePath $uninstaller -ArgumentList @(
    '/VERYSILENT',
    '/SUPPRESSMSGBOXES',
    '/NORESTART',
    "/LOG=$uninstallLog"
) -LogPath $uninstallLog -Label 'Silent uninstaller' -TimeoutMs 120000

# unins000.exe spawns a temp copy so its process can exit slightly before directory cleanup completes
$deadline = (Get-Date).AddSeconds(30)
while ((Get-Date) -lt $deadline) {
    $dirGone = -not (Test-Path -LiteralPath $targetDir)
    $regGone = -not (Test-Path -LiteralPath $uninstRegPath)
    if ($dirGone -and $regGone) {
        break
    }
    Start-Sleep -Milliseconds 500
}

if (Test-Path -LiteralPath $targetDir) {
    throw "Leftover install directory remains after uninstall: $targetDir"
}
if (Test-Path -LiteralPath $uninstRegPath) {
    throw "Leftover uninstall registry key remains after uninstall: $uninstRegPath"
}

Remove-Item -Force $installLog, $uninstallLog -ErrorAction SilentlyContinue
Write-Host "Installer smoke test passed cleanly for PureUtils $ver"
