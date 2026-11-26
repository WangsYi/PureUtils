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

$targetDir = $InstallDir
if ([string]::IsNullOrWhiteSpace($targetDir)) {
    $baseTemp = if (-not [string]::IsNullOrWhiteSpace($env:RUNNER_TEMP)) { $env:RUNNER_TEMP } else { [System.IO.Path]::GetTempPath() }
    $targetDir = Join-Path $baseTemp 'pureutils-smoke'
}

if (Test-Path -LiteralPath $targetDir) {
    Remove-Item -Recurse -Force $targetDir
}

Write-Host "Running silent install -> $targetDir"
$instProc = Start-Process -FilePath $resolvedInstaller -ArgumentList @(
    '/VERYSILENT',
    '/SUPPRESSMSGBOXES',
    '/NORESTART',
    "/DIR=$targetDir"
) -WindowStyle Hidden -Wait -PassThru
if ($instProc.ExitCode -ne 0) {
    throw "Silent installer exited with code $($instProc.ExitCode)"
}

foreach ($rel in @(
    'PureUtils.exe',
    'modules.json',
    'libmpv-2.dll',
    'turbojpeg.dll',
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
foreach ($mod in @('capture', 'history', 'viewer', 'video', 'zip')) {
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
$uninstProc = Start-Process -FilePath $uninstaller -ArgumentList @(
    '/VERYSILENT',
    '/SUPPRESSMSGBOXES',
    '/NORESTART'
) -WindowStyle Hidden -Wait -PassThru
if ($uninstProc.ExitCode -ne 0) {
    throw "Silent uninstaller exited with code $($uninstProc.ExitCode)"
}

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

Write-Host "Installer smoke test passed cleanly for PureUtils $ver"
