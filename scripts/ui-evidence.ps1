# ui-evidence.ps1 — 原生界面证据采集
#
# 为什么需要这个脚本：视觉返工里最贵的一次不是改错颜色，而是“截到的图不是
# 这一版”。旧进程、另一个 exe 名、另一份用户配置、另一套主题，都能让截图
# 看起来通过，却证明的是另一个工件。这里把 commit、exe 哈希、fixture 配置、
# 主题/强调色、截图和控件树绑在同一个目录里，身份对不上就不算证据。
#
#   .\scripts\ui-evidence.ps1 -View grammar -Theme light -Accent sky
#   .\scripts\ui-evidence.ps1 -View home -Theme dark -Accent mint
#   .\scripts\ui-evidence.ps1 -View settings -Theme light -Accent sky -Page general
#   .\scripts\ui-evidence.ps1 -View writing -Theme light -Accent coral -Keep
#   .\scripts\ui-evidence.ps1 -View chip    -Theme light -Accent sky
#
# 输出目录默认 build/ui-evidence/<时间>-<视图>-<主题>-<强调色>/，包含：
#   manifest.json      commit / 工作区状态 / exe SHA256 / 配置 / 主题 / 窗口 / 时间
#   screenshot.png     mygo-automation 原生窗口截图
#   control-tree.txt   snapshot -i --full 控件树
#   windows.txt        窗口列表与 visible/hidden 状态
#
# 使用独立 exe 名 fastDictionary-uiverify.exe，数据目录因此是
# %APPDATA%\fastDictionary-uiverify，不读也不改用户的 %APPDATA%\fastDictionary。
# 每次运行重建 fixture；结束时只停止自己启动的 PID。

param(
    [ValidateSet("home", "settings", "grammar", "writing", "chip")]
    [string]$View = "home",

    [ValidateSet("light", "dark")]
    [string]$Theme = "light",

    [ValidateSet("sky", "mint", "violet", "coral")]
    [string]$Accent = "sky",

    [ValidateSet("home", "general", "hotkey", "selection", "translate", "dict", "ai", "actions", "vocabulary", "about")]
    [string]$Page = "general",

    [string]$OutDir = "",

    # 保留 fixture 配置与日志，便于复现。
    [switch]$Keep,

    # 复用已构建的 e2e exe；默认每次都重建，避免跑的是旧二进制。
    [switch]$NoBuild
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$automation = Get-Command mygo-automation -ErrorAction SilentlyContinue
if (-not $automation) {
    throw "找不到 mygo-automation；先 go install github.com/egoist/mygo/cmd/mygo-automation@v0.9.0 或把 %GOPATH%\bin 加入 PATH"
}

$binDir = Join-Path $root "build\ui-evidence\bin"
$exe = Join-Path $binDir "fastDictionary-uiverify.exe"
if (-not $NoBuild) {
    New-Item -ItemType Directory -Force $binDir | Out-Null
    Write-Host "go build -tags e2e..." -ForegroundColor Cyan
    & go build -tags e2e -ldflags "-H=windowsgui" -o $exe .
    if ($LASTEXITCODE -ne 0) { throw "go build -tags e2e 失败" }
}
if (-not (Test-Path $exe)) {
    throw "缺少 $exe；去掉 -NoBuild 先构建"
}

$stamp = Get-Date -Format "yyyyMMdd-HHmmss"
if (-not $OutDir) {
    $OutDir = Join-Path $root "build\ui-evidence\$stamp-$View-$Theme-$Accent"
}
New-Item -ItemType Directory -Force $OutDir | Out-Null
$OutDir = (Resolve-Path $OutDir).Path

$dataDir = Join-Path $env:APPDATA "fastDictionary-uiverify"
$cfgPath = Join-Path $dataDir "config.json"
$socket = Join-Path ([System.IO.Path]::GetTempPath()) ("fd-uiverify-{0}.sock" -f $PID)
$proc = $null
$opened = $false

function Invoke-Automation {
    param([string[]]$AutomationArgs)
    $out = & mygo-automation @AutomationArgs 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "mygo-automation $($AutomationArgs -join ' ') 失败：$($out -join ' ')"
    }
    return @($out)
}

