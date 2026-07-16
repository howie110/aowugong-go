-- +goose Up
-- +goose NO TRANSACTION

DROP PROCEDURE IF EXISTS aowugong_add_index_if_missing;

-- +goose StatementBegin
CREATE PROCEDURE aowugong_add_index_if_missing(
    IN target_table VARCHAR(64),
    IN target_index VARCHAR(64),
    IN index_ddl TEXT
)
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.statistics
        WHERE table_schema = DATABASE()
          AND table_name = target_table
          AND index_name = target_index
    ) THEN
        SET @aowugong_index_ddl = index_ddl;
        PREPARE aowugong_index_statement FROM @aowugong_index_ddl;
        EXECUTE aowugong_index_statement;
        DEALLOCATE PREPARE aowugong_index_statement;
    END IF;
END;
-- +goose StatementEnd

CALL aowugong_add_index_if_missing(
    'basic_operation',
    'idx_basic_operation_cal_date_strategy_type',
    'ALTER TABLE basic_operation ADD INDEX idx_basic_operation_cal_date_strategy_type (cal_date, strategy_type), ALGORITHM=INPLACE, LOCK=NONE'
);
CALL aowugong_add_index_if_missing(
    'basic_operation',
    'idx_basic_operation_trade_date_strategy_type',
    'ALTER TABLE basic_operation ADD INDEX idx_basic_operation_trade_date_strategy_type (trade_date, strategy_type), ALGORITHM=INPLACE, LOCK=NONE'
);
CALL aowugong_add_index_if_missing(
    'basic_position',
    'idx_basic_position_trade_date_ts_code',
    'ALTER TABLE basic_position ADD INDEX idx_basic_position_trade_date_ts_code (trade_date, ts_code), ALGORITHM=INPLACE, LOCK=NONE'
);
CALL aowugong_add_index_if_missing(
    'tushare_daily',
    'idx_tushare_daily_ts_code_trade_date',
    'ALTER TABLE tushare_daily ADD INDEX idx_tushare_daily_ts_code_trade_date (ts_code, trade_date), ALGORITHM=INPLACE, LOCK=NONE'
);
CALL aowugong_add_index_if_missing(
    'tushare_etf_basic',
    'idx_tushare_etf_basic_ts_code',
    'ALTER TABLE tushare_etf_basic ADD INDEX idx_tushare_etf_basic_ts_code (ts_code(20)), ALGORITHM=INPLACE, LOCK=NONE'
);
CALL aowugong_add_index_if_missing(
    'tushare_stock_basic',
    'idx_tushare_stock_basic_ts_code',
    'ALTER TABLE tushare_stock_basic ADD INDEX idx_tushare_stock_basic_ts_code (ts_code(20)), ALGORITHM=INPLACE, LOCK=NONE'
);
CALL aowugong_add_index_if_missing(
    'tushare_trade_cal',
    'idx_tushare_trade_cal_cal_date',
    'ALTER TABLE tushare_trade_cal ADD INDEX idx_tushare_trade_cal_cal_date (cal_date(10)), ALGORITHM=INPLACE, LOCK=NONE'
);

DROP PROCEDURE IF EXISTS aowugong_add_index_if_missing;

-- +goose Down
SELECT '00002_existing_indexes is intentionally irreversible';
