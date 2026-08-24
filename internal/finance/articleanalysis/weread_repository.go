package articleanalysis

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	appdatabase "github.com/howiedata/aowugong-go/internal/database"
)

type weReadCredentialHealth struct {
	BoundAt       string
	LastCheckedAt string
	LastValidAt   string
	InvalidAt     string
	LastStatus    string
	LastError     string
	CheckCount    int64
	RefreshCount  int64
}

// SyncWeReadSource 把唯一投资文章来源切换为微信读书公众号。
// 输入：ctx 控制数据库操作。
// 输出：返回同步错误。
// 副作用：写入 investment_article_source，并按凭据是否存在决定是否启用。
func (r *Repository) SyncWeReadSource(ctx context.Context) error {
	// 1. 只有绑定凭据存在时才启用定时抓取，避免未绑定期间重复失败通知。
	var credentialCount int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM weread_article_credential WHERE id = 1").Scan(&credentialCount); err != nil {
		return fmt.Errorf("读取微信读书绑定状态: %w", err)
	}
	active := credentialCount == 1
	return r.upsertWeReadSource(ctx, active)
}

// SetWeReadSourceActive 更新微信读书投资文章来源启用状态。
// 输入：ctx 控制数据库操作，active 表示是否已经成功绑定。
// 输出：返回更新错误。
// 副作用：写入 investment_article_source。
func (r *Repository) SetWeReadSourceActive(ctx context.Context, active bool) error {
	// 1. 复用唯一来源写入逻辑，保留历史文章关联主键。
	return r.upsertWeReadSource(ctx, active)
}

// weReadSourceFetchStatus 读取微信读书来源最近一次实际抓取状态。
// 输入：ctx 控制 PostgreSQL 查询。
// 输出：返回状态和消息；来源尚未创建时返回空值。
// 副作用：只读 PostgreSQL。
func (r *Repository) weReadSourceFetchStatus(ctx context.Context) (string, string, error) {
	// 1. 读取唯一微信读书来源，区分凭据已保存和文章实际可抓取。
	var status, message string
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(last_fetch_status, ''), COALESCE(last_fetch_message, '')
		FROM investment_article_source
		WHERE source_code = 'wechat_aggregate'
	`).Scan(&status, &message)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("读取微信读书最近抓取状态: %w", err)
	}
	return status, message, nil
}

// clearWeReadSourceFetchError 清除重新绑定前遗留的抓取错误。
// 输入：ctx 控制 PostgreSQL 更新。
// 输出：更新失败时返回带上下文的错误。
// 副作用：更新 investment_article_source，但不伪造抓取时间。
func (r *Repository) clearWeReadSourceFetchError(ctx context.Context) error {
	// 1. 仅把来源恢复为待抓取状态，下一次真实抓取会覆盖结果。
	_, err := r.db.ExecContext(ctx, `
		UPDATE investment_article_source
		SET last_fetch_status = 'ready', last_fetch_message = NULL, updated_at = ?
		WHERE source_code = 'wechat_aggregate'
	`, appdatabase.TimestampText(time.Now()))
	if err != nil {
		return fmt.Errorf("清除微信读书历史抓取错误: %w", err)
	}
	return nil
}

// upsertWeReadSource 写入唯一微信读书文章来源元数据。
// 输入：ctx 控制数据库操作，active 决定任务是否读取该来源。
// 输出：返回写入错误。
// 副作用：写入 investment_article_source。
func (r *Repository) upsertWeReadSource(ctx context.Context, active bool) error {
	// 1. 使用原来源代码复用全部历史文章关联。
	status := "unbound"
	message := "尚未绑定微信读书"
	if active {
		status = "ready"
		message = ""
	}
	activeValue := 0
	if active {
		activeValue = 1
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO investment_article_source (
			source_code, source_name, source_type, feed_url, is_active,
			description, last_fetch_status, last_fetch_message
		) VALUES ('wechat_aggregate', '微信读书公众号', 'weread', 'weread://shelf', ?,
		          '直接读取微信读书书架中已启用公众号的最新文章。', ?, ?)
		ON CONFLICT(source_code) DO UPDATE SET
			source_name = '微信读书公众号', source_type = 'weread',
			feed_url = 'weread://shelf', is_active = excluded.is_active,
			description = excluded.description,
			last_fetch_status = CASE WHEN excluded.is_active = 1 AND COALESCE(investment_article_source.last_fetch_status, '') NOT IN ('', 'unbound') THEN investment_article_source.last_fetch_status ELSE excluded.last_fetch_status END,
			last_fetch_message = CASE WHEN excluded.is_active = 1 AND COALESCE(investment_article_source.last_fetch_status, '') NOT IN ('', 'unbound') THEN investment_article_source.last_fetch_message ELSE excluded.last_fetch_message END,
			updated_at = ?
	`, activeValue, status, nullableArticleText(message), appdatabase.TimestampText(time.Now()))
	if err != nil {
		return fmt.Errorf("同步微信读书文章来源: %w", err)
	}
	return nil
}

