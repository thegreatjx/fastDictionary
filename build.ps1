# build.ps1 — 正式版构建脚本
#
# 为什么需要这个脚本：`go build -o fastDictionary.exe .` 默认生成 **控制台子系统**
# 的 exe，双击会多出一个黑窗口，而且关掉黑窗口等于关掉程序。正式版必须加
# `-H=windowsgui`。这个坑已经踩过一次，所以把命令固化下来。
#
# 输出名必须是 fastDictionary.exe，和 go.mod 的模块名一致：这样 `go build .`
# 的默认产物就是快捷方式要的文件，不再靠 `-o` 硬撑。名字还得和 exe 文件名
# 一致 —— mygo 用 exe 名推导数据目录（%APPDATA%\<exe 名>），exe 一改名，
# 用户的配置和生词本就像凭空消失。构建结束会核对快捷方式目标，漂移时改回来。
#
#   .\build.ps1            构建正式版 fastDictionary.exe（GUI，无黑窗口），替换正在运行的实例
#   .\build.ps1 -Dev       构建开发版 fastDictionary-dev.exe（有控制台，能看到日志）
#   .\build.ps1 -Package   构建正式版并生成 dist/ 生产分发包（绿色目录、zip 及 sha256）
#   .\build.ps1 -Run       构建后启动程序
#   .\build.ps1 -NoStop    不停掉正在运行的实例（调试用；此时 exe 可能被占用）
#   .\build.ps1 -Overwrite 出包时若今日版本目录已存在，强制覆盖刷新而不生成 -r2
#   .\build.ps1 -Startup   同时创建/校正开机启动快捷方式（默认不创建）
#   .\build.ps1 -Version 1.2.0  写入「关于」与更新检查使用的版本号（发布流水线用 tag）
#   .\build.ps1 -Sign      用代码签名证书签 exe；证书路径与密码读环境变量
#                          FD_SIGN_CERT / FD_SIGN_PASSWORD，时间戳服务可用
#                          FD_SIGN_TIMESTAMP 覆盖

param(
    [switch]$Dev,
    [switch]$Run,
    [switch]$NoStop,
    [switch]$Package,
    [switch]$Overwrite,
    [switch]$Startup,
    [string]$Version = "1.1.0",
    [switch]$Sign
)

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

$ldflags = '-H=windowsgui'
$out = 'fastDictionary.exe'

if ($Dev) {
    # 开发版保留控制台：`go test` 的日志、panic 栈都能直接看到
    $ldflags = ''
    $out = 'fastDictionary-dev.exe'
}

# 把构建时间打进二进制，程序启动时写进日志、在「关于」里显示。
# 没有这个戳，「快捷方式打开的是不是新版本」只能靠猜。
# 时间戳不能带空格：go 按空格切分 -ldflags，带空格会被切成两段，链接器报错。
$stamp = Get-Date -Format 'yyyy-MM-ddTHH:mm'
$releaseVersion = $Version.TrimStart('v')
if ($ldflags) {
    $ldflags = "$ldflags -X main.buildStamp=$stamp -X main.appVersion=$releaseVersion"
}

# 先停掉正在运行的实例。Windows 不允许覆盖被映射为映像的 exe —— Go 的链接器
# 用「写临时文件再改名」绕过，所以构建能成功，但旧进程仍在内存里跑旧代码；
# 而单实例锁会让新启动的进程把命令行交给旧进程后立刻退出。用户看到的永远是
# 旧界面，这正是「快捷方式打开都是旧客户端」的直接原因。
#
# 放在 gofmt/vet 之后：这两步失败时程序还在运行，不会白停一次。
$wasRunning = $false

Write-Host "gofmt..." -ForegroundColor Cyan
$unformatted = & gofmt -l .
if ($unformatted) {
    Write-Host "以下文件未格式化，先跑 gofmt -w ." -ForegroundColor Yellow
    $unformatted | ForEach-Object { Write-Host "  $_" }
    exit 1
}

Write-Host "go vet..." -ForegroundColor Cyan
& go vet ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

if (-not $NoStop) {
    $running = Get-Process fastDictionary, fastDictionary-dev -ErrorAction SilentlyContinue
    if ($running) {
        $wasRunning = $true
        Write-Host "停止运行中的实例..." -ForegroundColor Cyan
        $running | Stop-Process -Force
        try {
            $running | Wait-Process -Timeout 3 -ErrorAction SilentlyContinue
        } catch {}
        Start-Sleep -Milliseconds 300
    }
}

