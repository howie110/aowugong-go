// Package vaultwardenbackup 提供 Vaultwarden 备份加密和异地邮件发送能力。
package vaultwardenbackup

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/howiedata/aowugong-go/internal/client"
)

const backupPattern = "vaultwarden-*.tar.gz"

const recoveryInstructions = `这是旧服务器已经完全损坏或丢失时使用的 Vaultwarden 灾难恢复包，不需要也不应连接旧服务器。请把本 ZIP、vaultwarden-age-key.txt、一台全新 Linux 服务器的 SSH 管理权限以及已备案域名的 DNS 管理权限一起交给可信 AI：AI 先在本地用 age 私钥解密 ZIP 内的 .tar.gz.age 文件并检查其中存在 vaultwarden.dump、vaultwarden-files.tar.gz 和 manifest.txt，私钥不得上传到任何服务器；然后在新服务器安装 Docker Engine 和 PostgreSQL 15，确保 aowugong.top、vault.aowugong.top、miniflux.aowugong.top、nextflux.aowugong.top 的 A 记录指向新服务器，上传解密后的备份和 ZIP 内 scripts 目录，执行 VAULTWARDEN_HOST=vault.aowugong.top bash scripts/install-vaultwarden.sh 建立全新的数据库账号、运行配置和 HTTPS 证书，再执行 BACKUP_ARCHIVE=解密后的备份路径 CONFIRM_DISASTER_RESTORE=YES bash scripts/restore-vaultwarden-disaster.sh 恢复数据库、附件、Send 和签名密钥，最后访问 https://vault.aowugong.top/api/config，检查网页登录、两步验证和至少一条密码记录。若目标服务器并非空白环境，AI 必须先停止并询问用户，不得覆盖已有 Vaultwarden；恢复完成后删除服务器上的明文备份，私钥只保留在用户本地。`

var recoveryScriptNames = []string{
	"install-vaultwarden.sh",
	"backup-vaultwarden.sh",
	"restore-vaultwarden-disaster.sh",
}

// Mailer 定义备份服务所需的邮件发送入口。
type Mailer interface {
	Send(ctx context.Context, message client.EmailMessage) error
}

// Options 描述备份来源、加密公钥、收件人和附件上限。
type Options struct {
	Directory                string
	RecoveryScriptsDirectory string
	AgeRecipient             string
	EmailTo                  string
	MaxAttachmentBytes       int64
	Now                      func() time.Time
}

// Result 描述本次异地备份邮件的文件和大小。
type Result struct {
	SourcePath string
	EmailTo    string
	Size       int64
}

// Service 加密最新 Vaultwarden 备份并发送到异地邮箱。
type Service struct {
	mailer  Mailer
	options Options
}

// NewService 创建 Vaultwarden 异地备份服务。
// 输入：mailer 发送附件邮件，options 提供目录、公钥和收件人。
// 输出：返回服务实例。
// 副作用：无，不读取文件或发送邮件。
func NewService(mailer Mailer, options Options) *Service {
	// 1. 补齐时钟，便于生产标题和测试断言保持一致。
	if options.Now == nil {
		options.Now = time.Now
	}
	return &Service{mailer: mailer, options: options}
}

