#!/usr/bin/env bash
set -euo pipefail
# 安装为发布账号 authorized_keys 的 forced command，仅允许这两个动作。
upload_dir="${BLOG_UPLOAD_DIR:-/var/lib/blog-publisher/incoming}"
binary="${BLOG_BINARY:-/opt/aowugong-go/current/aowugong}"
command="${SSH_ORIGINAL_COMMAND:-}"
if [[ ! "$command" =~ ^(upload|publish)\ ([1-9][0-9]*)$ ]]; then
  echo '只允许 upload <序号> 或 publish <序号>' >&2
  exit 1
fi
action="${BASH_REMATCH[1]}"
sequence="${BASH_REMATCH[2]}"
archive="$upload_dir/$sequence.tar.gz"
if [[ "$action" == upload ]]; then
  umask 027
  temporary="$(mktemp "$upload_dir/.upload-XXXXXXXX")"
  trap 'rm -f -- "$temporary"' EXIT
  head -c 67108865 > "$temporary"
  size="$(wc -c < "$temporary")"
  if (( size > 67108864 )); then echo '内容包超过 64 MiB' >&2; exit 1; fi
  mv -- "$temporary" "$archive"
else
  # 仅加载发布账号专用配置，不读取应用 .env。
  source "${BLOG_PUBLISHER_ENV:-/var/lib/blog-publisher/database.env}"
  export BLOG_DATABASE_URL
  "$binary" blog publish --archive "$archive" --sequence "$sequence"
  rm -- "$archive"
fi
