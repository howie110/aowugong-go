package position

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "golang.org/x/image/webp"
)

const (
	defaultBrokerName = "东莞证券"
	defaultSourceApp  = "同花顺"
)

var allowedImageExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true,
}

// OCRClient 定义仓位服务需要的通用文字识别能力。
type OCRClient interface {
	Recognize(ctx context.Context, image []byte) (map[string]any, error)
}

// UploadOptions 描述仓位截图存储和识别限制。
type UploadOptions struct {
	UploadDir   string
	TempDir     string
	MaxBytes    int
	OCRProvider string
}

// Upload 描述 HTTP 层读取后的单个上传文件。
type Upload struct {
	Filename string
	Data     []byte
}

// BatchRequest 描述一批同日期仓位截图的处理参数。
type BatchRequest struct {
	SnapshotDate string
	BrokerName   string
	SourceApp    string
	CreatedBy    string
	Files        []Upload
}

// ImportSummary 描述股票仓位导入页面摘要。
type ImportSummary struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Metrics     []ImportMetric `json:"metrics"`
}

// ImportMetric 描述股票仓位导入页面单项指标。
type ImportMetric struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Detail string `json:"detail"`
	Status string `json:"status"`
}

// Service 负责仓位截图识别、解析和持久化。
type Service struct {
	repository *Repository
	ocr        OCRClient
	options    UploadOptions
}

// NewService 创建仓位上传服务。
// 输入：repository 提供 SQLite 写入，ocr 提供外部识别，options 提供目录和限制。
// 输出：返回可并发复用的仓位服务。
// 副作用：无，不创建目录或访问外部接口。
func NewService(repository *Repository, ocr OCRClient, options UploadOptions) *Service {
	// 1. 规范化缺省值并保存显式依赖。
	if options.MaxBytes <= 0 {
		options.MaxBytes = 10 * 1024 * 1024
	}
	if strings.TrimSpace(options.OCRProvider) == "" {
		options.OCRProvider = "aliyun"
	}
	return &Service{repository: repository, ocr: ocr, options: options}
}

// Summary 读取仓位导入页面摘要。
// 输入：ctx 控制 SQLite 查询。
// 输出：返回 OCR 能力和最新导入日期；失败时返回错误。
// 副作用：只读 SQLite。
func (s *Service) Summary(ctx context.Context) (ImportSummary, error) {
	// 1. 读取有限条最近记录用于确定最新日期。
	recent, err := s.repository.Recent(ctx, 10)
	if err != nil {
		return ImportSummary{}, fmt.Errorf("读取仓位导入摘要: %w", err)
	}
	latest := "未知"
	if len(recent) > 0 {
		latest = recent[0].SnapshotDate
	}

	// 2. 返回与现有 React 页面一致的指标。
	return ImportSummary{
		Title:       "股票仓位导入",
		Description: "上传同花顺仓位截图，只展示导入记录。",
		Metrics: []ImportMetric{
			{Label: "OCR", Value: "Aliyun", Detail: "RecognizeGeneral", Status: "normal"},
			{Label: "最新日期", Value: latest, Detail: "finance_asset_snapshot", Status: "normal"},
		},
	}, nil
}

// Recent 读取最近仓位导入记录。
// 输入：ctx 控制查询，limit 是记录上限。
// 输出：返回最近快照；失败时返回错误。
// 副作用：只读 SQLite。
func (s *Service) Recent(ctx context.Context, limit int) ([]Snapshot, error) {
	// 1. 复用仓储层唯一查询入口。
	return s.repository.Recent(ctx, limit)
}

