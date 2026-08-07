#!/usr/bin/env bash
set -Eeuo pipefail

# probe_url 测试单个订阅地址的 DNS、连接和 HTTP 结果。
# 输入：url 是完整订阅地址。
# 输出：打印 curl 退出码、HTTP 状态、远端 IP、耗时和 URL。
# 副作用：发起一次只读外部 HTTP 请求。
probe_url() {
    # 1. 使用和 Miniflux 接近的超时限制请求，并保留网络层退出码。
    local url="$1"
    local result exit_code=0
    result="$(curl --location --silent --show-error --connect-timeout 5 --max-time 12 \
        --output /dev/null --write-out '%{http_code}|%{remote_ip}|%{time_total}' "${url}" 2>/dev/null)" || exit_code=$?

    # 2. 每个地址输出一行，便于本地与服务器结果直接比较。
    printf '%d|%s|%s\n' "${exit_code}" "${result}" "${url}"
}

# main 依次诊断命令行给出的全部订阅地址。
# 输入：一个或多个 URL 参数。
# 输出：按输入顺序输出网络诊断结果。
# 副作用：调用外部订阅站点，不写文件。
main() {
    # 1. 拒绝空参数，避免产生没有意义的探测。
    if [[ "$#" -eq 0 ]]; then
        echo "至少提供一个订阅 URL" >&2
        exit 1
    fi

    # 2. 逐个请求，避免并发 DNS 查询干扰诊断结论。
    local url
    for url in "$@"; do
        probe_url "${url}"
    done
}

main "$@"
