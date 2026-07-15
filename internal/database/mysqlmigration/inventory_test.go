package mysqlmigration

import "testing"

// TestAuditedTablesSeparateReachableAndHistoricalData 验证源库二十张有效表和六张历史表边界固定。
// 输入：代码维护的默认迁移表与历史跳过表。
// 输出：有效表共二十张，历史表共六张，且不重叠。
// 副作用：无。
func TestAuditedTablesSeparateReachableAndHistoricalData(t *testing.T) {
	// 1. 收集有效迁移表并核对数量。
	active := make(map[string]bool)
	for _, spec := range DefaultTableSpecs() {
		active[spec.Name] = true
	}
	if len(active) != 20 {
		t.Errorf("active table count = %d, want 20", len(active))
	}

	// 2. 核对六张历史表都有明确跳过原因且不进入有效表。
	historical := HistoricalTables()
	if len(historical) != 6 {
		t.Errorf("historical table count = %d, want 6", len(historical))
	}
	for name, reason := range historical {
		if active[name] {
			t.Errorf("historical table %s also active", name)
		}
		if reason == "" {
			t.Errorf("historical table %s missing reason", name)
		}
	}
}
