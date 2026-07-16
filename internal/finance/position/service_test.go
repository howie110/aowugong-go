package position

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

type fixedOCR struct{}

// Recognize 返回仓位上传测试使用的固定 OCR 内容。
// 输入：ctx 和 image 模拟正式客户端参数。
// 输出：返回可解析的账户资产 OCR 数据。
// 副作用：无。
func (fixedOCR) Recognize(ctx context.Context, image []byte) (map[string]any, error) {
	// 1. 返回两个识别阶段都可安全使用的全文结果。
	return map[string]any{
		"request_id": "test-ocr",
		"data":       map[string]any{"content": "账户 **5042\n总资产 100000\n总市值 60000\n可用 40000\n仓位 60%"},
	}, nil
}

// TestServiceProcessesImageAndStoresSnapshot 验证图片校验、OCR、解析和入库完整流程。
// 输入：有效 PNG 图片和固定 OCR 客户端。
// 输出：返回 saved 结果，并在上传目录和 MySQL 中留下快照。
// 副作用：在测试临时目录写入图片、裁剪图并写入隔离 MySQL schema。
func TestServiceProcessesImageAndStoresSnapshot(t *testing.T) {
	// 1. 创建迁移数据库、默认账户和上传服务。
	ctx := context.Background()
	root := t.TempDir()
	db := testdatabase.Open(t)
	repository := NewRepository(db)
	if err := repository.SyncDefaultAccounts(ctx); err != nil {
		t.Fatalf("SyncDefaultAccounts() error = %v", err)
	}
	service := NewService(repository, fixedOCR{}, UploadOptions{
		UploadDir: filepath.Join(root, "uploads"), TempDir: filepath.Join(root, "temp"),
		MaxBytes: 1024 * 1024, OCRProvider: "aliyun",
	})

	// 2. 生成有效图片并通过批量入口处理。
	response := service.ProcessBatch(ctx, BatchRequest{
		SnapshotDate: "2026-07-15", BrokerName: "东莞证券", SourceApp: "同花顺", CreatedBy: "admin",
		Files: []Upload{{Filename: "position.png", Data: makePositionTestPNG(t)}},
	})
	if len(response.Results) != 1 || response.Results[0].Status != "saved" || response.Results[0].Snapshot == nil {
		t.Fatalf("response = %#v, want one saved result", response)
	}

	// 3. 核对图片和数据库快照均已持久化。
	storedPath := response.Results[0].Snapshot.ImagePath
	if _, err := os.Stat(storedPath); err != nil {
		t.Errorf("stored image stat error = %v", err)
	}
	recent, err := service.Recent(ctx, 20)
	if err != nil {
		t.Fatalf("Recent() error = %v", err)
	}
	if len(recent) != 1 || recent[0].AccountAlias != "东莞证券-邓子豪" {
		t.Errorf("recent = %#v, want stored default account", recent)
	}
}

// makePositionTestPNG 创建仓位上传测试使用的小型 PNG。
// 输入：t 管理测试失败。
// 输出：返回有效 PNG 二进制。
// 副作用：无，仅写入内存缓冲区。
func makePositionTestPNG(t *testing.T) []byte {
	// 1. 创建满足 OCR 尺寸要求的纯色图片。
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 600, 800))
	for y := 0; y < 800; y++ {
		for x := 0; x < 600; x++ {
			img.Set(x, y, color.RGBA{R: 245, G: 245, B: 245, A: 255})
		}
	}

	// 2. 编码为 PNG 并返回。
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("png.Encode() error = %v", err)
	}
	return buffer.Bytes()
}