Write-Host "go build -> $out ($stamp)" -ForegroundColor Cyan
if ($ldflags) {
    & go build -ldflags $ldflags -o $out .
} else {
    & go build -o $out .
}
if ($LASTEXITCODE -ne 0) {
    # 构建失败时别把用户丢在没有程序的桌面上：把刚才停掉的实例拉回来。
    if ($wasRunning) {
        Start-Process -FilePath (Join-Path $PSScriptRoot $out) -WorkingDirectory $PSScriptRoot
        Write-Host "构建失败，已恢复运行中的旧实例" -ForegroundColor Yellow
    }
    exit $LASTEXITCODE
}


# 把 OCR 模型和 onnxruntime 复制到 exe 旁边。
#
# Indra OCR 在运行时从 exe 同目录的 indra-models\ 和 onnxruntime.dll 找模型。
# 不复制的话，程序照常启动（会回落到 Windows OCR），但截图识别会退回原来
# 那个识别质量差得多的引擎，而且没有任何提示——用户只会觉得"还是不准"。
#
# 开发时模型放在 build\indra-models，DLL 用本机 Python 的 onnxruntime；
# 正式版必须自己带一份，否则换台机器就失效。
$modelSrc = Join-Path $PSScriptRoot 'build\indra-models'
$modelDst = Join-Path $PSScriptRoot 'indra-models'
if (Test-Path $modelSrc) {
    if (-not (Test-Path $modelDst)) { New-Item -ItemType Directory -Path $modelDst | Out-Null }
    $models = @('det.onnx', 'rec.onnx', 'rec_cjk.onnx', 'script.onnx', 'meta.json')
    $copied = 0
    foreach ($m in $models) {
        $src = Join-Path $modelSrc $m
        if (Test-Path $src) {
            Copy-Item $src (Join-Path $modelDst $m) -Force
            $copied++
        }
    }
    Write-Host "indra models -> indra-models\ ($copied files)" -ForegroundColor Green
} else {
    Write-Host "没有 build\indra-models，截图识别会回落到 Windows OCR" -ForegroundColor Yellow
}

# onnxruntime.dll：优先用本机 Python 里那份（版本新），没有就跳过。
$ortDst = Join-Path $PSScriptRoot 'onnxruntime.dll'
if (-not (Test-Path $ortDst)) {
    $ortCandidates = @(
        (Join-Path $env:LOCALAPPDATA 'Programs\Python\Python312\Lib\site-packages\onnxruntime\capi\onnxruntime.dll'),
        (Join-Path $env:SystemRoot 'System32\onnxruntime.dll')
    )
    foreach ($c in $ortCandidates) {
        if (Test-Path $c) {
            Copy-Item $c $ortDst -Force
            Write-Host "onnxruntime.dll -> $((Get-Item $ortDst).Length / 1MB -as [int])MB" -ForegroundColor Green
            break
        }
    }
}
if (-not (Test-Path $ortDst)) {
    Write-Host "没有 onnxruntime.dll，截图识别会回落到 Windows OCR" -ForegroundColor Yellow
}

# OCR 依赖版本与校验。模型和 DLL 不进仓库，但发布包必须带，而且必须是我们
# 验证过的固定版本 —— 以前 DLL 是从构建机的 Python 安装目录直接拷的，换台
# 机器编译出的包可能跑不同的运行时。发布构建校验失败就停，避免带缺陷出包。
$deps = $null
$ocrDepsOK = $false
$depsManifestPath = Join-Path $PSScriptRoot 'scripts\ocr-deps.json'
if (Test-Path $depsManifestPath) {
    $deps = Get-Content $depsManifestPath -Raw | ConvertFrom-Json
    $bad = @()
    if (Test-Path $ortDst) {
        $h = (Get-FileHash $ortDst -Algorithm SHA256).Hash
        if ($h -ne $deps.onnxruntime.dllSha256) {
            $bad += "onnxruntime.dll（期望 v$($deps.onnxruntime.version)）"
        }
    } else {
        $bad += 'onnxruntime.dll'
    }
    foreach ($f in $deps.models.files) {
        $p = Join-Path $PSScriptRoot (Join-Path 'indra-models' $f.name)
        if (-not (Test-Path $p)) { $bad += $f.name; continue }
        if ((Get-FileHash $p -Algorithm SHA256).Hash -ne $f.sha256) { $bad += "$($f.name)（SHA256 不符）" }
    }
    $ocrDepsOK = $bad.Count -eq 0
    if (-not $ocrDepsOK) {
        Write-Host ("OCR 运行时依赖缺失或版本不符: " + ($bad -join ', ')) -ForegroundColor Yellow
        Write-Host "  运行 scripts\fetch-ocr-deps.ps1 获取固定版本（onnxruntime v$($deps.onnxruntime.version)）" -ForegroundColor Yellow
        if ($Package) { exit 1 }
    }
} else {
    Write-Host "没有 scripts\ocr-deps.json，跳过 OCR 依赖校验" -ForegroundColor Yellow
}
# 子系统校验：GUI 构建的子系统号必须是 2，否则就是又生成黑窗口版本。
# 直接读 PE 头，不依赖 objdump（mingw 的 objdump 在某些 PATH 下无法从
# PowerShell 管道调用）。
function Get-PESubsystem([string]$Path) {
    $fs = [System.IO.File]::OpenRead((Resolve-Path $Path))
    try {
        $br = New-Object System.IO.BinaryReader($fs)
        $fs.Position = 0x3C
        $peOffset = $br.ReadInt32()
        $fs.Position = $peOffset + 4 + 20 + 68   # PE sig + COFF header + optional header offset of Subsystem
        return $br.ReadUInt16()
    } finally { $fs.Dispose() }
}

