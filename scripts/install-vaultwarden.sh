#!/usr/bin/env bash
set -Eeuo pipefail

VAULTWARDEN_VERSION="${VAULTWARDEN_VERSION:-1.37.1}"
CADDY_VERSION="${CADDY_VERSION:-2.11.4-alpine}"
VAULTWARDEN_HOST="${VAULTWARDEN_HOST:-vault.aowugong.top}"
AOWUGONG_HOST="${AOWUGONG_HOST:-aowugong.top}"
MINIFLUX_HOST="${MINIFLUX_HOST:-miniflux.aowugong.top}"
NEXTFLUX_HOST="${NEXTFLUX_HOST:-nextflux.aowugong.top}"
APP_DIR="${APP_DIR:-/opt/vaultwarden}"
BACKUP_DIR="${BACKUP_DIR:-/opt/aowugong-go/shared/storage/backup/vaultwarden}"
POSTGRES_BIN="${POSTGRES_BIN:-/usr/pgsql-15/bin}"

# require_root 校验脚本必须由 root 执行。
# 输入：无。
# 输出：权限不足时退出。
# 副作用：无。
require_root() {
    # 1. 拒绝非 root 用户修改服务、数据库和证书目录。
    if [[ "${EUID}" -ne 0 ]]; then
        printf 'ERROR: 请使用 root 执行\n' >&2
        exit 1
    fi
}

# ensure_port_available 校验公网入口端口未被其他服务占用。
# 输入：端口号。
# 输出：端口空闲时正常返回，占用时退出。
# 副作用：无。
ensure_port_available() {
    local port="$1"

    # 1. 忽略本脚本已经部署的 Caddy，其余监听进程均视为冲突。
    if ss -lntp | grep -Eq ":${port}[[:space:]]"; then
        if ! docker ps --format '{{.Names}}' | grep -qx 'vaultwarden-caddy'; then
            printf 'ERROR: 端口 %s 已被占用\n' "${port}" >&2
            exit 1
        fi
    fi
}

# ensure_postgres_database 创建 Vaultwarden 专用角色和数据库。
# 输入：数据库密码。
# 输出：数据库可连接时正常返回。
# 副作用：写 PostgreSQL 角色和数据库元数据。
ensure_postgres_database() {
    local password="$1"
    local role_exists database_exists

    # 1. 创建或更新专用登录角色，密码仅保存到服务器私有环境文件。
    role_exists="$(runuser -u postgres -- "${POSTGRES_BIN}/psql" -At --dbname=postgres \
        --command="SELECT 1 FROM pg_roles WHERE rolname = 'vaultwarden'")"
    if [[ "${role_exists}" == "1" ]]; then
        runuser -u postgres -- "${POSTGRES_BIN}/psql" --set=ON_ERROR_STOP=1 --dbname=postgres \
            --command="ALTER ROLE vaultwarden WITH LOGIN PASSWORD '${password}'" >/dev/null
    else
        runuser -u postgres -- "${POSTGRES_BIN}/psql" --set=ON_ERROR_STOP=1 --dbname=postgres \
            --command="CREATE ROLE vaultwarden WITH LOGIN PASSWORD '${password}'" >/dev/null
    fi

    # 2. 只在数据库不存在时创建，并固定由专用角色拥有。
    database_exists="$(runuser -u postgres -- "${POSTGRES_BIN}/psql" -At --dbname=postgres \
        --command="SELECT 1 FROM pg_database WHERE datname = 'vaultwarden'")"
    if [[ "${database_exists}" != "1" ]]; then
        runuser -u postgres -- "${POSTGRES_BIN}/createdb" --owner=vaultwarden vaultwarden
    fi
}

