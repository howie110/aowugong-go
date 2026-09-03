#!/usr/bin/env bash

set -Eeuo pipefail

PG_VERSION="15"
PG_SERVICE="postgresql-${PG_VERSION}.service"
PG_DATA="/var/lib/pgsql/${PG_VERSION}/data"
MINIFLUX_VERSION="2.3.2"
MINIFLUX_SHA256="42db4484d87d045a3e2f99f90d211a210ea3d623a5f08dffa81ffb4dc9467f69"
MINIFLUX_URL="${MINIFLUX_URL:-https://miniflux.aowugong.top}"
MINIFLUX_LISTEN_ADDR="${MINIFLUX_LISTEN_ADDR:-127.0.0.1:5000}"
MINIFLUX_CATEGORY="投资文章"
MINIFLUX_CONFIG_DIR="/etc/miniflux"
MINIFLUX_CONFIG="${MINIFLUX_CONFIG_DIR}/miniflux.conf"
MINIFLUX_ACCESS_FILE="/root/miniflux-access.txt"

# set_pg_value 更新 PostgreSQL 单项配置，重复执行时不会追加重复配置。
# 输入：配置名和 PostgreSQL 配置值。
# 输出：无。
# 副作用：修改 PostgreSQL 主配置文件。
set_pg_value() {
    local key="$1"
    local value="$2"
    local config_file="${PG_DATA}/postgresql.conf"

    # 1. 替换已有配置；不存在时追加到文件末尾。
    if grep -Eq "^[#[:space:]]*${key}[[:space:]]*=" "${config_file}"; then
        sed -ri "s|^[#[:space:]]*${key}[[:space:]]*=.*|${key} = ${value}|" "${config_file}"
    else
        printf '%s = %s\n' "${key}" "${value}" >>"${config_file}"
    fi
}

# install_postgres 安装并初始化 PostgreSQL 15，应用低内存配置。
# 输入：无。
# 输出：无。
# 副作用：安装 RPM、初始化数据库目录并启用 systemd 服务。
install_postgres() {
    # 1. 安装 PGDG 软件源和 PostgreSQL 15。
    if ! rpm -q pgdg-redhat-repo >/dev/null 2>&1; then
        yum install -y https://download.postgresql.org/pub/repos/yum/reporpms/EL-7-x86_64/pgdg-redhat-repo-latest.noarch.rpm
    fi
    yum --disablerepo='pgdg*' --enablerepo=pgdg15,pgdg-common install -y \
        postgresql15 postgresql15-server postgresql15-contrib

    # 2. 首次安装时初始化数据库集群。
    if [[ ! -f "${PG_DATA}/PG_VERSION" ]]; then
        /usr/pgsql-15/bin/postgresql-15-setup initdb
    fi

    # 3. 限制为本机访问并按小内存服务器调优。
    set_pg_value "listen_addresses" "'127.0.0.1'"
    set_pg_value "max_connections" "20"
    set_pg_value "shared_buffers" "32MB"
    set_pg_value "effective_cache_size" "128MB"
    set_pg_value "work_mem" "2MB"
    set_pg_value "maintenance_work_mem" "32MB"
    set_pg_value "synchronous_commit" "on"

    # 4. 为 Miniflux 增加独立的本机密码认证规则。
    if ! grep -Fq "host miniflux miniflux 127.0.0.1/32 scram-sha-256" "${PG_DATA}/pg_hba.conf"; then
        sed -i '1ihost miniflux miniflux 127.0.0.1/32 scram-sha-256' "${PG_DATA}/pg_hba.conf"
    fi

    # 5. 启动 PostgreSQL 并设置开机自启。
    systemctl enable "${PG_SERVICE}"
    systemctl restart "${PG_SERVICE}"
}