// ProcessBatch 顺序处理一批截图并隔离单文件失败。
// 输入：ctx 控制处理，request 包含日期、账户来源、用户和文件。
// 输出：返回每张图片的 saved 或 failed 结果。
// 副作用：写入图片、临时裁剪图、SQLite，并调用阿里云 OCR。
func (s *Service) ProcessBatch(ctx context.Context, request BatchRequest) UploadResponse {
	// 1. 应用页面沿用的券商和来源默认值。
	if strings.TrimSpace(request.BrokerName) == "" {
		request.BrokerName = defaultBrokerName
	}
	if strings.TrimSpace(request.SourceApp) == "" {
		request.SourceApp = defaultSourceApp
	}

	// 2. 逐张处理，单张错误只写入对应结果。
	results := make([]UploadResult, 0, len(request.Files))
	for _, upload := range request.Files {
		result, err := s.processOne(ctx, request, upload)
		if err != nil {
			results = append(results, UploadResult{Filename: upload.Filename, Status: "failed", Error: err.Error()})
			continue
		}
		results = append(results, result)
	}
	return UploadResponse{SnapshotDate: request.SnapshotDate, Results: results}
}

// processOne 处理单张仓位截图并写入快照。
// 输入：ctx 控制处理，request 提供公共参数，upload 提供文件名和内容。
// 输出：成功返回 saved 结果；任一步失败时返回错误。
// 副作用：写入文件和 SQLite，并调用阿里云 OCR 两次。
func (s *Service) processOne(ctx context.Context, request BatchRequest, upload Upload) (UploadResult, error) {
	// 1. 校验日期、文件名、大小和图片像素。
	if _, err := time.Parse(time.DateOnly, request.SnapshotDate); err != nil {
		return UploadResult{}, fmt.Errorf("仓位日期格式无效: %w", err)
	}
	extension := strings.ToLower(filepath.Ext(upload.Filename))
	if !allowedImageExtensions[extension] {
		return UploadResult{}, fmt.Errorf("只支持 jpg、png、webp 图片")
	}
	if len(upload.Data) == 0 {
		return UploadResult{}, fmt.Errorf("图片内容为空")
	}
	if len(upload.Data) > s.options.MaxBytes {
		return UploadResult{}, fmt.Errorf("图片超过 %dMB", s.options.MaxBytes/(1024*1024))
	}
	decoded, _, err := image.Decode(bytes.NewReader(upload.Data))
	if err != nil {
		return UploadResult{}, fmt.Errorf("校验图片内容: %w", err)
	}
	if err := validateImageBounds(decoded.Bounds()); err != nil {
		return UploadResult{}, err
	}

	// 2. 保存原图并生成资产区域 JPEG 裁剪图。
	imagePath, imageSHA, err := s.saveOriginal(request.SnapshotDate, extension, upload.Data)
	if err != nil {
		return UploadResult{}, err
	}
	cropData, cropPath, err := s.saveAssetCrop(request.SnapshotDate, imageSHA, decoded)
	if err != nil {
		return UploadResult{}, err
	}
	_ = cropPath

	// 3. 识别资产区域并解析账户资产。
	rawAsset, err := s.ocr.Recognize(ctx, cropData)
	if err != nil {
		return UploadResult{}, fmt.Errorf("识别账户资产: %w", err)
	}
	snapshot, err := ParseAssetSnapshot(rawAsset, AssetMetadata{
		SnapshotDate: request.SnapshotDate, BrokerName: request.BrokerName, SourceApp: request.SourceApp,
		ImagePath: imagePath, ImageSHA256: imageSHA, OCRProvider: s.options.OCRProvider,
	})
	if err != nil {
		return UploadResult{}, fmt.Errorf("解析账户资产: %w", err)
	}
	alias, err := s.repository.AccountAlias(ctx, snapshot.BrokerName, snapshot.AccountSuffix)
	if err != nil {
		return UploadResult{}, fmt.Errorf("读取账户别名: %w", err)
	}
	if alias == "" {
		return UploadResult{}, fmt.Errorf("未知账户后四位：%s", snapshot.AccountSuffix)
	}
	snapshot.AccountAlias = alias

	// 4. 整图识别持仓；失败仅记录提示并保留旧持仓明细。
	rawFull, fullErr := s.ocr.Recognize(ctx, upload.Data)
	if fullErr != nil {
		snapshot.Warnings = append(snapshot.Warnings, "持仓明细解析失败："+fullErr.Error())
	} else {
		snapshot.Holdings = ParseHoldings(rawFull)
		snapshot.HoldingsParsed = true
	}

	// 5. 原子写入资产和可选持仓明细。
	stored, err := s.repository.Upsert(ctx, snapshot, rawAsset, request.CreatedBy)
	if err != nil {
		return UploadResult{}, fmt.Errorf("保存仓位快照: %w", err)
	}
	return UploadResult{Filename: upload.Filename, Status: "saved", Snapshot: &stored}, nil
}

