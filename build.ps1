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
#   .\build.ps1 -Run       构建后启动程序
#   .\build.ps1 -NoStop    不停掉正在运行的实例（调试用；此时 exe 可能被占用）

param(
    [switch]$Dev,
    [switch]$Run,
    [switch]$NoStop
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
if ($ldflags) {
    $ldflags = "$ldflags -X main.buildStamp=$stamp"
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
        Start-Sleep -Milliseconds 500
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

# 对账并纠正：快捷方式指向的必须就是刚构建的文件。
#
# 以前这里只报警，于是「构建成功、快捷方式打开旧客户端」照样发生 —— 报警
# 没人看，用户看到的就是旧界面。现在直接写回正确目标：快捷方式是这个项目
# 唯一的启动入口，让它指向刚构建的 exe 属于构建的一部分，而不是用户的活。
if (-not $Dev) {
    $expected = Join-Path $PSScriptRoot $out
    $shell = New-Object -ComObject WScript.Shell
    $lnkName = 'fast词典.lnk'
    $lnkPath = Join-Path ([Environment]::GetFolderPath('Desktop')) $lnkName
    if (Test-Path $lnkPath) {
        $lnk = $shell.CreateShortcut($lnkPath)
        if ($lnk.TargetPath -ne $expected) {
            Write-Host "快捷方式指向 $($lnk.TargetPath)，改为 $expected" -ForegroundColor Yellow
            $lnk.TargetPath = $expected
            $lnk.WorkingDirectory = $PSScriptRoot
            $lnk.Save()
        }
        Write-Host "快捷方式 -> $((New-Object -ComObject WScript.Shell).CreateShortcut($lnkPath).TargetPath)" -ForegroundColor Green
    } else {
        # 快捷方式被误删时重建，省得用户自己找 exe。
        $lnk = $shell.CreateShortcut($lnkPath)
        $lnk.TargetPath = $expected
        $lnk.WorkingDirectory = $PSScriptRoot
        $lnk.IconLocation = "$expected,0"
        $lnk.Description = 'fast词典'
        $lnk.Save()
        Write-Host "已重建快捷方式 $lnkName -> $expected" -ForegroundColor Green
    }
}

# 刚才被停掉的实例，构建完自动拉起来：否则用户点快捷方式只是重新启动一次
# 同一个旧进程留下的空白，托盘里什么也没有。
if ($Run -or $wasRunning) {
    Start-Process -FilePath (Join-Path $PSScriptRoot $out) -WorkingDirectory $PSScriptRoot
    Write-Host "started $out" -ForegroundColor Green
}
