#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=server-release-lib.sh
. "$SCRIPT_DIR/server-release-lib.sh"

APP_ROOT="${APP_ROOT:-/opt/aowugong-go}"
RUN_USER="${RUN_USER:-aowugong}"
RUN_GROUP="${RUN_GROUP:-aowugong}"
GO_SERVICE="aowugong-go"
CANARY_SERVICE="aowugong-go-canary"
LEGACY_SERVICE="${LEGACY_SERVICE:-aowugong-fastapi}"
LEGACY_PROJECT="${LEGACY_PROJECT:-/www/wwwroot/docker-file/aowugong-fastapi}"
LEGACY_RUN_USER="${LEGACY_RUN_USER:-root}"
canary_link="$APP_ROOT/canary"
canary_env="$APP_ROOT/shared/.env.sqlite-canary"
shared_env="$APP_ROOT/shared/.env"
sqlite_path="$APP_ROOT/shared/storage/data/aowugong.db"
cutover_id="$(date +%Y%m%d-%H%M%S)"
backup_prefix="$APP_ROOT/shared/storage/backup/sqlite-cutover-$cutover_id"
crontab_backup="$backup_prefix.fastapi-crontab"
shared_env_backup="$backup_prefix.env"
final_migration_report="$backup_prefix.migration-report.json"
final_migration_report_temp="$final_migration_report.tmp"
main_state="$backup_prefix.aowugong-go-state"
canary_state="$backup_prefix.canary-state"
legacy_state="${crontab_backup}.service-state"
old_current_target=""
old_previous_target=""
new_current_target=""
cutover_changed=0

require_root
for command_name in curl systemctl crontab awk stat runuser install sed pgrep; do
  require_command "$command_name"
done
[ -L "$canary_link" ] || die "缺少 SQLite canary 发布链接: $canary_link"
[ -f "$canary_env" ] || die "缺少 SQLite canary 环境文件: $canary_env"
[ -f "$shared_env" ] || die "缺少当前生产环境文件: $shared_env"
[ -x "$canary_link/aowugong" ] || die "缺少 canary 服务二进制"
[ -x "$canary_link/aowugong-migrate" ] || die "缺少 canary 迁移二进制"
[ -f "$canary_link/init/systemd/aowugong-go.service" ] || die "缺少正式 systemd 模板"
wait_for_health "http://127.0.0.1:2346/api/v1/health" 5 || die "2346 canary 未通过健康检查"
id "$RUN_USER" >/dev/null 2>&1 || die "运行用户不存在: $RUN_USER"
RUN_GROUP="$(id -gn "$RUN_USER")"
id "$LEGACY_RUN_USER" >/dev/null 2>&1 || die "旧 crontab 用户不存在: $LEGACY_RUN_USER"

mkdir -p "$(dirname "$backup_prefix")"
new_current_target="$(readlink -f "$canary_link")"
if [ -L "$APP_ROOT/current" ]; then
  old_current_target="$(readlink -f "$APP_ROOT/current")"
fi
if [ -L "$APP_ROOT/previous" ]; then
  old_previous_target="$(readlink -f "$APP_ROOT/previous")"
fi
install -m 0600 "$shared_env" "$shared_env_backup"
save_service_state "$GO_SERVICE" "$main_state"
save_service_state "$CANARY_SERVICE" "$canary_state"
save_service_state "$LEGACY_SERVICE" "$legacy_state"

# render_main_service 按指定发布版本恢复或安装正式 systemd 服务。
# 输入：release_path 是包含正式 unit 模板的发布目录。
# 输出：无；模板或写入失败时退出。
# 副作用：覆盖 /etc/systemd/system/aowugong-go.service。
render_main_service() {
  local release_path="$1"
  local service_temp
  service_temp="$(mktemp)"
  sed -e "s|@APP_ROOT@|$APP_ROOT|g" -e "s|@RUN_USER@|$RUN_USER|g" -e "s|@RUN_GROUP@|$RUN_GROUP|g" \
    "$release_path/init/systemd/aowugong-go.service" > "$service_temp"
  install -m 0644 "$service_temp" "/etc/systemd/system/${GO_SERVICE}.service"
  rm -f "$service_temp"
}

