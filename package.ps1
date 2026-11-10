# package.ps1 — 生产构建并出包
param(
    [switch]$Run,
    [switch]$NoStop,
    [switch]$Overwrite
)

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

& (Join-Path $PSScriptRoot "build.ps1") -Package -Run:$Run -NoStop:$NoStop -Overwrite:$Overwrite
