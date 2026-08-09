package githubbackup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeRepositoryStore struct {
	created  []string
	updated  []string
	failures map[string]error
}

type fakeRepositoryLister struct {
	repositories []Repository
	err          error
}

// ListRepositories 返回固定账号自有仓库列表。
// 输入：上下文。
// 输出：返回测试配置的仓库或错误。
// 副作用：无。
func (l fakeRepositoryLister) ListRepositories(context.Context) ([]Repository, error) {
	// 1. 返回构造时指定的发现结果。
	return l.repositories, l.err
}

// Create 模拟首次创建裸仓库目录。
// 输入：上下文、仓库和目标目录。
// 输出：配置失败时返回错误，否则成功。
// 副作用：记录仓库并创建隔离测试目录。
func (s *fakeRepositoryStore) Create(_ context.Context, repository Repository, target string) error {
	// 1. 先应用指定仓库失败，再记录并创建目录。
	if err := s.failures[repository.FullName]; err != nil {
		return err
	}
	s.created = append(s.created, repository.FullName)
	return os.MkdirAll(target, 0o700)
}

// Update 模拟更新已有裸仓库。
// 输入：上下文、仓库、目标目录、时间和保留批次数。
// 输出：配置失败时返回错误，否则成功。
// 副作用：记录被更新仓库。
func (s *fakeRepositoryStore) Update(_ context.Context, repository Repository, _ string, _ time.Time, _ int) error {
	// 1. 先应用指定仓库失败，再记录成功更新。
	if err := s.failures[repository.FullName]; err != nil {
		return err
	}
	s.updated = append(s.updated, repository.FullName)
	return nil
}

// TestBackupCreatesOnlyConfiguredRepositories 验证首次运行合并账号自有和固定组织仓库。
// 输入：一个账号自有仓库及 KES-SCM、KES-BIS 两个固定组织仓库。
// 输出：创建三个 owner/repo.git 目录和 active 清单。
// 副作用：写入测试临时目录。
func TestBackupCreatesOnlyConfiguredRepositories(t *testing.T) {
	// 1. 使用一个账号自有仓库、两个固定组织仓库和记录型存储器执行首次备份。
	directory := t.TempDir()
	store := &fakeRepositoryStore{failures: make(map[string]error)}
	lister := fakeRepositoryLister{repositories: []Repository{{
		FullName: "howie/personal", CloneURL: "https://github.com/howie/personal.git",
	}}}
	service := NewService(lister, []string{"KES-IT/KES-SCM", "KES-IT/KES-BIS"}, store, Options{
		Directory: directory, RetentionRefs: 4,
		Now: func() time.Time { return time.Date(2026, 8, 9, 4, 0, 0, 0, time.UTC) },
	})
	result, err := service.Backup(context.Background())
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	// 2. 核对发现并创建一个账号自有仓库和两个固定组织仓库。
	if result.DiscoveredCount != 3 || result.CreatedCount != 3 || result.UpdatedCount != 0 || result.FailedCount != 0 {
		t.Fatalf("Backup() result = %+v", result)
	}
	for _, relative := range []string{"howie/personal.git", "KES-IT/KES-SCM.git", "KES-IT/KES-BIS.git"} {
		if _, err := os.Stat(filepath.Join(directory, filepath.FromSlash(relative))); err != nil {
			t.Errorf("repository %s not created: %v", relative, err)
		}
	}
	manifest, err := loadManifest(filepath.Join(directory, "manifest.json"))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}
	if len(manifest.Repositories) != 3 || manifest.Repositories["howie/personal"].Status != "active" ||
		manifest.Repositories["KES-IT/KES-SCM"].Status != "active" ||
		manifest.Repositories["KES-IT/KES-BIS"].Status != "active" {
		t.Errorf("manifest = %+v", manifest)
	}
}

// TestBackupRetainsRepositoryRemovedFromAllowlist 验证仓库移出白名单后仍永久保留最后副本。
// 输入：首次两个仓库，第二次仅保留 KES-SCM。
// 输出：KES-BIS 标记 unavailable 且目录不删除。
// 副作用：写入测试临时目录。
func TestBackupRetainsRepositoryRemovedFromAllowlist(t *testing.T) {
	// 1. 首次创建两个仓库和清单。
	directory := t.TempDir()
	store := &fakeRepositoryStore{failures: make(map[string]error)}
	options := Options{Directory: directory, RetentionRefs: 4, Now: time.Now}
	lister := fakeRepositoryLister{}
	if _, err := NewService(lister, []string{"KES-IT/KES-SCM", "KES-IT/KES-BIS"}, store, options).Backup(context.Background()); err != nil {
		t.Fatalf("first Backup() error = %v", err)
	}

	// 2. 第二次缩小白名单并核对旧仓库只标记失联、不删除目录。
	result, err := NewService(lister, []string{"KES-IT/KES-SCM"}, store, options).Backup(context.Background())
	if err != nil {
		t.Fatalf("second Backup() error = %v", err)
	}
	if result.UpdatedCount != 1 || result.UnavailableCount != 1 {
		t.Errorf("second Backup() result = %+v", result)
	}
	retained := filepath.Join(directory, "KES-IT", "KES-BIS.git")
	if _, err := os.Stat(retained); err != nil {
		t.Errorf("retained repository missing: %v", err)
	}
	manifest, err := loadManifest(filepath.Join(directory, "manifest.json"))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}
	if manifest.Repositories["KES-IT/KES-BIS"].Status != "unavailable" {
		t.Errorf("KES-BIS status = %q, want unavailable", manifest.Repositories["KES-IT/KES-BIS"].Status)
	}
}

