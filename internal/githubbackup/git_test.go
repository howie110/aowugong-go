package githubbackup

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeCommandRunner struct {
	deleted []string
}

// Run 模拟列出三批内部引用并记录删除动作。
// 输入：上下文、目录、环境和 Git 参数。
// 输出：列引用命令返回固定文本，其他命令成功。
// 副作用：记录 update-ref -d 的引用名。
func (r *fakeCommandRunner) Run(_ context.Context, _ string, _ []string, args ...string) (string, error) {
	// 1. 为内部引用查询返回新旧三批测试数据。
	if len(args) >= 3 && args[0] == "for-each-ref" && args[2] == archiveRefPrefix {
		return strings.Join([]string{
			archiveRefPrefix + "20260809T040000Z/heads/main",
			archiveRefPrefix + "20260802T040000Z/heads/main",
			archiveRefPrefix + "20260726T040000Z/heads/main",
		}, "\n"), nil
	}

	// 2. 记录旧批次引用删除，其他命令直接成功。
	if len(args) == 3 && args[0] == "update-ref" && args[1] == "-d" {
		r.deleted = append(r.deleted, args[2])
	}
	return "", nil
}

// TestPruneArchivesKeepsNewestBatches 验证历史引用只删除超出保留数量的旧批次。
// 输入：三批内部引用和保留两批配置。
// 输出：仅删除最旧批次引用。
// 副作用：修改命令执行替身的删除记录。
func TestPruneArchivesKeepsNewestBatches(t *testing.T) {
	// 1. 使用命令替身清理三批引用。
	runner := &fakeCommandRunner{}
	store := newGitStore("test-token", runner)
	if err := store.pruneArchives(context.Background(), "test.git", nil, 2); err != nil {
		t.Fatalf("pruneArchives() error = %v", err)
	}

	// 2. 断言仅最旧批次被删除。
	want := archiveRefPrefix + "20260726T040000Z/heads/main"
	if len(runner.deleted) != 1 || runner.deleted[0] != want {
		t.Errorf("deleted = %v, want [%s]", runner.deleted, want)
	}
}

// TestSafeGitActionHidesArguments 验证命令错误上下文不会输出仓库 URL 或令牌。
// 输入：带私有仓库 URL 的 clone 参数。
// 输出：仅返回 clone 子命令。
// 副作用：无。
func TestSafeGitActionHidesArguments(t *testing.T) {
	// 1. 提取动作并核对参数没有进入日志文本。
	if got := safeGitAction([]string{"clone", "https://github.com/KES-IT/KES-SCM.git"}); got != "clone" {
		t.Errorf("safeGitAction() = %q, want clone", got)
	}
}

// TestGitStoreCreatesUpdatesAndArchivesBareRepository 验证真实 Git 裸克隆和更新前引用归档链路。
// 输入：本机临时源仓库的两次提交。
// 输出：裸仓库更新到第二次提交，历史引用仍指向第一次提交。
// 副作用：调用本机 Git 并写入测试临时目录，不访问网络。
func TestGitStoreCreatesUpdatesAndArchivesBareRepository(t *testing.T) {
	// 1. 缺少 Git 时跳过集成测试，否则创建包含第一次提交的临时源仓库。
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	runTestGit(t, root, "init", "--initial-branch=main", source)
	runTestGit(t, source, "config", "user.email", "backup-test@example.com")
	runTestGit(t, source, "config", "user.name", "Backup Test")
	file := filepath.Join(source, "README.md")
	if err := os.WriteFile(file, []byte("first\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	runTestGit(t, source, "add", "README.md")
	runTestGit(t, source, "commit", "-m", "first")
	firstCommit := strings.TrimSpace(runTestGit(t, source, "rev-parse", "HEAD"))

	// 2. 首次创建裸仓库，再给源仓库追加第二次提交。
	target := filepath.Join(root, "backup", "KES-IT", "KES-SCM.git")
	store := NewGitStore("test-token")
	repository := Repository{FullName: "KES-IT/KES-SCM", CloneURL: source}
	if err := store.Create(context.Background(), repository, target); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := os.WriteFile(file, []byte("second\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	runTestGit(t, source, "add", "README.md")
	runTestGit(t, source, "commit", "-m", "second")
	secondCommit := strings.TrimSpace(runTestGit(t, source, "rev-parse", "HEAD"))

	// 3. 更新裸仓库并核对主分支与更新前归档分别指向新旧提交。
	backupTime := time.Date(2026, 8, 9, 4, 0, 0, 0, time.UTC)
	if err := store.Update(context.Background(), repository, target, backupTime, 4); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	current := strings.TrimSpace(runTestGit(t, target, "rev-parse", "refs/heads/main"))
	archived := strings.TrimSpace(runTestGit(t, target, "rev-parse",
		archiveRefPrefix+"20260809T040000Z/heads/main"))
	if current != secondCommit || archived != firstCommit {
		t.Errorf("current/archive = %s/%s, want %s/%s", current, archived, secondCommit, firstCommit)
	}
}

// runTestGit 在指定目录执行测试 Git 命令并返回输出。
// 输入：t 管理测试失败，directory 是工作目录，args 是 Git 参数。
// 输出：成功返回标准输出，失败直接终止当前测试。
// 副作用：按参数读写临时 Git 仓库。
func runTestGit(t *testing.T, directory string, args ...string) string {
	t.Helper()
	// 1. 执行 Git 并在失败时输出完整测试命令结果。
	command := exec.Command("git", args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s error = %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output)
}
