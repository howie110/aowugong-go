package githubbackup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const archiveRefPrefix = "refs/aowugong-backup/"

// RepositoryStore 定义裸仓库创建和增量更新能力。
type RepositoryStore interface {
	Create(ctx context.Context, repository Repository, target string) error
	Update(ctx context.Context, repository Repository, target string, now time.Time, retention int) error
}

// commandRunner 定义测试可替换的 Git 命令执行能力。
type commandRunner interface {
	Run(ctx context.Context, directory string, environment []string, args ...string) (string, error)
}

// execRunner 使用服务器安装的 Git 执行裸仓库操作。
type execRunner struct{}

// Run 执行一次 Git 命令并返回合并输出。
// 输入：ctx 控制超时，directory 是工作目录，environment 是附加环境变量，args 是 Git 参数。
// 输出：成功返回命令输出；失败返回不包含认证令牌的错误。
// 副作用：按 Git 参数读取或修改本地仓库，并可能访问 GitHub。
func (execRunner) Run(ctx context.Context, directory string, environment []string, args ...string) (string, error) {
	// 1. 组装受上下文控制的 Git 进程和认证环境。
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = directory
	command.Env = append(os.Environ(), environment...)

	// 2. 返回有限业务上下文，不把环境中的令牌写入错误。
	output, err := command.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("执行 git %s: %w: %s", safeGitAction(args), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

// GitStore 使用裸 Git 仓库保存代码和每次更新前的历史引用。
type GitStore struct {
	token  string
	runner commandRunner
}

// NewGitStore 创建 GitHub 裸仓库存储器。
// 输入：token 是只读 GitHub Token。
// 输出：返回使用系统 Git 的存储器。
// 副作用：无，不立即创建文件或访问 GitHub。
func NewGitStore(token string) *GitStore {
	// 1. 保存令牌并注入正式命令执行器。
	return &GitStore{token: strings.TrimSpace(token), runner: execRunner{}}
}

// newGitStore 创建可替换命令执行器的存储器。
// 输入：token 是测试令牌，runner 执行 Git 命令。
// 输出：返回可测试存储器。
// 副作用：无。
func newGitStore(token string, runner commandRunner) *GitStore {
	// 1. 保存显式依赖供隔离测试使用。
	return &GitStore{token: strings.TrimSpace(token), runner: runner}
}

// Create 首次克隆一个 GitHub 裸仓库。
// 输入：ctx 控制网络操作，repository 是白名单仓库，target 是最终目录。
// 输出：克隆并原子放置成功返回 nil。
// 副作用：调用 GitHub，创建目标目录和裸仓库；失败时清理临时目录。
func (s *GitStore) Create(ctx context.Context, repository Repository, target string) error {
	// 1. 准备父目录、临时目标和不落盘令牌的 AskPass 环境。
	if err := s.validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("创建仓库父目录: %w", err)
	}
	temporary := fmt.Sprintf("%s.tmp-%d", target, time.Now().UnixNano())
	defer os.RemoveAll(temporary)
	environment, cleanup, err := s.askPassEnvironment()
	if err != nil {
		return err
	}
	defer cleanup()

	// 2. 克隆到同一文件系统的临时目录，避免中断留下半成品。
	if _, err := s.runner.Run(ctx, filepath.Dir(target), environment,
		"clone", "--bare", repository.CloneURL, temporary); err != nil {
		return fmt.Errorf("克隆仓库 %s: %w", repository.FullName, err)
	}

	// 3. 仅在完整克隆成功后切换为正式目录。
	if err := os.Rename(temporary, target); err != nil {
		return fmt.Errorf("安装仓库 %s: %w", repository.FullName, err)
	}
	return nil
}

// Update 更新裸仓库并保留更新前的分支和标签引用。
// 输入：ctx 控制网络操作，repository 是白名单仓库，target 是裸仓库目录，now 是归档时间，retention 是保留批次数。
// 输出：快照、抓取和清理全部成功返回 nil。
// 副作用：调用 GitHub，写入裸仓库引用并清理超出批次数的内部归档引用；不删除仓库。
func (s *GitStore) Update(ctx context.Context, repository Repository, target string, now time.Time, retention int) error {
	// 1. 校验配置并创建临时 AskPass 环境。
	if err := s.validate(); err != nil {
		return err
	}
	if retention < 1 {
		return fmt.Errorf("历史引用保留批次数必须大于零")
	}
	environment, cleanup, err := s.askPassEnvironment()
	if err != nil {
		return err
	}
	defer cleanup()

	// 2. 更新前保存当前分支和标签，使强推或远端删除不能抹掉最后代码。
	stamp := now.UTC().Format("20060102T150405Z")
	if err := s.archiveCurrentRefs(ctx, target, environment, stamp); err != nil {
		return fmt.Errorf("归档仓库 %s 当前引用: %w", repository.FullName, err)
	}

	// 3. 重设无令牌远端地址并强制同步当前分支和标签，不执行 prune。
	if _, err := s.runner.Run(ctx, target, environment, "remote", "set-url", "origin", repository.CloneURL); err != nil {
		return fmt.Errorf("更新仓库 %s 远端: %w", repository.FullName, err)
	}
	if _, err := s.runner.Run(ctx, target, environment,
		"fetch", "--force", "--tags", "origin", "+refs/heads/*:refs/heads/*"); err != nil {
		return fmt.Errorf("抓取仓库 %s: %w", repository.FullName, err)
	}

	// 4. 只清理应用自己的超额历史引用，不触碰分支、标签或仓库文件。
	if err := s.pruneArchives(ctx, target, environment, retention); err != nil {
		return fmt.Errorf("清理仓库 %s 历史引用: %w", repository.FullName, err)
	}
	return nil
}

// archiveCurrentRefs 把当前分支和标签复制到一次备份批次下。
// 输入：ctx 控制命令，target 是裸仓库，environment 是认证环境，stamp 是批次标识。
// 输出：全部引用复制成功返回 nil。
// 副作用：在裸仓库写入 refs/aowugong-backup 下的内部引用。
func (s *GitStore) archiveCurrentRefs(ctx context.Context, target string, environment []string, stamp string) error {
	// 1. 读取当前分支和标签的完整引用及对象编号。
	output, err := s.runner.Run(ctx, target, environment,
		"for-each-ref", "--format=%(refname) %(objectname)", "refs/heads", "refs/tags")
	if err != nil {
		return err
	}

	// 2. 逐条复制到本批次命名空间，保留原引用层级。
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return fmt.Errorf("解析 Git 引用失败: %q", line)
		}
		name := strings.TrimPrefix(fields[0], "refs/")
		archiveName := archiveRefPrefix + stamp + "/" + name
		if _, err := s.runner.Run(ctx, target, environment, "update-ref", archiveName, fields[1]); err != nil {
			return err
		}
	}
	return nil
}

