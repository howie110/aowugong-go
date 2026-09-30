#!/usr/bin/env bash
set -euo pipefail
# 只发布文章，不加载应用 .env、不重启服务、不触发通知。
archive="${1:?需要内容压缩包}"
sequence="${2:?需要发布序号}"
root="${BLOG_CONTENT_ROOT:-/opt/aowugong-go/shared/storage/blog}"
binary="${BLOG_BINARY:-/opt/aowugong-go/current/aowugong}"
[[ "$sequence" =~ ^[1-9][0-9]*$ ]] || { echo '发布序号无效' >&2; exit 1; }
exec "$binary" blog publish --archive "$archive" --root "$root" --sequence "$sequence"
