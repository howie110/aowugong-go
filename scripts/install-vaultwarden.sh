#!/usr/bin/env bash
set -Eeuo pipefail

VAULTWARDEN_VERSION="${VAULTWARDEN_VERSION:-1.37.1}"
CADDY_VERSION="${CADDY_VERSION:-2.11.4-alpine}"
CERTBOT_VERSION="${CERTBOT_VERSION:-v5.4.0}"
PUBLIC_IP="${PUBLIC_IP:-}"
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
VAULTWARDEN_DOMAIN=https://${PUBLIC_IP}
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
      - /etc/letsencrypt:/etc/letsencrypt:ro
      - /var/www/certbot:/var/www/certbot:ro
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
EOF

    # 4. 使用公网 IP 证书终止 TLS，并把 HTTP 请求统一跳转到 HTTPS。
    cat >"${APP_DIR}/caddy/Caddyfile" <<EOF
{
    auto_https off
    admin off
    servers :2345 {
        listener_wrappers {
            http_redirect
            tls
        }
    }
}

:80 {
    handle /.well-known/acme-challenge/* {
        root * /var/www/certbot
        file_server
    }
    handle {
        redir https://${PUBLIC_IP}{uri} 308
    }
}

:443 {
    tls /etc/letsencrypt/live/vaultwarden-ip/fullchain.pem /etc/letsencrypt/live/vaultwarden-ip/privkey.pem
    encode zstd gzip
    header {
        Strict-Transport-Security "max-age=31536000"
        X-Content-Type-Options "nosniff"
        Referrer-Policy "no-referrer"
    }
    reverse_proxy 127.0.0.1:8222
}

:2345 {
    tls /etc/letsencrypt/live/vaultwarden-ip/fullchain.pem /etc/letsencrypt/live/vaultwarden-ip/privkey.pem
    encode zstd gzip
    header {
        Strict-Transport-Security "max-age=31536000"
        X-Content-Type-Options "nosniff"
        Referrer-Policy "same-origin"
    }
    reverse_proxy 127.0.0.1:12345
}
EOF
}

# issue_ip_certificate 首次申请 Let’s Encrypt 公网 IP 短期证书。
# 输入：无。
# 输出：证书已存在或申请成功时正常返回。
# 副作用：临时监听 80 端口、写证书目录并访问 Let’s Encrypt。
issue_ip_certificate() {
    local certificate_path="/etc/letsencrypt/live/vaultwarden-ip/fullchain.pem"

    # 1. 已有未损坏证书时不重复申请。
    if [[ -s "${certificate_path}" ]]; then
        return
    fi

    # 2. 临时启动只服务 ACME 文件的轻量 HTTP 容器。
    docker pull busybox:1.37
    docker rm -f vaultwarden-acme-http >/dev/null 2>&1 || true
    docker run -d --rm --name vaultwarden-acme-http \
        -p 80:80 -v /var/www/certbot:/www:ro busybox:1.37 \
        httpd -f -p 80 -h /www >/dev/null
    trap 'docker rm -f vaultwarden-acme-http >/dev/null 2>&1 || true' RETURN

    # 3. 使用 Certbot 5.4 的 IP 地址支持申请 6 天短期证书。
    docker run --rm \
        -v /etc/letsencrypt:/etc/letsencrypt \
        -v /var/lib/letsencrypt:/var/lib/letsencrypt \
        -v /var/log/letsencrypt:/var/log/letsencrypt \
        -v /var/www/certbot:/var/www/certbot \
        "certbot/certbot:${CERTBOT_VERSION}" certonly \
        --non-interactive --agree-tos --register-unsafely-without-email \
        --preferred-profile shortlived --webroot --webroot-path /var/www/certbot \
        --cert-name vaultwarden-ip --ip-address "${PUBLIC_IP}"

    # 4. 停止临时入口，正式入口随后由 Caddy 接管。
    docker rm -f vaultwarden-acme-http >/dev/null 2>&1 || true
    docker image rm busybox:1.37 >/dev/null 2>&1 || true
    trap - RETURN
}

# install_systemd_units 安装备份和证书续期定时器。
# 输入：无。
# 输出：两个定时器已启用。
# 副作用：写 systemd unit 并重载管理器。
install_systemd_units() {
    # 1. 安装脚本到固定系统路径，避免发布目录切换影响任务。
    install -m 0750 "$(dirname "$0")/backup-vaultwarden.sh" /usr/local/sbin/vaultwarden-backup
    install -m 0750 "$(dirname "$0")/renew-vaultwarden-certificate.sh" /usr/local/sbin/vaultwarden-certificate-renew

    # 2. 安装每日备份服务和定时器。
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

    # 3. 安装每日证书续期服务和定时器。
    cat >/etc/systemd/system/vaultwarden-certificate-renew.service <<'EOF'
[Unit]
Description=Renew Vaultwarden IP certificate
After=docker.service network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/vaultwarden-certificate-renew
EOF

    cat >/etc/systemd/system/vaultwarden-certificate-renew.timer <<'EOF'
[Unit]
Description=Daily Vaultwarden IP certificate renewal check

[Timer]
OnCalendar=*-*-* 04:20:00
Persistent=true
RandomizedDelaySec=10m

[Install]
WantedBy=timers.target
EOF

    # 4. 重载 systemd 并启用两个定时器。
    systemctl daemon-reload
    systemctl enable --now vaultwarden-backup.timer vaultwarden-certificate-renew.timer
}

# main 部署 Vaultwarden、HTTPS 和自动维护任务。
# 输入：通过环境变量覆盖版本、IP 和目录。
# 输出：打印访问地址和服务状态。
# 副作用：启动 Docker、写 PostgreSQL、申请证书并启动容器。
main() {
    local database_password attempt

    # 1. 校验运行条件和公网入口端口。
    require_root
	if [[ -z "${PUBLIC_IP}" ]]; then
		printf 'ERROR: 请通过 PUBLIC_IP 提供当前服务器公网 IP\n' >&2
		exit 1
	fi
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
    docker pull "certbot/certbot:${CERTBOT_VERSION}"

    # 4. 申请公网 IP 证书并启动正式容器。
    issue_ip_certificate
    docker compose --project-directory "${APP_DIR}" up -d
    docker compose --project-directory "${APP_DIR}" restart caddy

    # 5. 安装维护任务，等待 HTTPS 就绪后生成首份备份。
    install_systemd_units
    for attempt in $(seq 1 30); do
        if curl --fail --silent "https://${PUBLIC_IP}/api/config" >/dev/null; then
            break
        fi
        sleep 1
    done
    curl --fail --silent --show-error "https://${PUBLIC_IP}/api/config" >/dev/null
    /usr/local/sbin/vaultwarden-backup
    printf 'Vaultwarden 已部署: https://%s\n' "${PUBLIC_IP}"
}

main "$@"
