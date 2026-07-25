#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=server-release-lib.sh
. "$SCRIPT_DIR/server-release-lib.sh"

VERSION="${1:-}"
REPOSITORY="${REPOSITORY:-howie110/aowugong-go}"
APP_ROOT="${APP_ROOT:-/opt/aowugong-go}"
RUN_USER="${RUN_USER:-aowugong}"
RUN_GROUP="${RUN_GROUP:-aowugong}"
DEPLOY_MODE="${DEPLOY_MODE:-canary}"
MIGRATE_FROM_MYSQL="${MIGRATE_FROM_MYSQL:-false}"
REFRESH_FROM_MYSQL="${REFRESH_FROM_MYSQL:-false}"
ENV_FILE="${ENV_FILE:-}"
RELEASE_ARCHIVE="${RELEASE_ARCHIVE:-}"
HEALTH_ATTEMPTS="${HEALTH_ATTEMPTS:-180}"
MAIN_SERVICE="aowugong-go"
CANARY_SERVICE="aowugong-go-canary"

case "$DEPLOY_MODE" in
  canary)
    APP_PORT="${APP_PORT:-2346}"
    SCHEDULER_ENABLED="${SCHEDULER_ENABLED:-false}"
    SERVICE_NAME="$CANARY_SERVICE"
    ;;
  main)
    APP_PORT="${APP_PORT:-2345}"
    SCHEDULER_ENABLED="${SCHEDULER_ENABLED:-true}"
    SERVICE_NAME="$MAIN_SERVICE"
    ;;
  *)
    die "DEPLOY_MODE 只能是 canary 或 main"
    ;;
esac

require_root
for command_name in curl tar sha256sum systemctl install sed useradd runuser swapon swapoff mkswap sysctl df; do
  require_command "$command_name"
done
[ -n "$VERSION" ] || die "用法: deploy-release.sh <v版本号>"
printf '%s' "$VERSION" | grep -Eq '^v[0-9A-Za-z._-]+$' || die "版本号格式无效: $VERSION"
printf '%s' "$APP_PORT" | grep -Eq '^[0-9]+$' || die "APP_PORT 必须是数字"
[ "$APP_PORT" -ge 1 ] && [ "$APP_PORT" -le 65535 ] || die "APP_PORT 超出有效范围"
case "$SCHEDULER_ENABLED" in
  true|false) ;;
  *) die "SCHEDULER_ENABLED 只能是 true 或 false" ;;
esac
if [ "$DEPLOY_MODE" = "canary" ]; then
  [ "$APP_PORT" != "2345" ] || die "并行验收不能占用正式 2345 端口"
  [ "$SCHEDULER_ENABLED" = "false" ] || die "并行验收必须关闭调度器"
fi

ensure_swap
if ! id "$RUN_USER" >/dev/null 2>&1; then
  useradd --system --home-dir "$APP_ROOT" --create-home --shell /usr/sbin/nologin "$RUN_USER"
fi
RUN_GROUP="$(id -gn "$RUN_USER")"

package="aowugong-go-${VERSION}-linux-amd64"
archive="${package}.tar.gz"
download_base="https://github.com/${REPOSITORY}/releases/download/${VERSION}"
temporary_directory="$(mktemp -d)"
release_directory="$APP_ROOT/releases/$VERSION"
sqlite_path="$APP_ROOT/shared/storage/data/aowugong.db"
shared_env="$APP_ROOT/shared/.env"
canary_env="$APP_ROOT/shared/.env.sqlite-canary"
runtime_env=""
mode_changed=0
env_changed=0
migration_created=0
old_current_target=""
old_previous_target=""
old_canary_target=""
main_state="$temporary_directory/main.service-state"
canary_state="$temporary_directory/canary.service-state"
shared_env_backup="$temporary_directory/shared.env.before"
canary_env_backup="$temporary_directory/canary.env.before"

