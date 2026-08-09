#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=server-release-lib.sh
. "$SCRIPT_DIR/server-release-lib.sh"

APP_ROOT="${APP_ROOT:-/opt/aowugong-go}"
RUN_USER="${RUN_USER:-aowugong}"
RUN_GROUP="${RUN_GROUP:-aowugong}"
SERVICE_NAME="aowugong-go"

require_root
for command_name in curl systemctl install sed; do require_command "$command_name"; done
id "$RUN_USER" >/dev/null 2>&1 || die "运行用户不存在: $RUN_USER"
RUN_GROUP="$(id -gn "$RUN_USER")"
[ -L "$APP_ROOT/current" ] || die "缺少 current 发布产物"
[ -L "$APP_ROOT/previous" ] || die "没有可回滚的 previous 发布产物"

current_target="$(readlink -f "$APP_ROOT/current")"
previous_target="$(readlink -f "$APP_ROOT/previous")"
[ -d "$previous_target/migrations/postgres" ] || die "上一版本不是 PostgreSQL 发布产物"
address="$(read_env_value "$APP_ROOT/shared/.env" AOWUGONG_HTTP_ADDRESS)"
port="${address##*:}"
temporary_service="$(mktemp)"
changed=1

# render_service 按指定发布目录安装正式 systemd unit。
# 输入：release_path 是发布目录。
# 输出：无。
# 副作用：覆盖 aowugong-go systemd unit。
render_service() {
  local release_path="$1"
  sed -e "s|@APP_ROOT@|$APP_ROOT|g" -e "s|@RUN_USER@|$RUN_USER|g" -e "s|@RUN_GROUP@|$RUN_GROUP|g" \
    "$release_path/init/systemd/aowugong-go.service" > "$temporary_service"
  install -m 0644 "$temporary_service" "/etc/systemd/system/${SERVICE_NAME}.service"
}

# restore_current 在回滚失败时恢复操作前版本。
# 输入：由 EXIT trap 取得退出状态。
# 输出：保留原退出状态。
# 副作用：恢复 current 链接和服务。
restore_current() {
  local exit_code=$?
  trap - EXIT
  if [ "$changed" -eq 1 ]; then
    set +e
    point_symlink "$APP_ROOT/current" "$current_target"
    render_service "$current_target"
    systemctl daemon-reload
    systemctl restart "$SERVICE_NAME" >/dev/null 2>&1 || true
    wait_for_health "http://127.0.0.1:${port}/api/v1/health" 30 || true
  fi
  rm -f "$temporary_service"
  exit "$exit_code"
}
trap restore_current EXIT

point_symlink "$APP_ROOT/current" "$previous_target"
render_service "$previous_target"
systemctl daemon-reload
systemctl restart "$SERVICE_NAME"
wait_for_health "http://127.0.0.1:${port}/api/v1/health" 30 || die "上一版本健康检查失败"
point_symlink "$APP_ROOT/previous" "$current_target"

changed=0
trap - EXIT
rm -f "$temporary_service"
printf '已回滚到 PostgreSQL 发布产物: %s\n' "$previous_target"
