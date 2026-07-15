#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=server-release-lib.sh
. "$SCRIPT_DIR/server-release-lib.sh"

APP_ROOT="${APP_ROOT:-/opt/aowugong-go}"
MODE="${1:-release}"
GO_SERVICE="aowugong-go"
LEGACY_SERVICE="${LEGACY_SERVICE:-aowugong-fastapi}"
LEGACY_PROJECT="${LEGACY_PROJECT:-/www/wwwroot/docker-file/aowugong-fastapi}"
LEGACY_RUN_USER="${LEGACY_RUN_USER:-root}"
CRONTAB_BACKUP="${CRONTAB_BACKUP:-}"

require_root
case "$MODE" in
  release)
    [ -L "$APP_ROOT/previous" ] || die "没有可回滚的 previous 发布产物"
    current_target="$(readlink -f "$APP_ROOT/current")"
    previous_target="$(readlink -f "$APP_ROOT/previous")"
    address="$(read_env_value "$APP_ROOT/shared/.env" AOWUGONG_HTTP_ADDRESS)"
    port="${address##*:}"
    point_symlink "$APP_ROOT/current" "$previous_target"
    if ! systemctl restart "$GO_SERVICE" || ! wait_for_health "http://127.0.0.1:${port}/api/v1/health" 30; then
      point_symlink "$APP_ROOT/current" "$current_target"
      systemctl restart "$GO_SERVICE" >/dev/null 2>&1 || true
      wait_for_health "http://127.0.0.1:${port}/api/v1/health" 30 || true
      die "上一 Go 版本启动失败，已恢复回滚前版本"
    fi
    point_symlink "$APP_ROOT/previous" "$current_target"
    printf '已回滚到上一 Go 发布产物: %s\n' "$previous_target"
    ;;
  fastapi)
    legacy_state=""
    systemctl stop "$GO_SERVICE" || true
    set_env_value "$APP_ROOT/shared/.env" AOWUGONG_HTTP_ADDRESS "0.0.0.0:2346"
    set_env_value "$APP_ROOT/shared/.env" AOWUGONG_SCHEDULER_ENABLED false
    if [ -n "$CRONTAB_BACKUP" ]; then
      restore_crontab "$LEGACY_RUN_USER" "$CRONTAB_BACKUP" "$LEGACY_PROJECT"
      legacy_state="${CRONTAB_BACKUP}.service-state"
    fi
    restore_service_state "$LEGACY_SERVICE" "$legacy_state"
    wait_for_health "http://127.0.0.1:2345/api/v1/health" 15 || \
      wait_for_health "http://127.0.0.1:2345/api/v1/openapi.json" 15 || \
      die "FastAPI 恢复后健康检查失败"
    printf '已停止 Go 正式入口并恢复 FastAPI。\n'
    ;;
  *)
    die "用法: rollback.sh [release|fastapi]"
    ;;
esac