# render_main_service 安装正式 systemd 服务文件。
# 输入：release_path 是包含模板的发布目录。
# 输出：无；模板缺失或写入失败时退出。
# 副作用：覆盖正式 systemd unit。
render_main_service() {
  local release_path="$1"
  local service_temp="$temporary_directory/aowugong-go.service.rendered"
  sed -e "s|@APP_ROOT@|$APP_ROOT|g" -e "s|@RUN_USER@|$RUN_USER|g" -e "s|@RUN_GROUP@|$RUN_GROUP|g" \
    "$release_path/init/systemd/aowugong-go.service" > "$service_temp"
  install -m 0644 "$service_temp" "/etc/systemd/system/${MAIN_SERVICE}.service"
}

# render_canary_service 安装独立的 2346 验收 systemd 服务文件。
# 输入：release_path 是包含模板的发布目录。
# 输出：无；模板缺失或写入失败时退出。
# 副作用：覆盖临时 canary systemd unit。
render_canary_service() {
  local release_path="$1"
  local service_temp="$temporary_directory/aowugong-go-canary.service.rendered"
  sed -e "s|@APP_ROOT@|$APP_ROOT|g" -e "s|@RUN_USER@|$RUN_USER|g" -e "s|@RUN_GROUP@|$RUN_GROUP|g" \
    "$release_path/init/systemd/aowugong-go-canary.service" > "$service_temp"
  install -m 0644 "$service_temp" "/etc/systemd/system/${CANARY_SERVICE}.service"
}

# configure_runtime_env 写入当前发布模式的 SQLite 运行配置。
# 输入：file 是环境文件，release_link 是 current 或 canary，port 和 scheduler 控制监听与调度。
# 输出：无；配置写入失败时退出。
# 副作用：修改指定环境文件。
configure_runtime_env() {
  local file="$1"
  local release_link="$2"
  local port="$3"
  local scheduler="$4"
  set_env_value "$file" AOWUGONG_ENV production
  set_env_value "$file" AOWUGONG_HTTP_ADDRESS "0.0.0.0:$port"
  set_env_value "$file" AOWUGONG_SQLITE_PATH "$sqlite_path"
  set_env_default "$file" AOWUGONG_SQLITE_MAX_OPEN_CONNS "4"
  set_env_default "$file" AOWUGONG_SQLITE_MAX_IDLE_CONNS "2"
  set_env_default "$file" AOWUGONG_SQLITE_BUSY_TIMEOUT_MS "5000"
  set_env_value "$file" AOWUGONG_SQLITE_SKIP_MIGRATIONS false
  set_env_value "$file" AOWUGONG_DEV_UPSTREAM_URL ""
  set_env_value "$file" AOWUGONG_STATIC_DIR "$release_link/web/dist"
  set_env_value "$file" AOWUGONG_MIGRATIONS_DIR "$release_link/migrations/sqlite"
  set_env_value "$file" AOWUGONG_WORK_NAVIGATION_PATH "$APP_ROOT/shared/storage/private/work/navigation.json"
  set_env_value "$file" AOWUGONG_BACKUP_DIR "$APP_ROOT/shared/storage/backup"
  set_env_value "$file" AOWUGONG_POSITION_UPLOAD_DIR "$APP_ROOT/shared/storage/uploads/positions"
  set_env_value "$file" AOWUGONG_POSITION_TEMP_DIR "$APP_ROOT/shared/storage/temp/positions"
  set_env_value "$file" AOWUGONG_SCHEDULER_ENABLED "$scheduler"
  chown "$RUN_USER:$RUN_GROUP" "$file"
}

# validate_runtime_env 检查新服务和一次性迁移需要的凭据。
# 输入：file 是即将加载的环境文件，require_mysql 表示是否需要旧库连接。
# 输出：配置完整返回成功，否则退出。
# 副作用：只读环境文件。
validate_runtime_env() {
  local file="$1"
  local require_mysql="$2"
  local required_key
  local value
  for required_key in AOWUGONG_JWT_SECRET AOWUGONG_ENCRYPTION_KEY; do
    value="$(read_env_value "$file" "$required_key")"
    [ -n "$value" ] || die "$file 缺少必需配置: $required_key"
    case "$value" in
      replace-with-*) die "$file 仍使用示例值: $required_key" ;;
    esac
  done
  if [ "$require_mysql" = "true" ]; then
    for required_key in AOWUGONG_MYSQL_HOST AOWUGONG_MYSQL_DATABASE AOWUGONG_MYSQL_USER AOWUGONG_MYSQL_PASSWORD; do
      value="$(read_env_value "$file" "$required_key")"
      [ -n "$value" ] || die "$file 缺少迁移配置: $required_key"
    done
  fi
}