$subsystem = Get-PESubsystem $out
$subsystemName = switch ($subsystem) {
    2 { "Windows GUI" }
    3 { "Windows CUI (控制台)" }
    default { "unknown($subsystem)" }
}
if ($ldflags -and $subsystem -ne 2) {
    Write-Host "构建结果不是 GUI 子系统（$subsystemName），双击会出现黑窗口" -ForegroundColor Red
    exit 1
}
Write-Host "ok: $out ($((Get-Item $out).Length) bytes)  Subsystem=$subsystemName  $stamp" -ForegroundColor Green

# 可选签名。证书不进仓库；没有证书时不上车，构建仍然可用（只是 SmartScreen
# 会警告未知发布者）。签名放在子系统校验之后，签的必须是已经合格的产物。
if ($Sign) {
    $certPath = $env:FD_SIGN_CERT
    if (-not $certPath -or -not (Test-Path $certPath)) {
        Write-Host "-Sign 需要环境变量 FD_SIGN_CERT 指向 .pfx 证书" -ForegroundColor Red
        exit 1
    }
    $signtool = Get-Command signtool.exe -ErrorAction SilentlyContinue
    if (-not $signtool) {
        Write-Host "找不到 signtool.exe（安装 Windows SDK 后重试）" -ForegroundColor Red
        exit 1
    }
    $timestamp = $env:FD_SIGN_TIMESTAMP
    if (-not $timestamp) { $timestamp = 'http://timestamp.digicert.com' }
    $signArgs = @('sign', '/fd', 'SHA256', '/f', $certPath, '/tr', $timestamp, '/td', 'SHA256')
    if ($env:FD_SIGN_PASSWORD) { $signArgs += @('/p', $env:FD_SIGN_PASSWORD) }
    $signArgs += (Join-Path $PSScriptRoot $out)
    & $signtool.Source @signArgs
    if ($LASTEXITCODE -ne 0) {
        Write-Host "代码签名失败" -ForegroundColor Red
        exit $LASTEXITCODE
    }
    Write-Host "已签名: $out" -ForegroundColor Green
}

