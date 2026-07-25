param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[a-z0-9_]+$')]
    [string]$JobName,
    [ValidatePattern('^[A-Za-z0-9.-]+$')]
    [string]$Server = "8.138.123.59",
    [ValidatePattern('^[A-Za-z_][A-Za-z0-9_-]*$')]
    [string]$User = "root",
    [ValidatePattern('^/[A-Za-z0-9._/-]+$')]
    [string]$AppRoot = "/opt/aowugong-go"
)

$ErrorActionPreference = "Stop"

# 1. 只允许固定格式任务名，并通过 SSH 使用服务器现有环境和 SQLite 文件。
$remoteCommand = "set -a && . '$AppRoot/shared/.env' && set +a && '$AppRoot/current/aowugong' job '$JobName'"
& ssh "$User@$Server" $remoteCommand
if ($LASTEXITCODE -ne 0) {
    throw "线上任务 $JobName 执行失败，状态码 $LASTEXITCODE。"
}
