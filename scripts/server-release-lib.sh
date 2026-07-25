#!/usr/bin/env bash

# server-release-lib.sh 集中保存发布、切换和回滚脚本共用的服务器操作。

# die 输出错误并终止当前发布脚本。
# 输入：任意错误上下文文本。
# 输出：向标准错误输出带前缀的信息。
# 副作用：以状态码 1 结束当前进程。
die() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

# require_root 校验当前脚本由 root 执行。
# 输入：无。
# 输出：无；非 root 时终止。
# 副作用：无。
require_root() {
  [ "$(id -u)" -eq 0 ] || die "请使用 root 执行服务器发布脚本"
}

# require_command 校验服务器具备指定命令。
# 输入：命令名称。
# 输出：无；命令缺失时终止。
# 副作用：无。
require_command() {
  command -v "$1" >/dev/null 2>&1 || die "缺少命令: $1"
}

# set_env_value 原子设置环境文件中的单个键值。
# 输入：环境文件、键名和值。
# 输出：无；读写失败时终止。
# 副作用：覆盖指定环境文件并保留原所有者。
set_env_value() {
  local file="$1"
  local key="$2"
  local value="$3"
  local temp
  local owner
  local group
  temp="$(mktemp)"
  awk -v key="$key" -v value="$value" '
    BEGIN { replaced = 0 }
    $0 ~ "^" key "=" { print key "=" value; replaced = 1; next }
    { print }
    END { if (!replaced) print key "=" value }
  ' "$file" > "$temp"
  owner="$(stat -c '%u' "$file" 2>/dev/null || id -u)"
  group="$(stat -c '%g' "$file" 2>/dev/null || id -g)"
  install -m 0600 -o "$owner" -g "$group" "$temp" "$file"
  rm -f "$temp"
}

# set_env_default 仅在键为空或缺失时写入默认值。
# 输入：环境文件、键名和默认值。
# 输出：无。
# 副作用：必要时修改指定环境文件。
set_env_default() {
  local file="$1"
  local key="$2"
  local value="$3"
  [ -n "$(read_env_value "$file" "$key")" ] || set_env_value "$file" "$key" "$value"
}

# read_env_value 读取环境文件中指定键的首个值。
# 输入：环境文件和键名。
# 输出：输出值；键不存在时输出空字符串。
# 副作用：无。
read_env_value() {
  local file="$1"
  local key="$2"
  awk -F= -v key="$key" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "$file"
}

# wait_for_health 重试访问服务健康地址。
# 输入：健康地址和可选重试次数。
# 输出：成功返回 0，耗尽重试返回 1。
# 副作用：发起本机 HTTP 请求并在失败间隔休眠。
wait_for_health() {
  local url="$1"
  local attempts="${2:-30}"
  local index
  for index in $(seq 1 "$attempts"); do
    if curl --fail --silent --show-error --max-time 3 "$url" >/dev/null; then
      return 0
    fi
    sleep 2
  done
  return 1
}

# point_symlink 原子切换发布软链接。
# 输入：软链接路径和目标目录。
# 输出：无；切换失败时终止。
# 副作用：创建或替换指定软链接。
point_symlink() {
  local link_path="$1"
  local target_path="$2"
  local temporary="${link_path}.next"
  ln -sfn "$target_path" "$temporary"
  mv -Tf "$temporary" "$link_path"
}

# ensure_swap 在服务器没有 Swap 时创建 2GB swapfile。
# 输入：无。
# 输出：无；条件异常时终止。
# 副作用：可能创建 /swapfile、修改 fstab 和 swappiness。
ensure_swap() {
  local swap_state
  local available_kb
  local fstab_added=0
  local sysctl_path="/etc/sysctl.d/99-aowugong-go-swap.conf"
  swap_state="$(swapon --show --noheadings 2>/dev/null || true)"
  if [ -n "$swap_state" ]; then
    printf 'Swap 已存在，不做修改。\n'
    return 0
  fi

  printf '未发现 Swap，创建 2GB /swapfile。\n'
  if [ -e /swapfile ]; then
    die "/swapfile 已存在但未启用，请人工确认后再部署"
  fi
  [ ! -e "$sysctl_path" ] || die "$sysctl_path 已存在但系统没有 Swap，请人工确认"
  available_kb="$(df -Pk / | awk 'NR == 2 { print $4 }')"
  printf '%s' "$available_kb" | grep -Eq '^[0-9]+$' || die "无法读取根文件系统剩余空间"
  [ "$available_kb" -ge 2621440 ] || die "根文件系统剩余空间不足 2.5GB，拒绝创建 2GB swapfile"
  if command -v fallocate >/dev/null 2>&1; then
    fallocate -l 2G /swapfile || {
      rm -f /swapfile
      die "创建 /swapfile 失败"
    }
  else
    dd if=/dev/zero of=/swapfile bs=1M count=2048 status=progress || {
      rm -f /swapfile
      die "写入 /swapfile 失败"
    }
  fi
  chmod 0600 /swapfile || {
    rm -f /swapfile
    die "设置 /swapfile 权限失败"
  }
  mkswap /swapfile >/dev/null || {
    rm -f /swapfile
    die "格式化 /swapfile 失败"
  }
  swapon /swapfile || {
    rm -f /swapfile
    die "启用 /swapfile 失败"
  }
  if ! grep -Eq '^/swapfile[[:space:]]' /etc/fstab; then
    if ! printf '/swapfile none swap sw 0 0\n' >> /etc/fstab; then
      swapoff /swapfile || true
      rm -f /swapfile
      die "写入 /etc/fstab 失败，已撤销新 Swap"
    fi
    fstab_added=1
  fi
  if ! printf 'vm.swappiness=10\n' > "$sysctl_path" || ! sysctl -p "$sysctl_path" >/dev/null; then
    rm -f "$sysctl_path"
    if [ "$fstab_added" -eq 1 ]; then
      sed -i '\|^/swapfile none swap sw 0 0$|d' /etc/fstab || true
    fi
    swapoff /swapfile || true
    rm -f /swapfile
    die "设置 swappiness 失败，已撤销新 Swap"
  fi
}

