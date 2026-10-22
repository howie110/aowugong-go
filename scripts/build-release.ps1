param(
    [string]$Version = "dev-$(Get-Date -Format 'yyyyMMdd-HHmmss')"
)
$ErrorActionPreference = "Stop"
# 保留已有调用方式；验证、文件清单与打包只在 Node 入口维护。
node (Join-Path $PSScriptRoot "build-release.mjs") $Version
if ($LASTEXITCODE -ne 0) { throw "Release build failed ($LASTEXITCODE)" }