// TestBackupFailureKeepsLastSuccessfulRepository 验证仓库失去权限时保留最后成功副本和时间。
// 输入：先成功创建，再让 KES-BIS 更新失败。
// 输出：返回部分失败，清单标记 failed，目录和最后成功时间保留。
// 副作用：写入测试临时目录。
func TestBackupFailureKeepsLastSuccessfulRepository(t *testing.T) {
	// 1. 首次成功备份并记住 KES-BIS 的成功时间。
	directory := t.TempDir()
	store := &fakeRepositoryStore{failures: make(map[string]error)}
	service := NewService(fakeRepositoryLister{}, []string{"KES-IT/KES-SCM", "KES-IT/KES-BIS"}, store,
		Options{Directory: directory, RetentionRefs: 4, Now: time.Now})
	if _, err := service.Backup(context.Background()); err != nil {
		t.Fatalf("first Backup() error = %v", err)
	}
	before, err := loadManifest(filepath.Join(directory, "manifest.json"))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}
	lastSuccess := before.Repositories["KES-IT/KES-BIS"].LastSuccessAt

	// 2. 模拟组织权限撤销，核对失败状态不删除或覆盖最后成功信息。
	store.failures["KES-IT/KES-BIS"] = errors.New("permission denied")
	result, err := service.Backup(context.Background())
	if err == nil || !strings.Contains(err.Error(), "KES-IT/KES-BIS") {
		t.Fatalf("second Backup() error = %v, want repository failure", err)
	}
	if result.UpdatedCount != 1 || result.FailedCount != 1 {
		t.Errorf("second Backup() result = %+v", result)
	}
	after, loadErr := loadManifest(filepath.Join(directory, "manifest.json"))
	if loadErr != nil {
		t.Fatalf("loadManifest() error = %v", loadErr)
	}
	record := after.Repositories["KES-IT/KES-BIS"]
	if record.Status != "failed" || record.LastSuccessAt != lastSuccess || !strings.Contains(record.LastError, "permission denied") {
		t.Errorf("failed repository record = %+v", record)
	}
	if _, statErr := os.Stat(filepath.Join(directory, "KES-IT", "KES-BIS.git")); statErr != nil {
		t.Errorf("last successful repository was removed: %v", statErr)
	}
}

// TestBackupRejectsUnsafeRepositoryName 验证白名单名称不能逃逸备份目录。
// 输入：包含上级路径的伪造仓库名。
// 输出：返回名称无效错误且不调用存储器。
// 副作用：无。
func TestBackupRejectsUnsafeRepositoryName(t *testing.T) {
	// 1. 执行带路径逃逸名称的备份。
	store := &fakeRepositoryStore{failures: make(map[string]error)}
	service := NewService(fakeRepositoryLister{}, []string{"../secret"}, store,
		Options{Directory: t.TempDir(), RetentionRefs: 4, Now: time.Now})
	_, err := service.Backup(context.Background())

	// 2. 核对在文件操作前拒绝不安全名称。
	if err == nil || !strings.Contains(err.Error(), "仓库名称无效") {
		t.Fatalf("Backup() error = %v, want invalid repository name", err)
	}
	if len(store.created) != 0 || len(store.updated) != 0 {
		t.Fatalf("store was called: created=%v updated=%v", store.created, store.updated)
	}
}

// TestBackupDiscoveryFailureDoesNotChangeLocalState 验证 GitHub API 失败时不修改已有备份。
// 输入：返回鉴权错误的账号仓库发现器。
// 输出：返回发现错误且不创建备份根目录。
// 副作用：不写入目标临时路径。
func TestBackupDiscoveryFailureDoesNotChangeLocalState(t *testing.T) {
	// 1. 使用尚不存在的目标目录和失败发现器执行备份。
	directory := filepath.Join(t.TempDir(), "github")
	store := &fakeRepositoryStore{failures: make(map[string]error)}
	service := NewService(fakeRepositoryLister{err: errors.New("bad credentials")},
		[]string{"KES-IT/KES-SCM", "KES-IT/KES-BIS"}, store,
		Options{Directory: directory, RetentionRefs: 4, Now: time.Now})
	_, err := service.Backup(context.Background())

	// 2. 核对错误明确且发现失败前没有写入任何文件。
	if err == nil || !strings.Contains(err.Error(), "发现 GitHub 账号自有仓库") {
		t.Fatalf("Backup() error = %v", err)
	}
	if _, statErr := os.Stat(directory); !os.IsNotExist(statErr) {
		t.Fatalf("backup directory changed on discovery failure: %v", statErr)
	}
}