# rollback_cutover 恢复切换前的服务、环境、链接和旧任务。
# 输入：由 EXIT trap 自动取得失败状态。
# 输出：保留原失败状态。
# 副作用：停止 SQLite 正式入口并恢复原生产服务。
rollback_cutover() {
  local exit_code=$?
  trap - EXIT
  if [ "$cutover_changed" -eq 1 ]; then
    set +e
    systemctl stop "$GO_SERVICE" >/dev/null 2>&1 || true
    install -m 0600 -o "$RUN_USER" -g "$RUN_GROUP" "$shared_env_backup" "$shared_env"
    if [ -n "$old_current_target" ]; then
      point_symlink "$APP_ROOT/current" "$old_current_target"
      if [ -f "$old_current_target/init/systemd/aowugong-go.service" ]; then
        render_main_service "$old_current_target"
      fi
    else
      rm -f "$APP_ROOT/current" "/etc/systemd/system/${GO_SERVICE}.service"
    fi
    if [ -n "$old_previous_target" ]; then
      point_symlink "$APP_ROOT/previous" "$old_previous_target"
    else
      rm -f "$APP_ROOT/previous"
    fi
    systemctl daemon-reload
    restore_service_state "$GO_SERVICE" "$main_state"
    restore_crontab "$LEGACY_RUN_USER" "$crontab_backup" "$LEGACY_PROJECT"
    restore_service_state "$LEGACY_SERVICE" "$legacy_state"
    restore_service_state "$CANARY_SERVICE" "$canary_state"
    printf 'SQLite 切换失败，已恢复切换前的生产服务；下次切换会重新同步 MySQL。\n' >&2
  fi
  exit "$exit_code"
}
trap rollback_cutover EXIT

# 1. 先停止所有可能写旧 MySQL 或目标 SQLite 的进程与旧定时任务。
cutover_changed=1
remove_legacy_crontab "$LEGACY_RUN_USER" "$LEGACY_PROJECT" "$crontab_backup"
systemctl stop "$GO_SERVICE" >/dev/null 2>&1 || true
systemctl stop "$LEGACY_SERVICE" >/dev/null 2>&1 || true
systemctl stop "$CANARY_SERVICE"
systemctl is-active --quiet "$GO_SERVICE" && die "正式 Go 服务未能停止"
systemctl is-active --quiet "$LEGACY_SERVICE" && die "FastAPI 服务未能停止"
systemctl is-active --quiet "$CANARY_SERVICE" && die "SQLite canary 未能停止"
pgrep -f "$APP_ROOT/.*/aowugong job " >/dev/null && die "仍有 aowugong CLI 任务运行，拒绝迁移"
pgrep -f "app\.finance\.jobs\.job_runner" >/dev/null && die "仍有 FastAPI 定时任务运行，拒绝迁移"

# 2. 在停写窗口内从 MySQL 原子重建 SQLite，补齐并行验收期间的新数据。
if ! runuser -u "$RUN_USER" -- bash -c \
  'set -a; . "$1"; set +a; exec "$2" --confirm' \
  bash "$canary_env" "$canary_link/aowugong-migrate" > "$final_migration_report_temp"; then
  rm -f "$final_migration_report_temp"
  die "最终 MySQL 到 SQLite 迁移或核验失败"
fi
install -m 0600 "$final_migration_report_temp" "$final_migration_report"
rm -f "$final_migration_report_temp"
cat "$final_migration_report"
printf '最终迁移核验报告: %s\n' "$final_migration_report"
chown "$RUN_USER:$RUN_GROUP" "$sqlite_path"
chmod 0600 "$sqlite_path"

# 3. 把已验收发布提升为 current，并切换正式环境到 2345 和内嵌调度器。
install -m 0600 -o "$RUN_USER" -g "$RUN_GROUP" "$canary_env" "$shared_env"
set_env_value "$shared_env" AOWUGONG_HTTP_ADDRESS "0.0.0.0:2345"
set_env_value "$shared_env" AOWUGONG_SCHEDULER_ENABLED true
set_env_value "$shared_env" AOWUGONG_STATIC_DIR "$APP_ROOT/current/web/dist"
set_env_value "$shared_env" AOWUGONG_MIGRATIONS_DIR "$APP_ROOT/current/migrations/sqlite"
if [ -n "$old_current_target" ]; then
  point_symlink "$APP_ROOT/previous" "$old_current_target"
fi
point_symlink "$APP_ROOT/current" "$new_current_target"
render_main_service "$new_current_target"
systemctl daemon-reload
systemctl enable "$GO_SERVICE" >/dev/null
systemctl restart "$GO_SERVICE"
wait_for_health "http://127.0.0.1:2345/api/v1/health" 30 || die "SQLite 正式服务健康检查失败"

# 4. 成功后停用旧服务和临时 canary；保留 MySQL 数据用于紧急回滚。
systemctl disable "$LEGACY_SERVICE" >/dev/null 2>&1 || true
systemctl stop "$LEGACY_SERVICE" >/dev/null 2>&1 || true
systemctl stop "$CANARY_SERVICE" >/dev/null 2>&1 || true
rm -f "$canary_link" "$canary_env" "/etc/systemd/system/${CANARY_SERVICE}.service"
systemctl daemon-reload

cutover_changed=0
trap - EXIT
printf '生产切换完成：Go 已接管 2345，SQLite 已最终同步，内嵌调度已启用。\n'
printf '紧急回滚记录：env=%s crontab=%s\n' "$shared_env_backup" "$crontab_backup"
