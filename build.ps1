# PureUtils（PureShot + PureClip + PureImage + PureZip）Go/MyGo 开发与打包脚本：build / test / run / publish / pack / clean
param(
    [ValidateSet('build', 'test', 'run', 'publish', 'pack', 'clean', 'help')]
    [string]$Command = 'build',
    [ValidateSet('Release', 'Debug')]
    [string]$Configuration = 'Release',
    [switch]$CoreOnly,
    [ValidateSet('pureshot', 'pureclip', 'pureimage', 'purezip')]
    [string[]]$Exclude = @(),
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

    function Build-Binary([string]$OutPath, [bool]$NoFreeImage, [string[]]$Excludes = @()) {
        $args = @('build', '-trimpath', '-buildvcs=false')
        $ldflags = @()
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
            $tags = Get-BuildTags ([bool]$CoreOnly) $Exclude
            $testArgs = @('test', '-buildvcs=false', '-count=1')
            if ($tags.Count -gt 0) {
                $testArgs += @('-tags', ($tags -join ' '))
            }
            $testArgs += './...'
            Invoke-Go $testArgs
            Write-Host '测试通过' -ForegroundColor Green
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
            Copy-Item -Force (Join-Path $root 'resources\zip.ico') (Join-Path $out 'resources\zip.ico')
            Copy-Item -Force (Join-Path $root 'resources\zip.png') (Join-Path $out 'resources\zip.png')
            $turbo = Join-Path $root 'libs\turbojpeg\turbojpeg.dll'
            if (Test-Path $turbo) {
                Copy-Item -Force $turbo (Join-Path $out 'turbojpeg.dll')
            }

            Build-Binary (Join-Path $outCore 'PureUtils.exe') $true
            Copy-Item -Force (Join-Path $root 'resources\icon.png') (Join-Path $outCore 'resources\icon.png')
            Copy-Item -Force (Join-Path $root 'resources\icon.ico') (Join-Path $outCore 'resources\icon.ico')
            Copy-Item -Force (Join-Path $root 'resources\zip.ico') (Join-Path $outCore 'resources\zip.ico')
            Copy-Item -Force (Join-Path $root 'resources\zip.png') (Join-Path $outCore 'resources\zip.png')
            Set-Content -Encoding UTF8 (Join-Path $outCore 'modules.json') "{`n  `"capture`": true,`n  `"history`": true,`n  `"viewer`": false,`n  `"zip`": true`n}"

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
            Copy-Item -Force (Join-Path $root 'resources\zip.ico') (Join-Path $resStage 'zip.ico')
            Copy-Item -Force (Join-Path $root 'resources\zip.png') (Join-Path $resStage 'zip.png')
            $turbo = Join-Path $root 'libs\turbojpeg\turbojpeg.dll'
            if (Test-Path $turbo) {
                $turboDst = Join-Path $stage 'turbojpeg.dll'
                $needCopy = -not (Test-Path $turboDst)
                if (-not $needCopy) {
                    $needCopy = (Get-FileHash $turbo -Algorithm SHA256).Hash -ne (Get-FileHash $turboDst -Algorithm SHA256).Hash
                }
                if ($needCopy) {
                    Copy-Item -Force $turbo $turboDst
                }
            }

            $iscc = Find-ISCC
            $iss = Join-Path $root 'installer\pureutils.iss'
            & $iscc "/DStageDir=$stage" "/DOutputDir=$outDir" $iss
            if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
            Write-Host "已生成模块化安装包：$outDir\PureUtils-1.0.0-setup.exe" -ForegroundColor Green
        }
        'clean' {
            Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $root 'PureUtils.exe')
            Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $root 'ynm3000.exe')
            Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $root 'ShotClip.exe')
            Remove-Item -Recurse -Force -ErrorAction SilentlyContinue (Join-Path $root 'artifacts\publish')
            Remove-Item -Recurse -Force -ErrorAction SilentlyContinue (Join-Path $root 'artifacts\publish-core')
            Remove-Item -Recurse -Force -ErrorAction SilentlyContinue (Join-Path $root 'artifacts\stage')
            Remove-Item -Recurse -Force -ErrorAction SilentlyContinue (Join-Path $root 'artifacts\installer')
            Write-Host '已清理构建产物' -ForegroundColor Green
        }
        'help' {
            Write-Host '用法: .\build.ps1 [-Command build|test|run|publish|pack|clean] [-CoreOnly] [-Exclude pureshot,pureclip,pureimage,purezip] [-AppArgs ...]'
            Write-Host '  build     编译 PureUtils.exe（加 -CoreOnly 不含 PureImage 看图，或用 -Exclude 裁剪指定插件）'
            Write-Host '  test      运行 go test -count=1 ./...（支持 -CoreOnly 与 -Exclude）'
            Write-Host '  run       直接运行（可传 AppArgs，例如 --launcher、--history、--view 或 --zip）'
            Write-Host '  publish   输出完整版到 artifacts\publish 与精简版到 artifacts\publish-core'
            Write-Host '  pack      构建双版本并用 Inno Setup 打包模块化安装程序'
            Write-Host '  clean     删除构建与打包产物'
        }
    }
} finally {
    Pop-Location
}