# run_data_migration 从旧 MySQL 重建并核对 SQLite 数据。
# 输入：无，使用当前 release_directory 和 runtime_env。
# 输出：迁移成功后输出核对报告和保存路径；任一表核对失败时退出。
# 副作用：只读 MySQL，原子重写 SQLite 业务表并保存核对报告。
run_data_migration() {
  local report_path="$APP_ROOT/shared/storage/backup/mysql-to-sqlite-${VERSION}-$(date +%Y%m%d-%H%M%S).json"
  local report_temp="$temporary_directory/migration-report.json"
  validate_runtime_env "$runtime_env" true
  if ! runuser -u "$RUN_USER" -- bash -c \
    'set -a; . "$1"; set +a; exec "$2" --confirm' \
    bash "$runtime_env" "$release_directory/aowugong-migrate" > "$report_temp"; then
    rm -f "$report_temp"
    die "MySQL 到 SQLite 迁移或核验失败"
  fi
  install -m 0600 "$report_temp" "$report_path"
  cat "$report_path"
  printf '迁移核验报告: %s\n' "$report_path"
  chown "$RUN_USER:$RUN_GROUP" "$sqlite_path"
  chmod 0600 "$sqlite_path"
}

# cleanup_deploy 在发布失败时恢复原服务、环境文件和符号链接。
# 输入：由 EXIT trap 自动取得退出码。
# 输出：保留原退出码。
# 副作用：停止失败版本并恢复发布前状态。
cleanup_deploy() {
  local exit_code=$?
  trap - EXIT
  set +e
  if [ "$migration_created" -eq 1 ]; then
    systemctl stop "$SERVICE_NAME" >/dev/null 2>&1 || true
    rm -f "$sqlite_path" "${sqlite_path}-wal" "${sqlite_path}-shm"
    printf '首次 SQLite 发布未完成，已移除目标文件并要求下次重新迁移。\n' >&2
  fi
  if [ "$env_changed" -eq 1 ] && [ -f "$shared_env_backup" ]; then
    install -m 0600 -o "$RUN_USER" -g "$RUN_GROUP" "$shared_env_backup" "$shared_env"
  fi
  if [ "$mode_changed" -eq 1 ] && [ "$DEPLOY_MODE" = "canary" ]; then
    systemctl stop "$CANARY_SERVICE" >/dev/null 2>&1 || true
    if [ -n "$old_canary_target" ]; then
      point_symlink "$APP_ROOT/canary" "$old_canary_target"
      render_canary_service "$old_canary_target"
      if [ -f "$canary_env_backup" ]; then
        install -m 0600 -o "$RUN_USER" -g "$RUN_GROUP" "$canary_env_backup" "$canary_env"
      fi
      systemctl daemon-reload
      restore_service_state "$CANARY_SERVICE" "$canary_state"
    else
      rm -f "$APP_ROOT/canary" "/etc/systemd/system/${CANARY_SERVICE}.service"
      if [ -f "$canary_env_backup" ]; then
        install -m 0600 -o "$RUN_USER" -g "$RUN_GROUP" "$canary_env_backup" "$canary_env"
      else
        rm -f "$canary_env"
      fi
      systemctl daemon-reload >/dev/null 2>&1 || true
    fi
    printf '并行发布失败，已恢复原 2346 验收服务。\n' >&2
  fi
  if [ "$mode_changed" -eq 1 ] && [ "$DEPLOY_MODE" = "main" ]; then
    if [ -n "$old_current_target" ]; then
      point_symlink "$APP_ROOT/current" "$old_current_target"
      render_main_service "$old_current_target"
      if [ -n "$old_previous_target" ]; then
        point_symlink "$APP_ROOT/previous" "$old_previous_target"
      else
        rm -f "$APP_ROOT/previous"
      fi
      systemctl daemon-reload
      restore_service_state "$MAIN_SERVICE" "$main_state"
    else
      systemctl disable --now "$MAIN_SERVICE" >/dev/null 2>&1 || true
      rm -f "$APP_ROOT/current" "$APP_ROOT/previous" "/etc/systemd/system/${MAIN_SERVICE}.service"
      systemctl daemon-reload >/dev/null 2>&1 || true
    fi
    printf '正式发布失败，已恢复上一版本。\n' >&2
  fi
  rm -rf "$temporary_directory"
  exit "$exit_code"
}
trap cleanup_deploy EXIT

