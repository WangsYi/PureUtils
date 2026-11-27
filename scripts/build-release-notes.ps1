param(
    [Parameter(Mandatory)][string]$Version,
    [Parameter(Mandatory)][string]$InstallerSizeMB,
    [Parameter(Mandatory)][long]$InstallerBytes,
    [Parameter(Mandatory)][string]$Sha256,
    [Parameter(Mandatory)][string]$OutputPath,
    [string]$Ref = 'HEAD',
    [string]$NotesZh = '',
    [string]$NotesEn = '',
    [string]$NotesFile = '',
    [string]$Repo = 'WangsYi/PureUtils'
)

$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8
$utf8NoBom = [System.Text.UTF8Encoding]::new($false)

$ver = $Version.Trim() -replace '^[vV]+', ''
if ($ver -notmatch '^\d+\.\d+\.\d+$') {
    throw "Invalid release version '$Version'; expected X.Y.Z"
}
$tag = "v$ver"
$targetSemver = [version]$ver
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)

# Find the highest semantic version tag strictly below $ver
$allTags = @(& git -C $root tag --list 'v[0-9]*.[0-9]*.[0-9]*' 2>$null)
$prevTag = $null
$bestPrevVer = $null
foreach ($t in $allTags) {
    if ($t -match '^v(\d+\.\d+\.\d+)$') {
        $cand = [version]$Matches[1]
        if ($cand -lt $targetSemver) {
            if ($null -eq $bestPrevVer -or $cand -gt $bestPrevVer) {
                $bestPrevVer = $cand
                $prevTag = $t
            }
        }
    }
}

$rangeSpec = if ($prevTag) { "$prevTag..$Ref" } else { $Ref }

$logArgs = @('-C', $root, '-c', 'i18n.logOutputEncoding=utf-8', 'log')
if ($prevTag) {
    $logArgs += "$prevTag..$Ref"
} else {
    $logArgs += @('-n', '15', $Ref)
}
$logArgs += @('--pretty=format:%s||%h', '--no-merges')

$rawLog = @(& git @logArgs 2>$null)
$commits = New-Object System.Collections.Generic.List[psobject]
foreach ($entry in $rawLog) {
    if ([string]::IsNullOrWhiteSpace($entry)) { continue }
    $idx = $entry.LastIndexOf('||')
    if ($idx -lt 0) { continue }
    $subj = $entry.Substring(0, $idx).Trim()
    $hash = $entry.Substring($idx + 2).Trim()
    if ([string]::IsNullOrWhiteSpace($subj) -or $subj -match '\[skip ci\]') { continue }

    $cat = 'other'
    if ($subj -match '^(?<type>feat|fix|perf|refactor|style|docs|ci|build|chore|test)(\([^)]*\))?!?:') {
        switch ($Matches['type'].ToLowerInvariant()) {
            'feat' { $cat = 'feat' }
            'fix' { $cat = 'fix' }
            { $_ -in @('perf', 'refactor', 'style') } { $cat = 'perf' }
        }
    }
    $commits.Add([pscustomobject]@{
        Subject  = $subj
        Hash     = $hash
        Category = $cat
    })
}

$resolvedNotesFile = $NotesFile
if ([string]::IsNullOrWhiteSpace($resolvedNotesFile)) {
    $candidate = Join-Path $root "release-notes\$tag.md"
    if (Test-Path -LiteralPath $candidate) {
        $resolvedNotesFile = $candidate
    }
} elseif (-not [System.IO.Path]::IsPathRooted($resolvedNotesFile)) {
    $candidate = Join-Path $root $resolvedNotesFile
    if (Test-Path -LiteralPath $candidate) {
        $resolvedNotesFile = $candidate
    }
}

$zhBody = ''
$enBody = ''
if (-not [string]::IsNullOrWhiteSpace($resolvedNotesFile) -and (Test-Path -LiteralPath $resolvedNotesFile)) {
    $curatedRaw = [System.IO.File]::ReadAllText($resolvedNotesFile, $utf8NoBom)
    if ($curatedRaw -match '(?s)<!--\s*zh:start\s*-->(.*?)<!--\s*zh:end\s*-->') {
        $zhBody = $Matches[1].Trim()
    }
    if ($curatedRaw -match '(?s)<!--\s*en:start\s*-->(.*?)<!--\s*en:end\s*-->') {
        $enBody = $Matches[1].Trim()
    }
}

if (-not [string]::IsNullOrWhiteSpace($NotesZh)) {
    $zhBody = $NotesZh.Trim()
}
if (-not [string]::IsNullOrWhiteSpace($NotesEn)) {
    $enBody = $NotesEn.Trim()
}

function Format-ConciseCommits([string]$Lang) {
    $picked = @($commits | Where-Object { $_.Category -in @('feat', 'fix', 'perf') })
    if ($picked.Count -eq 0) {
        $picked = @($commits)
    }
    if ($picked.Count -eq 0) {
        if ($Lang -eq 'zh') {
            return "- ``$tag`` 维护更新。"
        }
        return "- Maintenance release for ``$tag``."
    }
    $buf = New-Object System.Collections.Generic.List[string]
    foreach ($item in $picked) {
        $buf.Add("- $($item.Subject) (``$($item.Hash)``)")
    }
    return ($buf -join "`n")
}

if ([string]::IsNullOrWhiteSpace($zhBody)) {
    $zhBody = Format-ConciseCommits 'zh'
}
if ([string]::IsNullOrWhiteSpace($enBody)) {
    $enBody = Format-ConciseCommits 'en'
}

$shaLower = $Sha256.Trim().ToLowerInvariant()
$sizeLabel = $InstallerSizeMB.Trim()

$lines = New-Object System.Collections.Generic.List[string]
$lines.Add('### 更新内容')
$lines.Add('')
$lines.Add($zhBody)
$lines.Add('')
$lines.Add('---')
$lines.Add('')
$lines.Add('### Highlights')
$lines.Add('')
$lines.Add($enBody)
$lines.Add('')
$lines.Add('---')
$lines.Add('')
$lines.Add("**安装包 / Installer**: ``PureUtils-$ver-setup.exe`` ($sizeLabel MB) · SHA-256: ``$shaLower``")
if ($prevTag) {
    $lines.Add("**Changelog**: https://github.com/$Repo/compare/$prevTag...$tag")
}

$outParent = Split-Path -Parent $OutputPath
if (-not [string]::IsNullOrWhiteSpace($outParent)) {
    New-Item -ItemType Directory -Force -Path $outParent | Out-Null
}
[System.IO.File]::WriteAllText($OutputPath, ($lines -join "`n") + "`n", $utf8NoBom)
Write-Host "Generated concise release notes ($rangeSpec, commits=$($commits.Count)) -> $OutputPath"
