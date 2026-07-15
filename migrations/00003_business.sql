CREATE TABLE basic_operation (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
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

CREATE INDEX idx_basic_operation_cal_date_strategy_type
    ON basic_operation (cal_date, strategy_type);
CREATE INDEX idx_basic_operation_trade_date_strategy_type
    ON basic_operation (trade_date, strategy_type);

CREATE TABLE basic_position (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    create_date TEXT,
    update_date TEXT,
    trade_date TEXT,
    ts_code TEXT,
    vol REAL
);

CREATE INDEX idx_basic_position_trade_date_ts_code
    ON basic_position (trade_date, ts_code);

CREATE TABLE finance_broker_account (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    broker_name TEXT NOT NULL,
    account_suffix TEXT NOT NULL,
    account_alias TEXT NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (broker_name, account_suffix)
);

CREATE TABLE finance_asset_snapshot (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
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
    parse_status TEXT NOT NULL,
    created_by TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (snapshot_date, broker_name, account_suffix),
    UNIQUE (snapshot_date, account_suffix)
);

CREATE INDEX idx_finance_asset_snapshot_date
    ON finance_asset_snapshot (snapshot_date);
CREATE INDEX idx_finance_asset_snapshot_account
    ON finance_asset_snapshot (broker_name, account_suffix);

CREATE TABLE finance_position_holding_snapshot (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
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
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (snapshot_date, account_suffix, security_name)
);

CREATE INDEX idx_finance_holding_date
    ON finance_position_holding_snapshot (snapshot_date);
CREATE INDEX idx_finance_holding_account
    ON finance_position_holding_snapshot (broker_name, account_suffix);
CREATE INDEX idx_finance_holding_security
    ON finance_position_holding_snapshot (security_name);

CREATE TABLE investment_article_source (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_code TEXT NOT NULL UNIQUE,
    source_name TEXT NOT NULL,
    source_type TEXT NOT NULL,
    feed_url TEXT NOT NULL,
    weight NUMERIC NOT NULL DEFAULT 1,
    is_active INTEGER NOT NULL DEFAULT 1,
    description TEXT,
    last_fetch_at TEXT,
    last_fetch_status TEXT,
    last_fetch_message TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_investment_article_source_active
    ON investment_article_source (is_active, source_type);

CREATE TABLE investment_article (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id INTEGER NOT NULL,
    article_key TEXT NOT NULL UNIQUE,
    external_id TEXT,
    title TEXT NOT NULL,
    link TEXT NOT NULL,
    author TEXT,
    published_at TEXT,
    summary TEXT,
    content TEXT,
    raw_entry_json TEXT,
    prompt_feedback TEXT,
    fetch_status TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (source_id) REFERENCES investment_article_source (id) ON DELETE CASCADE
);

CREATE INDEX idx_investment_article_source_date
    ON investment_article (source_id, published_at);
CREATE INDEX idx_investment_article_created_at
    ON investment_article (created_at);

CREATE TABLE investment_article_analysis (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    article_id INTEGER NOT NULL UNIQUE,
    status TEXT NOT NULL,
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
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (article_id) REFERENCES investment_article (id) ON DELETE CASCADE
);

CREATE INDEX idx_investment_article_analysis_status
    ON investment_article_analysis (status);
CREATE INDEX idx_investment_article_analysis_analyzed_at
    ON investment_article_analysis (analyzed_at);

CREATE TABLE mahjong_game_record (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    played_date TEXT NOT NULL UNIQUE,
    result_amount NUMERIC NOT NULL,
    source_filename TEXT,
    created_by TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_mahjong_game_record_date
    ON mahjong_game_record (played_date);
CREATE INDEX idx_mahjong_game_record_result
    ON mahjong_game_record (result_amount);

CREATE TABLE service_monitor_result (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_code TEXT NOT NULL,
    target_name TEXT NOT NULL,
    target_url TEXT NOT NULL,
    status TEXT NOT NULL,
    http_status INTEGER,
    latency_ms INTEGER,
    error_message TEXT,
    checked_at TEXT NOT NULL
);

CREATE INDEX idx_service_monitor_target_time
    ON service_monitor_result (target_code, checked_at);
CREATE INDEX idx_service_monitor_status_time
    ON service_monitor_result (status, checked_at);

CREATE TABLE subscription_record (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    service_name TEXT NOT NULL UNIQUE,
    note TEXT,
    category TEXT NOT NULL DEFAULT '生活',
    annual_fee NUMERIC NOT NULL DEFAULT 0,
    monthly_fee NUMERIC NOT NULL DEFAULT 0,
    starts_on TEXT,
    expires_on TEXT NOT NULL,
    created_by TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_subscription_record_expires_on
    ON subscription_record (expires_on);
CREATE INDEX idx_subscription_record_category
    ON subscription_record (category);

CREATE TABLE tushare_daily (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ts_code TEXT,
    trade_date TEXT,
    open REAL,
    high REAL,
    low REAL,
    close REAL,
    pre_close REAL,
    change REAL,
    pct_chg REAL,
    vol REAL,
    amount REAL,
    create_date TEXT,
    update_date TEXT
);

CREATE INDEX idx_tushare_daily_trade_date
    ON tushare_daily (trade_date);
CREATE INDEX idx_tushare_daily_ts_code_trade_date
    ON tushare_daily (ts_code, trade_date);

CREATE TABLE tushare_etf_basic (
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

CREATE INDEX idx_tushare_etf_basic_ts_code
    ON tushare_etf_basic (ts_code);

CREATE TABLE tushare_stock_basic (
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

CREATE INDEX idx_tushare_stock_basic_ts_code
    ON tushare_stock_basic (ts_code);

CREATE TABLE tushare_trade_cal (
    exchange TEXT,
    cal_date TEXT,
    is_open INTEGER,
    pretrade_date TEXT,
    create_date TEXT,
    update_date TEXT
);

CREATE INDEX idx_tushare_trade_cal_cal_date
    ON tushare_trade_cal (cal_date);

ALTER TABLE job_execution ADD COLUMN source TEXT NOT NULL DEFAULT 'scheduler';
ALTER TABLE job_execution ADD COLUMN duration_ms INTEGER;
ALTER TABLE job_execution ADD COLUMN message TEXT;

CREATE INDEX idx_job_execution_name_started
    ON job_execution (job_id, started_at);

CREATE TABLE notification_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel TEXT NOT NULL,
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    status TEXT NOT NULL,
    error_message TEXT,
    sent_at TEXT NOT NULL
);

CREATE INDEX idx_notification_log_sent_at
    ON notification_log (sent_at);
