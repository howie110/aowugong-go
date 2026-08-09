-- +goose Up

CREATE TABLE IF NOT EXISTS job_execution (
    id BIGSERIAL PRIMARY KEY,
    job_id TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT,
    duration_ms INTEGER,
    source TEXT NOT NULL DEFAULT 'scheduler',
    message TEXT,
    error_message TEXT,
    created_at TEXT NOT NULL DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS'))
);
CREATE INDEX IF NOT EXISTS idx_job_execution_name_started
    ON job_execution(job_id, started_at);

CREATE TABLE IF NOT EXISTS job_execution_lock (
    lock_name TEXT PRIMARY KEY,
    owner_token TEXT NOT NULL,
    acquired_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_job_execution_lock_expires
    ON job_execution_lock(expires_at);

CREATE TABLE IF NOT EXISTS notification_log (
    id BIGSERIAL PRIMARY KEY,
    channel TEXT NOT NULL,
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    status TEXT NOT NULL,
    error_message TEXT,
    sent_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_notification_log_sent_at
    ON notification_log(sent_at);

CREATE TABLE IF NOT EXISTS aowugong_fastapi_users (
    id BIGSERIAL PRIMARY KEY,
    username TEXT NOT NULL,
    email TEXT NOT NULL,
    password TEXT NOT NULL,
    full_name TEXT,
    phone TEXT,
    is_active INTEGER NOT NULL DEFAULT 1,
    is_superuser INTEGER NOT NULL DEFAULT 0,
    created_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    updated_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    CONSTRAINT uq_aowugong_users_username UNIQUE(username),
    CONSTRAINT uq_aowugong_users_email UNIQUE(email)
);

CREATE TABLE IF NOT EXISTS aowugong_roles (
    id BIGSERIAL PRIMARY KEY,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    is_active INTEGER NOT NULL DEFAULT 1,
    is_system INTEGER NOT NULL DEFAULT 0,
    created_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    updated_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    CONSTRAINT uq_aowugong_roles_code UNIQUE(code),
    CONSTRAINT uq_aowugong_roles_name UNIQUE(name)
);

CREATE TABLE IF NOT EXISTS aowugong_permissions (
    id BIGSERIAL PRIMARY KEY,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    "group" TEXT NOT NULL,
    description TEXT,
    created_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    updated_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    CONSTRAINT uq_aowugong_permissions_code UNIQUE(code)
);

CREATE TABLE IF NOT EXISTS aowugong_user_roles (
    user_id BIGINT NOT NULL,
    role_id BIGINT NOT NULL,
    PRIMARY KEY(user_id, role_id),
    CONSTRAINT fk_aowugong_user_roles_user
        FOREIGN KEY(user_id) REFERENCES aowugong_fastapi_users(id) ON DELETE CASCADE,
    CONSTRAINT fk_aowugong_user_roles_role
        FOREIGN KEY(role_id) REFERENCES aowugong_roles(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_aowugong_user_roles_role
    ON aowugong_user_roles(role_id);

CREATE TABLE IF NOT EXISTS aowugong_role_permissions (
    role_id BIGINT NOT NULL,
    permission_id BIGINT NOT NULL,
    PRIMARY KEY(role_id, permission_id),
    CONSTRAINT fk_aowugong_role_permissions_role
        FOREIGN KEY(role_id) REFERENCES aowugong_roles(id) ON DELETE CASCADE,
    CONSTRAINT fk_aowugong_role_permissions_permission
        FOREIGN KEY(permission_id) REFERENCES aowugong_permissions(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_aowugong_role_permissions_permission
    ON aowugong_role_permissions(permission_id);

CREATE TABLE IF NOT EXISTS basic_operation (
    id BIGSERIAL PRIMARY KEY,
    cal_date TEXT,
    trade_date TEXT,
    strategy_type TEXT,
    ts_code TEXT,
    ts_name TEXT,
    operate_num INTEGER,
    order_id INTEGER,
    trade_id INTEGER,
    create_date TEXT,
    update_date TEXT
);
CREATE INDEX IF NOT EXISTS idx_basic_operation_cal_date_strategy_type
    ON basic_operation(cal_date, strategy_type);
CREATE INDEX IF NOT EXISTS idx_basic_operation_trade_date_strategy_type
    ON basic_operation(trade_date, strategy_type);

CREATE TABLE IF NOT EXISTS basic_position (
    id BIGSERIAL PRIMARY KEY,
    create_date TEXT,
    update_date TEXT,
    trade_date TEXT,
    ts_code TEXT,
    vol REAL
);
CREATE INDEX IF NOT EXISTS idx_basic_position_trade_date_ts_code
    ON basic_position(trade_date, ts_code);

CREATE TABLE IF NOT EXISTS finance_broker_account (
    id BIGSERIAL PRIMARY KEY,
    broker_name TEXT NOT NULL,
    account_suffix TEXT NOT NULL,
    account_alias TEXT NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    updated_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    CONSTRAINT uq_finance_broker_account UNIQUE(broker_name, account_suffix)
);

CREATE TABLE IF NOT EXISTS finance_asset_snapshot (
    id BIGSERIAL PRIMARY KEY,
    snapshot_date TEXT NOT NULL,
    broker_name TEXT NOT NULL,
    source_app TEXT NOT NULL,
    account_suffix TEXT NOT NULL,
    account_alias TEXT,
    total_asset NUMERIC NOT NULL,
    market_value NUMERIC NOT NULL,
    available_cash NUMERIC NOT NULL,
    other_amount NUMERIC NOT NULL,
    position_percent NUMERIC,
    image_path TEXT,
    image_sha256 TEXT,
    ocr_provider TEXT,
    provider_request_id TEXT,
    raw_ocr_json TEXT,
    warnings_json TEXT,
    parse_status TEXT NOT NULL DEFAULT 'parsed',
    created_by TEXT,
    created_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    updated_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    CONSTRAINT uq_finance_asset_snapshot UNIQUE(snapshot_date, broker_name, account_suffix),
    CONSTRAINT uq_finance_asset_snapshot_day_account UNIQUE(snapshot_date, account_suffix)
);
CREATE INDEX IF NOT EXISTS idx_finance_asset_snapshot_date
    ON finance_asset_snapshot(snapshot_date);
CREATE INDEX IF NOT EXISTS idx_finance_asset_snapshot_account
    ON finance_asset_snapshot(broker_name, account_suffix);

CREATE TABLE IF NOT EXISTS finance_position_holding_snapshot (
    id BIGSERIAL PRIMARY KEY,
    snapshot_date TEXT NOT NULL,
    broker_name TEXT NOT NULL,
    source_app TEXT NOT NULL,
    account_suffix TEXT NOT NULL,
    account_alias TEXT,
    security_name TEXT NOT NULL,
    security_code TEXT,
    market_value NUMERIC NOT NULL,
    quantity NUMERIC,
    available_quantity NUMERIC,
    profit_amount NUMERIC,
    profit_percent NUMERIC,
    cost_price NUMERIC,
    current_price NUMERIC,
    image_sha256 TEXT,
    created_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    updated_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    CONSTRAINT uq_finance_holding_day_account_security
        UNIQUE(snapshot_date, account_suffix, security_name)
);
CREATE INDEX IF NOT EXISTS idx_finance_holding_date
    ON finance_position_holding_snapshot(snapshot_date);
CREATE INDEX IF NOT EXISTS idx_finance_holding_account
    ON finance_position_holding_snapshot(broker_name, account_suffix);
CREATE INDEX IF NOT EXISTS idx_finance_holding_security
    ON finance_position_holding_snapshot(security_name);

CREATE TABLE IF NOT EXISTS investment_article_source (
    id BIGSERIAL PRIMARY KEY,
    source_code TEXT NOT NULL,
    source_name TEXT NOT NULL,
    source_type TEXT NOT NULL,
    feed_url TEXT NOT NULL,
    weight NUMERIC NOT NULL DEFAULT 1.00,
    is_active INTEGER NOT NULL DEFAULT 1,
    description TEXT,
    last_fetch_at TEXT,
    last_fetch_status TEXT,
    last_fetch_message TEXT,
    created_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    updated_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    CONSTRAINT uq_investment_article_source_code UNIQUE(source_code)
);
CREATE INDEX IF NOT EXISTS idx_investment_article_source_active
    ON investment_article_source(is_active, source_type);

CREATE TABLE IF NOT EXISTS investment_article (
    id BIGSERIAL PRIMARY KEY,
    source_id BIGINT NOT NULL,
    article_key TEXT NOT NULL,
    external_id TEXT,
    title TEXT NOT NULL,
    link TEXT NOT NULL,
    author TEXT,
    published_at TEXT,
    summary TEXT,
    content TEXT,
    raw_entry_json TEXT,
    prompt_feedback TEXT,
    fetch_status TEXT NOT NULL DEFAULT 'fetched',
    created_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    updated_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    CONSTRAINT uq_investment_article_key UNIQUE(article_key),
    CONSTRAINT fk_investment_article_source
        FOREIGN KEY(source_id) REFERENCES investment_article_source(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_investment_article_source_date
    ON investment_article(source_id, published_at);
CREATE INDEX IF NOT EXISTS idx_investment_article_created_at
    ON investment_article(created_at);

CREATE TABLE IF NOT EXISTS investment_article_analysis (
    id BIGSERIAL PRIMARY KEY,
    article_id BIGINT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    model_name TEXT,
    prompt_version TEXT,
    summary TEXT,
    overall_sentiment TEXT,
    confidence NUMERIC,
    market_mood TEXT,
    market_mood_reason TEXT,
    market_prediction TEXT,
    market_prediction_reason TEXT,
    short_term_json TEXT,
    mid_term_json TEXT,
    long_term_json TEXT,
    recommendations_json TEXT,
    risks_json TEXT,
    raw_result_json TEXT,
    error_message TEXT,
    analyzed_at TEXT,
    created_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    updated_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    CONSTRAINT uq_investment_article_analysis_article UNIQUE(article_id),
    CONSTRAINT fk_investment_article_analysis_article
        FOREIGN KEY(article_id) REFERENCES investment_article(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_investment_article_analysis_status
    ON investment_article_analysis(status);
CREATE INDEX IF NOT EXISTS idx_investment_article_analysis_analyzed_at
    ON investment_article_analysis(analyzed_at);

CREATE TABLE IF NOT EXISTS investment_signal_group (
    id BIGSERIAL PRIMARY KEY,
    canonical_name TEXT NOT NULL,
    group_type TEXT NOT NULL DEFAULT 'other',
    source TEXT NOT NULL DEFAULT 'deepseek',
    model_name TEXT,
    created_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    updated_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    CONSTRAINT uq_investment_signal_group_name UNIQUE(canonical_name)
);
CREATE INDEX IF NOT EXISTS idx_investment_signal_group_type
    ON investment_signal_group(group_type);

CREATE TABLE IF NOT EXISTS investment_signal_alias (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL,
    alias_name TEXT NOT NULL,
    normalized_name TEXT NOT NULL,
    confidence NUMERIC NOT NULL DEFAULT 0.0000,
    source TEXT NOT NULL DEFAULT 'deepseek',
    model_name TEXT,
    created_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    updated_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    CONSTRAINT uq_investment_signal_alias_normalized UNIQUE(normalized_name),
    CONSTRAINT fk_investment_signal_alias_group
        FOREIGN KEY(group_id) REFERENCES investment_signal_group(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_investment_signal_alias_group
    ON investment_signal_alias(group_id);

CREATE TABLE IF NOT EXISTS mahjong_game_record (
    id BIGSERIAL PRIMARY KEY,
    played_date TEXT NOT NULL,
    result_amount NUMERIC NOT NULL,
    source_filename TEXT,
    created_by TEXT,
    created_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    updated_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    CONSTRAINT uq_mahjong_game_record_date UNIQUE(played_date)
);
CREATE INDEX IF NOT EXISTS idx_mahjong_game_record_date
    ON mahjong_game_record(played_date);
CREATE INDEX IF NOT EXISTS idx_mahjong_game_record_result
    ON mahjong_game_record(result_amount);

CREATE TABLE IF NOT EXISTS service_monitor_result (
    id BIGSERIAL PRIMARY KEY,
    target_code TEXT NOT NULL,
    target_name TEXT NOT NULL,
    target_url TEXT NOT NULL,
    status TEXT NOT NULL,
    http_status INTEGER,
    latency_ms INTEGER,
    error_message TEXT,
    checked_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_service_monitor_target_time
    ON service_monitor_result(target_code, checked_at);
CREATE INDEX IF NOT EXISTS idx_service_monitor_status_time
    ON service_monitor_result(status, checked_at);

CREATE TABLE IF NOT EXISTS subscription_record (
    id BIGSERIAL PRIMARY KEY,
    service_name TEXT NOT NULL,
    note TEXT,
    category TEXT NOT NULL DEFAULT '生活',
    annual_fee NUMERIC NOT NULL DEFAULT 0.00,
    monthly_fee NUMERIC NOT NULL DEFAULT 0.00,
    starts_on TEXT,
    expires_on TEXT NOT NULL,
    created_by TEXT,
    created_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    updated_at TEXT DEFAULT (TO_CHAR(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')),
    CONSTRAINT uq_subscription_record_service_name UNIQUE(service_name)
);
CREATE INDEX IF NOT EXISTS idx_subscription_record_expires_on
    ON subscription_record(expires_on);
CREATE INDEX IF NOT EXISTS idx_subscription_record_category
    ON subscription_record(category);

CREATE TABLE IF NOT EXISTS tushare_daily (
    id BIGSERIAL PRIMARY KEY,
    ts_code TEXT,
    trade_date TEXT,
    open REAL,
    high REAL,
    low REAL,
    close REAL,
    pre_close REAL,
    "change" REAL,
    pct_chg REAL,
    vol REAL,
    amount REAL,
    create_date TEXT,
    update_date TEXT
);
CREATE INDEX IF NOT EXISTS idx_tushare_daily_trade_date
    ON tushare_daily(trade_date);
CREATE INDEX IF NOT EXISTS idx_tushare_daily_ts_code_trade_date
    ON tushare_daily(ts_code, trade_date);

CREATE TABLE IF NOT EXISTS tushare_etf_basic (
    ts_code TEXT,
    csname TEXT,
    extname TEXT,
    cname TEXT,
    index_code TEXT,
    index_name TEXT,
    setup_date TEXT,
    list_date TEXT,
    list_status TEXT,
    exchange TEXT,
    mgr_name TEXT,
    custod_name TEXT,
    mgt_fee REAL,
    etf_type TEXT,
    create_date TEXT,
    update_date TEXT
);
CREATE INDEX IF NOT EXISTS idx_tushare_etf_basic_ts_code
    ON tushare_etf_basic(ts_code);

CREATE TABLE IF NOT EXISTS tushare_stock_basic (
    ts_code TEXT,
    symbol TEXT,
    name TEXT,
    area TEXT,
    industry TEXT,
    cnspell TEXT,
    market TEXT,
    list_date TEXT,
    act_name TEXT,
    act_ent_type TEXT,
    create_date TEXT,
    update_date TEXT
);
CREATE INDEX IF NOT EXISTS idx_tushare_stock_basic_ts_code
    ON tushare_stock_basic(ts_code);

CREATE TABLE IF NOT EXISTS tushare_trade_cal (
    exchange TEXT,
    cal_date TEXT,
    is_open INTEGER,
    pretrade_date TEXT,
    create_date TEXT,
    update_date TEXT
);
CREATE INDEX IF NOT EXISTS idx_tushare_trade_cal_cal_date
    ON tushare_trade_cal(cal_date);

INSERT INTO investment_signal_group(canonical_name, group_type, source, model_name)
VALUES('证券行业', 'sector', 'manual', NULL)
ON CONFLICT(canonical_name) DO NOTHING;

INSERT INTO investment_signal_alias(
    group_id, alias_name, normalized_name, confidence, source, model_name
)
SELECT signal_group.id, seed.alias_name, LOWER(TRIM(seed.alias_name)), 1.0000, 'manual', NULL
FROM investment_signal_group AS signal_group
CROSS JOIN (
    SELECT '券商' AS alias_name
    UNION ALL SELECT '券商板块'
    UNION ALL SELECT '证券板块'
    UNION ALL SELECT '中信证券'
) AS seed
WHERE signal_group.canonical_name = '证券行业'
ON CONFLICT(normalized_name) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS tushare_trade_cal;
DROP TABLE IF EXISTS tushare_stock_basic;
DROP TABLE IF EXISTS tushare_etf_basic;
DROP TABLE IF EXISTS tushare_daily;
DROP TABLE IF EXISTS subscription_record;
DROP TABLE IF EXISTS service_monitor_result;
DROP TABLE IF EXISTS mahjong_game_record;
DROP TABLE IF EXISTS investment_signal_alias;
DROP TABLE IF EXISTS investment_signal_group;
DROP TABLE IF EXISTS investment_article_analysis;
DROP TABLE IF EXISTS investment_article;
DROP TABLE IF EXISTS investment_article_source;
DROP TABLE IF EXISTS finance_position_holding_snapshot;
DROP TABLE IF EXISTS finance_asset_snapshot;
DROP TABLE IF EXISTS finance_broker_account;
DROP TABLE IF EXISTS basic_position;
DROP TABLE IF EXISTS basic_operation;
DROP TABLE IF EXISTS aowugong_role_permissions;
DROP TABLE IF EXISTS aowugong_user_roles;
DROP TABLE IF EXISTS aowugong_permissions;
DROP TABLE IF EXISTS aowugong_roles;
DROP TABLE IF EXISTS aowugong_fastapi_users;
DROP TABLE IF EXISTS notification_log;
DROP TABLE IF EXISTS job_execution_lock;
DROP TABLE IF EXISTS job_execution;
