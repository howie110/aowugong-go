#!/usr/bin/env bash

set -Eeuo pipefail

# 1. 核对服务、版本和本机健康接口。
echo "SERVICES"
systemctl is-active postgresql-15.service
systemctl is-enabled postgresql-15.service
systemctl is-active miniflux.service
systemctl is-enabled miniflux.service
echo "HEALTH"
curl -fsS http://127.0.0.1:5000/healthcheck
echo
echo "VERSION"
/usr/local/bin/miniflux -version

# 2. 核对 PostgreSQL 监听范围、低内存参数和数据库大小。
echo "POSTGRES_CONFIG"
cd /tmp
runuser -u postgres -- /usr/pgsql-15/bin/psql -At --dbname=postgres --command="SHOW listen_addresses"
runuser -u postgres -- /usr/pgsql-15/bin/psql -At --dbname=postgres --command="SHOW max_connections"
runuser -u postgres -- /usr/pgsql-15/bin/psql -At --dbname=postgres --command="SHOW shared_buffers"
runuser -u postgres -- /usr/pgsql-15/bin/psql -At --dbname=postgres --command="SHOW work_mem"
runuser -u postgres -- /usr/pgsql-15/bin/psql -At --dbname=postgres --command="SHOW maintenance_work_mem"
echo "DATABASE_SIZE"
runuser -u postgres -- /usr/pgsql-15/bin/psql -At --dbname=postgres \
	--command="SELECT pg_size_pretty(pg_database_size('miniflux'))"

# 3. 使用应用密钥验证分类 API，不输出密钥本身。
echo "CATEGORIES"
token="$(</etc/miniflux/aowugong-api-token)"
curl -fsS --header "X-Auth-Token: ${token}" http://127.0.0.1:5000/v1/categories
echo

# 4. 输出服务内存、整机内存、端口和现有应用健康状态。
echo "SERVICE_MEMORY_BYTES"
systemctl show postgresql-15.service --property=MemoryCurrent
systemctl show miniflux.service --property=MemoryCurrent
echo "SYSTEM_MEMORY"
free -h
echo "PORTS"
ss -lntp
echo "AOWUGONG"
curl -fsS http://127.0.0.1:12345/api/v1/health
echo

# 5. 核对 aowugong 已配置 Miniflux 且旧 RSS 参数不存在，不输出配置值。
echo "AOWUGONG_MINIFLUX_CONFIG"
for key in MINIFLUX_BASE_URL MINIFLUX_MONITOR_URL MINIFLUX_API_TOKEN MINIFLUX_CATEGORY; do
	if ! grep -Eq "^${key}=.+" /opt/aowugong-go/shared/.env; then
		echo "missing ${key}" >&2
		exit 1
	fi
	echo "${key}=configured"
done
if grep -Eq '^(INVESTMENT_ARTICLE_AGGREGATE_RSS_URL|WECHAT_RSS_MONITOR_URL)=' /opt/aowugong-go/shared/.env; then
	echo "旧 WeChatRSS 配置仍存在" >&2
	exit 1
fi

# 6. 检查 Miniflux 最近十分钟警告及错误。
echo "MINIFLUX_WARNINGS"
journalctl -u miniflux.service --since "10 minutes ago" --no-pager -p warning
