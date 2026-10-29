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
DEPLOY_MODE="${DEPLOY_MODE:-main}"
PUBLIC_URL="${PUBLIC_URL:-https://aowugong.top}"
ENV_FILE="${ENV_FILE:-}"
RELEASE_ARCHIVE="${RELEASE_ARCHIVE:-}"
HEALTH_ATTEMPTS="${HEALTH_ATTEMPTS:-60}"
SYSTEMD_UNIT_DIR="${SYSTEMD_UNIT_DIR:-/etc/systemd/system}"
MAIN_SERVICE="aowugong-go"
CANARY_SERVICE="aowugong-go-canary"

case "$DEPLOY_MODE" in
  main)
    APP_PORT="${APP_PORT:-12345}"
    BIND_ADDRESS="127.0.0.1:$APP_PORT"
    VPN_PUBLIC_URL="${PUBLIC_URL%/}"
    SCHEDULER_ENABLED="${SCHEDULER_ENABLED:-true}"
    SERVICE_NAME="$MAIN_SERVICE"
    RELEASE_LINK="$APP_ROOT/current"
    RUNTIME_ENV="$APP_ROOT/shared/.env"
    ;;
  canary)
    APP_PORT="${APP_PORT:-2346}"
    BIND_ADDRESS="127.0.0.1:$APP_PORT"
    VPN_PUBLIC_URL="${PUBLIC_URL%/}"
    SCHEDULER_ENABLED=false
    SERVICE_NAME="$CANARY_SERVICE"
    RELEASE_LINK="$APP_ROOT/canary"
    RUNTIME_ENV="$APP_ROOT/shared/.env.canary"
    ;;
  *) die "DEPLOY_MODE 只能是 main 或 canary" ;;
esac

require_root
for command_name in curl tar sha256sum systemctl install sed; do
  require_command "$command_name"
done
[ -n "$VERSION" ] || die "用法: deploy-release.sh <v版本号>"
printf '%s' "$VERSION" | grep -Eq '^v[0-9A-Za-z._-]+$' || die "版本号格式无效: $VERSION"
printf '%s' "$APP_PORT" | grep -Eq '^[0-9]+$' || die "APP_PORT 必须是数字"
id "$RUN_USER" >/dev/null 2>&1 || die "运行用户不存在，请先执行 initialize-server.sh"
[ -d "$APP_ROOT/releases" ] && [ -f "$APP_ROOT/shared/.env" ] || die "服务器未初始化，请先执行 initialize-server.sh"
RUN_GROUP="$(id -gn "$RUN_USER")"

package="aowugong-go-${VERSION}-linux-amd64"
archive="${package}.tar.gz"
download_base="https://github.com/${REPOSITORY}/releases/download/${VERSION}"
temporary_directory="$(mktemp -d)"
release_directory="$APP_ROOT/releases/$VERSION"
shared_env="$APP_ROOT/shared/.env"
old_release_target=""
old_previous_target=""
service_state="$temporary_directory/service.state"
runtime_env_backup="$temporary_directory/runtime.env.before"
unit_backup="$temporary_directory/unit.before"
changed=0

# configure_runtime_env 写入 PostgreSQL 运行路径和当前发布参数。
# 输入：file 是环境文件，release_link 是发布软链接。
# 输出：无。
# 副作用：原子修改环境文件。
configure_runtime_env() {
  local file="$1"
  local release_link="$2"
  set_env_value "$file" AOWUGONG_ENV production
  set_env_value "$file" AOWUGONG_HTTP_ADDRESS "$BIND_ADDRESS"
  set_env_default "$file" AOWUGONG_DATABASE_MAX_OPEN_CONNS 8
  set_env_default "$file" AOWUGONG_DATABASE_MAX_IDLE_CONNS 4
  set_env_default "$file" AOWUGONG_DATABASE_CONN_MAX_LIFETIME_MINUTES 30
  set_env_value "$file" AOWUGONG_DATABASE_SKIP_MIGRATIONS false
  set_env_value "$file" AOWUGONG_DEV_UPSTREAM_URL ""
  set_env_value "$file" AOWUGONG_STATIC_DIR "$release_link/web/dist"
  set_env_value "$file" AOWUGONG_MIGRATIONS_DIR "$release_link/migrations/postgres"
  set_env_value "$file" AOWUGONG_WORK_NAVIGATION_PATH "$APP_ROOT/shared/storage/private/work/navigation.json"
  set_env_value "$file" AOWUGONG_BACKUP_DIR "$APP_ROOT/shared/storage/backup"
  set_env_value "$file" AOWUGONG_POSITION_UPLOAD_DIR "$APP_ROOT/shared/storage/uploads/positions"
  set_env_value "$file" AOWUGONG_POSITION_TEMP_DIR "$APP_ROOT/shared/storage/temp/positions"
  set_env_value "$file" VPN_SOURCE_DIR "$APP_ROOT/shared/storage/private/vpn"
  set_env_value "$file" VPN_PUBLIC_URL "$VPN_PUBLIC_URL"
  set_env_value "$file" AOWUGONG_PUBLIC_URL "$VPN_PUBLIC_URL"
  set_env_value "$file" AOWUGONG_SCHEDULER_ENABLED "$SCHEDULER_ENABLED"
  chown "$RUN_USER:$RUN_GROUP" "$file"
}