# write_runtime_files 写入 Compose、Caddy 和私有运行配置。
# 输入：数据库密码。
# 输出：生成可由 Docker Compose 使用的配置文件。
# 副作用：写 /opt/vaultwarden 下的运行文件。
write_runtime_files() {
    local password="$1"

    # 1. 创建持久化数据、证书验证和备份目录。
    install -d -m 0750 "${APP_DIR}" "${APP_DIR}/data" "${APP_DIR}/caddy-data" "${APP_DIR}/caddy-config"
    install -d -m 0755 "${APP_DIR}/caddy" /var/www/certbot
    install -d -m 0700 "${BACKUP_DIR}"

    # 2. 保存只供 root 读取的数据库地址和公开访问地址。
    cat >"${APP_DIR}/.env" <<EOF
VAULTWARDEN_DATABASE_URL=postgresql://vaultwarden:${password}@127.0.0.1:5432/vaultwarden
VAULTWARDEN_DOMAIN=https://${VAULTWARDEN_HOST}
EOF
    chmod 0600 "${APP_DIR}/.env"

    # 3. 固定官方镜像版本，并仅在宿主机回环地址暴露应用端口。
    cat >"${APP_DIR}/compose.yaml" <<EOF
services:
  vaultwarden:
    image: vaultwarden/server:${VAULTWARDEN_VERSION}-alpine
    container_name: vaultwarden
    restart: unless-stopped
    network_mode: host
    environment:
      DATABASE_URL: \${VAULTWARDEN_DATABASE_URL}
      DOMAIN: \${VAULTWARDEN_DOMAIN}
      ROCKET_ADDRESS: 127.0.0.1
      ROCKET_PORT: 8222
      SIGNUPS_ALLOWED: "false"
      INVITATIONS_ALLOWED: "false"
      SHOW_PASSWORD_HINT: "false"
      TZ: Asia/Shanghai
      LOG_LEVEL: warn
    volumes:
      - ${APP_DIR}/data:/data
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"

  caddy:
    image: caddy:${CADDY_VERSION}
    container_name: vaultwarden-caddy
    restart: unless-stopped
    network_mode: host
    depends_on:
      - vaultwarden
    volumes:
      - ${APP_DIR}/caddy/Caddyfile:/etc/caddy/Caddyfile:ro
      - ${APP_DIR}/caddy-data:/data
      - ${APP_DIR}/caddy-config:/config
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
EOF

    # 4. 只为正式域名启用 HTTPS，所有应用仍只通过本机回环地址互通。
    cat >"${APP_DIR}/caddy/Caddyfile" <<EOF
{
    admin off
}

${AOWUGONG_HOST} {
    encode zstd gzip
    header {
        Strict-Transport-Security "max-age=31536000"
        X-Content-Type-Options "nosniff"
        Referrer-Policy "same-origin"
    }
    reverse_proxy 127.0.0.1:12345
}

www.${AOWUGONG_HOST} {
    redir https://${AOWUGONG_HOST}{uri} 308
}

${VAULTWARDEN_HOST} {
    encode zstd gzip
    header {
        Strict-Transport-Security "max-age=31536000"
        X-Content-Type-Options "nosniff"
        Referrer-Policy "no-referrer"
    }
    reverse_proxy 127.0.0.1:8222
}

${MINIFLUX_HOST} {
    encode zstd gzip
    header {
        Strict-Transport-Security "max-age=31536000"
        X-Content-Type-Options "nosniff"
        Referrer-Policy "same-origin"
    }
    reverse_proxy 127.0.0.1:5000
}

${NEXTFLUX_HOST} {
    encode zstd gzip
    header {
        Strict-Transport-Security "max-age=31536000"
        X-Content-Type-Options "nosniff"
        Referrer-Policy "same-origin"
    }
    reverse_proxy 127.0.0.1:5001
}
EOF
}

