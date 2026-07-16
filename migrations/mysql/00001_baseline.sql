-- +goose Up
-- +goose NO TRANSACTION

CREATE TABLE IF NOT EXISTS job_execution (
    id BIGINT NOT NULL AUTO_INCREMENT,
    job_id VARCHAR(100) NOT NULL,
    status VARCHAR(20) NOT NULL,
    started_at DATETIME(6) NULL,
    finished_at DATETIME(6) NULL,
    duration_ms BIGINT NULL,
    source VARCHAR(20) NOT NULL DEFAULT 'scheduler',
    message TEXT NULL,
    error_message TEXT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    KEY idx_job_execution_name_started (job_id, started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS notification_log (
    id BIGINT NOT NULL AUTO_INCREMENT,
    channel VARCHAR(30) NOT NULL,
    title VARCHAR(500) NOT NULL,
    message TEXT NOT NULL,
    status VARCHAR(20) NOT NULL,
    error_message TEXT NULL,
    sent_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    KEY idx_notification_log_sent_at (sent_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS aowugong_fastapi_users (
    id INT NOT NULL AUTO_INCREMENT,
    username VARCHAR(50) NOT NULL,
    email VARCHAR(100) NOT NULL,
    password VARCHAR(255) NOT NULL,
    full_name VARCHAR(100) NULL,
    phone VARCHAR(20) NULL,
    is_active TINYINT(1) NOT NULL DEFAULT 1,
    is_superuser TINYINT(1) NOT NULL DEFAULT 0,
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_aowugong_users_username (username),
    UNIQUE KEY uq_aowugong_users_email (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS aowugong_roles (
    id INT NOT NULL AUTO_INCREMENT,
    code VARCHAR(50) NOT NULL,
    name VARCHAR(50) NOT NULL,
    description VARCHAR(255) NULL,
    is_active TINYINT(1) NOT NULL DEFAULT 1,
    is_system TINYINT(1) NOT NULL DEFAULT 0,
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_aowugong_roles_code (code),
    UNIQUE KEY uq_aowugong_roles_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS aowugong_permissions (
    id INT NOT NULL AUTO_INCREMENT,
    code VARCHAR(100) NOT NULL,
    name VARCHAR(50) NOT NULL,
    `group` VARCHAR(50) NOT NULL,
    description VARCHAR(255) NULL,
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_aowugong_permissions_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS aowugong_user_roles (
    user_id INT NOT NULL,
    role_id INT NOT NULL,
    PRIMARY KEY (user_id, role_id),
    KEY idx_aowugong_user_roles_role (role_id),
    CONSTRAINT fk_aowugong_user_roles_user FOREIGN KEY (user_id) REFERENCES aowugong_fastapi_users (id) ON DELETE CASCADE,
    CONSTRAINT fk_aowugong_user_roles_role FOREIGN KEY (role_id) REFERENCES aowugong_roles (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS aowugong_role_permissions (
    role_id INT NOT NULL,
    permission_id INT NOT NULL,
    PRIMARY KEY (role_id, permission_id),
    KEY idx_aowugong_role_permissions_permission (permission_id),
    CONSTRAINT fk_aowugong_role_permissions_role FOREIGN KEY (role_id) REFERENCES aowugong_roles (id) ON DELETE CASCADE,
    CONSTRAINT fk_aowugong_role_permissions_permission FOREIGN KEY (permission_id) REFERENCES aowugong_permissions (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS basic_operation (
    id INT NOT NULL AUTO_INCREMENT,
    cal_date VARCHAR(10) NULL,
    trade_date VARCHAR(10) NULL,
    strategy_type VARCHAR(50) NULL,
    ts_code VARCHAR(20) NULL,
    ts_name VARCHAR(50) NULL,
    operate_num INT NULL,
    order_id INT NULL,
    trade_id INT NULL,
    create_date DATETIME NULL,
    update_date DATETIME NULL,
    PRIMARY KEY (id),
    KEY idx_basic_operation_cal_date_strategy_type (cal_date, strategy_type),
    KEY idx_basic_operation_trade_date_strategy_type (trade_date, strategy_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS basic_position (
    id INT NOT NULL AUTO_INCREMENT,
    create_date DATETIME NULL,
    update_date DATETIME NULL,
    trade_date VARCHAR(10) NULL,
    ts_code VARCHAR(20) NULL,
    vol DOUBLE NULL,
    PRIMARY KEY (id),
    KEY idx_basic_position_trade_date_ts_code (trade_date, ts_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS finance_broker_account (
    id BIGINT NOT NULL AUTO_INCREMENT,
    broker_name VARCHAR(50) NOT NULL,
    account_suffix VARCHAR(10) NOT NULL,
    account_alias VARCHAR(100) NOT NULL,
    is_active TINYINT(1) NOT NULL DEFAULT 1,
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_finance_broker_account (broker_name, account_suffix)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS finance_asset_snapshot (
    id BIGINT NOT NULL AUTO_INCREMENT,
    snapshot_date DATE NOT NULL,
    broker_name VARCHAR(50) NOT NULL,
    source_app VARCHAR(50) NOT NULL,
    account_suffix VARCHAR(10) NOT NULL,
    account_alias VARCHAR(100) NULL,
    total_asset DECIMAL(18,2) NOT NULL,
    market_value DECIMAL(18,2) NOT NULL,
    available_cash DECIMAL(18,2) NOT NULL,
    other_amount DECIMAL(18,2) NOT NULL,
    position_percent DECIMAL(8,4) NULL,
    image_path VARCHAR(500) NULL,
    image_sha256 CHAR(64) NULL,
    ocr_provider VARCHAR(50) NULL,
    provider_request_id VARCHAR(100) NULL,
    raw_ocr_json JSON NULL,
    warnings_json JSON NULL,
    parse_status VARCHAR(20) NOT NULL DEFAULT 'parsed',
    created_by VARCHAR(50) NULL,
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_finance_asset_snapshot (snapshot_date, broker_name, account_suffix),
    UNIQUE KEY uq_finance_asset_snapshot_day_account (snapshot_date, account_suffix),
    KEY idx_finance_asset_snapshot_date (snapshot_date),
    KEY idx_finance_asset_snapshot_account (broker_name, account_suffix)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS finance_position_holding_snapshot (
    id BIGINT NOT NULL AUTO_INCREMENT,
    snapshot_date DATE NOT NULL,
    broker_name VARCHAR(50) NOT NULL,
    source_app VARCHAR(50) NOT NULL,
    account_suffix VARCHAR(10) NOT NULL,
    account_alias VARCHAR(100) NULL,
    security_name VARCHAR(100) NOT NULL,
    security_code VARCHAR(20) NULL,
    market_value DECIMAL(18,2) NOT NULL,
    quantity DECIMAL(18,4) NULL,
    available_quantity DECIMAL(18,4) NULL,
    profit_amount DECIMAL(18,2) NULL,
    profit_percent DECIMAL(10,4) NULL,
    cost_price DECIMAL(18,4) NULL,
    current_price DECIMAL(18,4) NULL,
    image_sha256 CHAR(64) NULL,
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_finance_holding_day_account_security (snapshot_date, account_suffix, security_name),
    KEY idx_finance_holding_date (snapshot_date),
    KEY idx_finance_holding_account (broker_name, account_suffix),
    KEY idx_finance_holding_security (security_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS investment_article_source (
    id BIGINT NOT NULL AUTO_INCREMENT,
    source_code VARCHAR(50) NOT NULL,
    source_name VARCHAR(100) NOT NULL,
    source_type VARCHAR(50) NOT NULL,
    feed_url TEXT NOT NULL,
    weight DECIMAL(5,2) NOT NULL DEFAULT 1.00,
    is_active TINYINT(1) NOT NULL DEFAULT 1,
    description VARCHAR(255) NULL,
    last_fetch_at DATETIME NULL,
    last_fetch_status VARCHAR(30) NULL,
    last_fetch_message VARCHAR(500) NULL,
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_investment_article_source_code (source_code),
    KEY idx_investment_article_source_active (is_active, source_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS investment_article (
    id BIGINT NOT NULL AUTO_INCREMENT,
    source_id BIGINT NOT NULL,
    article_key CHAR(64) NOT NULL,
    external_id VARCHAR(255) NULL,
    title VARCHAR(500) NOT NULL,
    link VARCHAR(1000) NOT NULL,
    author VARCHAR(100) NULL,
    published_at DATETIME NULL,
    summary TEXT NULL,
    content MEDIUMTEXT NULL,
    raw_entry_json JSON NULL,
    prompt_feedback TEXT NULL,
    fetch_status VARCHAR(30) NOT NULL DEFAULT 'fetched',
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_investment_article_key (article_key),
    KEY idx_investment_article_source_date (source_id, published_at),
    KEY idx_investment_article_created_at (created_at),
    CONSTRAINT fk_investment_article_source FOREIGN KEY (source_id) REFERENCES investment_article_source (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS investment_article_analysis (
    id BIGINT NOT NULL AUTO_INCREMENT,
    article_id BIGINT NOT NULL,
    status VARCHAR(30) NOT NULL DEFAULT 'pending',
    model_name VARCHAR(100) NULL,
    prompt_version VARCHAR(50) NULL,
    summary TEXT NULL,
    overall_sentiment VARCHAR(30) NULL,
    confidence DECIMAL(6,4) NULL,
    market_mood VARCHAR(30) NULL,
    market_mood_reason TEXT NULL,
    market_prediction VARCHAR(30) NULL,
    market_prediction_reason TEXT NULL,
    short_term_json JSON NULL,
    mid_term_json JSON NULL,
    long_term_json JSON NULL,
    recommendations_json JSON NULL,
    risks_json JSON NULL,
    raw_result_json JSON NULL,
    error_message VARCHAR(1000) NULL,
    analyzed_at DATETIME NULL,
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_investment_article_analysis_article (article_id),
    KEY idx_investment_article_analysis_status (status),
    KEY idx_investment_article_analysis_analyzed_at (analyzed_at),
    CONSTRAINT fk_investment_article_analysis_article FOREIGN KEY (article_id) REFERENCES investment_article (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS mahjong_game_record (
    id BIGINT NOT NULL AUTO_INCREMENT,
    played_date DATE NOT NULL,
    result_amount DECIMAL(12,2) NOT NULL,
    source_filename VARCHAR(255) NULL,
    created_by VARCHAR(50) NULL,
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_mahjong_game_record_date (played_date),
    KEY idx_mahjong_game_record_date (played_date),
    KEY idx_mahjong_game_record_result (result_amount)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS service_monitor_result (
    id BIGINT NOT NULL AUTO_INCREMENT,
    target_code VARCHAR(80) NOT NULL,
    target_name VARCHAR(120) NOT NULL,
    target_url VARCHAR(500) NOT NULL,
    status VARCHAR(20) NOT NULL,
    http_status INT NULL,
    latency_ms INT NULL,
    error_message VARCHAR(1000) NULL,
    checked_at DATETIME NOT NULL,
    PRIMARY KEY (id),
    KEY idx_service_monitor_target_time (target_code, checked_at),
    KEY idx_service_monitor_status_time (status, checked_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS subscription_record (
    id BIGINT NOT NULL AUTO_INCREMENT,
    service_name VARCHAR(120) NOT NULL,
    note TEXT NULL,
    category VARCHAR(20) NOT NULL DEFAULT '生活',
    annual_fee DECIMAL(12,2) NOT NULL DEFAULT 0.00,
    monthly_fee DECIMAL(12,2) NOT NULL DEFAULT 0.00,
    starts_on DATE NULL,
    expires_on DATE NOT NULL,
    created_by VARCHAR(50) NULL,
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_subscription_record_service_name (service_name),
    KEY idx_subscription_record_expires_on (expires_on),
    KEY idx_subscription_record_category (category)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS tushare_daily (
    id INT NOT NULL AUTO_INCREMENT,
    ts_code VARCHAR(20) NULL,
    trade_date VARCHAR(20) NULL,
    open DOUBLE NULL,
    high DOUBLE NULL,
    low DOUBLE NULL,
    close DOUBLE NULL,
    pre_close DOUBLE NULL,
    `change` DOUBLE NULL,
    pct_chg DOUBLE NULL,
    vol DOUBLE NULL,
    amount DOUBLE NULL,
    create_date DATETIME NULL,
    update_date DATETIME NULL,
    PRIMARY KEY (id),
    KEY idx_tushare_daily_trade_date (trade_date),
    KEY idx_tushare_daily_ts_code_trade_date (ts_code, trade_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS tushare_etf_basic (
    ts_code VARCHAR(20) NULL,
    csname VARCHAR(255) NULL,
    extname VARCHAR(255) NULL,
    cname VARCHAR(255) NULL,
    index_code VARCHAR(20) NULL,
    index_name VARCHAR(255) NULL,
    setup_date VARCHAR(10) NULL,
    list_date VARCHAR(10) NULL,
    list_status VARCHAR(20) NULL,
    exchange VARCHAR(20) NULL,
    mgr_name VARCHAR(255) NULL,
    custod_name VARCHAR(255) NULL,
    mgt_fee DOUBLE NULL,
    etf_type VARCHAR(100) NULL,
    create_date DATETIME NULL,
    update_date DATETIME NULL,
    KEY idx_tushare_etf_basic_ts_code (ts_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS tushare_stock_basic (
    ts_code VARCHAR(20) NULL,
    symbol VARCHAR(20) NULL,
    name VARCHAR(100) NULL,
    area VARCHAR(100) NULL,
    industry VARCHAR(100) NULL,
    cnspell VARCHAR(100) NULL,
    market VARCHAR(100) NULL,
    list_date VARCHAR(10) NULL,
    act_name VARCHAR(255) NULL,
    act_ent_type VARCHAR(255) NULL,
    create_date DATETIME NULL,
    update_date DATETIME NULL,
    KEY idx_tushare_stock_basic_ts_code (ts_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS tushare_trade_cal (
    exchange VARCHAR(20) NULL,
    cal_date VARCHAR(10) NULL,
    is_open BIGINT NULL,
    pretrade_date VARCHAR(10) NULL,
    create_date DATETIME NULL,
    update_date DATETIME NULL,
    KEY idx_tushare_trade_cal_cal_date (cal_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- +goose Down
SELECT '00001_baseline is intentionally irreversible';
