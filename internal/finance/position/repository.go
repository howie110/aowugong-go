package position

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

var defaultAccounts = []struct {
	BrokerName    string
	AccountSuffix string
	AccountAlias  string
}{
	{BrokerName: "东莞证券", AccountSuffix: "5042", AccountAlias: "东莞证券-邓子豪"},
	{BrokerName: "东莞证券", AccountSuffix: "7521", AccountAlias: "东莞证券-吴素尤"},
}

// Repository 负责仓位相关 SQLite 读写。
type Repository struct {
	db *sql.DB
}

// NewRepository 创建仓位 SQLite 仓储。
// 输入：db 是已经执行版本化迁移的 SQLite 连接。
// 输出：返回仓位仓储。
// 副作用：无，不访问数据库。
func NewRepository(db *sql.DB) *Repository {
	// 1. 保存显式数据库依赖。
	return &Repository{db: db}
}

// SyncDefaultAccounts 写入或更新项目现有的两个默认账户别名。
// 输入：ctx 控制数据库操作。
// 输出：成功返回 nil，失败返回带业务上下文的错误。
// 副作用：写入 finance_broker_account。
func (r *Repository) SyncDefaultAccounts(ctx context.Context) error {
	// 1. 逐个 upsert 默认账户并保持启用。
	for _, account := range defaultAccounts {
		_, err := r.db.ExecContext(ctx, `
			INSERT INTO finance_broker_account (broker_name, account_suffix, account_alias, is_active)
			VALUES (?, ?, ?, 1)
			ON CONFLICT(broker_name, account_suffix) DO UPDATE SET
				account_alias = excluded.account_alias,
				is_active = 1,
				updated_at = CURRENT_TIMESTAMP
			WHERE finance_broker_account.account_alias IS NOT excluded.account_alias
				OR finance_broker_account.is_active <> 1
		`, account.BrokerName, account.AccountSuffix, account.AccountAlias)
		if err != nil {
			return fmt.Errorf("同步默认账户 %s: %w", account.AccountSuffix, err)
		}
	}
	return nil
}

// AccountAlias 按券商和账户后四位读取唯一别名。
// 输入：ctx 控制数据库操作，brokerName 和 accountSuffix 标识账户。
// 输出：精确匹配或后四位唯一匹配时返回别名；不存在时返回空字符串。
// 副作用：只读 SQLite。
func (r *Repository) AccountAlias(ctx context.Context, brokerName, accountSuffix string) (string, error) {
	// 1. 优先按券商和后四位精确查询。
	var alias string
	err := r.db.QueryRowContext(ctx, `
		SELECT account_alias FROM finance_broker_account
		WHERE broker_name = ? AND account_suffix = ? AND is_active = 1
		LIMIT 1
	`, brokerName, accountSuffix).Scan(&alias)
	if err == nil {
		return alias, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("查询账户别名: %w", err)
	}

	// 2. 券商 OCR 异常时只接受后四位唯一匹配。
	rows, err := r.db.QueryContext(ctx, `
		SELECT account_alias FROM finance_broker_account
		WHERE account_suffix = ? AND is_active = 1
		ORDER BY id
		LIMIT 2
	`, accountSuffix)
	if err != nil {
		return "", fmt.Errorf("按后四位查询账户别名: %w", err)
	}
	defer rows.Close()
	aliases := make([]string, 0, 2)
	for rows.Next() {
		if err := rows.Scan(&alias); err != nil {
			return "", fmt.Errorf("扫描账户别名: %w", err)
		}
		aliases = append(aliases, alias)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("遍历账户别名: %w", err)
	}
	if len(aliases) == 1 {
		return aliases[0], nil
	}
	return "", nil
}

