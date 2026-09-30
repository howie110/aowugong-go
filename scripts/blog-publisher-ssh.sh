#!/usr/bin/env bash
set -euo pipefail
# 安装为发布账号 authorized_keys 的 forced command，仅允许这两个动作。
root="${BLOG_CONTENT_ROOT:-/opt/aowugong-go/shared/storage/blog}"
binary="${BLOG_BINARY:-/opt/aowugong-go/current/aowugong}"
command="${SSH_ORIGINAL_COMMAND:-}"
if [[ ! "$command" =~ ^(upload|publish)\ ([1-9][0-9]*)$ ]]; then
  echo '只允许 upload <序号> 或 publish <序号>' >&2
  exit 1
fi
action="${BASH_REMATCH[1]}"
sequence="${BASH_REMATCH[2]}"
archive="$root/incoming/$sequence.tar.gz"
if [[ "$action" == upload ]]; then
  umask 027
  temporary="$(mktemp "$root/incoming/.upload-XXXXXXXX")"
  trap 'rm -f -- "$temporary"' EXIT
  head -c 67108865 > "$temporary"
  size="$(wc -c < "$temporary")"
  if (( size > 67108864 )); then echo '内容包超过 64 MiB' >&2; exit 1; fi
  mv -- "$temporary" "$archive"
else
  "$binary" blog publish --archive "$archive" --root "$root" --sequence "$sequence"
  rm -- "$archive"
fi