// loadWeReadCredentialCiphertext 读取单账号加密凭据。
// 输入：ctx 控制数据库查询。
// 输出：返回密文、版本、是否存在和错误。
// 副作用：只读 PostgreSQL。
func (r *Repository) loadWeReadCredentialCiphertext(ctx context.Context) ([]byte, int, bool, error) {
	// 1. 查询固定主键并区分未绑定状态。
	var ciphertext []byte
	var version int
	err := r.db.QueryRowContext(ctx, "SELECT ciphertext, encryption_version FROM weread_article_credential WHERE id = 1").Scan(&ciphertext, &version)
	if err == sql.ErrNoRows {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, fmt.Errorf("读取微信读书加密凭据: %w", err)
	}
	return ciphertext, version, true, nil
}

// saveWeReadCredentialCiphertext 原子保存单账号加密凭据和绑定生命周期。
// 输入：ctx 控制写入，ciphertext 是认证密文，version 是格式版本，newBinding 区分扫码和自动刷新。
// 输出：返回写入错误。
// 副作用：写入 weread_article_credential。
func (r *Repository) saveWeReadCredentialCiphertext(ctx context.Context, ciphertext []byte, version int, newBinding bool) error {
	// 1. 扫码重置寿命统计，自动刷新只替换密文并累计刷新次数。
	now := appdatabase.TimestampText(time.Now())
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO weread_article_credential(
			id, ciphertext, encryption_version, updated_at, bound_at, last_checked_at,
			last_valid_at, invalid_at, last_status, last_error, check_count, refresh_count
		)
		VALUES (1, ?, ?, ?, ?, ?, ?, NULL, 'valid', '', 0, 0)
		ON CONFLICT(id) DO UPDATE SET
			ciphertext = excluded.ciphertext,
			encryption_version = excluded.encryption_version,
			updated_at = excluded.updated_at,
			bound_at = CASE WHEN ? THEN excluded.bound_at ELSE weread_article_credential.bound_at END,
			last_checked_at = CASE WHEN ? THEN excluded.last_checked_at ELSE weread_article_credential.last_checked_at END,
			last_valid_at = CASE WHEN ? THEN excluded.last_valid_at ELSE weread_article_credential.last_valid_at END,
			invalid_at = CASE WHEN ? THEN NULL ELSE weread_article_credential.invalid_at END,
			last_status = CASE WHEN ? THEN 'valid' ELSE weread_article_credential.last_status END,
			last_error = CASE WHEN ? THEN '' ELSE weread_article_credential.last_error END,
			check_count = CASE WHEN ? THEN 0 ELSE weread_article_credential.check_count END,
			refresh_count = CASE WHEN ? THEN 0 ELSE weread_article_credential.refresh_count + 1 END
	`, ciphertext, version, now, now, now, now,
		newBinding, newBinding, newBinding, newBinding, newBinding, newBinding, newBinding, newBinding)
	if err != nil {
		return fmt.Errorf("保存微信读书加密凭据: %w", err)
	}
	return nil
}

// recordWeReadCredentialCheck 记录一次凭据探测结果。
// 输入：ctx 控制写入，status 是 valid、invalid 或 error，message 保存失败上下文。
// 输出：返回更新错误。
// 副作用：更新 weread_article_credential 的检查计数和时间。
func (r *Repository) recordWeReadCredentialCheck(ctx context.Context, status, message string, checkedAt time.Time) error {
	// 1. 有效检查推进最后有效时间，认证失效只固定记录首次失效时间。
	now := appdatabase.TimestampText(checkedAt)
	_, err := r.db.ExecContext(ctx, `
		UPDATE weread_article_credential SET
			last_checked_at = ?,
			last_valid_at = CASE WHEN ? = 'valid' THEN ? ELSE last_valid_at END,
			invalid_at = CASE
				WHEN ? = 'invalid' THEN COALESCE(invalid_at, ?)
				WHEN ? = 'valid' THEN NULL
				ELSE invalid_at
			END,
			last_status = ?, last_error = ?, check_count = check_count + 1
		WHERE id = 1
	`, now, status, now, status, now, status, status, message)
	if err != nil {
		return fmt.Errorf("记录微信读书凭据检查: %w", err)
	}
	return nil
}

// weReadCredentialHealth 读取当前凭据绑定寿命累计值。
// 输入：ctx 控制查询。
// 输出：返回健康统计、是否存在和错误。
// 副作用：只读 PostgreSQL。
func (r *Repository) weReadCredentialHealth(ctx context.Context) (weReadCredentialHealth, bool, error) {
	// 1. 查询单账号聚合字段并把 NULL 时间转换为空字符串。
	var health weReadCredentialHealth
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(bound_at, updated_at), COALESCE(last_checked_at, ''),
		       COALESCE(last_valid_at, ''), COALESCE(invalid_at, ''), last_status,
		       COALESCE(last_error, ''), check_count, refresh_count
		FROM weread_article_credential WHERE id = 1
	`).Scan(&health.BoundAt, &health.LastCheckedAt, &health.LastValidAt, &health.InvalidAt,
		&health.LastStatus, &health.LastError, &health.CheckCount, &health.RefreshCount)
	if err == sql.ErrNoRows {
		return weReadCredentialHealth{}, false, nil
	}
	if err != nil {
		return weReadCredentialHealth{}, false, fmt.Errorf("读取微信读书凭据健康统计: %w", err)
	}
	return health, true, nil
}

