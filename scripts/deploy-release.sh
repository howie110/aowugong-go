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
APP_PORT="${APP_PORT:-2346}"
SCHEDULER_ENABLED="${SCHEDULER_ENABLED:-false}"
ENV_FILE="${ENV_FILE:-}"
SERVICE_NAME="aowugong-go"

require_root
for command_name in curl tar sha256sum systemctl install sed useradd swapon swapoff mkswap sysctl df; do
  require_command "$command_name"
done
[ -n "$VERSION" ] || die "用法: deploy-release.sh <v版本号>"
printf '%s' "$VERSION" | grep -Eq '^v[0-9A-Za-z._-]+$' || die "版本号格式无效: $VERSION"

ensure_swap
if ! id "$RUN_USER" >/dev/null 2>&1; then
  useradd --system --home-dir "$APP_ROOT" --create-home --shell /usr/sbin/nologin "$RUN_USER"
fi
RUN_GROUP="$(id -gn "$RUN_USER")"

package="aowugong-go-${VERSION}-linux-amd64"
archive="${package}.tar.gz"
download_base="https://github.com/${REPOSITORY}/releases/download/${VERSION}"
temporary_directory="$(mktemp -d)"
deployment_changed=0
old_current_target=""
old_previous_target=""

render_service_file() {
  local release_path="$1"
  local service_temp="$temporary_directory/aowugong-go.service.rendered"
  sed -e "s|@APP_ROOT@|$APP_ROOT|g" -e "s|@RUN_USER@|$RUN_USER|g" -e "s|@RUN_GROUP@|$RUN_GROUP|g" \
    "$release_path/init/systemd/aowugong-go.service" > "$service_temp"
  install -m 0644 "$service_temp" "/etc/systemd/system/${SERVICE_NAME}.service"
}

cleanup_deploy() {
  local exit_code=$?
  trap - EXIT
  if [ "$deployment_changed" -eq 1 ]; then
    set +e
    if [ -n "$old_current_target" ]; then
      point_symlink "$APP_ROOT/current" "$old_current_target"
      if [ -f "$old_current_target/init/systemd/aowugong-go.service" ]; then
        render_service_file "$old_current_target"
      fi
      if [ -n "$old_previous_target" ]; then
        point_symlink "$APP_ROOT/previous" "$old_previous_target"
      else
        rm -f "$APP_ROOT/previous"
      fi
      systemctl daemon-reload
      systemctl restart "$SERVICE_NAME"
      printf '部署失败，已恢复上一发布产物 %s。\n' "$old_current_target" >&2
    else
      systemctl disable --now "$SERVICE_NAME" >/dev/null 2>&1 || true
      rm -f "$APP_ROOT/current" "$APP_ROOT/previous" "/etc/systemd/system/${SERVICE_NAME}.service"
      systemctl daemon-reload >/dev/null 2>&1 || true
      printf '首次部署失败，已移除未通过健康检查的服务入口。\n' >&2
    fi
  fi
  rm -rf "$temporary_directory"
  exit "$exit_code"
}
trap cleanup_deploy EXIT

curl --fail --location --retry 3 --output "$temporary_directory/$archive" "$download_base/$archive"
curl --fail --location --retry 3 --output "$temporary_directory/$archive.sha256" "$download_base/$archive.sha256"
(
  cd "$temporary_directory"
  sha256sum --check "$archive.sha256"
  tar -xzf "$archive"
)

source_release="$temporary_directory/$package"
for required_path in aowugong aowugong-migrate web/dist/index.html migrations configs/.env.example init/systemd/aowugong-go.service; do
  [ -e "$source_release/$required_path" ] || die "发布包缺少: $required_path"
done

mkdir -p "$APP_ROOT/releases" "$APP_ROOT/shared/storage/data" "$APP_ROOT/shared/storage/backup" \
  "$APP_ROOT/shared/storage/exports" "$APP_ROOT/shared/storage/uploads" "$APP_ROOT/shared/storage/temp" \
  "$APP_ROOT/shared/storage/logs" "$APP_ROOT/shared/storage/private"
release_directory="$APP_ROOT/releases/$VERSION"
if [ ! -d "$release_directory" ]; then
  mv "$source_release" "$release_directory"
fi
chown -R root:root "$release_directory"
chmod 0755 "$release_directory/aowugong" "$release_directory/aowugong-migrate" "$release_directory/scripts/"*.sh
chown -R "$RUN_USER:$RUN_GROUP" "$APP_ROOT/shared"
chmod 0750 "$APP_ROOT/shared" "$APP_ROOT/shared/storage"

shared_env="$APP_ROOT/shared/.env"
if [ -n "$ENV_FILE" ]; then
  [ -f "$ENV_FILE" ] || die "ENV_FILE 不存在: $ENV_FILE"
  install -m 0600 -o "$RUN_USER" -g "$RUN_GROUP" "$ENV_FILE" "$shared_env"
elif [ ! -f "$shared_env" ]; then
  install -m 0600 -o "$RUN_USER" -g "$RUN_GROUP" "$release_directory/configs/.env.example" "$shared_env"
  die "已创建 $shared_env；请填写真实密钥后重跑，或通过 ENV_FILE 提供生产配置"
fi

set_env_value "$shared_env" AOWUGONG_ENV production
set_env_value "$shared_env" AOWUGONG_HTTP_ADDRESS "0.0.0.0:$APP_PORT"
set_env_value "$shared_env" AOWUGONG_DATABASE_PATH "$APP_ROOT/shared/storage/data/aowugong.db"
set_env_value "$shared_env" AOWUGONG_STATIC_DIR "$APP_ROOT/current/web/dist"
set_env_value "$shared_env" AOWUGONG_MIGRATIONS_DIR "$APP_ROOT/current/migrations"
set_env_value "$shared_env" AOWUGONG_WORK_NAVIGATION_PATH "$APP_ROOT/shared/storage/private/work/navigation.json"
set_env_value "$shared_env" AOWUGONG_BACKUP_DIR "$APP_ROOT/shared/storage/backup"
set_env_value "$shared_env" AOWUGONG_POSITION_UPLOAD_DIR "$APP_ROOT/shared/storage/uploads/positions"
set_env_value "$shared_env" AOWUGONG_POSITION_TEMP_DIR "$APP_ROOT/shared/storage/temp/positions"
set_env_value "$shared_env" AOWUGONG_SCHEDULER_ENABLED "$SCHEDULER_ENABLED"
chown "$RUN_USER:$RUN_GROUP" "$shared_env"

if [ -L "$APP_ROOT/current" ]; then
  old_current_target="$(readlink -f "$APP_ROOT/current")"
fi
if [ -L "$APP_ROOT/previous" ]; then
  old_previous_target="$(readlink -f "$APP_ROOT/previous")"
fi
deployment_changed=1
if [ -n "$old_current_target" ]; then
  point_symlink "$APP_ROOT/previous" "$old_current_target"
fi
point_symlink "$APP_ROOT/current" "$release_directory"

render_service_file "$release_directory"
systemctl daemon-reload
systemctl enable "$SERVICE_NAME" >/dev/null
systemctl restart "$SERVICE_NAME"

health_url="http://127.0.0.1:${APP_PORT}/api/v1/health"
if ! wait_for_health "$health_url" 30; then
  systemctl status "$SERVICE_NAME" --no-pager || true
  die "新版本健康检查失败"
fi

deployment_changed=0
printf '部署完成: version=%s url=%s scheduler=%s\n' "$VERSION" "$health_url" "$SCHEDULER_ENABLED"