# 对账并纠正：快捷方式指向的必须就是刚构建的文件。
#
# 以前这里只报警，于是「构建成功、快捷方式打开旧客户端」照样发生 —— 报警
# 没人看，用户看到的就是旧界面。现在直接写回正确目标：快捷方式是这个项目
# 唯一的启动入口，让它指向刚构建的 exe 属于构建的一部分，而不是用户的活。
# 同时对账桌面和开始菜单快捷方式，确保用户从常用入口打开的是最新构建的 exe。
# 开机启动项只在显式传 -Startup 时创建/校正，避免用户没要求就被设为开机自启。
if (-not $Dev) {
    $expected = Join-Path $PSScriptRoot $out
    $shell = New-Object -ComObject WScript.Shell
    $lnkName = 'fast词典.lnk'

    $shortcutLocations = @(
        @{ Name = "桌面"; Dir = [Environment]::GetFolderPath('Desktop') },
        @{ Name = "开始菜单"; Dir = [Environment]::GetFolderPath('Programs') }
    )
    if ($Startup) {
        $shortcutLocations += @{ Name = "启动项"; Dir = [Environment]::GetFolderPath('Startup') }
    }

    foreach ($loc in $shortcutLocations) {
        if (-not (Test-Path $loc.Dir)) {
            New-Item -ItemType Directory -Path $loc.Dir -Force | Out-Null
        }
        $lnkPath = Join-Path $loc.Dir $lnkName
        $isNew = -not (Test-Path $lnkPath)
        $lnk = $shell.CreateShortcut($lnkPath)
        
        $changed = $false
        if ($lnk.TargetPath -ne $expected) {
            Write-Host "$($loc.Name)快捷方式指向 $($lnk.TargetPath)，改为 $expected" -ForegroundColor Yellow
            $lnk.TargetPath = $expected
            $changed = $true
        }
        if ($lnk.WorkingDirectory -ne $PSScriptRoot) {
            $lnk.WorkingDirectory = $PSScriptRoot
            $changed = $true
        }
        if ($lnk.IconLocation -ne "$expected,0") {
            $lnk.IconLocation = "$expected,0"
            $changed = $true
        }
        if ($lnk.Description -ne 'fast词典') {
            $lnk.Description = 'fast词典'
            $changed = $true
        }

        if ($isNew -or $changed) {
            $lnk.Save()
            if ($isNew) {
                Write-Host "已创建$($loc.Name)快捷方式: $lnkPath -> $expected" -ForegroundColor Green
            } else {
                Write-Host "已纠正$($loc.Name)快捷方式: $lnkPath -> $expected" -ForegroundColor Green
            }
        } else {
            Write-Host "$($loc.Name)快捷方式正常: $lnkPath -> $expected" -ForegroundColor Green
        }
    }
    [System.Runtime.InteropServices.Marshal]::ReleaseComObject($shell) | Out-Null
}