# install_systemd_units 安装备份定时器并清理旧的 IP 证书任务。
# 输入：无。
# 输出：备份定时器已启用。
# 副作用：写 systemd unit 并重载管理器。
install_systemd_units() {
    # 1. 安装备份脚本到固定系统路径，避免发布目录切换影响任务。
    install -m 0750 "$(dirname "$0")/backup-vaultwarden.sh" /usr/local/sbin/vaultwarden-backup

    # 2. 停用并移除旧的 IP 证书续期任务，保留证书文件便于回滚检查。
    systemctl disable --now vaultwarden-certificate-renew.timer >/dev/null 2>&1 || true
    rm -f /etc/systemd/system/vaultwarden-certificate-renew.service \
        /etc/systemd/system/vaultwarden-certificate-renew.timer \
        /usr/local/sbin/vaultwarden-certificate-renew

    # 3. 安装每日备份服务和定时器。
    cat >/etc/systemd/system/vaultwarden-backup.service <<'EOF'
[Unit]
Description=Backup Vaultwarden PostgreSQL and files
After=postgresql-15.service docker.service

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/vaultwarden-backup
Nice=10
IOSchedulingClass=best-effort
IOSchedulingPriority=7
EOF

    cat >/etc/systemd/system/vaultwarden-backup.timer <<'EOF'
[Unit]
Description=Daily Vaultwarden backup

[Timer]
OnCalendar=*-*-* 03:45:00
Persistent=true
RandomizedDelaySec=5m

[Install]
WantedBy=timers.target
EOF

    # 4. 重载 systemd 并只启用备份定时器。
    systemctl daemon-reload
    systemctl enable --now vaultwarden-backup.timer
}

# main 部署 Vaultwarden、域名 HTTPS 和自动备份任务。
# 输入：通过环境变量覆盖版本、域名和目录。
# 输出：打印访问地址和服务状态。
# 副作用：启动 Docker、写 PostgreSQL、申请证书并启动容器。
main() {
    local database_password attempt

    # 1. 校验运行条件和公网入口端口。
    require_root
    for host in "${AOWUGONG_HOST}" "${VAULTWARDEN_HOST}" "${MINIFLUX_HOST}" "${NEXTFLUX_HOST}"; do
        if [[ ! "${host}" =~ ^[A-Za-z0-9.-]+$ ]]; then
            printf 'ERROR: 域名格式无效: %s\n' "${host}" >&2
            exit 1
        fi
    done
    ensure_port_available 80
    ensure_port_available 443
    command -v docker >/dev/null
    command -v openssl >/dev/null
    [[ -x "${POSTGRES_BIN}/psql" ]]

    # 2. 启动官方容器运行时，并生成或复用数据库密码。
    systemctl enable --now docker
    if [[ -s "${APP_DIR}/.env" ]]; then
        database_password="$(sed -n 's#^VAULTWARDEN_DATABASE_URL=postgresql://vaultwarden:\([^@]*\)@.*#\1#p' "${APP_DIR}/.env")"
    else
        database_password="$(openssl rand -hex 32)"
    fi
    [[ -n "${database_password}" ]]

    # 3. 创建数据库和运行配置，拉取固定官方镜像。
    ensure_postgres_database "${database_password}"
    write_runtime_files "${database_password}"
    docker pull "vaultwarden/server:${VAULTWARDEN_VERSION}-alpine"
    docker pull "caddy:${CADDY_VERSION}"

    # 4. 启动正式容器，Caddy 根据域名自动申请和续期证书。
    docker compose --project-directory "${APP_DIR}" up -d
    docker compose --project-directory "${APP_DIR}" restart caddy

    # 5. 安装维护任务，等待 HTTPS 就绪后生成首份备份。
    install_systemd_units
    for attempt in $(seq 1 30); do
        if curl --fail --silent "https://${VAULTWARDEN_HOST}/api/config" >/dev/null; then
            break
        fi
        sleep 1
    done
    curl --fail --silent --show-error "https://${VAULTWARDEN_HOST}/api/config" >/dev/null
    /usr/local/sbin/vaultwarden-backup
    printf 'Vaultwarden 已部署: https://%s\n' "${VAULTWARDEN_HOST}"
}

main "$@"
