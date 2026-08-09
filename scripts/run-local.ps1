param(
    [string]$EnvFile = "",
    [string]$UpstreamURL = "http://8.138.123.59:2345",
    [string]$GoCommand = "go"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
if (-not $EnvFile) {
    $EnvFile = Join-Path $root ".env"
}

# 1. 可选加载 Git 忽略的本地环境文件，不执行其中任何 PowerShell 表达式。
$originalValues = @{}
$loadedKeys = [System.Collections.Generic.List[string]]::new()
if (Test-Path -LiteralPath $EnvFile -PathType Leaf) {
    foreach ($line in Get-Content -LiteralPath $EnvFile -Encoding UTF8) {
        if ($line -notmatch '^\s*([A-Za-z_][A-Za-z0-9_]*)=(.*)$') {
            continue
        }
        $key = $Matches[1]
        $value = $Matches[2].Trim()
        if (($value.StartsWith('"') -and $value.EndsWith('"')) -or ($value.StartsWith("'") -and $value.EndsWith("'"))) {
            $value = $value.Substring(1, $value.Length - 2)
        }
        $originalValues[$key] = [Environment]::GetEnvironmentVariable($key, "Process")
        [Environment]::SetEnvironmentVariable($key, $value, "Process")
        $loadedKeys.Add($key)
    }
}

try {
    # 2. 强制本地 2345 使用线上 API，关闭调度和真实交易，不直连生产 PostgreSQL。
    foreach ($key in @(
        "AOWUGONG_ENV",
        "AOWUGONG_HTTP_ADDRESS",
        "AOWUGONG_DEV_UPSTREAM_URL",
        "AOWUGONG_SCHEDULER_ENABLED",
        "FINANCE_ENABLE_REAL_TRADE"
    )) {
        if (-not $originalValues.ContainsKey($key)) {
            $originalValues[$key] = [Environment]::GetEnvironmentVariable($key, "Process")
            $loadedKeys.Add($key)
        }
    }
    $env:AOWUGONG_ENV = "development"
    $env:AOWUGONG_HTTP_ADDRESS = "127.0.0.1:2345"
    $env:AOWUGONG_DEV_UPSTREAM_URL = $UpstreamURL.TrimEnd("/")
    $env:AOWUGONG_SCHEDULER_ENABLED = "false"
    $env:FINANCE_ENABLE_REAL_TRADE = "false"
    if ($env:AOWUGONG_DEV_UPSTREAM_URL -notmatch '^https?://') {
        throw "线上 API 地址必须以 http:// 或 https:// 开头。"
    }
    if (-not (Test-Path -LiteralPath (Join-Path $root "web/dist/index.html") -PathType Leaf)) {
        throw "缺少 web/dist/index.html，请先在 web 目录执行 npm run build。"
    }

    # 3. 前台启动本地 Go；页面来自当前代码，全部 API 数据来自线上服务。
    Push-Location $root
    try {
        & $GoCommand run -buildvcs=false ./cmd/aowugong
        if ($LASTEXITCODE -ne 0) {
            throw "本地 Go 进程退出，状态码 $LASTEXITCODE。"
        }
    }
    finally {
        Pop-Location
    }
}
finally {
    # 4. 恢复脚本运行前的进程环境。
    foreach ($key in $loadedKeys) {
        [Environment]::SetEnvironmentVariable($key, $originalValues[$key], "Process")
    }
}
