package subscription

import (
	"context"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// TestServiceCRUDAndDerivedStatus 验证订阅增删改查和动态到期状态。
// 输入：固定业务日期和一条订阅写入请求。
// 输出：新增、更新、查询和删除结果及派生状态正确。
// 副作用：创建并写入临时 SQLite。
func TestServiceCRUDAndDerivedStatus(t *testing.T) {
	// 1. 创建固定当前日期的订阅服务并新增一条记录。
	service := newTestService(t)
	service.today = func() time.Time { return time.Date(2026, time.July, 15, 0, 0, 0, 0, time.Local) }
	created, err := service.Create(context.Background(), WriteRequest{
		ServiceName: "测试服务",
		Category:    "IT",
		AnnualFee:   "12.345",
		MonthlyFee:  "1",
		StartsOn:    "2026-01-01",
		ExpiresOn:   "2026-07-25",
	}, "admin")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.AnnualFee != "12.35" || created.MonthlyFee != "1.00" {
		t.Errorf("fees = %s/%s, want 12.35/1.00", created.AnnualFee, created.MonthlyFee)
	}
	if created.CurrentStatus != "订阅中" || created.DaysUntilExpiry != 10 {
		t.Errorf("derived status = %s/%d, want 订阅中/10", created.CurrentStatus, created.DaysUntilExpiry)
	}

	// 2. 更新到期日后读取记录并断言状态实时变为已结束。
	updated, err := service.Update(context.Background(), created.ID, WriteRequest{
		ServiceName: "测试服务",
		Category:    "IT",
		AnnualFee:   "12.35",
		MonthlyFee:  "1.00",
		ExpiresOn:   "2026-07-14",
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.CurrentStatus != "已结束" || updated.DaysUntilExpiry != -1 {
		t.Errorf("updated status = %s/%d, want 已结束/-1", updated.CurrentStatus, updated.DaysUntilExpiry)
	}

	// 3. 删除记录后列表中不再包含该服务。
	deleted, err := service.Delete(context.Background(), created.ID)
	if err != nil || !deleted {
		t.Fatalf("Delete() = %v, %v, want true, nil", deleted, err)
	}
	records, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	for _, record := range records {
		if record.ServiceName == "测试服务" {
			t.Fatal("deleted subscription remains in list")
		}
	}
}

// TestServiceListExpiringUsesExactReminderDay 验证到期提醒只匹配正好提前指定天数的记录。
// 输入：固定业务日期和多个不同到期日订阅。
// 输出：只返回精确命中提醒日的记录。
// 副作用：创建并写入临时 SQLite。
func TestServiceListExpiringUsesExactReminderDay(t *testing.T) {
	// 1. 使用默认六条订阅并把当前日期固定在一条记录到期前十天。
	service := newTestService(t)
	service.today = func() time.Time { return time.Date(2027, time.February, 25, 0, 0, 0, 0, time.Local) }

	// 2. 只应返回 2027-03-07 到期的阿里云服务器。
	records, err := service.ListExpiring(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListExpiring() error = %v", err)
	}
	if len(records) != 1 || records[0].ServiceName != "阿里云服务器" {
		t.Errorf("records = %#v, want 阿里云服务器", records)
	}
}

// newTestService 创建使用隔离 SQLite 的订阅测试服务。
// 输入：t 管理临时数据库生命周期。
// 输出：返回已连接临时仓储的订阅服务。
// 副作用：创建并迁移临时 SQLite。
func newTestService(t *testing.T) *Service {
	// 1. 打开隔离 SQLite schema 并执行完整迁移。
	t.Helper()
	db := testdatabase.Open(t)

	// 2. 返回仓储和服务的真实组合。
	return NewService(NewRepository(db))
}