# 生产分发包出包逻辑
if ($Package) {
    if ($Dev) {
        Write-Host "无法为开发版生成生产分发包，请去除 -Dev 参数" -ForegroundColor Red
        exit 1
    }
    
    $parentDir = Split-Path $PSScriptRoot -Parent
    $distDir = Join-Path $parentDir "dist"
    if (-not (Test-Path $distDir)) {
        New-Item -ItemType Directory -Path $distDir -Force | Out-Null
    }
    
    # 确定版本与发布目录名
    $pkgDate = Get-Date -Format 'yyyy.MM.dd'
    $basePkgName = "fastDictionary-v$pkgDate-win64"
    $pkgName = $basePkgName
    if (-not $Overwrite) {
        # 如果已有同名包且未指定 -Overwrite，则顺延 -r2, -r3...
        $rev = 2
        while ((Test-Path (Join-Path $distDir $pkgName)) -or (Test-Path (Join-Path $distDir "$pkgName.zip"))) {
            $pkgName = "fastDictionary-v$pkgDate-r$rev-win64"
            $rev++
        }
    } else {
        # 指定了 -Overwrite，清理既有同名目录与 zip 以便覆盖刷新
        $existingDir = Join-Path $distDir $pkgName
        if (Test-Path $existingDir) { Remove-Item -Recurse -Force $existingDir }
        $existingZip = Join-Path $distDir "$pkgName.zip"
        if (Test-Path $existingZip) { Remove-Item -Force $existingZip }
        $existingSha = Join-Path $distDir "$pkgName.zip.sha256"
        if (Test-Path $existingSha) { Remove-Item -Force $existingSha }
    }
    
    $targetPkgDir = Join-Path $distDir $pkgName
    Write-Host "正在生成生产分发包: $targetPkgDir" -ForegroundColor Cyan
    New-Item -ItemType Directory -Path $targetPkgDir -Force | Out-Null
    
    # 复制可执行文件与运行时依赖
    Copy-Item (Join-Path $PSScriptRoot $out) (Join-Path $targetPkgDir $out) -Force
    $exeHash = (Get-FileHash (Join-Path $targetPkgDir $out) -Algorithm SHA256).Hash
    "$exeHash  $out" | Set-Content (Join-Path $targetPkgDir "$out.sha256") -Encoding ASCII
    
    if (Test-Path (Join-Path $PSScriptRoot 'onnxruntime.dll')) {
        Copy-Item (Join-Path $PSScriptRoot 'onnxruntime.dll') (Join-Path $targetPkgDir 'onnxruntime.dll') -Force
    }
    
    if (Test-Path (Join-Path $PSScriptRoot 'indra-models')) {
        Copy-Item (Join-Path $PSScriptRoot 'indra-models') (Join-Path $targetPkgDir 'indra-models') -Recurse -Force
    }
    
    if (Test-Path (Join-Path $PSScriptRoot 'resources')) {
        Copy-Item (Join-Path $PSScriptRoot 'resources') (Join-Path $targetPkgDir 'resources') -Recurse -Force
    }
    
    if (Test-Path (Join-Path $PSScriptRoot 'docs')) {
        $docsDst = Join-Path $targetPkgDir 'docs'
        New-Item -ItemType Directory -Path $docsDst -Force | Out-Null
        # 只带文本文档：截图、基准数据与归档占了 docs 的绝大部分体积，
        # 对拿到 zip 的使用者没有用。
        Get-ChildItem (Join-Path $PSScriptRoot 'docs') -File | Copy-Item -Destination $docsDst -Force
    }
    
    if (Test-Path (Join-Path $PSScriptRoot 'README.md')) {
        Copy-Item (Join-Path $PSScriptRoot 'README.md') (Join-Path $targetPkgDir 'README.md') -Force
    }

    if (Test-Path (Join-Path $PSScriptRoot 'CHANGELOG.md')) {
        Copy-Item (Join-Path $PSScriptRoot 'CHANGELOG.md') (Join-Path $targetPkgDir 'CHANGELOG.md') -Force
    }
    
    if (Test-Path (Join-Path $PSScriptRoot 'LICENSE')) {
        Copy-Item (Join-Path $PSScriptRoot 'LICENSE') (Join-Path $targetPkgDir 'LICENSE') -Force
    }

    if (Test-Path (Join-Path $PSScriptRoot 'NOTICE')) {
        Copy-Item (Join-Path $PSScriptRoot 'NOTICE') (Join-Path $targetPkgDir 'NOTICE') -Force
    }
    
    # 写入 BUILD_INFO.txt
    $goVer = (& go version).Trim()
    $mygoVer = "v0.8.3"
    $goModPath = Join-Path $PSScriptRoot "go.mod"
    if (Test-Path $goModPath) {
        $match = Select-String -Path $goModPath -Pattern 'github\.com/egoist/mygo\s+(v[^\s]+)'
        if ($match -and $match.Matches.Groups.Count -gt 1) {
            $mygoVer = $match.Matches.Groups[1].Value
        }
    }
    $buildInfo = @"
fastDictionary portable build
=============================
Build:     $stamp
Version:   $releaseVersion
Mygo:      $mygoVer
Go:        $goVer
Platform:  Windows x64
Package:   $pkgName
"@
    if ($deps) {
        $buildInfo += @"

OCR runtime:
  onnxruntime v$($deps.onnxruntime.version)  sha256 $($deps.onnxruntime.dllSha256.Substring(0, 12))…
  Indra models revision $($deps.models.revision.Substring(0, 12))…  ($($deps.models.license))
"@
    }
    $buildInfo += @"

Contents:
  fastDictionary.exe  main application (Windows GUI subsystem)
  onnxruntime.dll     OCR runtime
  indra-models\       Indra OCR models (5 files)
  resources\          icons, spell dictionary and OCR helper scripts
  docs\               documentation
  README.md           usage and build notes
  CHANGELOG.md        version history
  NOTICE              third-party licenses
  LICENSE             license

User data: %APPDATA%\fastDictionary\
"@
    Set-Content -Path (Join-Path $targetPkgDir 'BUILD_INFO.txt') -Value $buildInfo -Encoding UTF8
    
    # 压缩为 zip
    $zipPath = Join-Path $distDir "$pkgName.zip"
    if (Test-Path $zipPath) { Remove-Item -Force $zipPath }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    [System.IO.Compression.ZipFile]::CreateFromDirectory($targetPkgDir, $zipPath, [System.IO.Compression.CompressionLevel]::Optimal, $false)
    
    # 计算 zip SHA256 校验和
    $zipHash = (Get-FileHash $zipPath -Algorithm SHA256).Hash
    "$zipHash  $pkgName.zip" | Set-Content "$zipPath.sha256" -Encoding ASCII
    
    Write-Host "生产分发包创建完成:" -ForegroundColor Green
    Write-Host "  目录: $targetPkgDir" -ForegroundColor Green
    Write-Host "  压缩包: $zipPath ($((Get-Item $zipPath).Length) bytes)" -ForegroundColor Green
    Write-Host "  校验和 (SHA256): $zipHash" -ForegroundColor Green
}

# 刚才被停掉的实例，构建完自动拉起来：否则用户点快捷方式只是重新启动一次
# 同一个旧进程留下的空白，托盘里什么也没有。
if ($Run -or $wasRunning) {
    Start-Process -FilePath (Join-Path $PSScriptRoot $out) -WorkingDirectory $PSScriptRoot
    Write-Host "started $out" -ForegroundColor Green
}
