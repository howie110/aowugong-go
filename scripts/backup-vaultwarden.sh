#!/usr/bin/env bash
set -Eeuo pipefail

APP_DIR="${APP_DIR:-/opt/vaultwarden}"
BACKUP_DIR="${BACKUP_DIR:-/opt/aowugong-go/shared/storage/backup/vaultwarden}"
POSTGRES_BIN="${POSTGRES_BIN:-/usr/pgsql-15/bin}"
RETENTION="${RETENTION:-14}"
BACKUP_GROUP="${BACKUP_GROUP:-aowugong}"

# collect_vaultwarden_files 归档附件、Send 和签名密钥。
# 输入：目标压缩包路径。
# 输出：生成只含存在文件的压缩包。
# 副作用：读取 Vaultwarden 数据目录并写临时文件。
collect_vaultwarden_files() {
    local output="$1"
    local name
    local -a items=()

    # 1. 收集恢复所需且当前真实存在的文件和目录。
    for name in attachments sends rsa_key.der rsa_key.pem rsa_key.pub.der config.json; do
        if [[ -e "${APP_DIR}/data/${name}" ]]; then
            items+=("${name}")
        fi
    done

    # 2. 始终生成合法压缩包，首次运行没有附件时允许内容为空。
    if [[ "${#items[@]}" -gt 0 ]]; then
        tar -C "${APP_DIR}/data" -czf "${output}" "${items[@]}"
    else
        tar -C "${APP_DIR}/data" -czf "${output}" --files-from /dev/null
    fi
}

# prune_backups 删除超出保留数量的旧备份及校验文件。
# 输入：无，使用全局备份目录和保留数量。
# 输出：保留最新指定份数。
# 副作用：删除旧 Vaultwarden 备份文件。
prune_backups() {
    local old_file

    # 1. 按时间倒序读取保留范围之外的正式备份。
    while IFS= read -r old_file; do
        [[ -n "${old_file}" ]] || continue

        # 2. 同时删除归档和对应 SHA-256 校验文件。
        rm -f -- "${old_file}" "${old_file}.sha256"
    done < <(find "${BACKUP_DIR}" -maxdepth 1 -type f -name 'vaultwarden-*.tar.gz' \
        -printf '%T@ %p\n' | sort -nr | tail -n "+$((RETENTION + 1))" | cut -d' ' -f2-)
}

# main 创建并校验一份 Vaultwarden 一致性备份。
# 输入：环境变量可覆盖目录和保留数量。
# 输出：打印正式备份路径。
# 副作用：读取 PostgreSQL 和数据目录，写备份并清理旧文件。
main() {
    local timestamp temporary_directory final_path
    umask 077

    # 1. 准备同磁盘临时目录，确保最终发布可以原子完成。
    timestamp="$(date +%Y%m%d-%H%M%S)"
    install -d -m 0750 -o root -g "${BACKUP_GROUP}" "${BACKUP_DIR}"
    temporary_directory="$(mktemp -d "${BACKUP_DIR}/.vaultwarden-${timestamp}.XXXXXX")"
    trap "rm -rf -- '${temporary_directory}'" EXIT

    # 2. 使用 pg_dump 创建一致性数据库备份并验证归档目录。
    runuser -u postgres -- "${POSTGRES_BIN}/pg_dump" --format=custom --dbname=vaultwarden \
        >"${temporary_directory}/vaultwarden.dump"
    "${POSTGRES_BIN}/pg_restore" --list "${temporary_directory}/vaultwarden.dump" >/dev/null

    # 3. 归档数据库外的附件、Send 和 JWT 签名密钥。
    collect_vaultwarden_files "${temporary_directory}/vaultwarden-files.tar.gz"
    printf 'created_at=%s\nvaultwarden_version=%s\ndatabase=PostgreSQL\n' \
        "$(date --iso-8601=seconds)" \
        "$(docker inspect vaultwarden --format '{{.Config.Image}}' 2>/dev/null || printf unknown)" \
        >"${temporary_directory}/manifest.txt"

    # 4. 打包、同步并原子发布最终备份及校验值。
    final_path="${BACKUP_DIR}/vaultwarden-${timestamp}.tar.gz"
    tar -C "${temporary_directory}" -czf "${final_path}.tmp" \
        vaultwarden.dump vaultwarden-files.tar.gz manifest.txt
    sync
    mv "${final_path}.tmp" "${final_path}"
    chown root:"${BACKUP_GROUP}" "${final_path}"
    chmod 0640 "${final_path}"
    sha256sum "${final_path}" >"${final_path}.sha256"
    chown root:"${BACKUP_GROUP}" "${final_path}.sha256"
    chmod 0640 "${final_path}.sha256"

    # 5. 清理旧备份并打印本次结果。
    prune_backups
    printf '%s\n' "${final_path}"
}

main "$@"
