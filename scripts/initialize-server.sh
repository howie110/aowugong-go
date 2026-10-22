#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$SCRIPT_DIR/server-release-lib.sh"
APP_ROOT="${APP_ROOT:-/opt/aowugong-go}"
RUN_USER="${RUN_USER:-aowugong}"
ENV_FILE="${ENV_FILE:-}"

# 仅显式初始化时准备机器和共享目录；日常发布不调用此入口。
require_root
for command_name in install useradd swapon swapoff mkswap sysctl df; do require_command "$command_name"; done
if [ ! -f "$APP_ROOT/shared/.env" ]; then
  [ -n "$ENV_FILE" ] && [ -f "$ENV_FILE" ] || die "首次初始化必须通过 ENV_FILE 提供生产配置"
fi
ensure_swap
if ! id "$RUN_USER" >/dev/null 2>&1; then
  useradd --system --home-dir "$APP_ROOT" --create-home --shell /usr/sbin/nologin "$RUN_USER"
fi
RUN_GROUP="$(id -gn "$RUN_USER")"
mkdir -p "$APP_ROOT/releases"
for directory in backup exports uploads temp logs private; do
  install -d -m 0750 -o "$RUN_USER" -g "$RUN_GROUP" "$APP_ROOT/shared/storage/$directory"
done
# 仅调整应用自身目录，不递归改写共享数据与独立服务备份的权限。
chown "$RUN_USER:$RUN_GROUP" "$APP_ROOT/shared" "$APP_ROOT/shared/storage"
if [ ! -f "$APP_ROOT/shared/.env" ]; then
  install -m 0600 -o "$RUN_USER" -g "$RUN_GROUP" "$ENV_FILE" "$APP_ROOT/shared/.env"
fi
printf '初始化完成；随后使用 deploy-release.sh 发布应用。\n'
