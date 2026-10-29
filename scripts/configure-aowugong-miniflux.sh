#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${AOWUGONG_ENV_FILE:-/opt/aowugong-go/shared/.env}"
BASE_URL="${MINIFLUX_BASE_URL:-http://127.0.0.1:5000}"
MONITOR_URL="${MINIFLUX_MONITOR_URL:-https://miniflux.aowugong.top/}"

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

# configure_env 原子更新 aowugong 正式环境中的 Miniflux 地址参数。
# 输入：全局环境文件和连接参数。
# 输出：成功时打印环境文件路径。
# 副作用：替换正式环境文件，并删除旧 WeChatRSS 与已废弃的 Token/分类配置项。
configure_env() {
    # 1. 校验环境文件存在且非空。
    require_file "${ENV_FILE}" "aowugong 环境文件"

    # 2. 在同目录生成新文件，保留原文件所有权和权限后原子替换。
    local directory temporary
    directory="$(dirname "${ENV_FILE}")"
    temporary="$(mktemp "${directory}/.env.miniflux.XXXXXX")"
    trap 'rm -f -- "${temporary}"' EXIT
    awk '!/^(MINIFLUX_(BASE_URL|MONITOR_URL|API_TOKEN|CATEGORY)|INVESTMENT_ARTICLE_AGGREGATE_RSS_URL|WECHAT_RSS_MONITOR_URL)=/' "${ENV_FILE}" >"${temporary}"
    {
        printf '\nMINIFLUX_BASE_URL=%s\n' "${BASE_URL}"
        printf 'MINIFLUX_MONITOR_URL=%s\n' "${MONITOR_URL}"
    } >>"${temporary}"
    chown --reference="${ENV_FILE}" "${temporary}"
    chmod --reference="${ENV_FILE}" "${temporary}"
    mv -f -- "${temporary}" "${ENV_FILE}"
    trap - EXIT

    # 3. 只打印安全结果，不回显配置值。
    echo "Miniflux 配置已写入：${ENV_FILE}"
}

configure_env
