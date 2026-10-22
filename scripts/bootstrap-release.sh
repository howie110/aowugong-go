#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-${1:-}}"
REPOSITORY="${REPOSITORY:-howie110/aowugong-go}"
[ -n "$VERSION" ] || {
  printf '用法: bootstrap-release.sh <v版本号>\n' >&2
  exit 1
}
command -v curl >/dev/null 2>&1 || {
  printf '缺少命令: curl\n' >&2
  exit 1
}

temporary_directory="$(mktemp -d)"
trap 'rm -rf "$temporary_directory"' EXIT
raw_base="https://raw.githubusercontent.com/${REPOSITORY}/${VERSION}/scripts"
for script in deploy-release.sh server-release-lib.sh initialize-server.sh; do
  curl --fail --location --retry 3 --output "$temporary_directory/$script" "$raw_base/$script"
  chmod 0755 "$temporary_directory/$script"
done
# bootstrap 是首次安装入口；普通升级直接调用 deploy-release.sh。
"$temporary_directory/initialize-server.sh"
"$temporary_directory/deploy-release.sh" "$VERSION"
