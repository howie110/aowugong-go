#!/usr/bin/env bash
set -Eeuo pipefail

ACCESS_FILE="${MINIFLUX_ACCESS_FILE:-/root/miniflux-access.txt}"
PASSWORD_FILE="${MINIFLUX_PASSWORD_FILE:-/etc/miniflux/admin-password}"
BASE_URL="${MINIFLUX_BASE_URL:-http://127.0.0.1:5000}"

# change_password 修改 Miniflux 管理员密码并同步服务器私有记录。
# 输入：NEW_PASSWORD 环境变量是新密码，私有记录提供当前账号和密码。
# 输出：成功时仅打印确认，不回显新旧密码。
# 副作用：调用 Miniflux 用户 API，并替换两个 root 私有密码文件。
change_password() {
    # 1. 校验新密码和现有 root 私有凭据。
    if [[ -z "${NEW_PASSWORD:-}" || "${NEW_PASSWORD}" == *$'\n'* || ${#NEW_PASSWORD} -lt 8 ]]; then
        echo "NEW_PASSWORD 必须是不少于 8 位的单行文本" >&2
        exit 1
    fi
    if [[ ! -s "${ACCESS_FILE}" ]]; then
        echo "Miniflux 登录信息不存在：${ACCESS_FILE}" >&2
        exit 1
    fi
    local username current_password user_id payload status
    username="$(awk -F= '$1 == "USERNAME" {sub(/^[^=]*=/, ""); print; exit}' "${ACCESS_FILE}")"
    current_password="$(awk -F= '$1 == "PASSWORD" {sub(/^[^=]*=/, ""); print; exit}' "${ACCESS_FILE}")"
    if [[ -z "${username}" || -z "${current_password}" ]]; then
        echo "Miniflux 当前登录信息不完整" >&2
        exit 1
    fi

    # 2. 查询当前用户主键并通过官方用户 API 修改密码。
    user_id="$(curl -fsS --user "${username}:${current_password}" "${BASE_URL}/v1/me" |
        python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
    payload="$(NEW_PASSWORD="${NEW_PASSWORD}" python3 -c 'import json,os; print(json.dumps({"password": os.environ["NEW_PASSWORD"]}))')"
    status="$(curl -sS --output /dev/null --write-out '%{http_code}' \
        --user "${username}:${current_password}" --request PUT \
        --header 'Content-Type: application/json' --data "${payload}" \
        "${BASE_URL}/v1/users/${user_id}")"
    if [[ "${status}" != "200" && "${status}" != "201" && "${status}" != "204" ]]; then
        echo "修改 Miniflux 密码失败：HTTP ${status}" >&2
        exit 1
    fi

    # 3. 使用新密码验证 API，再原子更新部署密码和登录记录。
    curl -fsS --user "${username}:${NEW_PASSWORD}" "${BASE_URL}/v1/me" >/dev/null
    local temporary
    temporary="$(mktemp /root/.miniflux-access.XXXXXX)"
    trap 'rm -f -- "${temporary}"' EXIT
    awk -v password="${NEW_PASSWORD}" '$1 ~ /^PASSWORD=/ {$0 = "PASSWORD=" password} {print}' "${ACCESS_FILE}" >"${temporary}"
    chmod 0600 "${temporary}"
    mv -f -- "${temporary}" "${ACCESS_FILE}"
    trap - EXIT
    printf '%s\n' "${NEW_PASSWORD}" >"${PASSWORD_FILE}"
    chown root:root "${PASSWORD_FILE}"
    chmod 0600 "${PASSWORD_FILE}"

    # 4. 返回不含凭据的执行结果。
    echo "Miniflux 管理员密码已修改并验证"
}

change_password
