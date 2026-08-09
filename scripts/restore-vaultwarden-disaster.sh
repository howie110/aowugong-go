#!/usr/bin/env bash
set -Eeuo pipefail

APP_DIR="${APP_DIR:-/opt/vaultwarden}"
POSTGRES_BIN="${POSTGRES_BIN:-/usr/pgsql-15/bin}"
BACKUP_ARCHIVE="${BACKUP_ARCHIVE:-}"
CONFIRM_DISASTER_RESTORE="${CONFIRM_DISASTER_RESTORE:-}"

# require_restore_inputs 校验全新服务器已完成基础安装且恢复参数明确。
# 输入：通过环境变量提供明文备份归档和确认口令。
# 输出：条件完整时正常返回，否则退出。
# 副作用：无。
require_restore_inputs() {
    # 1. 仅允许 root 在显式确认后执行覆盖恢复。
    if [[ "${EUID}" -ne 0 ]]; then
        printf 'ERROR: 请使用 root 执行\n' >&2
        exit 1
    fi
    if [[ "${CONFIRM_DISASTER_RESTORE}" != "YES" ]]; then
        printf 'ERROR: 请设置 CONFIRM_DISASTER_RESTORE=YES\n' >&2
        exit 1
    fi

    # 2. 要求先运行随包附带的安装脚本，创建新服务器运行环境。
    if [[ -z "${BACKUP_ARCHIVE}" || ! -f "${BACKUP_ARCHIVE}" ]]; then
        printf 'ERROR: 请通过 BACKUP_ARCHIVE 提供已在本地解密的 .tar.gz 备份\n' >&2
        exit 1
    fi
    command -v docker >/dev/null
    [[ -x "${POSTGRES_BIN}/pg_restore" ]]
    [[ -f "${APP_DIR}/compose.yaml" && -f "${APP_DIR}/.env" ]]
}

# restore_vaultwarden 在新服务器恢复密码库数据库和持久文件。
# 输入：已解密的完整 Vaultwarden 备份归档。
# 输出：恢复成功后打印访问配置接口的地址。
# 副作用：停止容器、重建 PostgreSQL 数据库并覆盖 Vaultwarden 持久文件。
restore_vaultwarden() {
    local temporary_directory public_address

    # 1. 解包并完整校验数据库、文件归档和清单。
    temporary_directory="$(mktemp -d /var/tmp/vaultwarden-disaster-restore.XXXXXX)"
    trap "rm -rf -- '${temporary_directory}'" EXIT
    tar -C "${temporary_directory}" -xzf "${BACKUP_ARCHIVE}"
    [[ -f "${temporary_directory}/vaultwarden.dump" ]]
    [[ -f "${temporary_directory}/vaultwarden-files.tar.gz" ]]
    [[ -f "${temporary_directory}/manifest.txt" ]]
    "${POSTGRES_BIN}/pg_restore" --list "${temporary_directory}/vaultwarden.dump" >/dev/null
    tar -tzf "${temporary_directory}/vaultwarden-files.tar.gz" >/dev/null
	chown postgres:postgres "${temporary_directory}/vaultwarden.dump"
	chmod 0750 "${temporary_directory}"

    # 2. 停止新建的空 Vaultwarden，断开连接后重建同名数据库。
    docker compose --project-directory "${APP_DIR}" stop vaultwarden
    runuser -u postgres -- "${POSTGRES_BIN}/psql" --set=ON_ERROR_STOP=1 --dbname=postgres \
        --command="SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'vaultwarden' AND pid <> pg_backend_pid()" >/dev/null
    runuser -u postgres -- "${POSTGRES_BIN}/dropdb" --if-exists vaultwarden
    runuser -u postgres -- "${POSTGRES_BIN}/createdb" --owner=vaultwarden vaultwarden
    runuser -u postgres -- "${POSTGRES_BIN}/pg_restore" --exit-on-error --role=vaultwarden --no-owner --no-privileges \
        --dbname=vaultwarden "${temporary_directory}/vaultwarden.dump"

    # 3. 清除空实例生成的同类文件并恢复附件、Send、配置和签名密钥。
    rm -rf -- "${APP_DIR}/data/attachments" "${APP_DIR}/data/sends"
    rm -f -- "${APP_DIR}/data/rsa_key.der" "${APP_DIR}/data/rsa_key.pem" \
        "${APP_DIR}/data/rsa_key.pub.der" "${APP_DIR}/data/config.json"
    tar -C "${APP_DIR}/data" -xzf "${temporary_directory}/vaultwarden-files.tar.gz"

    # 4. 启动服务并使用新安装环境中的地址验证配置接口。
    docker compose --project-directory "${APP_DIR}" start vaultwarden
    public_address="$(sed -n 's#^VAULTWARDEN_DOMAIN=##p' "${APP_DIR}/.env")"
    sleep 3
    curl --fail --silent --show-error "${public_address}/api/config" >/dev/null
    printf 'Vaultwarden 灾难恢复完成: %s\n' "${public_address}"
}

# main 执行仅面向全新服务器的灾难恢复流程。
# 输入：环境变量提供路径与确认口令。
# 输出：恢复结果。
# 副作用：调用数据库和文件恢复。
main() {
    # 1. 校验条件后执行唯一恢复入口。
    require_restore_inputs
    restore_vaultwarden
}

main "$@"