// upsertWeReadAccounts 保存本次书架发现的公众号并保留人工开关。
// 输入：ctx 控制事务，accounts 是最新书架公众号。
// 输出：返回写入错误。
// 副作用：写入 weread_article_account。
func (r *Repository) upsertWeReadAccounts(ctx context.Context, accounts []WeReadAccount) error {
	// 1. 在一个事务内更新标题和封面，冲突时不覆盖 is_enabled。
	transaction, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始保存微信读书公众号事务: %w", err)
	}
	defer transaction.Rollback()
	now := appdatabase.TimestampText(time.Now())
	for _, account := range accounts {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO weread_article_account(
				account_id, title, cover_url, is_enabled, fetch_interval_minutes, fetch_limit, discovered_at, updated_at
			)
			VALUES (?, ?, ?, TRUE, 720, 20, ?, ?)
			ON CONFLICT(account_id) DO UPDATE SET
				title = excluded.title, cover_url = excluded.cover_url, is_enabled = TRUE, updated_at = excluded.updated_at
		`, account.AccountID, account.Title, account.CoverURL, now, now); err != nil {
			return fmt.Errorf("保存微信读书公众号 %s: %w", account.Title, err)
		}
	}

	// 2. 删除已不在当前书架中的旧账号，避免重新绑定后继续抓取上一账号的公众号。
	if len(accounts) == 0 {
		if _, err := transaction.ExecContext(ctx, "DELETE FROM weread_article_account"); err != nil {
			return fmt.Errorf("清理微信读书旧公众号: %w", err)
		}
	} else {
		placeholders := make([]string, len(accounts))
		arguments := make([]any, len(accounts))
		for index, account := range accounts {
			placeholders[index] = "?"
			arguments[index] = account.AccountID
		}
		if _, err := transaction.ExecContext(ctx,
			"DELETE FROM weread_article_account WHERE account_id NOT IN ("+strings.Join(placeholders, ",")+")",
			arguments...,
		); err != nil {
			return fmt.Errorf("删除已离开书架的微信公众号: %w", err)
		}
	}

	// 3. 原子提交书架快照和人工开关保留结果。
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("提交微信读书公众号事务: %w", err)
	}
	return nil
}

// listWeReadAccounts 返回全部书架公众号。
// 输入：ctx 控制查询，enabledOnly 决定是否只返回启用项。
// 输出：返回按标题排序的公众号。
// 副作用：只读 PostgreSQL。
func (r *Repository) listWeReadAccounts(ctx context.Context, enabledOnly bool) ([]WeReadAccount, error) {
	// 1. 执行固定列查询并按展示标题排序。
	query := `SELECT account_id, title, cover_url, is_enabled,
		fetch_interval_minutes, fetch_limit, COALESCE(last_checked_at, ''),
		(SELECT COUNT(*) FROM investment_article article WHERE article.author = weread_article_account.title),
		(SELECT COUNT(*) FROM investment_article article
		 WHERE article.author = weread_article_account.title
		   AND SUBSTR(COALESCE(article.created_at, ''), 1, 10) = SUBSTR(?, 1, 10)),
		(SELECT COUNT(*) FROM investment_article article
		 LEFT JOIN investment_article_analysis analysis ON analysis.article_id = article.id AND analysis.status = 'success'
		 WHERE article.author = weread_article_account.title
		   AND article.fetch_status <> 'pending_parse' AND analysis.id IS NULL),
		(SELECT COUNT(*) FROM investment_article article
		 WHERE article.author = weread_article_account.title AND article.fetch_status = 'pending_parse'),
		COALESCE((SELECT MAX(article.created_at) FROM investment_article article WHERE article.author = weread_article_account.title), '')
		FROM weread_article_account`
	if enabledOnly {
		query += " WHERE is_enabled = TRUE"
	}
	query += " ORDER BY title ASC, account_id ASC"
	rows, err := r.db.QueryContext(ctx, query, appdatabase.TimestampText(time.Now()))
	if err != nil {
		return nil, fmt.Errorf("查询微信读书公众号: %w", err)
	}
	defer rows.Close()
	accounts := make([]WeReadAccount, 0)
	for rows.Next() {
		var account WeReadAccount
		if err := rows.Scan(&account.AccountID, &account.Title, &account.CoverURL, &account.Enabled, &account.FetchIntervalMinutes, &account.FetchLimit, &account.LastCheckedAt, &account.ArticleCount, &account.TodayInsertedCount, &account.PendingCount, &account.UnparsedCount, &account.LatestFetchedAt); err != nil {
			return nil, fmt.Errorf("扫描微信读书公众号: %w", err)
		}
		if account.FetchIntervalMinutes < 1 {
			account.FetchIntervalMinutes = 720
		}
		if account.FetchLimit < 1 {
			account.FetchLimit = 20
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历微信读书公众号: %w", err)
	}
	return accounts, nil
}

// setWeReadAccountEnabled 更新一个书架公众号的抓取开关。
// 输入：ctx 控制写入，accountID 是公众号，enabled 是目标状态。
// 输出：不存在或更新失败时返回错误。
// 副作用：写入 weread_article_account。
func (r *Repository) setWeReadAccountEnabled(ctx context.Context, accountID string, enabled bool) error {
	// 1. 只更新已由书架发现的公众号。
	result, err := r.db.ExecContext(ctx, `
		UPDATE weread_article_account SET is_enabled = ?, updated_at = ? WHERE account_id = ?
	`, enabled, appdatabase.TimestampText(time.Now()), accountID)
	if err != nil {
		return fmt.Errorf("更新微信读书公众号开关: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return fmt.Errorf("微信读书公众号不存在")
	}
	return nil
}

// existingWeReadExternalIDs 批量查找已经入库的 reviewId。
// 输入：ctx 控制查询，sourceID 是聚合来源，externalIDs 是候选标识。
// 输出：返回已存在标识集合。
// 副作用：只读 PostgreSQL。
func (r *Repository) existingWeReadExternalIDs(ctx context.Context, sourceID int64, externalIDs []string) (map[string]struct{}, error) {
	// 1. 空候选直接返回，非空候选构造受控占位符列表。
	result := make(map[string]struct{})
	if len(externalIDs) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(externalIDs))
	arguments := make([]any, 0, len(externalIDs)+1)
	arguments = append(arguments, sourceID)
	for index, value := range externalIDs {
		placeholders[index] = "?"
		arguments = append(arguments, value)
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT external_id FROM investment_article
		WHERE source_id = ? AND external_id IN (`+strings.Join(placeholders, ",")+`)
	`, arguments...)
	if err != nil {
		return nil, fmt.Errorf("批量查询微信读书文章标识: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var externalID string
		if err := rows.Scan(&externalID); err != nil {
			return nil, fmt.Errorf("扫描微信读书文章标识: %w", err)
		}
		result[externalID] = struct{}{}
	}
	return result, rows.Err()
}