// SendLatest 加密最新本地备份并作为邮件附件发送。
// 输入：ctx 控制文件处理和 SMTP 发送超时。
// 输出：返回源文件、收件人和大小；失败时返回带业务上下文的错误。
// 副作用：读取 Vaultwarden 备份、创建临时加密文件、发送邮件并删除临时文件。
func (s *Service) SendLatest(ctx context.Context) (Result, error) {
	// 1. 校验依赖并找到最新的完整 Vaultwarden 备份。
	if s == nil || s.mailer == nil {
		return Result{}, fmt.Errorf("Vaultwarden 邮件备份服务未配置")
	}
	sourcePath, info, err := latestBackup(s.options.Directory)
	if err != nil {
		return Result{}, err
	}
	if s.options.MaxAttachmentBytes > 0 && info.Size() > s.options.MaxAttachmentBytes {
		return Result{}, fmt.Errorf("Vaultwarden 备份大小 %d 超过邮件附件上限 %d", info.Size(), s.options.MaxAttachmentBytes)
	}

	// 2. 使用只包含公钥的 age 收件人配置创建临时加密文件。
	recipient, err := age.ParseX25519Recipient(strings.TrimSpace(s.options.AgeRecipient))
	if err != nil {
		return Result{}, fmt.Errorf("解析 Vaultwarden 备份 age 公钥: %w", err)
	}
	encryptedPath, err := encryptBackup(ctx, sourcePath, recipient)
	if err != nil {
		return Result{}, err
	}
	defer os.Remove(encryptedPath)

	// 3. 把加密备份和恢复说明打入同一个 ZIP，并核对最终邮件附件大小。
	now := s.options.Now()
	attachmentName := filepath.Base(sourcePath) + ".age"
	packagePath, packageName, err := packageRecovery(
		ctx, encryptedPath, attachmentName, s.options.RecoveryScriptsDirectory,
	)
	if err != nil {
		return Result{}, err
	}
	defer os.Remove(packagePath)
	packageInfo, err := os.Stat(packagePath)
	if err != nil {
		return Result{}, fmt.Errorf("读取 Vaultwarden 恢复包大小: %w", err)
	}
	if s.options.MaxAttachmentBytes > 0 && packageInfo.Size() > s.options.MaxAttachmentBytes {
		return Result{}, fmt.Errorf("Vaultwarden 恢复包大小 %d 超过邮件附件上限 %d", packageInfo.Size(), s.options.MaxAttachmentBytes)
	}

	// 4. 发送单个恢复包附件，正文只包含非敏感摘要。
	message := client.EmailMessage{
		To:      []string{s.options.EmailTo},
		Subject: "Aowugong Vaultwarden 加密备份 " + now.Format("2006-01-02"),
		Body: strings.Join([]string{
			"Vaultwarden 每周异地备份已生成。",
			"源文件：" + filepath.Base(sourcePath),
			fmt.Sprintf("原始大小：%d 字节", info.Size()),
			"ZIP 内包含 age 加密备份、使用说明.md 和全新服务器重建脚本，解密私钥需另行提供。",
		}, "\n"),
		Attachments: []client.EmailAttachment{{Name: packageName, Path: packagePath}},
	}
	if err := s.mailer.Send(ctx, message); err != nil {
		return Result{}, fmt.Errorf("发送 Vaultwarden 加密备份邮件: %w", err)
	}
	return Result{SourcePath: sourcePath, EmailTo: s.options.EmailTo, Size: info.Size()}, nil
}