# create_miniflux_database 创建独立数据库账户和数据库。
# 输入：数据库密码文件路径。
# 输出：无。
# 副作用：写入 PostgreSQL 角色和数据库元数据。
create_miniflux_database() {
    local password_file="$1"
    local database_password
    database_password="$(<"${password_file}")"

    # 1. 创建或更新 Miniflux 数据库角色。
    runuser -u postgres -- /usr/pgsql-15/bin/psql \
        --set=ON_ERROR_STOP=1 \
        --set=db_password="${database_password}" \
        --dbname=postgres <<'SQL'
SELECT format('CREATE ROLE miniflux LOGIN PASSWORD %L', :'db_password')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'miniflux') \gexec
SELECT format('ALTER ROLE miniflux WITH LOGIN PASSWORD %L', :'db_password') \gexec
SELECT 'CREATE DATABASE miniflux OWNER miniflux'
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'miniflux') \gexec
SQL

    # 2. 确保数据库归属正确并验证独立账户可连接。
    runuser -u postgres -- /usr/pgsql-15/bin/psql --set=ON_ERROR_STOP=1 \
        --dbname=postgres --command='ALTER DATABASE miniflux OWNER TO miniflux'
    PGPASSWORD="${database_password}" /usr/pgsql-15/bin/psql \
        --host=127.0.0.1 --username=miniflux --dbname=miniflux \
        --set=ON_ERROR_STOP=1 --command='SELECT 1' >/dev/null
}

# install_miniflux_binary 下载并校验 Miniflux 官方二进制。
# 输入：无。
# 输出：无。
# 副作用：写入 /usr/local/bin 并创建 miniflux 系统用户。
install_miniflux_binary() {
	local temp_dir
	local installed_hash=""
	local versioned_binary="/usr/local/bin/miniflux-${MINIFLUX_VERSION}"
	temp_dir="$(mktemp -d)"

    # 1. 创建最小权限的运行账户。
    if ! id miniflux >/dev/null 2>&1; then
        useradd --system --home-dir /var/lib/miniflux --create-home --shell /sbin/nologin miniflux
    fi

	# 2. 已安装文件校验正确时直接复用。
	if [[ -f "${versioned_binary}" ]]; then
		read -r installed_hash _ < <(sha256sum "${versioned_binary}")
	fi
	if [[ "${installed_hash}" == "${MINIFLUX_SHA256}" ]]; then
		chmod 0755 "${versioned_binary}"
		ln -sfn "${versioned_binary}" /usr/local/bin/miniflux
		rm -rf -- "${temp_dir}"
		return
	fi

	# 3. 下载二进制和官方校验文件。
    curl -fsSL --retry 3 \
        "https://github.com/miniflux/v2/releases/download/${MINIFLUX_VERSION}/miniflux-linux-amd64" \
        --output "${temp_dir}/miniflux-linux-amd64"
    curl -fsSL --retry 3 \
        "https://github.com/miniflux/v2/releases/download/${MINIFLUX_VERSION}/miniflux-linux-amd64.sha256" \
        --output "${temp_dir}/miniflux-linux-amd64.sha256"

	# 4. 校验成功后安装版本化文件并更新软链接。
    (
        cd "${temp_dir}"
        sha256sum --check miniflux-linux-amd64.sha256
    )
	install -m 0755 "${temp_dir}/miniflux-linux-amd64" "${versioned_binary}"
	ln -sfn "${versioned_binary}" /usr/local/bin/miniflux
    rm -rf -- "${temp_dir}"
}

