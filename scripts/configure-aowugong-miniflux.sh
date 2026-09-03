#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${AOWUGONG_ENV_FILE:-/opt/aowugong-go/shared/.env}"
TOKEN_FILE="${MINIFLUX_TOKEN_FILE:-/etc/miniflux/aowugong-api-token}"
BASE_URL="${MINIFLUX_BASE_URL:-http://127.0.0.1:5000}"
MONITOR_URL="${MINIFLUX_MONITOR_URL:-https://miniflux.aowugong.top/}"
CATEGORY="${MINIFLUX_CATEGORY:-投资文章}"

# require_file 校验服务器私有配置文件存在且不为空。
# 输入：文件路径和用途名称。
# 输出：校验成功时无输出，失败时终止脚本。
# 副作用：只读访问文件。
require_file() {
    # 1. 同时校验普通文件和非空内容。
    local path="$1"
    local name="$2"
    if [[ ! -f "${path}" || ! -s "${path}" ]]; then
        echo "${name} 不存在或为空：${path}" >&2
        exit 1
    fi
}

# configure_env 原子更新 aowugong 正式环境中的 Miniflux 参数。
# 输入：全局环境文件、Token 文件和连接参数。
# 输出：成功时打印环境文件路径，不输出 Token。
# 副作用：替换正式环境文件并删除旧 WeChatRSS 配置项。
configure_env() {
    # 1. 校验文件并读取不含换行的 Token。
    require_file "${ENV_FILE}" "aowugong 环境文件"
    require_file "${TOKEN_FILE}" "Miniflux API Token"
    local token
    token="$(tr -d '\r\n' <"${TOKEN_FILE}")"
    if [[ -z "${token}" || "${token}" == *$'\n'* ]]; then
        echo "Miniflux API Token 格式无效" >&2
        exit 1
    fi

    # 2. 在同目录生成新文件，保留原文件所有权和权限后原子替换。
    local directory temporary
    directory="$(dirname "${ENV_FILE}")"
    temporary="$(mktemp "${directory}/.env.miniflux.XXXXXX")"
    trap 'rm -f -- "${temporary}"' EXIT
    awk '!/^(MINIFLUX_(BASE_URL|MONITOR_URL|API_TOKEN|CATEGORY)|INVESTMENT_ARTICLE_AGGREGATE_RSS_URL|WECHAT_RSS_MONITOR_URL)=/' "${ENV_FILE}" >"${temporary}"
    {
        printf '\nMINIFLUX_BASE_URL=%s\n' "${BASE_URL}"
        printf 'MINIFLUX_MONITOR_URL=%s\n' "${MONITOR_URL}"
        printf 'MINIFLUX_API_TOKEN=%s\n' "${token}"
        printf 'MINIFLUX_CATEGORY=%s\n' "${CATEGORY}"
    } >>"${temporary}"
    chown --reference="${ENV_FILE}" "${temporary}"
    chmod --reference="${ENV_FILE}" "${temporary}"
    mv -f -- "${temporary}" "${ENV_FILE}"
    trap - EXIT

    # 3. 只打印安全结果，不回显任何密钥。
    echo "Miniflux 配置已写入：${ENV_FILE}"
}

configure_env
