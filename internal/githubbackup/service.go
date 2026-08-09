package githubbackup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const manifestVersion = 1

var repositorySegmentPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// Options 描述 GitHub 代码冷备份的目录、历史引用保留和时钟配置。
type Options struct {
	Directory     string
	RetentionRefs int
	Now           func() time.Time
}

// Service 备份账号自有仓库和固定组织仓库，并维护永不主动删除的清单。
type Service struct {
	lister               RepositoryLister
	requiredRepositories []Repository
	store                RepositoryStore
	options              Options
}

// NewService 创建账号自有仓库加固定组织仓库的代码备份服务。
// 输入：lister 发现账号自有仓库，requiredFullNames 是额外组织仓库，store 负责 Git，options 控制目录和保留批次。
// 输出：返回不会枚举其他组织仓库的服务。
// 副作用：无，不立即访问 GitHub 或创建目录。
func NewService(lister RepositoryLister, requiredFullNames []string, store RepositoryStore, options Options) *Service {
	// 1. 把固定组织仓库名转换为不含令牌的标准 HTTPS 克隆地址并排序去重。
	repositorySet := make(map[string]Repository)
	for _, fullName := range requiredFullNames {
		trimmed := strings.TrimSpace(fullName)
		if trimmed == "" {
			continue
		}
		repositorySet[trimmed] = Repository{
			FullName: trimmed,
			CloneURL: "https://github.com/" + trimmed + ".git",
		}
	}
	requiredRepositories := make([]Repository, 0, len(repositorySet))
	for _, repository := range repositorySet {
		requiredRepositories = append(requiredRepositories, repository)
	}
	sort.Slice(requiredRepositories, func(i, j int) bool {
		return requiredRepositories[i].FullName < requiredRepositories[j].FullName
	})
	if options.Now == nil {
		options.Now = time.Now
	}
	return &Service{lister: lister, requiredRepositories: requiredRepositories, store: store, options: options}
}