// pruneArchives 删除超过保留批次数的内部历史引用。
// 输入：ctx 控制命令，target 是裸仓库，environment 是认证环境，retention 是保留批次数。
// 输出：清理成功返回 nil。
// 副作用：仅删除 refs/aowugong-backup 下的旧引用。
func (s *GitStore) pruneArchives(ctx context.Context, target string, environment []string, retention int) error {
	// 1. 收集内部引用对应的唯一批次并按新到旧排序。
	output, err := s.runner.Run(ctx, target, environment,
		"for-each-ref", "--format=%(refname)", archiveRefPrefix)
	if err != nil {
		return err
	}
	batchSet := make(map[string]struct{})
	refsByBatch := make(map[string][]string)
	for _, reference := range strings.Fields(output) {
		remainder := strings.TrimPrefix(reference, archiveRefPrefix)
		parts := strings.SplitN(remainder, "/", 2)
		if len(parts) != 2 || parts[0] == "" {
			continue
		}
		batchSet[parts[0]] = struct{}{}
		refsByBatch[parts[0]] = append(refsByBatch[parts[0]], reference)
	}
	batches := make([]string, 0, len(batchSet))
	for batch := range batchSet {
		batches = append(batches, batch)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(batches)))

	// 2. 删除保留范围外批次的每个引用。
	if len(batches) <= retention {
		return nil
	}
	for _, batch := range batches[retention:] {
		for _, reference := range refsByBatch[batch] {
			if _, err := s.runner.Run(ctx, target, environment, "update-ref", "-d", reference); err != nil {
				return err
			}
		}
	}
	return nil
}

// askPassEnvironment 创建 Git 认证脚本和一次性令牌环境。
// 输入：无，使用存储器内令牌。
// 输出：返回 Git 附加环境、清理函数和可能的文件错误。
// 副作用：在系统临时目录创建权限受限脚本，调用清理函数后删除。
func (s *GitStore) askPassEnvironment() ([]string, func(), error) {
	// 1. 创建不包含令牌本身的临时 AskPass 脚本。
	directory, err := os.MkdirTemp("", "aowugong-git-askpass-")
	if err != nil {
		return nil, nil, fmt.Errorf("创建 Git 认证临时目录: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	name, content := "askpass.sh", "#!/bin/sh\ncase \"$1\" in\n  *Username*) printf '%s\\n' x-access-token ;;\n  *) printf '%s\\n' \"$GITHUB_BACKUP_TOKEN\" ;;\nesac\n"
	if runtime.GOOS == "windows" {
		name = "askpass.cmd"
		content = "@echo off\r\necho %~1 | findstr /I Username >nul\r\nif not errorlevel 1 (echo x-access-token) else (echo %GITHUB_BACKUP_TOKEN%)\r\n"
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("写入 Git 认证脚本: %w", err)
	}

	// 2. 令牌仅放进子进程环境，脚本和远端 URL 均不持久化令牌。
	environment := []string{
		"GIT_ASKPASS=" + path,
		"GIT_TERMINAL_PROMPT=0",
		"GITHUB_BACKUP_TOKEN=" + s.token,
	}
	return environment, cleanup, nil
}

// validate 校验 Git 存储器执行依赖。
// 输入：无。
// 输出：令牌和命令执行器有效时返回 nil。
// 副作用：无。
func (s *GitStore) validate() error {
	// 1. 阻止缺失认证或执行器时启动备份。
	if s == nil || s.runner == nil || s.token == "" {
		return fmt.Errorf("GitHub 代码备份存储配置不完整")
	}
	return nil
}

// safeGitAction 返回不包含仓库地址和令牌的 Git 动作名。
// 输入：args 是 Git 参数。
// 输出：返回首个动作或 unknown。
// 副作用：无。
func safeGitAction(args []string) string {
	// 1. 日志仅保留 Git 子命令，避免意外输出地址或认证信息。
	if len(args) == 0 {
		return "unknown"
	}
	return args[0]
}
