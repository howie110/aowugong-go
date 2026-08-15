package position

import "testing"

// TestParseAssetSnapshotReadsAccountAndMoneyFromOCRContent 验证仓位截图文本回退解析。
// 输入：包含账户后四位、资产、市值、现金和仓位的 OCR 内容。
// 输出：返回金额勾稽一致的账户快照，并修正常见 OCR 字母误识别。
// 副作用：无。
func TestParseAssetSnapshotReadsAccountAndMoneyFromOCRContent(t *testing.T) {
	// 1. 构造阿里云 OCR 规范化响应。
	raw := map[string]any{
		"request_id": "ocr-request-1",
		"data": map[string]any{
			"content": "账户 **5O42\n总资产 100,000.00\n总市值 60,000.00\n可用 40,000.00\n仓位 60.00%",
		},
	}

	// 2. 解析资产快照并核对金额和账户。
	snapshot, err := ParseAssetSnapshot(raw, AssetMetadata{
		SnapshotDate: "2026-07-15", BrokerName: "东莞证券", SourceApp: "同花顺",
		ImagePath: "storage/uploads/positions/test.png", ImageSHA256: "abc", OCRProvider: "aliyun",
	})
	if err != nil {
		t.Fatalf("ParseAssetSnapshot() error = %v", err)
	}
	if snapshot.AccountSuffix != "5042" {
		t.Errorf("account suffix = %q, want 5042", snapshot.AccountSuffix)
	}
	if snapshot.TotalAsset != 100000 || snapshot.MarketValue != 60000 || snapshot.AvailableCash != 40000 || snapshot.OtherAmount != 0 {
		t.Errorf("snapshot money = %#v", snapshot)
	}
	if snapshot.PositionPercent == nil || *snapshot.PositionPercent != 60 {
		t.Errorf("position percent = %#v, want 60", snapshot.PositionPercent)
	}
	if snapshot.ProviderRequestID != "ocr-request-1" || len(snapshot.Warnings) != 0 {
		t.Errorf("snapshot metadata = %#v", snapshot)
	}
}

// TestParseNumberRepairsOCRThousandsSeparators 验证金额千分位被 OCR 识别成小数点时仍能还原原值。
// 输入：普通千分位、多小数点金额和中文负号数字。
// 输出：每个数字都返回预期金额。
// 副作用：无。
func TestParseNumberRepairsOCRThousandsSeparators(t *testing.T) {
	// 1. 覆盖线上出现的多小数点格式及原有常规格式。
	cases := map[string]float64{
		"422.345.00": 422345.00,
		"1,234.56":   1234.56,
		"558，851.72": 558851.72,
		"－2.430.28":  -2430.28,
	}
	for input, expected := range cases {
		actual, ok := parseNumber(input)
		if !ok || actual != expected {
			t.Errorf("parseNumber(%q) = %v, %v; want %v, true", input, actual, ok, expected)
		}
	}
}
