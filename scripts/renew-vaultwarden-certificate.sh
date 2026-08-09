#!/usr/bin/env bash
set -Eeuo pipefail

APP_DIR="${APP_DIR:-/opt/vaultwarden}"
CERTBOT_VERSION="${CERTBOT_VERSION:-v5.4.0}"
CERTIFICATE_PATH="${CERTIFICATE_PATH:-/etc/letsencrypt/live/vaultwarden-ip/fullchain.pem}"

# certificate_fingerprint 读取当前证书 SHA-256 指纹。
# 输入：无。
# 输出：证书存在时打印指纹，否则打印 missing。
# 副作用：无。
certificate_fingerprint() {
    # 1. 用稳定指纹判断 Certbot 是否真正更新了证书。
    if [[ -s "${CERTIFICATE_PATH}" ]]; then
        openssl x509 -in "${CERTIFICATE_PATH}" -noout -fingerprint -sha256
    else
        printf 'missing\n'
    fi
}

# main 检查并续期 Vaultwarden 公网 IP 证书。
# 输入：环境变量可覆盖运行目录和 Certbot 版本。
# 输出：证书有效时正常返回。
# 副作用：访问 Let’s Encrypt，证书变化时重启 Caddy。
main() {
    local before after

    # 1. 记录续期前指纹并运行官方 Certbot 容器。
    before="$(certificate_fingerprint)"
    docker run --rm \
        -v /etc/letsencrypt:/etc/letsencrypt \
        -v /var/lib/letsencrypt:/var/lib/letsencrypt \
        -v /var/log/letsencrypt:/var/log/letsencrypt \
        -v /var/www/certbot:/var/www/certbot \
        "certbot/certbot:${CERTBOT_VERSION}" renew --cert-name vaultwarden-ip --quiet

    # 2. 证书变化时重启 Caddy，使新证书立即生效。
    after="$(certificate_fingerprint)"
    if [[ "${before}" != "${after}" ]]; then
        docker compose --project-directory "${APP_DIR}" restart caddy
    fi

    # 3. 校验证书至少还能使用一天，并验证公网接口。
    openssl x509 -checkend 86400 -noout -in "${CERTIFICATE_PATH}"
    curl --fail --silent --show-error "https://8.138.123.59/api/config" >/dev/null
}

main "$@"