# validate_runtime_env 拒绝缺少生产密钥或 PostgreSQL 地址的环境。
# 输入：file 是待加载环境文件。
# 输出：配置完整返回成功，否则终止。
# 副作用：只读环境文件。
validate_runtime_env() {
  local file="$1"
  local key
  local value
  for key in AOWUGONG_JWT_SECRET AOWUGONG_ENCRYPTION_KEY AOWUGONG_DATABASE_URL; do
    value="$(read_env_value "$file" "$key")"
    [ -n "$value" ] || die "$file 缺少必需配置: $key"
    case "$value" in replace-with-*) die "$file 仍使用示例值: $key" ;; esac
  done
}

# cleanup_deploy 在失败时恢复软链接、环境和服务状态。
# 输入：由 EXIT trap 取得退出状态。
# 输出：保留原退出状态。
# 副作用：恢复发布前服务器状态。
cleanup_deploy() {
  local exit_code=$?
  trap - EXIT
  set +e
  if [ "$changed" -eq 1 ]; then
    systemctl stop "$SERVICE_NAME" >/dev/null 2>&1 || true
    if [ -n "$old_release_target" ]; then
      point_symlink "$RELEASE_LINK" "$old_release_target"
    else
      rm -f "$RELEASE_LINK"
    fi
    if [ -f "$unit_backup" ]; then
      install -m 0644 "$unit_backup" "$SYSTEMD_UNIT_DIR/${SERVICE_NAME}.service"
    else
      rm -f "$SYSTEMD_UNIT_DIR/${SERVICE_NAME}.service"
    fi
    if [ -f "$runtime_env_backup" ]; then
      install -m 0600 -o "$RUN_USER" -g "$RUN_GROUP" "$runtime_env_backup" "$RUNTIME_ENV"
    else
      rm -f "$RUNTIME_ENV"
    fi
    if [ "$DEPLOY_MODE" = "main" ]; then
      if [ -n "$old_previous_target" ]; then
        point_symlink "$APP_ROOT/previous" "$old_previous_target"
      else
        rm -f "$APP_ROOT/previous"
      fi
    fi
    systemctl daemon-reload >/dev/null 2>&1 || true
    restore_service_state "$SERVICE_NAME" "$service_state"
  fi
  rm -rf "$temporary_directory"
  exit "$exit_code"
}
trap cleanup_deploy EXIT

# 1. 下载并校验不含编译依赖的发布产物。
if [ -n "$RELEASE_ARCHIVE" ]; then
  [ -f "$RELEASE_ARCHIVE" ] || die "本地发布包不存在: $RELEASE_ARCHIVE"
  [ -f "${RELEASE_ARCHIVE}.sha256" ] || die "发布包缺少校验文件"
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
for required_path in aowugong web/dist/index.html migrations/postgres \
  "init/systemd/${SERVICE_NAME}.service"; do
  [ -e "$source_release/$required_path" ] || die "发布包缺少: $required_path"
done

# 2. 安装独立版本目录，不修改 shared 权限或其他服务。
[ ! -e "$release_directory" ] || die "版本目录已存在，请使用新版本号: $VERSION"
mv "$source_release" "$release_directory"
chown -R root:root "$release_directory"
chmod 0755 "$release_directory/aowugong" "$release_directory/scripts/"*.sh

# 3. 原子切换软链接并启动目标服务。
save_service_state "$SERVICE_NAME" "$service_state"
if [ -L "$RELEASE_LINK" ]; then old_release_target="$(readlink -f "$RELEASE_LINK")"; fi
if [ "$DEPLOY_MODE" = "main" ] && [ -L "$APP_ROOT/previous" ]; then
  old_previous_target="$(readlink -f "$APP_ROOT/previous")"
fi
if [ -f "$RUNTIME_ENV" ]; then install -m 0600 "$RUNTIME_ENV" "$runtime_env_backup"; fi
if [ -f "$SYSTEMD_UNIT_DIR/${SERVICE_NAME}.service" ]; then
  install -m 0600 "$SYSTEMD_UNIT_DIR/${SERVICE_NAME}.service" "$unit_backup"
fi
changed=1
if [ "$DEPLOY_MODE" = "canary" ]; then
  install -m 0600 -o "$RUN_USER" -g "$RUN_GROUP" "${ENV_FILE:-$shared_env}" "$RUNTIME_ENV"
elif [ -n "$ENV_FILE" ]; then
  install -m 0600 -o "$RUN_USER" -g "$RUN_GROUP" "$ENV_FILE" "$RUNTIME_ENV"
fi
configure_runtime_env "$RUNTIME_ENV" "$RELEASE_LINK"
validate_runtime_env "$RUNTIME_ENV"
if [ "$DEPLOY_MODE" = "main" ] && [ -n "$old_release_target" ]; then
  point_symlink "$APP_ROOT/previous" "$old_release_target"
fi
point_symlink "$RELEASE_LINK" "$release_directory"
render_service "$release_directory" "$SERVICE_NAME" "$SYSTEMD_UNIT_DIR" "$APP_ROOT" "$RUN_USER" "$RUN_GROUP" "$temporary_directory/${SERVICE_NAME}.service"
systemctl daemon-reload
systemctl enable "$SERVICE_NAME" >/dev/null
systemctl restart "$SERVICE_NAME"
health_url="http://127.0.0.1:${APP_PORT}/api/v1/health"
wait_for_health "$health_url" "$HEALTH_ATTEMPTS" || die "新版本健康检查失败"

changed=0
trap - EXIT
rm -rf "$temporary_directory"
printf '部署完成: mode=%s version=%s url=%s database=PostgreSQL\n' "$DEPLOY_MODE" "$VERSION" "$health_url"
