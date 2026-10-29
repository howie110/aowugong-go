#!/usr/bin/env bash
set -Eeuo pipefail

# install-vaultwarden-alert.sh 安装 Vaultwarden 备份失败/恢复的微信告警。
# 输入：无，可通过 CONFIG_FILE 覆盖配置路径。
# 输出：安装结果和未配置提示。
# 副作用：写 /usr/local/sbin、systemd drop-in 和告警 unit；不启动备份任务。

CONFIG_FILE="${CONFIG_FILE:-/etc/aowugong-alert.env}"
STATE_DIR="${STATE_DIR:-/var/lib/aowugong-alert}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

[ "$(id -u)" -eq 0 ] || { echo "请使用 root 执行" >&2; exit 1; }

install -m 0750 "${SCRIPT_DIR}/vaultwarden-backup-notify.sh" /usr/local/sbin/vaultwarden-backup-notify
install -d -m 0700 "$STATE_DIR"
install -d -m 0755 /etc/systemd/system/vaultwarden-backup.service.d

cat >/etc/systemd/system/vaultwarden-backup-alert.service <<'EOF'
[Unit]
Description=Notify WeChat when Vaultwarden backup fails
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/vaultwarden-backup-notify failure
EOF

cat >/etc/systemd/system/vaultwarden-backup.service.d/10-alert.conf <<'EOF'
[Unit]
OnFailure=vaultwarden-backup-alert.service

[Service]
ExecStartPost=/usr/local/sbin/vaultwarden-backup-notify recovery
EOF

systemctl daemon-reload
echo "已安装 Vaultwarden 备份失败/恢复告警单元"

if [ ! -f "$CONFIG_FILE" ]; then
  cat <<EOF
未找到 ${CONFIG_FILE}。请写入并设置 0600：
ALERT_API_URL=http://127.0.0.1:12345/api/v1/notifications/wechat
ALERT_API_TOKEN=<server-alerts 项目 token>
STATE_DIR=${STATE_DIR}
EOF
fi
