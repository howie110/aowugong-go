#!/usr/bin/env bash
set -euo pipefail

# 本脚本配置 Miniflux 的按订阅代理地址。
# 输入：第一个参数是 Miniflux 配置文件，第二个参数是本机 HTTP 代理地址。
# 输出：更新配置并重启 Miniflux。
# 副作用：备份并写入 Miniflux 配置，短暂重启 miniflux 服务。

config_path="${1:-/etc/miniflux/miniflux.conf}"
proxy_url="${2:-http://127.0.0.1:6152}"

# 1. 校验配置文件及本机代理监听状态。
if [[ ! -f "${config_path}" ]]; then
  echo "Miniflux 配置文件不存在: ${config_path}" >&2
  exit 2
fi
if ! curl --silent --show-error --proxy "${proxy_url}" --connect-timeout 8 --max-time 20 --output /dev/null https://miniflux.app/; then
  echo "本机代理不可用: ${proxy_url}" >&2
  exit 1
fi

# 2. 原样备份配置并替换代理参数。
backup_path="${config_path}.bak.$(date +%Y%m%d%H%M%S)"
cp -a "${config_path}" "${backup_path}"
temp_path="$(mktemp)"
trap 'rm -f "${temp_path}"' EXIT
grep -v '^HTTP_CLIENT_PROXY=' "${config_path}" >"${temp_path}" || true
printf '\nHTTP_CLIENT_PROXY=%s\n' "${proxy_url}" >>"${temp_path}"
cat "${temp_path}" >"${config_path}"

# 3. 重启 Miniflux 并检查服务状态。
systemctl restart miniflux
systemctl is-active --quiet miniflux
systemctl --no-pager --full status miniflux