# validate_legacy_crontab_file 校验旧项目任务标记是否成对。
# 输入：crontab 文本文件路径。
# 输出：格式正确返回 0，否则返回 1。
# 副作用：无。
validate_legacy_crontab_file() {
  local path="$1"
  awk '
    $0 == "# BEGIN aowugong-fastapi" {
      if (inside || seen) exit 1
      inside = 1
      seen = 1
      next
    }
    $0 == "# END aowugong-fastapi" {
      if (!inside) exit 1
      inside = 0
      next
    }
    END { if (inside) exit 1 }
  ' "$path"
}

# remove_legacy_crontab 移除旧 FastAPI 项目的定时任务并保存副本。
# 输入：crontab 用户、旧项目路径和备份路径。
# 输出：无；标记异常或写入失败时返回非零。
# 副作用：修改指定用户 crontab 并写入备份文件。
remove_legacy_crontab() {
  local run_user="$1"
  local legacy_project="$2"
  local backup_path="$3"
  local current_path
  local filtered_path
  current_path="$(mktemp)"
  filtered_path="$(mktemp)"
  crontab -u "$run_user" -l > "$current_path" 2>/dev/null || true
  if ! validate_legacy_crontab_file "$current_path"; then
    rm -f "$current_path" "$filtered_path"
    printf 'ERROR: 旧 crontab 的 aowugong-fastapi 标记不成对，拒绝修改\n' >&2
    return 1
  fi
  install -m 0600 /dev/null "$backup_path"
  awk -v legacy="$legacy_project" -v removed="$backup_path" '
    $0 == "# BEGIN aowugong-fastapi" { skipping = 1; print >> removed; next }
    skipping { print >> removed; if ($0 == "# END aowugong-fastapi") skipping = 0; next }
    index($0, "app.finance.jobs.job_runner") { print >> removed; next }
    { print }
  ' "$current_path" > "$filtered_path"
  crontab -u "$run_user" "$filtered_path"
  rm -f "$current_path" "$filtered_path"
}

# restore_crontab 把备份中的旧项目任务恢复到当前 crontab。
# 输入：crontab 用户、备份路径和可选旧项目路径。
# 输出：无；恢复失败时返回非零。
# 副作用：修改指定用户 crontab。
restore_crontab() {
  local run_user="$1"
  local backup_path="$2"
  local legacy_project="${3:-}"
  local current_path
  local filtered_path
  [ -f "$backup_path" ] || return 0
  current_path="$(mktemp)"
  filtered_path="$(mktemp)"
  crontab -u "$run_user" -l > "$current_path" 2>/dev/null || true
  if ! validate_legacy_crontab_file "$current_path"; then
    rm -f "$current_path" "$filtered_path"
    printf 'ERROR: 当前 crontab 的 aowugong-fastapi 标记不成对，拒绝恢复\n' >&2
    return 1
  fi
  awk -v legacy="$legacy_project" '
    $0 == "# BEGIN aowugong-fastapi" { skipping = 1; next }
    skipping { if ($0 == "# END aowugong-fastapi") skipping = 0; next }
    index($0, "app.finance.jobs.job_runner") { next }
    { print }
  ' "$current_path" > "$filtered_path"
  if [ -s "$backup_path" ]; then
    [ ! -s "$filtered_path" ] || printf '\n' >> "$filtered_path"
    cat "$backup_path" >> "$filtered_path"
  fi
  crontab -u "$run_user" "$filtered_path"
  rm -f "$current_path" "$filtered_path"
}

# save_service_state 保存 systemd 服务启用和运行状态。
# 输入：服务名和状态文件路径。
# 输出：无。
# 副作用：写入权限为 0600 的状态文件。
save_service_state() {
  local service_name="$1"
  local state_path="$2"
  local enabled_state
  local active_state
  enabled_state="$(systemctl is-enabled "$service_name" 2>/dev/null || true)"
  active_state="$(systemctl is-active "$service_name" 2>/dev/null || true)"
  printf 'enabled=%s\nactive=%s\n' "$enabled_state" "$active_state" > "$state_path"
  chmod 0600 "$state_path"
}

# restore_service_state 恢复先前记录的 systemd 服务状态。
# 输入：服务名和状态文件路径。
# 输出：无；systemd 操作失败时返回非零。
# 副作用：启停及启用或禁用指定服务。
restore_service_state() {
  local service_name="$1"
  local state_path="$2"
  local enabled_state="enabled"
  local active_state="active"
  if [ -f "$state_path" ]; then
    enabled_state="$(awk -F= '$1 == "enabled" { print $2; exit }' "$state_path")"
    active_state="$(awk -F= '$1 == "active" { print $2; exit }' "$state_path")"
  fi
  case "$enabled_state" in
    enabled|enabled-runtime|linked|linked-runtime) systemctl enable "$service_name" >/dev/null ;;
    disabled) systemctl disable "$service_name" >/dev/null 2>&1 || true ;;
  esac
  if [ "$active_state" = "active" ]; then
    systemctl start "$service_name"
  else
    systemctl stop "$service_name" >/dev/null 2>&1 || true
  fi
}
