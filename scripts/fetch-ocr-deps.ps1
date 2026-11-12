# fetch-ocr-deps.ps1 — 下载并校验 OCR 运行时依赖
#
# 模型和 onnxruntime.dll 不进仓库（体积），但发布包必须带。以前 build.ps1
# 直接从构建机的 Python 安装目录复制 DLL，版本随机器变，换了台机器编译出来
# 的包可能行为不同，而且没有记录。这个脚本按 scripts\ocr-deps.json 里的固定
# 版本与 SHA256 下载，校验通过才落盘；重复执行会跳过已就绪的文件。
#
#   .\scripts\fetch-ocr-deps.ps1          缺什么下什么
#   .\scripts\fetch-ocr-deps.ps1 -Force   全部重新下载
#
# 下载源：
#   onnxruntime: GitHub 官方 release
#   Indra 模型:  HuggingFace datalab-to/indra-ocr（固定 revision）
# 模型许可证为 OpenRAIL，发布包需附 NOTICE，见仓库根目录 NOTICE。

param(
    [switch]$Force
)

$ErrorActionPreference = "Stop"
$root = Split-Path $PSScriptRoot -Parent
Set-Location $root

$manifestPath = Join-Path $PSScriptRoot 'ocr-deps.json'
if (-not (Test-Path $manifestPath)) {
    throw "缺少依赖清单: $manifestPath"
}
$manifest = Get-Content $manifestPath -Raw | ConvertFrom-Json

function Get-Sha256([string]$Path) {
    (Get-FileHash $Path -Algorithm SHA256).Hash.ToUpperInvariant()
}

function Test-PinnedFile([string]$Path, [string]$Hash) {
    if (-not (Test-Path $Path)) { return $false }
    return (Get-Sha256 $Path) -eq $Hash
}

function Save-Verified([string]$Url, [string]$Dest, [string]$Hash) {
    $tmp = "$Dest.download"
    if (Test-Path $tmp) { Remove-Item -Force $tmp }
    Write-Host "下载 $Url" -ForegroundColor Cyan
    Invoke-WebRequest -Uri $Url -OutFile $tmp -UseBasicParsing
    $got = Get-Sha256 $tmp
    if ($got -ne $Hash) {
        Remove-Item -Force $tmp
        throw "SHA256 不匹配: $Dest`n  期望 $Hash`n  实际 $got"
    }
    Move-Item -Force $tmp $Dest
}

# --- onnxruntime -----------------------------------------------------------
$ort = $manifest.onnxruntime
$dllPath = Join-Path $root 'onnxruntime.dll'
if ($Force -or -not (Test-PinnedFile $dllPath $ort.dllSha256)) {
    $tmpDir = Join-Path $env:TEMP ("fd-ort-" + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $tmpDir | Out-Null
    try {
        $zip = Join-Path $tmpDir 'onnxruntime.zip'
        Save-Verified $ort.url $zip $ort.zipSha256

        Add-Type -AssemblyName System.IO.Compression.FileSystem
        [System.IO.Compression.ZipFile]::ExtractToDirectory($zip, $tmpDir)
        $inner = Join-Path $tmpDir ($ort.dllInZip -replace '/', '\')
        if (-not (Test-Path $inner)) {
            throw "压缩包中没有 $($ort.dllInZip)"
        }
        $got = Get-Sha256 $inner
        if ($got -ne $ort.dllSha256) {
            throw "onnxruntime.dll SHA256 不匹配: 期望 $($ort.dllSha256)，实际 $got"
        }
        Copy-Item $inner $dllPath -Force
    } finally {
        Remove-Item -Recurse -Force $tmpDir -ErrorAction SilentlyContinue
    }
}
Write-Host "onnxruntime v$($ort.version) 已就绪 ($($ort.dllSha256.Substring(0, 12))…)" -ForegroundColor Green

# --- Indra OCR 模型 ---------------------------------------------------------
$modelDir = Join-Path $root 'indra-models'
if (-not (Test-Path $modelDir)) { New-Item -ItemType Directory -Path $modelDir | Out-Null }

$base = "https://huggingface.co/datalab-to/indra-ocr/resolve/$($manifest.models.revision)"
foreach ($file in $manifest.models.files) {
    $dest = Join-Path $modelDir $file.name
    if ($Force -or -not (Test-PinnedFile $dest $file.sha256)) {
        Save-Verified "$base/$($file.name)" $dest $file.sha256
        Write-Host "  $($file.name) ok" -ForegroundColor Green
    } else {
        Write-Host "  $($file.name) 已存在" -ForegroundColor DarkGray
    }
}

Write-Host ""
Write-Host "OCR 运行时依赖已就绪：onnxruntime.dll + indra-models\（5 个文件）" -ForegroundColor Green
Write-Host "模型许可证: OpenRAIL（$($manifest.models.source)），分发时随包附 NOTICE。" -ForegroundColor DarkGray
