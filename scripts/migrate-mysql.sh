#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=server-release-lib.sh
. "$SCRIPT_DIR/server-release-lib.sh"

APP_ROOT="${APP_ROOT:-/opt/aowugong-go}"
RUN_USER="${RUN_USER:-aowugong}"
RUN_GROUP="${RUN_GROUP:-$(id -gn "$RUN_USER" 2>/dev/null || printf aowugong)}"
SERVICE_NAME="aowugong-go"
LEGACY_ENV_FILE="${LEGACY_ENV_FILE:-/www/wwwroot/docker-file/aowugong-fastapi/.env}"
TARGET_DB="${TARGET_DB:-$APP_ROOT/shared/storage/data/aowugong.db}"
BATCH_SIZE="${BATCH_SIZE:-2000}"

require_root
for command_name in systemctl curl mv chown chmod; do
  require_command "$command_name"
done
[ -x "$APP_ROOT/current/aowugong-migrate" ] || die "找不到迁移程序: $APP_ROOT/current/aowugong-migrate"
[ -f "$LEGACY_ENV_FILE" ] || die "找不到旧项目数据库配置: $LEGACY_ENV_FILE"
[ -f "$APP_ROOT/shared/.env" ] || die "找不到 Go 生产配置: $APP_ROOT/shared/.env"
case "$TARGET_DB" in
  "$APP_ROOT"/shared/storage/data/*) ;;
  *) die "TARGET_DB 必须位于 $APP_ROOT/shared/storage/data" ;;
esac

mkdir -p "$(dirname "$TARGET_DB")" "$APP_ROOT/shared/storage/exports"
next_db="${TARGET_DB}.next"
report="$APP_ROOT/shared/storage/exports/mysql-sqlite-$(date +%Y%m%d-%H%M%S).json"
was_active=0
service_stopped=0
swap_started=0
swap_complete=0
backup_db=""

rollback_migration_swap() {
  local exit_code=$?
  trap - EXIT
  if [ "$swap_complete" -eq 0 ] && { [ "$service_stopped" -eq 1 ] || [ "$swap_started" -eq 1 ]; }; then
    set +e
    if [ "$service_stopped" -eq 1 ]; then
      systemctl stop "$SERVICE_NAME" >/dev/null 2>&1 || true
    fi
    if [ "$swap_started" -eq 1 ]; then
      rm -f "$TARGET_DB" "${TARGET_DB}-wal" "${TARGET_DB}-shm"
      if [ -n "$backup_db" ] && [ -f "$backup_db" ]; then
        mv "$backup_db" "$TARGET_DB"
      fi
    fi
    if [ "$was_active" -eq 1 ]; then
      systemctl start "$SERVICE_NAME" >/dev/null 2>&1 || true
    fi
    printf 'SQLite 切换失败，已恢复原数据库和服务状态。\n' >&2
  fi
  exit "$exit_code"
}
trap rollback_migration_swap EXIT

rm -f "$next_db" "${next_db}-wal" "${next_db}-shm"

"$APP_ROOT/current/aowugong-migrate" \
  -mode migrate \
  -env-file "$LEGACY_ENV_FILE" \
  -sqlite "$next_db" \
  -migrations "$APP_ROOT/current/migrations" \
  -report "$report" \
  -batch-size "$BATCH_SIZE" >/dev/null

[ ! -e "${next_db}-wal" ] && [ ! -e "${next_db}-shm" ] || die "新 SQLite 仍存在 WAL/SHM，拒绝切换"
if systemctl is-active --quiet "$SERVICE_NAME"; then
  was_active=1
  systemctl stop "$SERVICE_NAME"
  service_stopped=1
fi
[ ! -e "${TARGET_DB}-wal" ] && [ ! -e "${TARGET_DB}-shm" ] || die "旧 SQLite 停止服务后仍存在 WAL/SHM，拒绝删除未 checkpoint 数据"
if [ -f "$TARGET_DB" ]; then
  backup_db="${TARGET_DB}.before-$(date +%Y%m%d-%H%M%S)"
  mv "$TARGET_DB" "$backup_db"
fi
swap_started=1
mv "$next_db" "$TARGET_DB"
chown "$RUN_USER:$RUN_GROUP" "$TARGET_DB" "$report"
chmod 0640 "$TARGET_DB" "$report"

if [ "$was_active" -eq 1 ]; then
  systemctl start "$SERVICE_NAME"
  address="$(read_env_value "$APP_ROOT/shared/.env" AOWUGONG_HTTP_ADDRESS)"
  port="${address##*:}"
  wait_for_health "http://127.0.0.1:${port}/api/v1/health" 30 || die "迁移后 Go 健康检查失败"
  service_stopped=0
fi

swap_complete=1
trap - EXIT
printf '迁移及逐表核验完成: database=%s report=%s previous=%s\n' "$TARGET_DB" "$report" "${backup_db:-none}"