# configure_miniflux 写入运行配置和 systemd 服务。
# 输入：数据库密码文件路径。
# 输出：无。
# 副作用：写入 /etc/miniflux 和 systemd 配置。
configure_miniflux() {
    local password_file="$1"
    local database_password
    database_password="$(<"${password_file}")"

    # 1. 写入仅 Miniflux 可读的数据库连接信息。
    install -d -m 0750 -o root -g miniflux "${MINIFLUX_CONFIG_DIR}"
    printf 'host=127.0.0.1 port=5432 user=miniflux password=%s dbname=miniflux sslmode=disable\n' \
        "${database_password}" >"${MINIFLUX_CONFIG_DIR}/database-url"
    chown root:miniflux "${MINIFLUX_CONFIG_DIR}/database-url"
    chmod 0640 "${MINIFLUX_CONFIG_DIR}/database-url"

    # 2. 配置低并发抓取、完整保留条目和 API 服务。
    cat >"${MINIFLUX_CONFIG}" <<EOF
DATABASE_URL_FILE=${MINIFLUX_CONFIG_DIR}/database-url
LISTEN_ADDR=${MINIFLUX_LISTEN_ADDR}
BASE_URL=${MINIFLUX_URL}
RUN_MIGRATIONS=1
WORKER_POOL_SIZE=4
DATABASE_MAX_CONNS=10
DATABASE_MIN_CONNS=1
BATCH_SIZE=20
POLLING_FREQUENCY=60
POLLING_LIMIT_PER_HOST=1
POLLING_SCHEDULER=entry_frequency
SCHEDULER_ENTRY_FREQUENCY_MIN_INTERVAL=60
SCHEDULER_ENTRY_FREQUENCY_MAX_INTERVAL=1440
CLEANUP_ARCHIVE_READ_DAYS=-1
CLEANUP_ARCHIVE_UNREAD_DAYS=-1
LOG_LEVEL=info
LOG_DATE_TIME=1
WATCHDOG=0
EOF
    chown root:miniflux "${MINIFLUX_CONFIG}"
    chmod 0640 "${MINIFLUX_CONFIG}"

    # 3. 写入资源受控的 systemd 服务。
    cat >/etc/systemd/system/miniflux.service <<EOF
[Unit]
Description=Miniflux Feed Reader
After=network-online.target ${PG_SERVICE}
Wants=network-online.target
Requires=${PG_SERVICE}

[Service]
Type=simple
User=miniflux
Group=miniflux
ExecStart=/usr/local/bin/miniflux -config-file ${MINIFLUX_CONFIG}
Restart=on-failure
RestartSec=5s
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=true
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
}

