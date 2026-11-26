# PureUtils（PureShot + PureClip + PureImage + PureVideo + PureZip）Go/MyGo 开发与打包脚本：build / test / run / publish / pack / clean
param(
    [ValidateSet('build', 'test', 'ui', 'e2e', 'coverage', 'ci', 'run', 'publish', 'pack', 'clean', 'help')]
    [string]$Command = 'build',
    [ValidateSet('Release', 'Debug')]
    [string]$Configuration = 'Release',
    [string]$Version = '1.2.0',
    [switch]$CoreOnly,
    [ValidateSet('pureshot', 'pureclip', 'pureimage', 'purevideo', 'purezip')]
    [string[]]$Exclude = @(),
    [string]$ShotDir = '',
    [string]$CovDir = '',
    [double]$MinCoverage = 0,
    [string[]]$AppArgs = @()
)

$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
Push-Location $root
try {
    # 本机 Go 环境把 GOSUMDB 设成了 off，导致 MyGo 要求的 Go 工具链无法校验；
    # 这里只在本脚本进程内恢复校验，不修改用户或全局配置。
    $env:GOTOOLCHAIN = 'auto'
    $env:CGO_ENABLED = '0'
    $env:GOSUMDB = 'sum.golang.org'

    function Invoke-Go([string[]]$GoArgs) {
        & go @GoArgs
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    }

    function Get-BuildTags([bool]$NoFreeImage, [string[]]$Excludes) {
        $seen = @{}
        $tags = New-Object System.Collections.Generic.List[string]
        if ($NoFreeImage) {
            $seen['nofreeimage'] = $true
            $tags.Add('nofreeimage')
        }
        foreach ($item in $Excludes) {
            if ([string]::IsNullOrWhiteSpace($item)) { continue }
            $tag = 'no' + $item.Trim().ToLowerInvariant()
            if (-not $seen.ContainsKey($tag)) {
                $seen[$tag] = $true
                $tags.Add($tag)
            }
        }
        return ,$tags.ToArray()
    }

    function Build-Binary([string]$OutPath, [bool]$NoFreeImage, [string[]]$Excludes = @(), [bool]$Cover = $false, [string]$CoverPkg = '') {
        $args = @('build', '-trimpath', '-buildvcs=false')
        if ($Cover) {
            $args += '-cover'
            if (-not [string]::IsNullOrWhiteSpace($CoverPkg)) {
                $args += "-coverpkg=$CoverPkg"
            }
        }
        $ldflags = @("-X=pureutils/pkg/purekit/core.AppVersion=$Version")
        if ($Configuration -eq 'Release') {
            $ldflags += @('-s', '-w', '-H=windowsgui')
        }
        if ($ldflags.Count -gt 0) {
            $args += @('-ldflags', ($ldflags -join ' '))
        }
        $tags = Get-BuildTags $NoFreeImage $Excludes
        if ($tags.Count -gt 0) {
            $args += @('-tags', ($tags -join ' '))
        }
        $args += @('-o', $OutPath, '.')
        Invoke-Go $args
    }

    function Find-ISCC {
        $candidates = @(
            $env:INNO_SETUP_ISCC,
            (Join-Path $root 'tools\innosetup\ISCC.exe'),
            (Join-Path (Split-Path -Parent $root) 'freeimage\tools\innosetup\ISCC.exe'),
            'C:\Program Files (x86)\Inno Setup 7\ISCC.exe',
            'C:\Program Files\Inno Setup 7\ISCC.exe',
            'C:\Program Files (x86)\Inno Setup 6\ISCC.exe',
            'C:\Program Files\Inno Setup 6\ISCC.exe'
        )
        foreach ($c in $candidates) {
            if ($c -and (Test-Path -LiteralPath $c)) { return $c }
        }
        $cmd = Get-Command iscc -ErrorAction SilentlyContinue
        if ($cmd) { return $cmd.Source }
        throw '未找到 Inno Setup ISCC.exe（可通过 $env:INNO_SETUP_ISCC 指定路径）'
    }

    function Ensure-LibMPV {
        $mpvDir = Join-Path $root 'libs\mpv'
        $mpvDll = Join-Path $mpvDir 'libmpv-2.dll'
        if (-not (Test-Path $mpvDll) -or ((Get-Item $mpvDll).Length -lt 1048576)) {
            Write-Host '未检测到 libs\mpv\libmpv-2.dll，正在自动下载并解压...' -ForegroundColor Cyan
            Invoke-Go @('run', './tools/fetchmpv', '-out', $mpvDir) | Out-Host
        }
        if (-not (Test-Path $mpvDll)) {
            throw "未找到 libmpv-2.dll：$mpvDll"
        }
        return $mpvDll
    }

    function Copy-FileIfChanged([string]$Src, [string]$Dst) {
        $need = -not (Test-Path $Dst)
        if (-not $need) {
            $need = (Get-FileHash $Src -Algorithm SHA256).Hash -ne (Get-FileHash $Dst -Algorithm SHA256).Hash
        }
        if ($need) {
            Copy-Item -Force $Src $Dst
        }
    }

    function Run-Tests([bool]$NoFreeImage, [string[]]$Excludes = @()) {
        $tags = Get-BuildTags $NoFreeImage $Excludes
        $testArgs = @('test', '-buildvcs=false', '-count=1')
        if ($tags.Count -gt 0) {
            $testArgs += @('-tags', ($tags -join ' '))
        }
        $testArgs += './...'
        Invoke-Go $testArgs
        Write-Host '测试通过' -ForegroundColor Green
    }

    function Run-UI {
        $resolvedShotDir = if ([string]::IsNullOrWhiteSpace($ShotDir)) {
            Join-Path $root 'artifacts\ui-screenshots'
        } elseif ([System.IO.Path]::IsPathRooted($ShotDir)) {
            $ShotDir
        } else {
            Join-Path $root $ShotDir
        }
        $enDir = Join-Path $resolvedShotDir 'en'
        $zhDir = Join-Path $resolvedShotDir 'zh'
        if (Test-Path -LiteralPath $enDir) { Remove-Item -Recurse -Force $enDir }
        if (Test-Path -LiteralPath $zhDir) { Remove-Item -Recurse -Force $zhDir }
        New-Item -ItemType Directory -Force -Path $enDir, $zhDir | Out-Null

        $prevEn = $env:PUREUTILS_EN_SHOT_DIR
        $prevZh = $env:PUREUTILS_ZH_SHOT_DIR
        try {
            $env:PUREUTILS_EN_SHOT_DIR = $enDir
            $env:PUREUTILS_ZH_SHOT_DIR = $zhDir

            Invoke-Go @('test', '-buildvcs=false', '-count=1', '-v', './internal/host', '-run', '^(TestI18nAudit|TestEnglishSettingsLayout|TestChineseSettingsLayout|TestSettingsLiveLanguageSwitch|TestEnglishLauncherLayout|TestChineseLauncherLayout|TestSettingsWindowUnifiedTabsAndPersistence|TestLauncherHeadlessUIInteraction)$')
            Invoke-Go @('test', '-buildvcs=false', '-count=1', '-v', './pkg/purekit/ui')
            Invoke-Go @('test', '-buildvcs=false', '-count=1', '-v', './plugins/pureclip', '-run', '^(TestEnglishPureClipLayout|TestChinesePureClipLayout|TestPureClipHistoryViewBusAndSettingsTab)$')
            Invoke-Go @('test', '-buildvcs=false', '-count=1', '-v', './plugins/pureshot', '-run', '^(TestEnglishPureShotLayout|TestChinesePureShotLayout|TestPureShotSettingsTabPersistence)$')
            Invoke-Go @('test', '-buildvcs=false', '-count=1', '-v', './plugins/pureimage', '-run', '^(TestEnglishPureImageLayout|TestChinesePureImageLayout|TestViewerHeadlessEmptyMenusToolbarAndPin)$')
            Invoke-Go @('test', '-buildvcs=false', '-count=1', '-v', './plugins/purevideo', '-run', '^(TestEnglishPureVideoLayout|TestChinesePureVideoLayout|TestPlayer|TestPureVideoPluginContractAndSettingsUI)')
            Invoke-Go @('test', '-buildvcs=false', '-count=1', '-v', './plugins/purezip', '-run', '^(TestEnglishPureZipLayout|TestChinesePureZipLayout|TestZipView|TestCompressAndExtractDialogsHeadless|TestZipJobViewsAndSettingsTabPersistence)')

            $enPngs = @(Get-ChildItem -LiteralPath $enDir -Filter '*.png' -File)
            $zhPngs = @(Get-ChildItem -LiteralPath $zhDir -Filter '*.png' -File)
            if ($enPngs.Count -lt 40) {
                throw ("英文 UI 截图数量异常：{0}（期望 >= 40）" -f $enPngs.Count)
            }
            if ($zhPngs.Count -lt 12) {
                throw ("中文 UI 截图数量异常：{0}（期望 >= 12）" -f $zhPngs.Count)
            }
            Write-Host ("UI 测试通过（EN 截图 {0} 张，ZH 截图 {1} 张 -> {2}）" -f $enPngs.Count, $zhPngs.Count, $resolvedShotDir) -ForegroundColor Green
        } finally {
            if ($null -eq $prevEn) { Remove-Item Env:PUREUTILS_EN_SHOT_DIR -ErrorAction SilentlyContinue } else { $env:PUREUTILS_EN_SHOT_DIR = $prevEn }
            if ($null -eq $prevZh) { Remove-Item Env:PUREUTILS_ZH_SHOT_DIR -ErrorAction SilentlyContinue } else { $env:PUREUTILS_ZH_SHOT_DIR = $prevZh }
        }
    }

    function Run-E2E {
        Invoke-Go @('test', '-buildvcs=false', '-count=1', '-v', './internal/host', '-run', '^TestE2E')

        $e2eOut = Join-Path $root 'artifacts\e2e'
        New-Item -ItemType Directory -Force -Path $e2eOut | Out-Null
        $fullBin = Join-Path $e2eOut 'PureUtils.exe'
        $coreBin = Join-Path $e2eOut 'PureUtils-core.exe'
        Build-Binary $fullBin $false @()
        Build-Binary $coreBin $true @()

        $prevBin = $env:PUREUTILS_E2E_BIN
        $prevCore = $env:PUREUTILS_E2E_CORE_BIN
        try {
            $env:PUREUTILS_E2E_BIN = $fullBin
            $env:PUREUTILS_E2E_CORE_BIN = $coreBin
            Invoke-Go @('test', '-buildvcs=false', '-count=1', '-v', '-tags', 'e2e', '-timeout', '5m', './e2e/...')
            Write-Host 'E2E 测试通过（无头跨插件 + 实机二进制）' -ForegroundColor Green
        } finally {
            if ($null -eq $prevBin) { Remove-Item Env:PUREUTILS_E2E_BIN -ErrorAction SilentlyContinue } else { $env:PUREUTILS_E2E_BIN = $prevBin }
            if ($null -eq $prevCore) { Remove-Item Env:PUREUTILS_E2E_CORE_BIN -ErrorAction SilentlyContinue } else { $env:PUREUTILS_E2E_CORE_BIN = $prevCore }
        }
    }

    function Get-TotalCovPct([string]$ProfilePath) {
        $lines = & go tool cover "-func=$ProfilePath"
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        $last = ($lines | Select-Object -Last 1)
        if ($last -match '([\d.]+)%') {
            return [double]$Matches[1]
        }
        throw "无法从 $ProfilePath 解析总覆盖率：$last"
    }

    function Get-PkgCovMap([string]$InputDirs) {
        $lines = & go tool covdata percent "-i=$InputDirs"
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        $map = @{}
        foreach ($line in $lines) {
            if ($line -match '^\s*([^\s]+)\s+coverage:\s+([\d.]+)%\s+of statements') {
                $map[$Matches[1]] = [double]$Matches[2]
            }
        }
        return $map
    }

    function Run-Coverage {
        $resolvedCovDir = if ([string]::IsNullOrWhiteSpace($CovDir)) {
            Join-Path $root 'artifacts\coverage'
        } elseif ([System.IO.Path]::IsPathRooted($CovDir)) {
            $CovDir
        } else {
            Join-Path $root $CovDir
        }
        $unitDir = Join-Path $resolvedCovDir 'unit'
        $headlessE2EDir = Join-Path $resolvedCovDir 'headless-e2e'
        $binE2EDir = Join-Path $resolvedCovDir 'bin-e2e'
        $binsDir = Join-Path $resolvedCovDir 'bins'
        foreach ($d in @($unitDir, $headlessE2EDir, $binE2EDir, $binsDir)) {
            if (Test-Path -LiteralPath $d) { Remove-Item -Recurse -Force $d }
            New-Item -ItemType Directory -Force -Path $d | Out-Null
        }

        $pkgs = 'pureutils,pureutils/internal/...,pureutils/pkg/...,pureutils/plugins/...'

        Invoke-Go @('test', '-buildvcs=false', '-count=1', '-cover', "-coverpkg=$pkgs", './...', '-args', "-test.gocoverdir=$unitDir")
        Invoke-Go @('test', '-buildvcs=false', '-count=1', '-cover', "-coverpkg=$pkgs", './internal/host', '-run', '^TestE2E', '-args', "-test.gocoverdir=$headlessE2EDir")

        $fullBin = Join-Path $binsDir 'PureUtils.exe'
        $coreBin = Join-Path $binsDir 'PureUtils-core.exe'
        Build-Binary $fullBin $false @() $true $pkgs
        Build-Binary $coreBin $true @() $true $pkgs

        $prevCov = $env:GOCOVERDIR
        $prevBin = $env:PUREUTILS_E2E_BIN
        $prevCore = $env:PUREUTILS_E2E_CORE_BIN
        try {
            $env:GOCOVERDIR = $binE2EDir
            $env:PUREUTILS_E2E_BIN = $fullBin
            $env:PUREUTILS_E2E_CORE_BIN = $coreBin
            Invoke-Go @('test', '-buildvcs=false', '-count=1', '-tags', 'e2e', '-timeout', '5m', './e2e/...')
        } finally {
            if ($null -eq $prevCov) { Remove-Item Env:GOCOVERDIR -ErrorAction SilentlyContinue } else { $env:GOCOVERDIR = $prevCov }
            if ($null -eq $prevBin) { Remove-Item Env:PUREUTILS_E2E_BIN -ErrorAction SilentlyContinue } else { $env:PUREUTILS_E2E_BIN = $prevBin }
            if ($null -eq $prevCore) { Remove-Item Env:PUREUTILS_E2E_CORE_BIN -ErrorAction SilentlyContinue } else { $env:PUREUTILS_E2E_CORE_BIN = $prevCore }
        }

        $unitOut = Join-Path $resolvedCovDir 'unit.out'
        $headlessE2EOut = Join-Path $resolvedCovDir 'headless-e2e.out'
        $binE2EOut = Join-Path $resolvedCovDir 'bin-e2e.out'
        $e2eOut = Join-Path $resolvedCovDir 'e2e.out'
        $combinedOut = Join-Path $resolvedCovDir 'combined.out'

        Invoke-Go @('tool', 'covdata', 'textfmt', "-i=$unitDir", '-o', $unitOut)
        Invoke-Go @('tool', 'covdata', 'textfmt', "-i=$headlessE2EDir", '-o', $headlessE2EOut)
        Invoke-Go @('tool', 'covdata', 'textfmt', "-i=$binE2EDir", '-o', $binE2EOut)
        Invoke-Go @('tool', 'covdata', 'textfmt', "-i=$headlessE2EDir,$binE2EDir", '-o', $e2eOut)
        Invoke-Go @('tool', 'covdata', 'textfmt', "-i=$unitDir,$binE2EDir", '-o', $combinedOut)

        $unitPct = Get-TotalCovPct $unitOut
        $headlessPct = Get-TotalCovPct $headlessE2EOut
        $binPct = Get-TotalCovPct $binE2EOut
        $e2ePct = Get-TotalCovPct $e2eOut
        $combinedPct = Get-TotalCovPct $combinedOut

        $unitMap = Get-PkgCovMap $unitDir
        $e2eMap = Get-PkgCovMap "$headlessE2EDir,$binE2EDir"
        $combMap = Get-PkgCovMap "$unitDir,$binE2EDir"

        $allPkgs = @($unitMap.Keys + $e2eMap.Keys + $combMap.Keys | Sort-Object -Unique)
        $lines = New-Object System.Collections.Generic.List[string]
        $lines.Add('### PureUtils Test Coverage Summary (Unit + UI + Headless E2E + Real-Binary E2E)')
        $lines.Add('')
        $lines.Add('| Scope | Statement Coverage |')
        $lines.Add('| --- | ---: |')
        $lines.Add(('| Unit + UI + Headless (`./...`) | **{0:F1}%** |' -f $unitPct))
        $lines.Add(('| Headless Cross-Plugin E2E (`TestE2E*`) | {0:F1}% |' -f $headlessPct))
        $lines.Add(('| Real-Binary E2E (`./e2e/...` via `GOCOVERDIR`) | {0:F1}% |' -f $binPct))
        $lines.Add(('| All E2E Merged (Headless + Real Binary) | **{0:F1}%** |' -f $e2ePct))
        $lines.Add(('| **Combined Total (Unit + UI + All E2E)** | **{0:F1}%** |' -f $combinedPct))
        if ($MinCoverage -gt 0) {
            $gateStatus = if ($combinedPct -ge $MinCoverage) { 'PASS' } else { 'FAIL' }
            $lines.Add(('| Threshold Gate (>= {0:F1}%) | **{1}** |' -f $MinCoverage, $gateStatus))
        }
        $lines.Add('')
        $lines.Add('| Package | Unit + UI | E2E (All) | Combined |')
        $lines.Add('| --- | ---: | ---: | ---: |')
        foreach ($pkg in $allPkgs) {
            $uStr = if ($unitMap.ContainsKey($pkg)) { ('{0:F1}%' -f $unitMap[$pkg]) } else { '—' }
            $eStr = if ($e2eMap.ContainsKey($pkg)) { ('{0:F1}%' -f $e2eMap[$pkg]) } else { '—' }
            $cStr = if ($combMap.ContainsKey($pkg)) { ('**{0:F1}%**' -f $combMap[$pkg]) } else { '—' }
            $lines.Add(("| ``{0}`` | {1} | {2} | {3} |" -f $pkg, $uStr, $eStr, $cStr))
        }
        $lines.Add('')

        $md = $lines -join "`n"
        $summaryPath = Join-Path $resolvedCovDir 'summary.md'
        Set-Content -LiteralPath $summaryPath -Value $md -Encoding UTF8
        if (-not [string]::IsNullOrWhiteSpace($env:GITHUB_STEP_SUMMARY)) {
            Add-Content -LiteralPath $env:GITHUB_STEP_SUMMARY -Value $md -Encoding UTF8
        }

        Write-Host ("覆盖率汇总：Unit+UI={0:F1}% | Headless E2E={1:F1}% | Binary E2E={2:F1}% | All E2E={3:F1}% | Combined={4:F1}%" -f $unitPct, $headlessPct, $binPct, $e2ePct, $combinedPct) -ForegroundColor Cyan
        Write-Host ("覆盖率报告已生成：{0}" -f $summaryPath) -ForegroundColor Green

        if ($MinCoverage -gt 0 -and $combinedPct -lt $MinCoverage) {
            throw ("合并覆盖率 {0:F1}% 低于最低门槛 {1:F1}%" -f $combinedPct, $MinCoverage)
        }
    }

    switch ($Command) {
        'build' {
            Build-Binary (Join-Path $root 'PureUtils.exe') ([bool]$CoreOnly) $Exclude
            $tags = Get-BuildTags ([bool]$CoreOnly) $Exclude
            if ($tags.Count -gt 0) {
                Write-Host ("已生成裁剪版 PureUtils.exe（tags: {0}）" -f ($tags -join ',')) -ForegroundColor Green
            } else {
                Write-Host '已生成完整版 PureUtils.exe（含全部内建插件）' -ForegroundColor Green
            }
        }
        'test' {
            Run-Tests ([bool]$CoreOnly) $Exclude
        }
        'ui' {
            Run-UI
        }
        'e2e' {
            Run-E2E
        }
        'coverage' {
            Run-Coverage
        }
        'ci' {
            Invoke-Go @('vet', '-buildvcs=false', '-unsafeptr=false', './...')
            Invoke-Go @('vet', '-buildvcs=false', '-unsafeptr=false', '-tags', 'nofreeimage', './...')
            Invoke-Go @('vet', '-buildvcs=false', '-unsafeptr=false', '-tags', 'e2e', './e2e/...')
            Run-Tests $false @()
            Run-Tests $true @()
            Run-UI
            Run-E2E
            Write-Host '全部 CI 检查通过' -ForegroundColor Green
        }
        'run' {
            $tags = Get-BuildTags ([bool]$CoreOnly) $Exclude
            $runArgs = @('run', '-buildvcs=false')
            if ($tags.Count -gt 0) { $runArgs += @('-tags', ($tags -join ' ')) }
            $runArgs += @('.') + $AppArgs
            Invoke-Go $runArgs
        }
        'publish' {
            $out = Join-Path $root 'artifacts\publish'
            $outCore = Join-Path $root 'artifacts\publish-core'
            New-Item -ItemType Directory -Force -Path (Join-Path $out 'resources') | Out-Null
            New-Item -ItemType Directory -Force -Path (Join-Path $outCore 'resources') | Out-Null

            Build-Binary (Join-Path $out 'PureUtils.exe') $false
            Copy-Item -Force (Join-Path $root 'resources\icon.png') (Join-Path $out 'resources\icon.png')
            Copy-Item -Force (Join-Path $root 'resources\icon.ico') (Join-Path $out 'resources\icon.ico')
            Copy-Item -Force (Join-Path $root 'resources\image.ico') (Join-Path $out 'resources\image.ico')
            Copy-Item -Force (Join-Path $root 'resources\image.png') (Join-Path $out 'resources\image.png')
            Copy-Item -Force (Join-Path $root 'resources\video.ico') (Join-Path $out 'resources\video.ico')
            Copy-Item -Force (Join-Path $root 'resources\video.png') (Join-Path $out 'resources\video.png')
            Copy-Item -Force (Join-Path $root 'resources\zip.ico') (Join-Path $out 'resources\zip.ico')
            Copy-Item -Force (Join-Path $root 'resources\zip.png') (Join-Path $out 'resources\zip.png')
            $turbo = Join-Path $root 'libs\turbojpeg\turbojpeg.dll'
            if (Test-Path $turbo) {
                Copy-Item -Force $turbo (Join-Path $out 'turbojpeg.dll')
            }
            $mpvDll = Ensure-LibMPV
            Copy-Item -Force $mpvDll (Join-Path $out 'libmpv-2.dll')

            Build-Binary (Join-Path $outCore 'PureUtils.exe') $true
            Copy-Item -Force (Join-Path $root 'resources\icon.png') (Join-Path $outCore 'resources\icon.png')
            Copy-Item -Force (Join-Path $root 'resources\icon.ico') (Join-Path $outCore 'resources\icon.ico')
            Copy-Item -Force (Join-Path $root 'resources\video.ico') (Join-Path $outCore 'resources\video.ico')
            Copy-Item -Force (Join-Path $root 'resources\video.png') (Join-Path $outCore 'resources\video.png')
            Copy-Item -Force (Join-Path $root 'resources\zip.ico') (Join-Path $outCore 'resources\zip.ico')
            Copy-Item -Force (Join-Path $root 'resources\zip.png') (Join-Path $outCore 'resources\zip.png')
            Copy-Item -Force $mpvDll (Join-Path $outCore 'libmpv-2.dll')
            Set-Content -Encoding UTF8 (Join-Path $outCore 'modules.json') "{`n  `"capture`": true,`n  `"history`": true,`n  `"viewer`": false,`n  `"video`": true,`n  `"zip`": true`n}"

            Write-Host "已发布完整版到 $out，精简版到 $outCore" -ForegroundColor Green
        }
        'pack' {
            $stage = Join-Path $root 'artifacts\stage'
            $resStage = Join-Path $stage 'resources'
            $outDir = Join-Path $root 'artifacts\installer'
            New-Item -ItemType Directory -Force -Path $resStage | Out-Null
            New-Item -ItemType Directory -Force -Path $outDir | Out-Null

            Build-Binary (Join-Path $stage 'PureUtils-full.exe') $false
            Build-Binary (Join-Path $stage 'PureUtils-core.exe') $true

            Copy-Item -Force (Join-Path $root 'resources\icon.png') (Join-Path $resStage 'icon.png')
            Copy-Item -Force (Join-Path $root 'resources\icon.ico') (Join-Path $resStage 'icon.ico')
            Copy-Item -Force (Join-Path $root 'resources\setup.ico') (Join-Path $resStage 'setup.ico')
            Copy-Item -Force (Join-Path $root 'resources\image.ico') (Join-Path $resStage 'image.ico')
            Copy-Item -Force (Join-Path $root 'resources\image.png') (Join-Path $resStage 'image.png')
            Copy-Item -Force (Join-Path $root 'resources\video.ico') (Join-Path $resStage 'video.ico')
            Copy-Item -Force (Join-Path $root 'resources\video.png') (Join-Path $resStage 'video.png')
            Copy-Item -Force (Join-Path $root 'resources\zip.ico') (Join-Path $resStage 'zip.ico')
            Copy-Item -Force (Join-Path $root 'resources\zip.png') (Join-Path $resStage 'zip.png')
            $turbo = Join-Path $root 'libs\turbojpeg\turbojpeg.dll'
            if (Test-Path $turbo) {
                Copy-FileIfChanged $turbo (Join-Path $stage 'turbojpeg.dll')
            }
            $mpvDll = Ensure-LibMPV
            Copy-FileIfChanged $mpvDll (Join-Path $stage 'libmpv-2.dll')

            $iscc = Find-ISCC
            $iss = Join-Path $root 'installer\pureutils.iss'
            $isccArgs = @("/DStageDir=$stage", "/DOutputDir=$outDir", "/DAppVersion=$Version", $iss)
            & $iscc @isccArgs
            if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
            Write-Host "已生成模块化安装包（内嵌 libmpv）：$outDir\PureUtils-$Version-setup.exe" -ForegroundColor Green
        }
        'clean' {
            Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $root 'PureUtils.exe')
            Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $root 'ynm3000.exe')
            Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $root 'ShotClip.exe')
            Remove-Item -Recurse -Force -ErrorAction SilentlyContinue (Join-Path $root 'artifacts\publish')
            Remove-Item -Recurse -Force -ErrorAction SilentlyContinue (Join-Path $root 'artifacts\publish-core')
            Remove-Item -Recurse -Force -ErrorAction SilentlyContinue (Join-Path $root 'artifacts\stage')
            Remove-Item -Recurse -Force -ErrorAction SilentlyContinue (Join-Path $root 'artifacts\installer')
            Remove-Item -Recurse -Force -ErrorAction SilentlyContinue (Join-Path $root 'artifacts\ui-screenshots')
            Remove-Item -Recurse -Force -ErrorAction SilentlyContinue (Join-Path $root 'artifacts\e2e')
            Remove-Item -Recurse -Force -ErrorAction SilentlyContinue (Join-Path $root 'artifacts\e2e-logs')
            Remove-Item -Recurse -Force -ErrorAction SilentlyContinue (Join-Path $root 'artifacts\coverage')
            Write-Host '已清理构建产物' -ForegroundColor Green
        }
        'help' {
            Write-Host '用法: .\build.ps1 [-Command build|test|ui|e2e|coverage|ci|run|publish|pack|clean] [-Version 1.2.0] [-CoreOnly] [-Exclude pureshot,pureclip,pureimage,purevideo,purezip] [-ShotDir ...] [-CovDir ...] [-MinCoverage 50] [-AppArgs ...]'
            Write-Host '  build     编译 PureUtils.exe（加 -CoreOnly 不含 PureImage 看图，或用 -Exclude 裁剪指定插件）'
            Write-Host '  test      运行 go test -count=1 ./...（支持 -CoreOnly 与 -Exclude）'
            Write-Host '  ui        运行中英文/深浅色 UI 布局、交互与 i18n 审计测试并导出截图'
            Write-Host '  e2e       运行无头跨插件 E2E 与真实编译二进制 IPC/打包解压 E2E 测试'
            Write-Host '  coverage  采集单测 + UI + 无头 E2E + 实机二进制 GOCOVERDIR 合并覆盖率并生成报告'
            Write-Host '  ci        按顺序执行全套 CI 检查（vet / 双版本 test / ui / e2e）'
            Write-Host '  run       直接运行（可传 AppArgs，例如 --launcher、--history、--view、--video 或 --zip）'
            Write-Host '  publish   输出完整版到 artifacts\publish 与精简版到 artifacts\publish-core'
            Write-Host '  pack      构建双版本并用 Inno Setup 打包模块化安装程序'
            Write-Host '  clean     删除构建与打包产物'
        }
    }
} finally {
    Pop-Location
}