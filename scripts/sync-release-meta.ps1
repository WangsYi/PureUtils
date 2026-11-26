param(
    [Parameter(Mandatory)]
    [string]$Version,
    [string]$InstallerSizeMB = ''
)

$ErrorActionPreference = 'Stop'
$clean = $Version.Trim() -replace '^[vV]+', ''
if ($clean -notmatch '^(\d+)\.(\d+)\.(\d+)$') {
    throw "Invalid semantic version '$Version'; expected X.Y.Z (e.g. 1.2.0)"
}
$major = [int]$Matches[1]
$minor = [int]$Matches[2]
$patch = [int]$Matches[3]

$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$utf8NoBom = [System.Text.UTF8Encoding]::new($false)

function Update-TextFile([string]$RelPath, [scriptblock]$Transform) {
    $fullPath = Join-Path $root $RelPath
    if (-not (Test-Path -LiteralPath $fullPath)) {
        throw "Missing target file: $fullPath"
    }
    $orig = [System.IO.File]::ReadAllText($fullPath, $utf8NoBom)
    $next = & $Transform $orig
    if ($next -ne $orig) {
        [System.IO.File]::WriteAllText($fullPath, $next, $utf8NoBom)
        Write-Host "Updated $RelPath -> v$clean"
    } else {
        Write-Host "Unchanged $RelPath (already v$clean)"
    }
}

Update-TextFile 'pkg\purekit\core\version.go' {
    param($s)
    [regex]::Replace($s, '(?m)^var AppVersion = "\d+\.\d+\.\d+"', "var AppVersion = `"$clean`"")
}

Update-TextFile 'installer\pureutils.iss' {
    param($s)
    [regex]::Replace($s, '(?m)^(\s*#define AppVersion\s+)"\d+\.\d+\.\d+"', "`${1}`"$clean`"")
}

Update-TextFile 'build.ps1' {
    param($s)
    $s = [regex]::Replace($s, '(?m)^(\s*\[string\]\$Version\s*=\s*)''\d+\.\d+\.\d+''', "`${1}'$clean'")
    $s = [regex]::Replace($s, '-Version \d+\.\d+\.\d+', "-Version $clean")
    return $s
}

Update-TextFile '.github\workflows\ci.yml' {
    param($s)
    [regex]::Replace($s, '-Version "\d+\.\d+\.\d+-ci"', "-Version `"$clean-ci`"")
}

Update-TextFile 'tools\genicon\main.go' {
    param($s)
    $s = [regex]::Replace($s, '(name="com\.pureutils\.app"\s+version=")\d+\.\d+\.\d+\.0(")', "`${1}$clean.0`${2}")
    $s = [regex]::Replace($s, 'le\.PutUint32\(fixed\[8:\],\s*uint32\(\d+\)<<16\|uint32\(\d+\)\)', "le.PutUint32(fixed[8:], uint32($major)<<16|uint32($minor))")
    $s = [regex]::Replace($s, 'le\.PutUint32\(fixed\[12:\],\s*uint32\(\d+\)<<16\|uint32\(0\)\)', "le.PutUint32(fixed[12:], uint32($patch)<<16|uint32(0))")
    $s = [regex]::Replace($s, '\{"FileVersion",\s*"\d+\.\d+\.\d+"\}', "{`"FileVersion`", `"$clean`"}")
    $s = [regex]::Replace($s, '\{"ProductVersion",\s*"\d+\.\d+\.\d+"\}', "{`"ProductVersion`", `"$clean`"}")
    return $s
}

foreach ($readme in @('README.md', 'README.en.md')) {
    Update-TextFile $readme {
        param($s)
        [regex]::Replace($s, 'PureUtils-\d+\.\d+\.\d+-setup\.exe', "PureUtils-$clean-setup.exe")
    }
}

Update-TextFile 'docs\index.html' {
    param($s)
    $s = [regex]::Replace($s, '<span class="brand-ver">v\d+\.\d+\.\d+</span>', "<span class=`"brand-ver`">v$clean</span>")
    $s = [regex]::Replace($s, '下载安装程序 · v\d+\.\d+\.\d+', "下载安装程序 · v$clean")
    $s = [regex]::Replace($s, 'Download Installer · v\d+\.\d+\.\d+', "Download Installer · v$clean")
    $s = [regex]::Replace($s, 'PureUtils-\d+\.\d+\.\d+-setup\.exe', "PureUtils-$clean-setup.exe")
    if (-not [string]::IsNullOrWhiteSpace($InstallerSizeMB)) {
        $sizeLabel = "$($InstallerSizeMB.Trim()) MB"
        $escapedVer = [regex]::Escape($clean)
        $s = [regex]::Replace($s, "(PureUtils-$escapedVer-setup\.exe\s+\()[^)]+(\))", "`${1}$sizeLabel`${2}")
    }
    return $s
}
