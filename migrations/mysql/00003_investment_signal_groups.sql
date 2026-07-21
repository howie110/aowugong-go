-- +goose Up
CREATE TABLE IF NOT EXISTS investment_signal_group (
    id BIGINT NOT NULL AUTO_INCREMENT,
    canonical_name VARCHAR(100) NOT NULL,
    group_type VARCHAR(30) NOT NULL DEFAULT 'other',
    source VARCHAR(30) NOT NULL DEFAULT 'deepseek',
    model_name VARCHAR(100) NULL,
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_investment_signal_group_name (canonical_name),
    KEY idx_investment_signal_group_type (group_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS investment_signal_alias (
    id BIGINT NOT NULL AUTO_INCREMENT,
    group_id BIGINT NOT NULL,
    alias_name VARCHAR(100) NOT NULL,
    normalized_name VARCHAR(100) NOT NULL,
    confidence DECIMAL(5,4) NOT NULL DEFAULT 0.0000,
    source VARCHAR(30) NOT NULL DEFAULT 'deepseek',
    model_name VARCHAR(100) NULL,
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_investment_signal_alias_normalized (normalized_name),
    KEY idx_investment_signal_alias_group (group_id),
    CONSTRAINT fk_investment_signal_alias_group FOREIGN KEY (group_id) REFERENCES investment_signal_group (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT IGNORE INTO investment_signal_group(canonical_name, group_type, source, model_name)
VALUES('证券行业', 'sector', 'manual', NULL);

INSERT IGNORE INTO investment_signal_alias(
    group_id, alias_name, normalized_name, confidence, source, model_name
)
SELECT signal_group.id, seed.alias_name, LOWER(TRIM(seed.alias_name)), 1.0000, 'manual', NULL
FROM investment_signal_group signal_group
CROSS JOIN (
    SELECT '券商' AS alias_name
    UNION ALL SELECT '券商板块'
    UNION ALL SELECT '证券板块'
    UNION ALL SELECT '中信证券'
) AS seed
WHERE signal_group.canonical_name = '证券行业';

-- +goose Down
DROP TABLE IF EXISTS investment_signal_alias;
DROP TABLE IF EXISTS investment_signal_group;
