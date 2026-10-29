package articleanalysis

import (
	"context"
	"testing"

	"github.com/howiedata/aowugong-go/internal/client"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// TestUpdateWeReadAccountIsAtomic 验证设置和开关一次写入，非法设置不会改动开关。
func TestUpdateWeReadAccountIsAtomic(t *testing.T) {
	// 1. 准备一个已发现的公众号和可执行更新的服务。
	ctx := context.Background()
	db := testdatabase.Open(t)
	repository := NewRepository(db)
	if err := repository.upsertWeReadAccounts(ctx, []WeReadAccount{{
		AccountID: "MP_WXS_100", Title: "测试公众号",
	}}); err != nil {
		t.Fatalf("upsertWeReadAccounts() error = %v", err)
	}
	service := NewService(repository, ServiceOptions{
		WeRead: NewWeReadSource(repository, client.NewWeReadArticleClient(nil), "test-secret"),
	})

	// 2. 同时更新设置和开关。
	enabled := false
	settings := WeReadAccountSettings{FetchIntervalMinutes: 60, FetchLimit: 5}
	if err := service.UpdateWeReadAccount(ctx, "MP_WXS_100", &settings, &enabled); err != nil {
		t.Fatalf("UpdateWeReadAccount() error = %v", err)
	}
	var interval, limit int
	var isEnabled bool
	if err := db.QueryRowContext(ctx, `SELECT is_enabled, fetch_interval_minutes, fetch_limit
		FROM weread_article_account WHERE account_id = 'MP_WXS_100'`).Scan(&isEnabled, &interval, &limit); err != nil {
		t.Fatalf("query account: %v", err)
	}
	if isEnabled || interval != 60 || limit != 5 {
		t.Fatalf("account = enabled:%v interval:%d limit:%d", isEnabled, interval, limit)
	}

	// 3. 非法设置必须在写库前失败，不能留下“开关已改、设置失败”的部分状态。
	invalid := WeReadAccountSettings{FetchIntervalMinutes: 1, FetchLimit: 5}
	enabled = true
	if err := service.UpdateWeReadAccount(ctx, "MP_WXS_100", &invalid, &enabled); err == nil {
		t.Fatal("UpdateWeReadAccount() error = nil, want validation error")
	}
	if err := db.QueryRowContext(ctx, `SELECT is_enabled, fetch_interval_minutes, fetch_limit
		FROM weread_article_account WHERE account_id = 'MP_WXS_100'`).Scan(&isEnabled, &interval, &limit); err != nil {
		t.Fatalf("query account after invalid update: %v", err)
	}
	if isEnabled || interval != 60 || limit != 5 {
		t.Fatalf("account after invalid update = enabled:%v interval:%d limit:%d", isEnabled, interval, limit)
	}

	// 4. 只传开关时保留原设置。
	enabled = true
	if err := service.UpdateWeReadAccount(ctx, "MP_WXS_100", nil, &enabled); err != nil {
		t.Fatalf("enabled-only UpdateWeReadAccount() error = %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT is_enabled, fetch_interval_minutes, fetch_limit
		FROM weread_article_account WHERE account_id = 'MP_WXS_100'`).Scan(&isEnabled, &interval, &limit); err != nil {
		t.Fatalf("query account after enabled-only update: %v", err)
	}
	if !isEnabled || interval != 60 || limit != 5 {
		t.Fatalf("account after enabled-only update = enabled:%v interval:%d limit:%d", isEnabled, interval, limit)
	}
}