// Backup 创建或更新账号自有仓库和固定组织仓库并持久化状态清单。
// 输入：ctx 控制 Git 网络和本地操作。
// 输出：返回发现、新增、更新、失联和失败数量；任一仓库失败时返回汇总错误。
// 副作用：调用 GitHub，创建或修改裸仓库和 manifest.json；绝不删除仓库目录。
func (s *Service) Backup(ctx context.Context) (Result, error) {
	// 1. 校验服务配置和固定组织仓库名称，避免路径逃逸。
	if err := s.validate(); err != nil {
		return Result{}, err
	}
	for _, repository := range s.requiredRepositories {
		if err := validateRepository(repository); err != nil {
			return Result{}, err
		}
	}

	// 2. 先发现账号自有仓库并合并固定组织仓库；API 失败时不修改任何本地状态。
	discovered, err := s.lister.ListRepositories(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("发现 GitHub 账号自有仓库: %w", err)
	}
	repositorySet := make(map[string]Repository, len(discovered)+len(s.requiredRepositories))
	for _, repository := range discovered {
		if err := validateRepository(repository); err != nil {
			return Result{}, err
		}
		repositorySet[repository.FullName] = repository
	}
	for _, repository := range s.requiredRepositories {
		repositorySet[repository.FullName] = repository
	}
	repositories := make([]Repository, 0, len(repositorySet))
	for _, repository := range repositorySet {
		repositories = append(repositories, repository)
	}
	sort.Slice(repositories, func(i, j int) bool { return repositories[i].FullName < repositories[j].FullName })

	// 3. 创建备份根目录并读取上一版状态清单。
	root, err := filepath.Abs(s.options.Directory)
	if err != nil {
		return Result{}, fmt.Errorf("解析 GitHub 备份目录: %w", err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return Result{}, fmt.Errorf("创建 GitHub 备份目录: %w", err)
	}
	manifestPath := filepath.Join(root, "manifest.json")
	manifest, err := loadManifest(manifestPath)
	if err != nil {
		return Result{}, err
	}

	// 4. 逐个创建或更新裸仓库，单仓库失败不阻断其他仓库。
	now := s.options.Now()
	timestamp := now.UTC().Format(time.RFC3339)
	result := Result{DiscoveredCount: len(repositories)}
	failures := make([]string, 0)
	for _, repository := range repositories {
		target, relativePath, err := repositoryPath(root, repository.FullName)
		if err != nil {
			return result, err
		}
		record := manifest.Repositories[repository.FullName]
		record.FullName = repository.FullName
		record.CloneURL = repository.CloneURL
		record.RelativePath = relativePath
		record.LastSeenAt = timestamp

		operation := "update"
		if _, statErr := os.Stat(target); statErr != nil {
			if !os.IsNotExist(statErr) {
				err = fmt.Errorf("检查仓库目录: %w", statErr)
			} else {
				operation = "create"
				err = s.store.Create(ctx, repository, target)
			}
		} else {
			err = s.store.Update(ctx, repository, target, now, s.options.RetentionRefs)
		}

		if err != nil {
			result.FailedCount++
			record.Status = "failed"
			record.LastError = truncateError(err)
			failures = append(failures, repository.FullName)
		} else {
			record.Status = "active"
			record.LastSuccessAt = timestamp
			record.LastError = ""
			if operation == "create" {
				result.CreatedCount++
			} else {
				result.UpdatedCount++
			}
		}
		manifest.Repositories[repository.FullName] = record
	}

	// 5. 保留历史清单中不再发现或配置的仓库和目录，只标记失联状态。
	configured := make(map[string]struct{}, len(repositories))
	for _, repository := range repositories {
		configured[repository.FullName] = struct{}{}
	}
	for fullName, record := range manifest.Repositories {
		if _, ok := configured[fullName]; ok {
			continue
		}
		record.Status = "unavailable"
		manifest.Repositories[fullName] = record
		result.UnavailableCount++
	}

	// 6. 原子写入本次状态；仓库失败由调用方统一记录并发送微信通知。
	manifest.Version = manifestVersion
	manifest.UpdatedAt = timestamp
	if err := saveManifest(manifestPath, manifest); err != nil {
		return result, err
	}
	if len(failures) > 0 {
		return result, fmt.Errorf("%d 个 GitHub 仓库备份失败: %s", len(failures), strings.Join(failures, ", "))
	}
	return result, nil
}

// validate 校验服务依赖和备份参数。
// 输入：无。
// 输出：配置完整返回 nil。
// 副作用：无。
func (s *Service) validate() error {
	// 1. 要求发现器、至少一个固定组织仓库、存储器、目录和正保留批次数。
	if s == nil || s.lister == nil || s.store == nil || len(s.requiredRepositories) == 0 ||
		strings.TrimSpace(s.options.Directory) == "" || s.options.RetentionRefs < 1 {
		return fmt.Errorf("GitHub 代码备份配置不完整")
	}
	return nil
}

// validateRepository 校验仓库名和克隆地址均可安全使用。
// 输入：repository 是一个白名单仓库。
// 输出：名称或 HTTPS 地址无效时返回错误。
// 副作用：无。
func validateRepository(repository Repository) error {
	// 1. 完整名称必须严格由安全的 owner/repo 两段组成。
	parts := strings.Split(repository.FullName, "/")
	if len(parts) != 2 || !validRepositorySegment(parts[0]) || !validRepositorySegment(parts[1]) {
		return fmt.Errorf("GitHub 仓库名称无效: %q", repository.FullName)
	}

	// 2. 克隆地址只接受不含认证信息的 HTTPS URL。
	parsed, err := url.Parse(repository.CloneURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("GitHub 仓库克隆地址无效: %q", repository.CloneURL)
	}
	return nil
}

// validRepositorySegment 判断仓库名片段能否安全映射到文件路径。
// 输入：value 是 owner 或 repo 片段。
// 输出：安全返回 true。
// 副作用：无。
func validRepositorySegment(value string) bool {
	// 1. 拒绝路径语义名称并限制为 GitHub 常见安全字符。
	return value != "" && value != "." && value != ".." && repositorySegmentPattern.MatchString(value)
}

// repositoryPath 生成仓库绝对目录并验证没有逃逸备份根目录。
// 输入：root 是绝对根目录，fullName 是 owner/repo。
// 输出：返回绝对目录和斜线分隔的相对目录。
// 副作用：无。
func repositoryPath(root, fullName string) (string, string, error) {
	// 1. 按 owner/repo.git 生成可读目录结构。
	parts := strings.Split(fullName, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("GitHub 仓库名称无效: %q", fullName)
	}
	target := filepath.Join(root, parts[0], parts[1]+".git")
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return "", "", fmt.Errorf("计算仓库相对路径: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("GitHub 仓库路径越界: %q", fullName)
	}
	return target, filepath.ToSlash(relative), nil
}

// loadManifest 读取上一版备份状态清单。
// 输入：path 是 manifest.json 路径。
// 输出：文件不存在时返回空清单，格式无效时返回错误。
// 副作用：读取本地文件。
func loadManifest(path string) (Manifest, error) {
	// 1. 不存在时建立版本化空清单。
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Manifest{Version: manifestVersion, Repositories: make(map[string]ManifestRepository)}, nil
	}
	if err != nil {
		return Manifest{}, fmt.Errorf("读取 GitHub 备份清单: %w", err)
	}

	// 2. 严格解析已有清单，避免静默覆盖损坏状态。
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("解析 GitHub 备份清单: %w", err)
	}
	if manifest.Version != manifestVersion {
		return Manifest{}, fmt.Errorf("不支持的 GitHub 备份清单版本: %d", manifest.Version)
	}
	if manifest.Repositories == nil {
		manifest.Repositories = make(map[string]ManifestRepository)
	}
	return manifest, nil
}

// saveManifest 原子写入备份状态清单。
// 输入：path 是目标文件，manifest 是完整清单。
// 输出：持久化成功返回 nil。
// 副作用：创建权限受限的临时文件并替换 manifest.json。
func saveManifest(path string, manifest Manifest) error {
	// 1. 编码带缩进 JSON 并写入同目录临时文件。
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("编码 GitHub 备份清单: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".manifest-*.json")
	if err != nil {
		return fmt.Errorf("创建 GitHub 备份清单临时文件: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("限制 GitHub 备份清单权限: %w", err)
	}
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("写入 GitHub 备份清单: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("同步 GitHub 备份清单: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭 GitHub 备份清单: %w", err)
	}

	// 2. Linux 原子替换；Windows 测试环境在目标存在时先移除旧文件。
	if err := os.Rename(temporaryPath, path); err != nil {
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("替换 GitHub 备份清单: %w", err)
		}
		if retryErr := os.Rename(temporaryPath, path); retryErr != nil {
			return fmt.Errorf("替换 GitHub 备份清单: %w", retryErr)
		}
	}
	return nil
}

// truncateError 截断写入清单的单仓库错误，避免状态文件无限增长。
// 输入：err 是 Git 或文件错误。
// 输出：返回最多 300 个字符的单行文本。
// 副作用：无。
func truncateError(err error) string {
	// 1. 清理换行并按字符截断错误摘要。
	text := strings.Join(strings.Fields(err.Error()), " ")
	runes := []rune(text)
	if len(runes) > 300 {
		return string(runes[:300])
	}
	return text
}
