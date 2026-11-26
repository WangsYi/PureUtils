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
$rangeLabel = if ($prevTag) { "$prevTag..$tag" } else { $tag }

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
            'docs' { $cat = 'docs' }
            { $_ -in @('ci', 'build', 'chore', 'test') } { $cat = 'ci' }
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

$usedManual = (-not [string]::IsNullOrWhiteSpace($zhBody)) -or (-not [string]::IsNullOrWhiteSpace($enBody))

function Format-GroupedCommits([string]$Lang) {
    $groups = if ($Lang -eq 'zh') {
        @(
            @{ Key = 'feat'; Title = '#### ✨ 新功能' },
            @{ Key = 'fix'; Title = '#### 🐛 问题修复' },
            @{ Key = 'perf'; Title = '#### ⚡ 体验与优化' },
            @{ Key = 'docs'; Title = '#### 📝 文档' },
            @{ Key = 'ci'; Title = '#### 📦 构建与发布' },
            @{ Key = 'other'; Title = '#### 🔧 其他变更' }
        )
    } else {
        @(
            @{ Key = 'feat'; Title = '#### ✨ Features' },
            @{ Key = 'fix'; Title = '#### 🐛 Bug Fixes' },
            @{ Key = 'perf'; Title = '#### ⚡ Improvements' },
            @{ Key = 'docs'; Title = '#### 📝 Documentation' },
            @{ Key = 'ci'; Title = '#### 📦 Build, CI & Release' },
            @{ Key = 'other'; Title = '#### 🔧 Other Changes' }
        )
    }

    $buf = New-Object System.Collections.Generic.List[string]
    $total = $commits.Count
    $featCount = @($commits | Where-Object { $_.Category -eq 'feat' }).Count
    $fixCount = @($commits | Where-Object { $_.Category -eq 'fix' }).Count

    if ($total -eq 0) {
        if ($Lang -eq 'zh') {
            $buf.Add("本版本为 ``$tag`` 常规维护发布。")
        } else {
            $buf.Add("Maintenance release for ``$tag``.")
        }
        return ($buf -join "`n")
    }

    if ($Lang -eq 'zh') {
        $buf.Add("本版本共包含 **$total** 项变更（新增特性 **$featCount** 项，问题修复 **$fixCount** 项）：")
    } else {
        $buf.Add("This release includes **$total** changes (**$featCount** features, **$fixCount** bug fixes):")
    }
    $buf.Add('')

    foreach ($g in $groups) {
        $items = @($commits | Where-Object { $_.Category -eq $g.Key })
        if ($items.Count -eq 0) { continue }
        $buf.Add($g.Title)
        $buf.Add('')
        foreach ($item in $items) {
            $buf.Add("- $($item.Subject) (``$($item.Hash)``)")
        }
        $buf.Add('')
    }

    return ($buf -join "`n").Trim()
}

if ([string]::IsNullOrWhiteSpace($zhBody)) {
    $zhBody = Format-GroupedCommits 'zh'
}
if ([string]::IsNullOrWhiteSpace($enBody)) {
    $enBody = Format-GroupedCommits 'en'
}

$bytesFmt = $InstallerBytes.ToString('N0', [System.Globalization.CultureInfo]::InvariantCulture)
$shaLower = $Sha256.Trim().ToLowerInvariant()
$sizeLabel = $InstallerSizeMB.Trim()

$lines = New-Object System.Collections.Generic.List[string]
$lines.Add("## PureUtils $tag")
$lines.Add('')
$lines.Add('### 中文更新说明')
$lines.Add('')
$lines.Add($zhBody)
$lines.Add('')
$lines.Add('---')
$lines.Add('')
$lines.Add('### English Release Notes')
$lines.Add('')
$lines.Add($enBody)
$lines.Add('')
$lines.Add('---')
$lines.Add('')
$lines.Add('### 安装包信息 / Installer')
$lines.Add('')
$lines.Add('| 文件 / File | 大小 / Size | SHA-256 |')
$lines.Add('|---|---|---|')
$lines.Add("| ``PureUtils-$ver-setup.exe`` | $sizeLabel MB ($bytesFmt bytes) | ``$shaLower`` |")
$lines.Add('| `SHA256SUMS.txt` | — | SHA-256 校验清单 / Checksum manifest |')
$lines.Add('')

if ($usedManual -and $commits.Count -gt 0) {
    $lines.Add('<details>')
    $lines.Add("<summary>完整提交记录 / Full Commit Log ($rangeLabel)</summary>")
    $lines.Add('')
    foreach ($c in $commits) {
        $lines.Add("- $($c.Subject) (``$($c.Hash)``)")
    }
    $lines.Add('')
    $lines.Add('</details>')
    $lines.Add('')
}

if ($prevTag) {
    $lines.Add("**完整变更对比 / Full Changelog**: https://github.com/$Repo/compare/$prevTag...$tag")
    $lines.Add('')
}

$lines.Add('---')
$lines.Add('')
$lines.Add('> 本版本由 GitHub Actions 自动化流水线完成编译、测试、打包与校验。  ')
$lines.Add('> Built, tested, packaged, and verified automatically via GitHub Actions.')

$outParent = Split-Path -Parent $OutputPath
if (-not [string]::IsNullOrWhiteSpace($outParent)) {
    New-Item -ItemType Directory -Force -Path $outParent | Out-Null
}
[System.IO.File]::WriteAllText($OutputPath, ($lines -join "`n") + "`n", $utf8NoBom)
Write-Host "Generated bilingual release notes ($rangeSpec, commits=$($commits.Count)) -> $OutputPath"