# 1. 下载、校验并安装不含编译依赖的发布产物。
if [ -n "$RELEASE_ARCHIVE" ]; then
  [ -f "$RELEASE_ARCHIVE" ] || die "本地发布包不存在: $RELEASE_ARCHIVE"
  [ -f "${RELEASE_ARCHIVE}.sha256" ] || die "本地发布包缺少校验文件: ${RELEASE_ARCHIVE}.sha256"
  install -m 0644 "$RELEASE_ARCHIVE" "$temporary_directory/$archive"
  install -m 0644 "${RELEASE_ARCHIVE}.sha256" "$temporary_directory/$archive.sha256"
else
  curl --fail --location --retry 3 --output "$temporary_directory/$archive" "$download_base/$archive"
  curl --fail --location --retry 3 --output "$temporary_directory/$archive.sha256" "$download_base/$archive.sha256"
fi
(
  cd "$temporary_directory"
  sha256sum --check "$archive.sha256"
  tar -xzf "$archive"
)

source_release="$temporary_directory/$package"
for required_path in \
  aowugong aowugong-migrate web/dist/index.html migrations/sqlite configs/.env.example \
  init/systemd/aowugong-go.service init/systemd/aowugong-go-canary.service; do
  [ -e "$source_release/$required_path" ] || die "发布包缺少: $required_path"
done

mkdir -p "$APP_ROOT/releases" "$APP_ROOT/shared/storage/data" "$APP_ROOT/shared/storage/backup" \
  "$APP_ROOT/shared/storage/exports" "$APP_ROOT/shared/storage/uploads" "$APP_ROOT/shared/storage/temp" \
  "$APP_ROOT/shared/storage/logs" "$APP_ROOT/shared/storage/private"
if [ ! -d "$release_directory" ]; then
  mv "$source_release" "$release_directory"
fi
for required_path in \
  aowugong aowugong-migrate web/dist/index.html migrations/sqlite configs/.env.example \
  init/systemd/aowugong-go.service init/systemd/aowugong-go-canary.service; do
  [ -e "$release_directory/$required_path" ] || die "已安装发布目录不完整: $required_path"
done
chown -R root:root "$release_directory"
chmod 0755 "$release_directory/aowugong" "$release_directory/aowugong-migrate" "$release_directory/scripts/"*.sh
chown -R "$RUN_USER:$RUN_GROUP" "$APP_ROOT/shared"
chmod 0750 "$APP_ROOT/shared" "$APP_ROOT/shared/storage"

# 2. 从现有生产配置派生 canary 环境，正式发布才修改 shared/.env。
if [ ! -f "$shared_env" ]; then
  if [ -n "$ENV_FILE" ]; then
    [ -f "$ENV_FILE" ] || die "ENV_FILE 不存在: $ENV_FILE"
    install -m 0600 -o "$RUN_USER" -g "$RUN_GROUP" "$ENV_FILE" "$shared_env"
  else
    install -m 0600 -o "$RUN_USER" -g "$RUN_GROUP" "$release_directory/configs/.env.example" "$shared_env"
    die "已创建 $shared_env；请填写真实密钥后重跑，或通过 ENV_FILE 提供生产配置"
  fi
fi

