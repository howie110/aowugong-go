param(
    [string]$JobName = "",
    [string]$EnvFile = "",
    [string]$GoCommand = "go"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
if (-not $EnvFile) {
    $EnvFile = Join-Path $root ".env"
}

# 1. 从 Git 忽略的环境文件读取键值，不执行其中任何 PowerShell 表达式。
if (-not (Test-Path -LiteralPath $EnvFile -PathType Leaf)) {
    throw "本地配置不存在: $EnvFile"
}
$originalValues = @{}
$loadedKeys = [System.Collections.Generic.List[string]]::new()
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

try {
    # 2. 强制本地只经回环隧道连接，并关闭迁移、自动调度和真实交易。
    if ($env:AOWUGONG_MYSQL_HOST -notin @("127.0.0.1", "localhost")) {
        throw "本地运行只允许通过 127.0.0.1 SSH 隧道连接 MySQL。"
    }
    if (-not $env:AOWUGONG_MYSQL_PORT) {
        throw "缺少 AOWUGONG_MYSQL_PORT；SSH 隧道建议使用 13306。"
    }
    foreach ($key in @("AOWUGONG_MYSQL_SKIP_MIGRATIONS", "AOWUGONG_SCHEDULER_ENABLED", "FINANCE_ENABLE_REAL_TRADE")) {
        if (-not $originalValues.ContainsKey($key)) {
            $originalValues[$key] = [Environment]::GetEnvironmentVariable($key, "Process")
            $loadedKeys.Add($key)
        }
    }
    $env:AOWUGONG_MYSQL_SKIP_MIGRATIONS = "true"
    $env:AOWUGONG_SCHEDULER_ENABLED = "false"
    $env:FINANCE_ENABLE_REAL_TRADE = "false"

    # 3. 在执行 Go 前主动探测隧道，避免任务误判为数据库故障。
    $client = [System.Net.Sockets.TcpClient]::new()
    try {
        $connect = $client.BeginConnect($env:AOWUGONG_MYSQL_HOST, [int]$env:AOWUGONG_MYSQL_PORT, $null, $null)
        if (-not $connect.AsyncWaitHandle.WaitOne(3000)) {
            throw "MySQL 隧道连接超时。"
        }
        $client.EndConnect($connect)
    }
    finally {
        $client.Dispose()
    }

    # 4. 空任务名启动本地 HTTP；提供任务名时走与线上相同的 Registry.Run。
    Push-Location $root
    try {
        if ($JobName) {
            & $GoCommand run ./cmd/aowugong job $JobName
        }
        else {
            & $GoCommand run ./cmd/aowugong
        }
        if ($LASTEXITCODE -ne 0) {
            throw "本地 Go 进程退出，状态码 $LASTEXITCODE。"
        }
    }
    finally {
        Pop-Location
    }
}
finally {
    # 5. 恢复调用脚本前的进程环境，避免生产凭据残留到后续命令。
    foreach ($key in $loadedKeys) {
        [Environment]::SetEnvironmentVariable($key, $originalValues[$key], "Process")
    }
}
