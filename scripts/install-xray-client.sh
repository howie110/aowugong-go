#!/usr/bin/env bash
set -euo pipefail

# 本脚本安装 Xray 客户端并注册 systemd 服务。
# 输入：第一个参数是已解压的官方 Linux amd64 目录，第二个参数是已限制本机监听的 Xray JSON 配置。
# 输出：安装 Xray 二进制、资源文件、私密配置和 systemd 服务。
# 副作用：写入 /usr/local、创建 xray 系统用户并启动 xray 服务。

package_dir="${1:-}"
config_path="${2:-}"

# 1. 校验安装包和配置文件。
if [[ -z "${package_dir}" || -z "${config_path}" ]]; then
  echo "用法: $0 <Xray-linux-64目录> <config.json>" >&2
  exit 2
fi
if [[ ! -f "${package_dir}/xray" || ! -f "${package_dir}/geoip.dat" || ! -f "${package_dir}/geosite.dat" || ! -f "${config_path}" ]]; then
  echo "Xray 文件目录或配置文件不完整" >&2
  exit 2
fi

# 2. 准备专用用户和安装目录。
if ! id xray >/dev/null 2>&1; then
  useradd --system --no-create-home --shell /sbin/nologin xray
fi
install -d -m 0755 /usr/local/share/xray /usr/local/etc/xray

# 3. 安装官方二进制及规则资源。
install -m 0755 "${package_dir}/xray" /usr/local/bin/xray
install -m 0644 "${package_dir}/geoip.dat" /usr/local/share/xray/geoip.dat
install -m 0644 "${package_dir}/geosite.dat" /usr/local/share/xray/geosite.dat
install -o xray -g xray -m 0600 "${config_path}" /usr/local/etc/xray/config.json

# 4. 创建仅供系统服务使用的 Xray 单元。
cat >/etc/systemd/system/xray.service <<'UNIT'
[Unit]
Description=Xray local proxy client
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=xray
Group=xray
Environment=XRAY_LOCATION_ASSET=/usr/local/share/xray
ExecStart=/usr/local/bin/xray run -c /usr/local/etc/xray/config.json
Restart=on-failure
RestartSec=5s
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
UNIT

# 5. 校验配置并启动服务。
/usr/local/bin/xray run -test -c /usr/local/etc/xray/config.json
systemctl daemon-reload
systemctl enable --now xray
systemctl --no-pager --full status xray
