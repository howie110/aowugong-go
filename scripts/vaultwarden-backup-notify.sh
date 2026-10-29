#!/usr/bin/env bash
set -Eeuo pipefail

# vaultwarden-backup-notify.sh 在 Vaultwarden 备份失败或恢复时调用微信通知 API。
# 输入：failure、recovery 或 dry-run。
# 输出：dry-run 打印请求；其余模式成功后静默。
# 副作用：读取 /etc/aowugong-alert.env，写状态文件，调用本机通知 API。

MODE="${1:-}"
CONFIG_FILE="${CONFIG_FILE:-/etc/aowugong-alert.env}"
STATE_DIR="${STATE_DIR:-/var/lib/aowugong-alert}"
MARKER="${STATE_DIR}/vaultwarden-backup.failed"

case "$MODE" in
  failure|recovery|dry-run) ;;
  *)
    echo "用法: $0 failure|recovery|dry-run" >&2
    exit 2
    ;;
esac

if [ -f "$CONFIG_FILE" ]; then
  # shellcheck disable=SC1090
  . "$CONFIG_FILE"
fi

install -d -m 0700 "$STATE_DIR"
if command -v flock >/dev/null 2>&1; then
  exec 9>"${STATE_DIR}/.lock"
  flock -n 9 || exit 0
fi

now="$(date '+%Y-%m-%d %H:%M:%S')"
host="$(hostname)"
unit="vaultwarden-backup.service"
payload_mode="$MODE"
if [ "$MODE" = "dry-run" ]; then
  payload_mode="${2:-failure}"
fi

case "$MODE" in
  failure)
    [ -f "$MARKER" ] && exit 0
    ;;
  recovery)
    [ -f "$MARKER" ] || exit 0
    ;;
esac

if [ -z "${ALERT_API_URL:-}" ] || [ -z "${ALERT_API_TOKEN:-}" ]; then
  echo "未配置 ALERT_API_URL 或 ALERT_API_TOKEN: ${CONFIG_FILE}" >&2
  [ "$MODE" = "recovery" ] && exit 0
  exit 1
fi

if [ "$payload_mode" = "failure" ]; then
  request_id="vaultwarden-backup-$(date +%Y%m%dT%H%M%S)"
  title="Vaultwarden 备份失败"
  content="主机：${host}；单元：${unit}；时间：${now}；状态：备份失败；处理：检查 /usr/local/sbin/vaultwarden-backup 和 journalctl -u ${unit}"
else
  request_id="vaultwarden-backup-recovery-$(date +%Y%m%dT%H%M%S)"
  title="Vaultwarden 备份恢复"
  content="主机：${host}；单元：${unit}；时间：${now}；状态：备份已恢复"
fi

payload="$(printf '{"request_id":"%s","title":"%s","content":"%s"}' \
  "$request_id" "$title" "$content")"

if [ "$MODE" = "dry-run" ]; then
  printf 'URL: %s\nPayload: %s\n' "$ALERT_API_URL" "$payload"
  exit 0
fi

if curl -fsS --max-time 25 -X POST "$ALERT_API_URL" \
  -H "Authorization: Bearer ${ALERT_API_TOKEN}" \
  -H 'Content-Type: application/json' \
  --data-binary "$payload" >/dev/null; then
  if [ "$MODE" = "failure" ]; then
    : >"$MARKER"
    chmod 0600 "$MARKER"
  else
    rm -f "$MARKER"
  fi
  exit 0
fi

echo "通知 API 发送失败: ${MODE}" >&2
[ "$MODE" = "recovery" ] && exit 0
exit 1
