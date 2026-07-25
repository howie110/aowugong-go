#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=server-release-lib.sh
. "$SCRIPT_DIR/server-release-lib.sh"

APP_ROOT="${APP_ROOT:-/opt/aowugong-go}"
RUN_USER="${RUN_USER:-aowugong}"
RUN_GROUP="${RUN_GROUP:-aowugong}"
MODE="${1:-release}"
GO_SERVICE="aowugong-go"
LEGACY_SERVICE="${LEGACY_SERVICE:-aowugong-fastapi}"
LEGACY_PROJECT="${LEGACY_PROJECT:-/www/wwwroot/docker-file/aowugong-fastapi}"
LEGACY_RUN_USER="${LEGACY_RUN_USER:-root}"
CRONTAB_BACKUP="${CRONTAB_BACKUP:-}"
CUTOVER_ENV_BACKUP="${CUTOVER_ENV_BACKUP:-}"

require_root
for command_name in curl systemctl install sed crontab; do
  require_command "$command_name"
done
id "$RUN_USER" >/dev/null 2>&1 || die "运行用户不存在: $RUN_USER"
RUN_GROUP="$(id -gn "$RUN_USER")"

# render_main_service 按指定发布产物安装正式 systemd 服务。
# 输入：release_path 是包含正式 unit 模板的发布目录。
# 输出：无；模板或写入失败时退出。
# 副作用：覆盖 aowugong-go systemd unit。
render_main_service() {
  local release_path="$1"
  local service_temp
  service_temp="$(mktemp)"
  sed -e "s|@APP_ROOT@|$APP_ROOT|g" -e "s|@RUN_USER@|$RUN_USER|g" -e "s|@RUN_GROUP@|$RUN_GROUP|g" \
    "$release_path/init/systemd/aowugong-go.service" > "$service_temp"
  install -m 0644 "$service_temp" "/etc/systemd/system/${GO_SERVICE}.service"
  rm -f "$service_temp"
}