// packageRecovery 把加密备份、AI 恢复说明和全新服务器脚本打入单个 ZIP。
// 输入：ctx 控制取消，encryptedPath 是 age 文件，attachmentName 是包内备份名，scriptsDirectory 是恢复脚本目录。
// 输出：返回临时 ZIP 路径和邮件显示名称；打包失败时返回错误。
// 副作用：读取加密备份和恢复脚本，并在系统临时目录创建恢复包。
func packageRecovery(ctx context.Context, encryptedPath, attachmentName, scriptsDirectory string) (string, string, error) {
	// 1. 创建仅当前用户可读写的临时 ZIP，并准备失败清理。
	packageFile, err := os.CreateTemp("", "vaultwarden-recovery-*.zip")
	if err != nil {
		return "", "", fmt.Errorf("创建 Vaultwarden 恢复包: %w", err)
	}
	packagePath := packageFile.Name()
	cleanup := true
	defer func() {
		_ = packageFile.Close()
		if cleanup {
			_ = os.Remove(packagePath)
		}
	}()
	if err := packageFile.Chmod(0o600); err != nil {
		return "", "", fmt.Errorf("限制 Vaultwarden 恢复包权限: %w", err)
	}
	zipWriter := zip.NewWriter(packageFile)

	// 2. 使用 Store 模式写入已加密压缩的备份，避免重复压缩浪费资源。
	backupFile, err := os.Open(encryptedPath)
	if err != nil {
		_ = zipWriter.Close()
		return "", "", fmt.Errorf("打开 Vaultwarden 加密备份: %w", err)
	}
	backupHeader := &zip.FileHeader{Name: filepath.Base(attachmentName), Method: zip.Store}
	backupHeader.SetMode(0o600)
	backupPart, err := zipWriter.CreateHeader(backupHeader)
	if err != nil {
		_ = backupFile.Close()
		_ = zipWriter.Close()
		return "", "", fmt.Errorf("创建 Vaultwarden 恢复包备份项: %w", err)
	}
	if _, err := io.Copy(backupPart, &contextReader{ctx: ctx, reader: backupFile}); err != nil {
		_ = backupFile.Close()
		_ = zipWriter.Close()
		return "", "", fmt.Errorf("写入 Vaultwarden 恢复包备份项: %w", err)
	}
	if err := backupFile.Close(); err != nil {
		_ = zipWriter.Close()
		return "", "", fmt.Errorf("关闭 Vaultwarden 加密备份: %w", err)
	}

	// 3. 写入固定 UTF-8 使用说明，供可信 AI 在恢复前读取。
	instructionHeader := &zip.FileHeader{Name: "使用说明.md", Method: zip.Deflate}
	instructionHeader.SetMode(0o600)
	instructionPart, err := zipWriter.CreateHeader(instructionHeader)
	if err != nil {
		_ = zipWriter.Close()
		return "", "", fmt.Errorf("创建 Vaultwarden 使用说明: %w", err)
	}
	if _, err := io.WriteString(instructionPart, recoveryInstructions+"\n"); err != nil {
		_ = zipWriter.Close()
		return "", "", fmt.Errorf("写入 Vaultwarden 使用说明: %w", err)
	}

	// 4. 写入创建运行环境和恢复数据所需的固定脚本，确保不依赖旧服务器或代码仓库。
	for _, scriptName := range recoveryScriptNames {
		scriptPath := filepath.Join(scriptsDirectory, scriptName)
		if err := writeRecoveryScript(ctx, zipWriter, scriptPath, filepath.ToSlash(filepath.Join("scripts", scriptName))); err != nil {
			_ = zipWriter.Close()
			return "", "", err
		}
	}

	// 5. 关闭、同步并发布完整 ZIP 临时文件。
	if err := zipWriter.Close(); err != nil {
		return "", "", fmt.Errorf("关闭 Vaultwarden 恢复包: %w", err)
	}
	if err := packageFile.Sync(); err != nil {
		return "", "", fmt.Errorf("同步 Vaultwarden 恢复包: %w", err)
	}
	if err := packageFile.Close(); err != nil {
		return "", "", fmt.Errorf("关闭 Vaultwarden 恢复包文件: %w", err)
	}
	cleanup = false
	packageName := strings.TrimSuffix(filepath.Base(attachmentName), ".tar.gz.age") + "-recovery.zip"
	return packagePath, packageName, nil
}

// writeRecoveryScript 把单个可执行脚本写入恢复 ZIP。
// 输入：ctx 控制取消，zipWriter 是目标归档，sourcePath 是本地脚本，entryName 是包内路径。
// 输出：写入成功返回 nil；读取或写入失败时返回带脚本名的错误。
// 副作用：读取本地脚本并向 ZIP 追加文件。
func writeRecoveryScript(ctx context.Context, zipWriter *zip.Writer, sourcePath, entryName string) error {
	// 1. 打开固定脚本并创建带可执行权限的 ZIP 条目。
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("打开 Vaultwarden 恢复脚本 %s: %w", filepath.Base(sourcePath), err)
	}
	defer source.Close()
	header := &zip.FileHeader{Name: entryName, Method: zip.Deflate}
	header.SetMode(0o700)
	part, err := zipWriter.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("创建 Vaultwarden 恢复脚本条目 %s: %w", filepath.Base(sourcePath), err)
	}

	// 2. 响应任务取消并完整复制脚本内容。
	if _, err := io.Copy(part, &contextReader{ctx: ctx, reader: source}); err != nil {
		return fmt.Errorf("写入 Vaultwarden 恢复脚本 %s: %w", filepath.Base(sourcePath), err)
	}
	return nil
}

