// Package githubbackup 提供 GitHub 可访问仓库的低频代码冷备份能力。
package githubbackup

// Repository 描述 GitHub API 返回的可备份代码仓库。
type Repository struct {
	FullName string
	CloneURL string
}

// Result 汇总一次代码备份的发现、更新、失联和失败数量。
type Result struct {
	DiscoveredCount  int
	CreatedCount     int
	UpdatedCount     int
	UnavailableCount int
	FailedCount      int
}

// Manifest 保存历次发现的仓库和最后成功备份状态。
type Manifest struct {
	Version      int                           `json:"version"`
	UpdatedAt    string                        `json:"updated_at"`
	Repositories map[string]ManifestRepository `json:"repositories"`
}

// ManifestRepository 描述单个仓库的持久化路径和最后可用状态。
type ManifestRepository struct {
	FullName      string `json:"full_name"`
	CloneURL      string `json:"clone_url"`
	RelativePath  string `json:"relative_path"`
	Status        string `json:"status"`
	LastSeenAt    string `json:"last_seen_at,omitempty"`
	LastSuccessAt string `json:"last_success_at,omitempty"`
	LastError     string `json:"last_error,omitempty"`
}