// validateImageBounds 校验阿里云 OCR 支持的图片像素和长宽比。
// 输入：bounds 是解码图片边界。
// 输出：符合限制返回 nil，否则返回业务错误。
// 副作用：无。
func validateImageBounds(bounds image.Rectangle) error {
	// 1. 校验边长和最大像素边界。
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 15 || height <= 15 || width >= 8192 || height >= 8192 {
		return fmt.Errorf("图片长宽必须大于 15 且小于 8192 像素")
	}

	// 2. 校验双向长宽比。
	ratio := float64(width) / float64(height)
	if ratio >= 50 || ratio <= 1.0/50 {
		return fmt.Errorf("图片长宽比必须小于 50")
	}
	return nil
}

// saveOriginal 按日期目录保存原始上传图片。
// 输入：snapshotDate 是日期，extension 是扩展名，data 是原始图片。
// 输出：返回文件路径和 SHA-256；失败时返回错误。
// 副作用：创建目录并写入上传文件。
func (s *Service) saveOriginal(snapshotDate, extension string, data []byte) (string, string, error) {
	// 1. 创建日期目录并生成随机文件名。
	directory := filepath.Join(s.options.UploadDir, snapshotDate)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return "", "", fmt.Errorf("创建仓位上传目录: %w", err)
	}
	identifier, err := randomIdentifier()
	if err != nil {
		return "", "", err
	}
	path := filepath.Join(directory, identifier+extension)

	// 2. 写入文件并返回内容摘要。
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return "", "", fmt.Errorf("保存仓位截图: %w", err)
	}
	hash := sha256.Sum256(data)
	return path, hex.EncodeToString(hash[:]), nil
}

// saveAssetCrop 裁剪图片顶部资产区域并保存 JPEG。
// 输入：snapshotDate 是日期，imageSHA 标识原图，source 是解码图片。
// 输出：返回 JPEG 二进制和临时路径；失败时返回错误。
// 副作用：创建目录并写入临时裁剪图。
func (s *Service) saveAssetCrop(snapshotDate, imageSHA string, source image.Image) ([]byte, string, error) {
	// 1. 计算顶部 42% 且至少 720 像素的裁剪高度。
	bounds := source.Bounds()
	cropHeight := int(float64(bounds.Dy()) * 0.42)
	if cropHeight < 720 {
		cropHeight = 720
	}
	if cropHeight > bounds.Dy() {
		cropHeight = bounds.Dy()
	}
	crop := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), cropHeight))
	draw.Draw(crop, crop.Bounds(), source, bounds.Min, draw.Src)

	// 2. 编码并保存高质量 JPEG。
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, crop, &jpeg.Options{Quality: 95}); err != nil {
		return nil, "", fmt.Errorf("编码资产裁剪图: %w", err)
	}
	directory := filepath.Join(s.options.TempDir, snapshotDate)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, "", fmt.Errorf("创建仓位临时目录: %w", err)
	}
	prefix := imageSHA
	if len(prefix) > 16 {
		prefix = prefix[:16]
	}
	path := filepath.Join(directory, prefix+"_asset.jpg")
	if err := os.WriteFile(path, buffer.Bytes(), 0o640); err != nil {
		return nil, "", fmt.Errorf("保存资产裁剪图: %w", err)
	}
	return buffer.Bytes(), path, nil
}

// randomIdentifier 生成上传图片使用的随机十六进制名称。
// 输入：无。
// 输出：返回 32 位十六进制文本；随机源失败时返回错误。
// 副作用：读取操作系统安全随机源。
func randomIdentifier() (string, error) {
	// 1. 读取 16 字节随机数并编码。
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("生成上传文件名: %w", err)
	}
	return hex.EncodeToString(value), nil
}