// latestBackup 查找目录中修改时间最新的 Vaultwarden 正式备份。
// 输入：directory 是每日备份目录。
// 输出：返回最新文件路径和元数据；目录为空或读取失败时返回错误。
// 副作用：读取目录元数据。
func latestBackup(directory string) (string, os.FileInfo, error) {
	// 1. 匹配正式归档并按修改时间、文件名稳定排序。
	paths, err := filepath.Glob(filepath.Join(directory, backupPattern))
	if err != nil {
		return "", nil, fmt.Errorf("匹配 Vaultwarden 备份: %w", err)
	}
	type candidate struct {
		path string
		info os.FileInfo
	}
	candidates := make([]candidate, 0, len(paths))
	for _, path := range paths {
		info, statErr := os.Stat(path)
		if statErr != nil {
			return "", nil, fmt.Errorf("读取 Vaultwarden 备份 %s: %w", path, statErr)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		candidates = append(candidates, candidate{path: path, info: info})
	}
	if len(candidates) == 0 {
		return "", nil, fmt.Errorf("Vaultwarden 备份目录没有可发送文件: %s", directory)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].info.ModTime().Equal(candidates[j].info.ModTime()) {
			return candidates[i].path > candidates[j].path
		}
		return candidates[i].info.ModTime().After(candidates[j].info.ModTime())
	})
	return candidates[0].path, candidates[0].info, nil
}

// encryptBackup 使用 age 公钥流式加密一份备份到临时文件。
// 输入：ctx 控制取消，sourcePath 是明文归档，recipient 是 age 收件人公钥。
// 输出：返回临时加密文件路径；读取、加密或同步失败时返回错误。
// 副作用：读取源文件并在系统临时目录创建加密文件。
func encryptBackup(ctx context.Context, sourcePath string, recipient age.Recipient) (string, error) {
	// 1. 打开源文件并创建仅当前用户可读写的临时输出。
	source, err := os.Open(sourcePath)
	if err != nil {
		return "", fmt.Errorf("打开 Vaultwarden 备份: %w", err)
	}
	defer source.Close()
	target, err := os.CreateTemp("", "vaultwarden-*.tar.gz.age")
	if err != nil {
		return "", fmt.Errorf("创建 Vaultwarden 临时加密文件: %w", err)
	}
	targetPath := target.Name()
	cleanup := true
	defer func() {
		_ = target.Close()
		if cleanup {
			_ = os.Remove(targetPath)
		}
	}()
	if err := target.Chmod(0o600); err != nil {
		return "", fmt.Errorf("限制 Vaultwarden 临时加密文件权限: %w", err)
	}

	// 2. 建立 age 加密流并在复制过程中响应上下文取消。
	encrypted, err := age.Encrypt(target, recipient)
	if err != nil {
		return "", fmt.Errorf("建立 Vaultwarden age 加密流: %w", err)
	}
	if _, err := io.Copy(encrypted, &contextReader{ctx: ctx, reader: source}); err != nil {
		_ = encrypted.Close()
		return "", fmt.Errorf("加密 Vaultwarden 备份: %w", err)
	}
	if err := encrypted.Close(); err != nil {
		return "", fmt.Errorf("完成 Vaultwarden age 加密: %w", err)
	}
	if err := target.Sync(); err != nil {
		return "", fmt.Errorf("同步 Vaultwarden 加密文件: %w", err)
	}
	if err := target.Close(); err != nil {
		return "", fmt.Errorf("关闭 Vaultwarden 加密文件: %w", err)
	}
	cleanup = false
	return targetPath, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

// Read 在每次读取备份前检查任务是否已取消。
// 输入：data 是目标缓冲区。
// 输出：返回读取字节数；上下文取消或底层读取失败时返回错误。
// 副作用：读取底层备份流。
func (r *contextReader) Read(data []byte) (int, error) {
	// 1. 优先返回取消原因，否则继续读取备份内容。
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
}
