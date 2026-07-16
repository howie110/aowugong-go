#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=server-release-lib.sh
. "$SCRIPT_DIR/server-release-lib.sh"

APP_ROOT="${APP_ROOT:-/opt/aowugong-go}"
GO_SERVICE="aowugong-go"
LEGACY_SERVICE="${LEGACY_SERVICE:-aowugong-fastapi}"
LEGACY_PROJECT="${LEGACY_PROJECT:-/www/wwwroot/docker-file/aowugong-fastapi}"
LEGACY_RUN_USER="${LEGACY_RUN_USER:-$(stat -c '%U' "$LEGACY_PROJECT" 2>/dev/null || printf root)}"
cutover_id="$(date +%Y%m%d-%H%M%S)"
crontab_backup="$APP_ROOT/shared/storage/backup/fastapi-crontab-$cutover_id"
legacy_state="$crontab_backup.service-state"
cutover_complete=0

require_root
for command_name in curl systemctl crontab awk stat; do
  require_command "$command_name"
done
[ -f "$APP_ROOT/shared/.env" ] || die "缺少生产环境文件: $APP_ROOT/shared/.env"
wait_for_health "http://127.0.0.1:2346/api/v1/health" 5 || die "2346 上的 Go 并行服务未通过健康检查"

rollback_cutover() {
  local exit_code=$?
  trap - EXIT
  if [ "$cutover_complete" -eq 0 ]; then
    set +e
    set_env_value "$APP_ROOT/shared/.env" AOWUGONG_HTTP_ADDRESS "0.0.0.0:2346"
    set_env_value "$APP_ROOT/shared/.env" AOWUGONG_SCHEDULER_ENABLED false
    systemctl stop "$GO_SERVICE" >/dev/null 2>&1 || true
    restore_crontab "$LEGACY_RUN_USER" "$crontab_backup" "$LEGACY_PROJECT"
    restore_service_state "$LEGACY_SERVICE" "$legacy_state" >/dev/null 2>&1 || true
    printf '切换失败，已恢复 FastAPI 服务和旧 crontab。\n' >&2
  fi
  exit "$exit_code"
}
trap rollback_cutover EXIT

mkdir -p "$(dirname "$crontab_backup")"
save_service_state "$LEGACY_SERVICE" "$legacy_state"
remove_legacy_crontab "$LEGACY_RUN_USER" "$LEGACY_PROJECT" "$crontab_backup"
systemctl stop "$LEGACY_SERVICE"
if systemctl is-enabled --quiet "$LEGACY_SERVICE"; then
  systemctl disable "$LEGACY_SERVICE" >/dev/null
fi
if systemctl is-enabled --quiet "$LEGACY_SERVICE"; then
  die "旧 FastAPI 服务仍为 enabled，拒绝继续切换"
fi

systemctl stop "$GO_SERVICE" || true
set_env_value "$APP_ROOT/shared/.env" AOWUGONG_HTTP_ADDRESS "0.0.0.0:2345"
set_env_value "$APP_ROOT/shared/.env" AOWUGONG_SCHEDULER_ENABLED true
systemctl start "$GO_SERVICE"
wait_for_health "http://127.0.0.1:2345/api/v1/health" 30 || die "Go 切换到 2345 后健康检查失败"

cutover_complete=1
trap - EXIT
printf '生产切换完成：Go 已接管 2345，内嵌调度已启用，FastAPI 与 Go 共用的 MySQL 数据保持原位。\n'