# initialize_miniflux 初始化表、管理员和 aowugong API 密钥。
# 输入：管理员密码文件路径。
# 输出：无。
# 副作用：写 PostgreSQL、启动 Miniflux，并保存 API 密钥。
initialize_miniflux() {
    local admin_password_file="$1"
    local admin_password
	local api_response
	local api_token
	local categories_response
	local category_exists
	local category_payload
    admin_password="$(<"${admin_password_file}")"

    # 1. 执行数据库迁移。
    runuser -u miniflux -- /usr/local/bin/miniflux -config-file "${MINIFLUX_CONFIG}" -migrate

	# 2. 首次安装时通过一次性启动配置创建管理员。
	systemctl enable miniflux.service
	if [[ "$(runuser -u postgres -- /usr/pgsql-15/bin/psql -At --dbname=miniflux --command="SELECT count(*) FROM users WHERE username = 'admin'")" == "0" ]]; then
		chown root:miniflux "${admin_password_file}"
		chmod 0640 "${admin_password_file}"
		{
			printf 'CREATE_ADMIN=1\n'
			printf 'ADMIN_USERNAME=admin\n'
			printf 'ADMIN_PASSWORD_FILE=%s\n' "${admin_password_file}"
		} >>"${MINIFLUX_CONFIG}"
		systemctl restart miniflux.service
		for _ in $(seq 1 20); do
			if curl -fsS http://127.0.0.1:5000/healthcheck >/dev/null; then
				break
			fi
			sleep 1
		done
		sed -i '/^CREATE_ADMIN=/d; /^ADMIN_USERNAME=/d; /^ADMIN_PASSWORD_FILE=/d' "${MINIFLUX_CONFIG}"
		chown root:root "${admin_password_file}"
		chmod 0600 "${admin_password_file}"
	fi

	# 3. 启动服务并等待数据库健康检查通过。
	systemctl restart miniflux.service
    for _ in $(seq 1 20); do
        if curl -fsS http://127.0.0.1:5000/healthcheck >/dev/null; then
            break
        fi
        sleep 1
    done
    curl -fsS http://127.0.0.1:5000/healthcheck >/dev/null

	# 4. 确保投资文章拥有独立分类，避免 aowugong 读取其他订阅。
	categories_response="$(curl -fsS --user "admin:${admin_password}" http://127.0.0.1:5000/v1/categories)"
	category_exists="$(printf '%s' "${categories_response}" | CATEGORY_NAME="${MINIFLUX_CATEGORY}" python3 -c \
		'import json,os,sys; print("1" if any(item.get("title") == os.environ["CATEGORY_NAME"] for item in json.load(sys.stdin)) else "0")')"
	if [[ "${category_exists}" != "1" ]]; then
		category_payload="$(CATEGORY_NAME="${MINIFLUX_CATEGORY}" python3 -c \
			'import json,os; print(json.dumps({"title": os.environ["CATEGORY_NAME"]}, ensure_ascii=False))')"
		curl -fsS --user "admin:${admin_password}" \
			--header 'Content-Type: application/json' \
			--data "${category_payload}" \
			http://127.0.0.1:5000/v1/categories >/dev/null
	fi

	# 5. 创建专供 aowugong 使用的 API 密钥。
	if [[ ! -s "${MINIFLUX_CONFIG_DIR}/aowugong-api-token" ]]; then
        api_response="$(curl -fsS --user "admin:${admin_password}" \
            --header 'Content-Type: application/json' \
            --data '{"description":"aowugong-go"}' \
            http://127.0.0.1:5000/v1/api-keys)"
        api_token="$(printf '%s' "${api_response}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')"
        printf '%s\n' "${api_token}" >"${MINIFLUX_CONFIG_DIR}/aowugong-api-token"
        chmod 0600 "${MINIFLUX_CONFIG_DIR}/aowugong-api-token"
    fi

	# 6. 保存仅 root 可读的登录信息，便于后续登录和接入。
    {
        printf 'URL=%s\n' "${MINIFLUX_URL}"
		printf 'USERNAME=admin\n'
		printf 'CATEGORY=%s\n' "${MINIFLUX_CATEGORY}"
        printf 'PASSWORD=%s\n' "${admin_password}"
        printf 'API_TOKEN_FILE=%s\n' "${MINIFLUX_CONFIG_DIR}/aowugong-api-token"
    } >"${MINIFLUX_ACCESS_FILE}"
    chmod 0600 "${MINIFLUX_ACCESS_FILE}"
}

# main 完成 PostgreSQL 与 Miniflux 的可重复部署。
# 输入：无。
# 输出：打印非敏感安装结果。
# 副作用：安装服务、创建数据库并监听 5000 端口。
main() {
    local database_password_file="${MINIFLUX_CONFIG_DIR}/database-password"
    local admin_password_file="${MINIFLUX_CONFIG_DIR}/admin-password"

    # 1. 安装并启动 PostgreSQL。
    install_postgres

    # 2. 安装二进制和运行账户，再生成独立随机凭据。
    install_miniflux_binary
    install -d -m 0750 -o root -g miniflux "${MINIFLUX_CONFIG_DIR}"
    if [[ ! -s "${database_password_file}" ]]; then
        openssl rand -hex 24 >"${database_password_file}"
    fi
    if [[ ! -s "${admin_password_file}" ]]; then
        openssl rand -hex 16 >"${admin_password_file}"
    fi
    chmod 0600 "${database_password_file}" "${admin_password_file}"

    # 3. 创建数据库并配置 Miniflux。
    create_miniflux_database "${database_password_file}"
    configure_miniflux "${database_password_file}"
    initialize_miniflux "${admin_password_file}"

    # 4. 输出不含凭据的完成信息。
    echo "Miniflux ${MINIFLUX_VERSION} 已部署：${MINIFLUX_URL}"
    echo "登录信息：${MINIFLUX_ACCESS_FILE}"
    echo "API 密钥：${MINIFLUX_CONFIG_DIR}/aowugong-api-token"
}

main "$@"