if [ "$DEPLOY_MODE" = "canary" ]; then
  save_service_state "$CANARY_SERVICE" "$canary_state"
  if [ -L "$APP_ROOT/canary" ]; then
    old_canary_target="$(readlink -f "$APP_ROOT/canary")"
  fi
  if [ -f "$canary_env" ]; then
    install -m 0600 "$canary_env" "$canary_env_backup"
  fi
  mode_changed=1
  source_env="$shared_env"
  if [ -n "$ENV_FILE" ]; then
    [ -f "$ENV_FILE" ] || die "ENV_FILE 不存在: $ENV_FILE"
    source_env="$ENV_FILE"
  fi
  install -m 0600 -o "$RUN_USER" -g "$RUN_GROUP" "$source_env" "$canary_env"
  runtime_env="$canary_env"
  configure_runtime_env "$runtime_env" "$APP_ROOT/canary" "$APP_PORT" false
  point_symlink "$APP_ROOT/canary" "$release_directory"
  render_canary_service "$release_directory"
else
  save_service_state "$MAIN_SERVICE" "$main_state"
  if [ -L "$APP_ROOT/current" ]; then
    old_current_target="$(readlink -f "$APP_ROOT/current")"
  fi
  if [ -L "$APP_ROOT/previous" ]; then
    old_previous_target="$(readlink -f "$APP_ROOT/previous")"
  fi
  if [ -n "$old_current_target" ] && [ ! -d "$old_current_target/migrations/sqlite" ]; then
    die "当前正式版本仍使用 MySQL；首次 SQLite 切换必须先发布 canary，再运行 scripts/cutover.sh"
  fi
  if [ "$REFRESH_FROM_MYSQL" = "true" ]; then
    die "正式发布不能在线刷新 MySQL；请由 scripts/cutover.sh 停写后执行最终迁移"
  fi
  mode_changed=1
  install -m 0600 "$shared_env" "$shared_env_backup"
  env_changed=1
  if [ -n "$ENV_FILE" ]; then
    [ -f "$ENV_FILE" ] || die "ENV_FILE 不存在: $ENV_FILE"
    install -m 0600 -o "$RUN_USER" -g "$RUN_GROUP" "$ENV_FILE" "$shared_env"
  fi
  runtime_env="$shared_env"
  configure_runtime_env "$runtime_env" "$APP_ROOT/current" "$APP_PORT" "$SCHEDULER_ENABLED"
  if [ -n "$old_current_target" ]; then
    point_symlink "$APP_ROOT/previous" "$old_current_target"
  fi
  point_symlink "$APP_ROOT/current" "$release_directory"
  render_main_service "$release_directory"
fi

# 3. 首次建立或显式刷新 SQLite；个股日线仅统计跳过，不复制。
needs_migration=false
if [ ! -s "$sqlite_path" ]; then
  [ "$MIGRATE_FROM_MYSQL" = "true" ] || die "SQLite 尚未建立；首次切换请设置 MIGRATE_FROM_MYSQL=true"
  migration_created=1
  needs_migration=true
elif [ "$REFRESH_FROM_MYSQL" = "true" ]; then
  needs_migration=true
fi
validate_runtime_env "$runtime_env" "$needs_migration"
if [ "$needs_migration" = "true" ]; then
  systemctl stop "$CANARY_SERVICE" >/dev/null 2>&1 || true
  run_data_migration
fi

# 4. 启动目标模式并完成健康检查；canary 从不停止正式 2345 服务。
systemctl daemon-reload
if [ "$DEPLOY_MODE" = "main" ]; then
  systemctl enable "$MAIN_SERVICE" >/dev/null
fi
systemctl restart "$SERVICE_NAME"
health_url="http://127.0.0.1:${APP_PORT}/api/v1/health"
if ! wait_for_health "$health_url" "$HEALTH_ATTEMPTS"; then
  systemctl status "$SERVICE_NAME" --no-pager || true
  die "新版本健康检查失败"
fi

if [ "$DEPLOY_MODE" = "main" ]; then
  systemctl stop "$CANARY_SERVICE" >/dev/null 2>&1 || true
  rm -f "$APP_ROOT/canary" "$canary_env" "/etc/systemd/system/${CANARY_SERVICE}.service"
  systemctl daemon-reload
fi

mode_changed=0
env_changed=0
migration_created=0
trap - EXIT
rm -rf "$temporary_directory"
printf '部署完成: mode=%s version=%s url=%s scheduler=%s database=SQLite\n' \
  "$DEPLOY_MODE" "$VERSION" "$health_url" "$SCHEDULER_ENABLED"