// Upsert 新增或覆盖同日同账户资产快照，并按解析状态替换持仓明细。
// 输入：ctx 控制事务，snapshot 是解析结果，rawOCR 是原始响应，createdBy 是操作用户。
// 输出：返回写入后的完整快照；失败时返回带业务上下文的错误。
// 副作用：写入 finance_asset_snapshot 和可选的持仓明细表。
func (r *Repository) Upsert(ctx context.Context, snapshot Snapshot, rawOCR map[string]any, createdBy string) (Snapshot, error) {
	// 1. 序列化审计字段并开始原子事务。
	rawJSON, err := json.Marshal(rawOCR)
	if err != nil {
		return Snapshot{}, fmt.Errorf("序列化 OCR 响应: %w", err)
	}
	warningsJSON, err := json.Marshal(snapshot.Warnings)
	if err != nil {
		return Snapshot{}, fmt.Errorf("序列化仓位提示: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("开始仓位快照事务: %w", err)
	}
	defer tx.Rollback()

	// 2. 按日期和账户后四位 upsert 资产快照并取得稳定主键。
	err = tx.QueryRowContext(ctx, `
		INSERT INTO finance_asset_snapshot (
			snapshot_date, broker_name, source_app, account_suffix, account_alias,
			total_asset, market_value, available_cash, other_amount, position_percent,
			image_path, image_sha256, ocr_provider, provider_request_id,
			raw_ocr_json, warnings_json, parse_status, created_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'parsed', ?)
		ON CONFLICT(snapshot_date, account_suffix) DO UPDATE SET
			broker_name = excluded.broker_name,
			source_app = excluded.source_app,
			account_alias = excluded.account_alias,
			total_asset = excluded.total_asset,
			market_value = excluded.market_value,
			available_cash = excluded.available_cash,
			other_amount = excluded.other_amount,
			position_percent = excluded.position_percent,
			image_path = excluded.image_path,
			image_sha256 = excluded.image_sha256,
			ocr_provider = excluded.ocr_provider,
			provider_request_id = excluded.provider_request_id,
			raw_ocr_json = excluded.raw_ocr_json,
			warnings_json = excluded.warnings_json,
			parse_status = 'parsed',
			created_by = excluded.created_by,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id
	`, snapshot.SnapshotDate, snapshot.BrokerName, snapshot.SourceApp, snapshot.AccountSuffix, nullableText(snapshot.AccountAlias),
		snapshot.TotalAsset, snapshot.MarketValue, snapshot.AvailableCash, snapshot.OtherAmount, snapshot.PositionPercent,
		nullableText(snapshot.ImagePath), nullableText(snapshot.ImageSHA256), nullableText(snapshot.OCRProvider), nullableText(snapshot.ProviderRequestID),
		string(rawJSON), string(warningsJSON), nullableText(createdBy)).Scan(&snapshot.ID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("写入账户资产快照: %w", err)
	}

	// 3. 仅在本次明细解析成功时整体替换旧持仓。
	if snapshot.HoldingsParsed {
		if err := replaceHoldings(ctx, tx, snapshot); err != nil {
			return Snapshot{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Snapshot{}, fmt.Errorf("提交仓位快照事务: %w", err)
	}

	// 4. 重新读取数据库生成与查询接口一致的响应。
	stored, err := r.ByID(ctx, snapshot.ID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("读取已保存仓位快照: %w", err)
	}
	return stored, nil
}

// Recent 读取最近账户资产快照。
// 输入：ctx 控制查询，limit 是 1 到 200 的记录上限。
// 输出：按日期倒序、账户正序返回快照；失败时返回错误。
// 副作用：只读 SQLite。
func (r *Repository) Recent(ctx context.Context, limit int) ([]Snapshot, error) {
	// 1. 约束查询范围，防止页面无条件读取全部历史数据。
	if limit < 1 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := r.db.QueryContext(ctx, snapshotSelect+`
		ORDER BY snapshot_date DESC, account_suffix ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("查询最近仓位快照: %w", err)
	}
	defer rows.Close()

	// 2. 扫描并返回稳定 JSON 模型。
	results := make([]Snapshot, 0)
	for rows.Next() {
		snapshot, err := scanSnapshot(rows)
		if err != nil {
			return nil, fmt.Errorf("扫描最近仓位快照: %w", err)
		}
		results = append(results, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历最近仓位快照: %w", err)
	}
	return results, nil
}

// ByID 按主键读取单个资产快照。
// 输入：ctx 控制查询，id 是快照主键。
// 输出：返回快照；不存在或查询失败时返回错误。
// 副作用：只读 SQLite。
func (r *Repository) ByID(ctx context.Context, id int64) (Snapshot, error) {
	// 1. 执行主键范围查询并复用统一扫描逻辑。
	snapshot, err := scanSnapshot(r.db.QueryRowContext(ctx, snapshotSelect+" WHERE id = ?", id))
	if err != nil {
		return Snapshot{}, fmt.Errorf("按主键查询仓位快照: %w", err)
	}
	return snapshot, nil
}

// HoldingsByDate 读取指定日期的全部持仓明细。
// 输入：ctx 控制查询，snapshotDate 是 ISO 日期。
// 输出：按市值倒序返回持仓；失败时返回错误。
// 副作用：只读 SQLite。
func (r *Repository) HoldingsByDate(ctx context.Context, snapshotDate string) ([]Holding, error) {
	// 1. 使用日期范围约束查询并读取可空数值。
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, snapshot_date, broker_name, source_app, account_suffix,
		       COALESCE(account_alias, ''), security_name, COALESCE(security_code, ''),
		       market_value, quantity, available_quantity, profit_amount,
		       profit_percent, cost_price, current_price
		FROM finance_position_holding_snapshot
		WHERE snapshot_date = ?
		ORDER BY market_value DESC, id ASC
	`, snapshotDate)
	if err != nil {
		return nil, fmt.Errorf("查询日期持仓明细: %w", err)
	}
	defer rows.Close()
	results := make([]Holding, 0)
	for rows.Next() {
		var item Holding
		var quantity, available, profitAmount, profitPercent, costPrice, currentPrice sql.NullFloat64
		if err := rows.Scan(
			&item.ID, &item.SnapshotDate, &item.BrokerName, &item.SourceApp, &item.AccountSuffix,
			&item.AccountAlias, &item.SecurityName, &item.SecurityCode, &item.MarketValue,
			&quantity, &available, &profitAmount, &profitPercent, &costPrice, &currentPrice,
		); err != nil {
			return nil, fmt.Errorf("扫描日期持仓明细: %w", err)
		}
		item.Quantity = nullFloatPointer(quantity)
		item.AvailableQuantity = nullFloatPointer(available)
		item.ProfitAmount = nullFloatPointer(profitAmount)
		item.ProfitPercent = nullFloatPointer(profitPercent)
		item.CostPrice = nullFloatPointer(costPrice)
		item.CurrentPrice = nullFloatPointer(currentPrice)
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历日期持仓明细: %w", err)
	}
	return results, nil
}

const snapshotSelect = `
	SELECT id, snapshot_date, broker_name, source_app, account_suffix,
	       COALESCE(account_alias, ''), total_asset, market_value, available_cash,
	       other_amount, position_percent, COALESCE(image_path, ''),
	       COALESCE(image_sha256, ''), COALESCE(ocr_provider, ''),
	       COALESCE(provider_request_id, ''), COALESCE(warnings_json, '[]'),
	       COALESCE(created_by, ''), created_at, updated_at
	FROM finance_asset_snapshot`

type rowScanner interface {
	Scan(dest ...any) error
}

// replaceHoldings 删除并重建指定日期和账户的持仓明细。
// 输入：ctx 控制事务，tx 是当前事务，snapshot 包含待写入明细。
// 输出：成功返回 nil，失败返回错误。
// 副作用：写入 finance_position_holding_snapshot。
func replaceHoldings(ctx context.Context, tx *sql.Tx, snapshot Snapshot) error {
	// 1. 删除同日同账户旧明细。
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM finance_position_holding_snapshot
		WHERE snapshot_date = ? AND account_suffix = ?
	`, snapshot.SnapshotDate, snapshot.AccountSuffix); err != nil {
		return fmt.Errorf("删除旧持仓明细: %w", err)
	}

	// 2. 逐条写入本次 OCR 解析结果。
	for _, holding := range snapshot.Holdings {
		if strings.TrimSpace(holding.SecurityName) == "" {
			return fmt.Errorf("写入持仓明细: 证券名称不能为空")
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO finance_position_holding_snapshot (
				snapshot_date, broker_name, source_app, account_suffix, account_alias,
				security_name, security_code, market_value, quantity, available_quantity,
				profit_amount, profit_percent, cost_price, current_price, image_sha256
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, snapshot.SnapshotDate, snapshot.BrokerName, snapshot.SourceApp, snapshot.AccountSuffix,
			nullableText(snapshot.AccountAlias), holding.SecurityName, nullableText(holding.SecurityCode),
			holding.MarketValue, holding.Quantity, holding.AvailableQuantity, holding.ProfitAmount,
			holding.ProfitPercent, holding.CostPrice, holding.CurrentPrice, nullableText(snapshot.ImageSHA256))
		if err != nil {
			return fmt.Errorf("写入持仓明细 %s: %w", holding.SecurityName, err)
		}
	}
	return nil
}

// scanSnapshot 把数据库行转换为仓位快照。
// 输入：scanner 是 QueryRow 或 Rows。
// 输出：返回快照；扫描或提示 JSON 无效时返回错误。
// 副作用：无。
func scanSnapshot(scanner rowScanner) (Snapshot, error) {
	// 1. 扫描数据库字段和可空仓位百分比。
	var snapshot Snapshot
	var positionPercent sql.NullFloat64
	var warningsJSON string
	if err := scanner.Scan(
		&snapshot.ID, &snapshot.SnapshotDate, &snapshot.BrokerName, &snapshot.SourceApp,
		&snapshot.AccountSuffix, &snapshot.AccountAlias, &snapshot.TotalAsset, &snapshot.MarketValue,
		&snapshot.AvailableCash, &snapshot.OtherAmount, &positionPercent, &snapshot.ImagePath,
		&snapshot.ImageSHA256, &snapshot.OCRProvider, &snapshot.ProviderRequestID, &warningsJSON,
		&snapshot.CreatedBy, &snapshot.CreatedAt, &snapshot.UpdatedAt,
	); err != nil {
		return Snapshot{}, err
	}
	snapshot.PositionPercent = nullFloatPointer(positionPercent)

	// 2. 解码提示数组并保证空值返回 JSON 数组。
	if err := json.Unmarshal([]byte(warningsJSON), &snapshot.Warnings); err != nil {
		return Snapshot{}, fmt.Errorf("解析仓位提示 JSON: %w", err)
	}
	if snapshot.Warnings == nil {
		snapshot.Warnings = []string{}
	}
	return snapshot, nil
}

// nullableText 把空字符串转换为数据库 NULL。
// 输入：value 是待写入文本。
// 输出：空白值返回 nil，其他值返回原文本。
// 副作用：无。
func nullableText(value string) any {
	// 1. 统一可选文本的数据库转换规则。
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

// nullFloatPointer 把 sql.NullFloat64 转换为 JSON 可空指针。
// 输入：value 是数据库可空浮点数。
// 输出：有效时返回数值指针，否则返回 nil。
// 副作用：无。
func nullFloatPointer(value sql.NullFloat64) *float64 {
	// 1. 保持数据库 NULL 与 JSON null 语义一致。
	if !value.Valid {
		return nil
	}
	result := value.Float64
	return &result
}
