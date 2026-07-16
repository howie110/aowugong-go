param(
    [string]$SshHost = "8.138.123.59",
    [int]$SshPort = 22,
    [string]$SshUser = "root",
    [int]$LocalPort = 13306,
    [string]$IdentityFile = ""
)

$ErrorActionPreference = "Stop"

# 1. 校验本地端口未被占用，并定位系统 OpenSSH 客户端。
if (Get-NetTCPConnection -State Listen -LocalPort $LocalPort -ErrorAction SilentlyContinue) {
    throw "本地端口 $LocalPort 已被占用；请确认是否已有 MySQL 隧道。"
}
$ssh = Get-Command ssh.exe -ErrorAction Stop

# 2. 只把本机回环端口转发到服务器回环 MySQL，不开放局域网监听。
$arguments = @(
    "-N",
    "-T",
    "-p", $SshPort,
    "-o", "ExitOnForwardFailure=yes",
    "-o", "ServerAliveInterval=30",
    "-o", "ServerAliveCountMax=3",
    "-L", "127.0.0.1:${LocalPort}:127.0.0.1:3306"
)
if ($IdentityFile) {
    $arguments += @("-i", (Resolve-Path -LiteralPath $IdentityFile).Path)
}
$arguments += "${SshUser}@${SshHost}"

# 3. 在前台保持隧道，用户退出进程后连接自动关闭；密码由 ssh 交互读取且不落盘。
Write-Host "MySQL 隧道将监听 127.0.0.1:$LocalPort；按 Ctrl+C 关闭。"
& $ssh.Source @arguments
if ($LASTEXITCODE -ne 0) {
    throw "SSH MySQL 隧道退出，状态码 $LASTEXITCODE。"
}