try {
    # 1. 隔离 fixture：每次重建，保证主题/强调色就是命令行说的那套。
    if (Test-Path $dataDir) { Remove-Item -Recurse -Force $dataDir }
    New-Item -ItemType Directory -Force $dataDir | Out-Null
    $fixture = [ordered]@{
        hotkey_selection  = "Ctrl+Alt+F9"
        hotkey_input      = "Ctrl+Alt+F10"
        hotkey_screenshot = "Ctrl+Alt+F11"
        selection_trigger = "hotkey"
        theme_mode        = $Theme
        accent_theme      = $Accent
        onboarded         = $true
        close_on_blur     = $false
        always_on_top     = $false
    }
    $fixture | ConvertTo-Json | Set-Content -LiteralPath $cfgPath -Encoding utf8

    # 2. 启动独立实例。--elevated-takeover 跳过单实例锁；exe 名不同，正常也
    #    不会和用户正在运行的 fastDictionary.exe 撞锁。
    $appArgs = @("--elevated-takeover")
    switch ($View) {
        "home" { $appArgs += @("--page", "home") }
        "settings" { $appArgs += @("--settings", "--page", $Page) }
        "grammar" { $appArgs += @("--demo-action", "grammar") }
        "writing" { $appArgs += @("--demo-action", "writing") }
        "chip" { $appArgs += @("--chip") }
    }

    if (Test-Path $socket) { Remove-Item -Force $socket }
    $env:MYGO_AUTOMATION = $socket
    Write-Host "启动 $View ($Theme/$Accent)..." -ForegroundColor Cyan
    $proc = Start-Process -FilePath $exe -ArgumentList $appArgs -PassThru

    $deadline = (Get-Date).AddSeconds(30)
    while (-not (Test-Path $socket)) {
        if ($proc.HasExited) {
            throw "验证实例提前退出（exit=$($proc.ExitCode)）；日志：$(Join-Path $dataDir 'fastDictionary.log')"
        }
        if ((Get-Date) -gt $deadline) {
            throw "30 秒内没有等到自动化 socket：$socket"
        }
        Start-Sleep -Milliseconds 200
    }

    $openOut = Invoke-Automation @("open", "--socket", $socket)
    $opened = $true
    # 划词条在启动约 600ms 后才出现；其余视图的主窗口在 socket 就绪时已经可见。
    if ($View -eq "chip") {
        $deadline = (Get-Date).AddSeconds(10)
        do {
            Start-Sleep -Milliseconds 250
            $windows = Invoke-Automation @("windows")
            $chipVisible = @($windows | Where-Object { $_ -match '^w\d+' -and $_ -notmatch '\bhidden\b' }).Count -gt 0
        } while (-not $chipVisible -and (Get-Date) -lt $deadline)
    }
    $windows = Invoke-Automation @("windows")
    $windows | Set-Content -LiteralPath (Join-Path $OutDir "windows.txt") -Encoding utf8

    $targetLine = $windows | Where-Object { $_ -match '^w\d+' -and $_ -notmatch '\bhidden\b' } | Select-Object -First 1
    if (-not $targetLine) {
        $targetLine = $windows | Where-Object { $_ -match '^w\d+' } | Select-Object -First 1
    }
    if (-not $targetLine) {
        throw "自动化没有列出任何窗口：$($windows -join ' | ')"
    }
    $winId = ($targetLine -split '\s+')[0]
    $null = Invoke-Automation @("window", $winId)

    # 3. 原生截图 + 控件树。隐藏的弹窗窗口也能截；普通桌面截图不行。
    $shotPath = Join-Path $OutDir "screenshot.png"
    $null = Invoke-Automation @("screenshot", $shotPath)
    if (-not (Test-Path $shotPath)) { throw "截图没有生成：$shotPath" }

    $tree = Invoke-Automation @("snapshot", "-i", "--full")
    $tree | Set-Content -LiteralPath (Join-Path $OutDir "control-tree.txt") -Encoding utf8

    # 4. 读回配置：应用可能重写文件；以磁盘上的值为准，而不是命令行参数。
    $cfgLoaded = Get-Content -Raw -LiteralPath $cfgPath | ConvertFrom-Json
    if ($cfgLoaded.theme_mode -ne $Theme -or $cfgLoaded.accent_theme -ne $Accent) {
        throw "配置身份不符：要求 $Theme/$Accent，实际 $($cfgLoaded.theme_mode)/$($cfgLoaded.accent_theme)"
    }
    # 证据目录可能被分享；fixture 里没有密钥，但不要依赖这个前提。
    foreach ($secret in @("ai_key", "ai_key_enc", "ai_key_enc_by_provider")) {
        $prop = $cfgLoaded.PSObject.Properties[$secret]
        if ($prop) { $prop.Value = $null }
    }

    $commit = (& git rev-parse HEAD).Trim()
    $dirty = [bool]((& git status --porcelain) -join "")
    $goVersion = (& go version).Trim()
    $pageValue = $null
    if ($View -eq "settings") { $pageValue = $Page }
    $manifest = [ordered]@{
        view               = $View
        page               = $pageValue
        captured_at        = (Get-Date).ToString("o")
        git_commit         = $commit
        git_dirty          = $dirty
        exe                = $exe
        exe_sha256         = (Get-FileHash -Algorithm SHA256 $exe).Hash
        go_version         = $goVersion
        config_path        = $cfgPath
        config             = $cfgLoaded
        theme              = $cfgLoaded.theme_mode
        accent             = $cfgLoaded.accent_theme
        window             = $winId
        windows            = @($windows)
        screenshot         = "screenshot.png"
        control_tree       = "control-tree.txt"
        automation_socket  = $socket
    }
    $manifestPath = Join-Path $OutDir "manifest.json"
    $manifest | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $manifestPath -Encoding utf8

    Write-Host ""
    Write-Host "证据目录：$OutDir" -ForegroundColor Green
    Write-Host "  manifest：$manifestPath"
    Write-Host "  commit：  $commit$(if ($dirty) { '（工作区有未提交改动）' } else { '' })"
    Write-Host "  exe：     $($manifest.exe_sha256.Substring(0, 16))…"
    Write-Host "  主题：    $($cfgLoaded.theme_mode) / $($cfgLoaded.accent_theme)"
    Write-Host "  窗口：    $winId"
}
catch {
    if (Test-Path $dataDir) {
        $logPath = Join-Path $dataDir "fastDictionary.log"
        if (Test-Path $logPath) {
            Copy-Item -LiteralPath $logPath -Destination (Join-Path $OutDir "app.log") -ErrorAction SilentlyContinue
        }
    }
    throw
}
finally {
    if ($opened) { & mygo-automation close 2>$null | Out-Null }
    if ($proc -and -not $proc.HasExited) {
        Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        try { $proc.WaitForExit(5000) | Out-Null } catch {}
    }
    if (Test-Path $socket) { Remove-Item -Force $socket -ErrorAction SilentlyContinue }
    if (-not $Keep -and (Test-Path $dataDir)) {
        Remove-Item -Recurse -Force $dataDir -ErrorAction SilentlyContinue
    }
}