// bindWeReadExternalIDByLink 把历史同链接文章绑定到微信读书 reviewId。
// 输入：ctx 控制写入，sourceID、link 和 externalID 标识文章。
// 输出：命中历史文章返回 true。
// 副作用：可能更新 investment_article.external_id。
func (r *Repository) bindWeReadExternalIDByLink(ctx context.Context, sourceID int64, link, externalID string) (bool, error) {
	// 1. 原文链接命中时只替换来源内外部标识，保留原主键、文章键和分析结果。
	result, err := r.db.ExecContext(ctx, `
		UPDATE investment_article SET external_id = ? WHERE source_id = ? AND link = ?
	`, externalID, sourceID, link)
	if err != nil {
		return false, fmt.Errorf("绑定历史微信文章标识: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("读取历史微信文章绑定结果: %w", err)
	}
	return rows > 0, nil
}

// updateWeReadAccountSettings 更新单个公众号的抓取频率和单次数量。
// 输入：ctx 控制写入，accountID 是公众号，settings 是新的抓取节奏。
// 输出：不存在或更新失败时返回错误。
// 副作用：写入 weread_article_account。
func (r *Repository) updateWeReadAccountSettings(ctx context.Context, accountID string, settings WeReadAccountSettings) error {
	// 1. 按已校验参数更新节奏，并保持账号参与抓取。
	result, err := r.db.ExecContext(ctx, `
		UPDATE weread_article_account
		SET is_enabled = TRUE, fetch_interval_minutes = ?, fetch_limit = ?, updated_at = ?
		WHERE account_id = ?
	`, settings.FetchIntervalMinutes, settings.FetchLimit, appdatabase.TimestampText(time.Now()), accountID)
	if err != nil {
		return fmt.Errorf("更新微信读书公众号抓取策略: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return fmt.Errorf("微信读书公众号不存在")
	}
	return nil
}

// markWeReadAccountChecked 记录单个公众号最近一次访问微信读书列表的时间。
// 输入：ctx 控制写入，accountID 是公众号，checkedAt 是本次检查时间。
// 输出：返回写入错误。
// 副作用：写入 weread_article_account。
func (r *Repository) markWeReadAccountChecked(ctx context.Context, accountID string, checkedAt time.Time) error {
	// 1. 只更新节流时间，不改变人工配置。
	_, err := r.db.ExecContext(ctx, `
		UPDATE weread_article_account SET last_checked_at = ?, updated_at = ? WHERE account_id = ?
	`, appdatabase.TimestampText(checkedAt), appdatabase.TimestampText(time.Now()), accountID)
	if err != nil {
		return fmt.Errorf("记录微信读书公众号检查时间: %w", err)
	}
	return nil
}