case "$MODE" in
  release)
    # 1. SQLite 版本之间只切换发布产物，继续复用同一数据库文件。
    [ -L "$APP_ROOT/previous" ] || die "没有可回滚的 previous 发布产物"
    current_target="$(readlink -f "$APP_ROOT/current")"
    previous_target="$(readlink -f "$APP_ROOT/previous")"
    [ -d "$previous_target/migrations/sqlite" ] || \
      die "上一 Go 版本仍使用 MySQL，请使用 mysql-go 或 fastapi 回滚"
    address="$(read_env_value "$APP_ROOT/shared/.env" AOWUGONG_HTTP_ADDRESS)"
    port="${address##*:}"
    release_changed=1
    rollback_release_switch() {
      local exit_code=$?
      trap - EXIT
      if [ "$release_changed" -eq 1 ]; then
        set +e
        point_symlink "$APP_ROOT/current" "$current_target"
        render_main_service "$current_target"
        systemctl daemon-reload
        systemctl restart "$GO_SERVICE" >/dev/null 2>&1 || true
        wait_for_health "http://127.0.0.1:${port}/api/v1/health" 30 || true
        printf 'SQLite 版本回滚失败，已恢复回滚前版本。\n' >&2
      fi
      exit "$exit_code"
    }
    trap rollback_release_switch EXIT
    point_symlink "$APP_ROOT/current" "$previous_target"
    render_main_service "$previous_target"
    systemctl daemon-reload
    if ! systemctl restart "$GO_SERVICE" || ! wait_for_health "http://127.0.0.1:${port}/api/v1/health" 30; then
      die "上一 SQLite Go 版本启动失败"
    fi
    point_symlink "$APP_ROOT/previous" "$current_target"
    release_changed=0
    trap - EXIT
    printf '已回滚到上一 SQLite Go 发布产物: %s\n' "$previous_target"
    ;;
  mysql-go)
    # 1. 首次 SQLite 切换后使用切换前环境恢复上一 MySQL Go。
    [ -L "$APP_ROOT/previous" ] || die "没有可回滚的 previous 发布产物"
    [ -f "$CUTOVER_ENV_BACKUP" ] || die "mysql-go 回滚必须设置 CUTOVER_ENV_BACKUP"
    current_target="$(readlink -f "$APP_ROOT/current")"
    previous_target="$(readlink -f "$APP_ROOT/previous")"
    [ ! -d "$previous_target/migrations/sqlite" ] || die "previous 已是 SQLite 版本，请使用 release 回滚"
    current_env_backup="$(mktemp)"
    install -m 0600 "$APP_ROOT/shared/.env" "$current_env_backup"
    mysql_go_changed=1
    rollback_mysql_go_switch() {
      local exit_code=$?
      trap - EXIT
      if [ "$mysql_go_changed" -eq 1 ]; then
        set +e
        point_symlink "$APP_ROOT/current" "$current_target"
        install -m 0600 -o "$RUN_USER" -g "$RUN_GROUP" "$current_env_backup" "$APP_ROOT/shared/.env"
        render_main_service "$current_target"
        systemctl daemon-reload
        systemctl restart "$GO_SERVICE" >/dev/null 2>&1 || true
        wait_for_health "http://127.0.0.1:2345/api/v1/health" 30 || true
        printf 'MySQL Go 恢复失败，已恢复 SQLite 版本。\n' >&2
      fi
      rm -f "$current_env_backup"
      exit "$exit_code"
    }
    trap rollback_mysql_go_switch EXIT
    point_symlink "$APP_ROOT/current" "$previous_target"
    install -m 0600 -o "$RUN_USER" -g "$RUN_GROUP" "$CUTOVER_ENV_BACKUP" "$APP_ROOT/shared/.env"
    render_main_service "$previous_target"
    systemctl daemon-reload
    if ! systemctl restart "$GO_SERVICE" || ! wait_for_health "http://127.0.0.1:2345/api/v1/health" 30; then
      die "MySQL Go 启动或健康检查失败"
    fi
    point_symlink "$APP_ROOT/previous" "$current_target"
    mysql_go_changed=0
    trap - EXIT
    rm -f "$current_env_backup"
    printf '已恢复切换前的 MySQL Go 发布产物: %s\n' "$previous_target"
    ;;
  fastapi)
    # 1. 停止 Go，恢复旧 crontab 并明确启动保留的 FastAPI 服务。
    fastapi_env_backup="$(mktemp)"
    fastapi_crontab_backup="$(mktemp)"
    fastapi_go_state="$(mktemp)"
    fastapi_legacy_state="$(mktemp)"
    fastapi_had_crontab=0
    install -m 0600 "$APP_ROOT/shared/.env" "$fastapi_env_backup"
    if crontab -u "$LEGACY_RUN_USER" -l > "$fastapi_crontab_backup" 2>/dev/null; then
      fastapi_had_crontab=1
    fi
    save_service_state "$GO_SERVICE" "$fastapi_go_state"
    save_service_state "$LEGACY_SERVICE" "$fastapi_legacy_state"
    fastapi_changed=1

    # rollback_fastapi_switch 在 FastAPI 恢复失败时重新启用原 Go 服务。
    # 输入：由 EXIT trap 自动取得失败状态。
    # 输出：保留原失败状态。
    # 副作用：恢复共享环境、crontab 和切换前的两个 systemd 服务状态。
    rollback_fastapi_switch() {
      local exit_code=$?
      trap - EXIT
      if [ "$fastapi_changed" -eq 1 ]; then
        set +e
        systemctl stop "$LEGACY_SERVICE" >/dev/null 2>&1 || true
        install -m 0600 -o "$RUN_USER" -g "$RUN_GROUP" "$fastapi_env_backup" "$APP_ROOT/shared/.env"
        if [ "$fastapi_had_crontab" -eq 1 ]; then
          crontab -u "$LEGACY_RUN_USER" "$fastapi_crontab_backup"
        else
          crontab -u "$LEGACY_RUN_USER" -r >/dev/null 2>&1 || true
        fi
        restore_service_state "$LEGACY_SERVICE" "$fastapi_legacy_state"
        restore_service_state "$GO_SERVICE" "$fastapi_go_state"
        wait_for_health "http://127.0.0.1:2345/api/v1/health" 30 || true
        printf 'FastAPI 恢复失败，已恢复切换前的 Go 服务。\n' >&2
      fi
      rm -f "$fastapi_env_backup" "$fastapi_crontab_backup" "$fastapi_go_state" "$fastapi_legacy_state"
      exit "$exit_code"
    }
    trap rollback_fastapi_switch EXIT

    systemctl disable --now "$GO_SERVICE" >/dev/null 2>&1 || true
    set_env_value "$APP_ROOT/shared/.env" AOWUGONG_HTTP_ADDRESS "0.0.0.0:2346"
    set_env_value "$APP_ROOT/shared/.env" AOWUGONG_SCHEDULER_ENABLED false
    if [ -n "$CRONTAB_BACKUP" ]; then
      restore_crontab "$LEGACY_RUN_USER" "$CRONTAB_BACKUP" "$LEGACY_PROJECT"
    fi
    systemctl enable "$LEGACY_SERVICE" >/dev/null 2>&1 || true
    systemctl restart "$LEGACY_SERVICE"
    wait_for_health "http://127.0.0.1:2345/api/v1/health" 15 || \
      wait_for_health "http://127.0.0.1:2345/api/v1/openapi.json" 15 || \
      die "FastAPI 恢复后健康检查失败"
    fastapi_changed=0
    trap - EXIT
    rm -f "$fastapi_env_backup" "$fastapi_crontab_backup" "$fastapi_go_state" "$fastapi_legacy_state"
    printf '已停止 Go 正式入口并恢复 FastAPI。\n'
    ;;
  *)
    die "用法: rollback.sh [release|mysql-go|fastapi]"
    ;;
esac
