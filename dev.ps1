# dev.ps1 — 开发循环脚本
#
# 为什么需要它：每次改 UI 都要「构建 → 启动 → 截图 → 放大看清细节」四步，
# 其中放大是我手动用 PowerShell 拼的，最慢也最容易出错。这个脚本把这四步
# 固化成一条命令，改完代码直接看结果。
#
# 它**不产出正式版**：正式版走 build.ps1，这里只写 build\ 下的临时 exe 和
# 截图，不会动 fastDictionary.exe，也不会停正在运行的正式版。
#
#   .\dev.ps1 grammar              长难句卡片（带示例句子）
#   .\dev.ps1 dict                 词典卡片
#   .\dev.ps1 writing              写作改进卡片
#   .\dev.ps1 chip                 划词悬浮条
#   .\dev.ps1 chip-tip             悬浮条的悬停气泡
#   .\dev.ps1 <任意场景> -Zoom 2   同时输出 2 倍放大图
#   .\dev.ps1 <任意场景> -Watch    改代码后自动重跑（Ctrl+C 退出）
#
# 截图落在 build\shot-<场景>.png，放大图在 build\shot-<场景>-zoom.png。

param(
    [Parameter(Position = 0)]
    [string]$Scene = "grammar",
    [int]$Zoom = 0,
    [switch]$Watch,
    [int]$Delay = 3000,
    # Fraction of the screenshot height to zoom into: -CropFrom 0.1 -CropTo 0.5
    # zooms the middle band. Defaults zoom the whole image.
    [double]$CropFrom = 0,
    [double]$CropTo = 1
)

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

$exe = "build\dev-preview.exe"
$outDir = "build"
New-Item -ItemType Directory -Force -Path $outDir | Out-Null

# Each scene is the command line the app understands, so adding one is one
# line here rather than a new script.
$scenes = @{
    "grammar"  = @("--demo-selection", "The report was never discussed in public.", "--demo-action", "grammar", "--shot-window", "popup")
    "dict"     = @("--settings", "--page", "vocabulary", "--shot-window", "settings")
    "writing"  = @("--demo-action", "writing", "--shot-window", "popup")
    "chip"     = @("--chip", "--shot-window", "chip")
    "chip-tip" = @("--chip-tip", "--shot-window", "chip-tip")
    "settings" = @("--settings", "--page", "translate", "--shot-window", "settings")
    "ai"       = @("--settings", "--page", "ai", "--shot-window", "settings")
    "review"   = @("--review", "--shot-window", "settings")
    "ocr"      = @("--demo-action", "grammar", "--shot-window", "popup")
}

function Invoke-Scene {
    param([string]$Name)

    if (-not $scenes.ContainsKey($Name)) {
        Write-Host "未知场景: $Name" -ForegroundColor Red
        Write-Host "可用: $($scenes.Keys -join ', ')" -ForegroundColor Yellow
        return
    }

    $shot = Join-Path $outDir "shot-$Name.png"

    Write-Host "构建..." -ForegroundColor Cyan
    & go build -o $exe .
    if ($LASTEXITCODE -ne 0) {
        Write-Host "构建失败，保留上一次的截图" -ForegroundColor Red
        return
    }

    Write-Host "截图 ($Name)..." -ForegroundColor Cyan
    $args = $scenes[$Name] + @("--shot-delay", "$Delay", "--shot", $shot)
    # The app writes its log to stderr, and PowerShell turns any stderr line
    # of a native command into a terminating error when $ErrorActionPreference
    # is Stop. Dropping both streams keeps the log out of the way without
    # making a successful run look like a failure.
    $ErrorActionPreference = "Continue"
    & ".\$exe" @args *> $null
    $ErrorActionPreference = "Stop"

    if (-not (Test-Path $shot)) {
        Write-Host "没有生成截图：$shot" -ForegroundColor Red
        return
    }

    if ($Zoom -gt 1) {
        Add-Type -AssemblyName System.Drawing
        $src = [System.Drawing.Image]::FromFile((Resolve-Path $shot))

        # A full 2x of a tall card is a megapixel image where the interesting
        # part is a few lines of text. Crop takes a fraction of the height from
        # a fraction down the image, so the zoom stays readable.
        $srcW = $src.Width
        $srcH = $src.Height
        $srcX = 0
        $srcY = [int]($srcH * $CropFrom)
        $cropH = [int]($srcH * ($CropTo - $CropFrom))
        if ($cropH -lt 1) { $cropH = $srcH; $srcY = 0 }
        if ($srcY + $cropH -gt $srcH) { $cropH = $srcH - $srcY }

        $w = [int]($srcW * $Zoom)
        $h = [int]($cropH * $Zoom)
        $big = New-Object System.Drawing.Bitmap($w, $h)
        $g = [System.Drawing.Graphics]::FromImage($big)
        $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
        $dstRect = New-Object System.Drawing.Rectangle(0, 0, $w, $h)
        $srcRect = New-Object System.Drawing.Rectangle($srcX, $srcY, $srcW, $cropH)
        $g.DrawImage($src, $dstRect, $srcRect, [System.Drawing.GraphicsUnit]::Pixel)

        $zoomPath = Join-Path (Resolve-Path $outDir).Path "shot-$Name-zoom.png"
        $big.Save($zoomPath, [System.Drawing.Imaging.ImageFormat]::Png)
        $g.Dispose(); $big.Dispose(); $src.Dispose()
        Write-Host "放大图: $zoomPath ($w x $h)" -ForegroundColor Green
    }

    Write-Host "截图: $shot" -ForegroundColor Green
}

Invoke-Scene -Name $Scene

if ($Watch) {
    Write-Host ""
    Write-Host "监视 .go 文件变化，Ctrl+C 退出。" -ForegroundColor Cyan
    $stamp = Get-Date
    while ($true) {
        Start-Sleep -Milliseconds 700
        $newest = Get-ChildItem -Filter *.go -Recurse |
            Where-Object { $_.LastWriteTime -gt $stamp } |
            Select-Object -First 1
        if ($newest) {
            $stamp = Get-Date
            Write-Host ""
            Write-Host "检测到修改: $($newest.Name)" -ForegroundColor Yellow
            Invoke-Scene -Name $Scene
        }
    }
}
