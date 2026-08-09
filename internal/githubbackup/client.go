package githubbackup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultGitHubAPIURL = "https://api.github.com"
	githubPageSize      = 100
)

// RepositoryLister 定义账号自有仓库的发现能力。
type RepositoryLister interface {
	ListRepositories(ctx context.Context) ([]Repository, error)
}

// Client 使用 GitHub REST API 发现当前认证账号拥有的全部仓库。
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient 创建 GitHub 账号仓库发现客户端。
// 输入：token 是 GitHub Token，httpClient 控制请求超时和连接复用。
// 输出：返回可复用客户端。
// 副作用：无，不立即访问 GitHub。
func NewClient(token string, httpClient *http.Client) *Client {
	// 1. 使用正式 GitHub API 根地址创建客户端。
	return newClient(defaultGitHubAPIURL, token, httpClient)
}

// newClient 创建可替换 API 根地址的仓库发现客户端。
// 输入：baseURL 是 API 根地址，token 是令牌，httpClient 发起请求。
// 输出：返回客户端，供正式环境和隔离测试使用。
// 副作用：无。
func newClient(baseURL, token string, httpClient *http.Client) *Client {
	// 1. 补齐默认 HTTP 客户端并清理地址和令牌。
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token), httpClient: httpClient,
	}
}

// ListRepositories 分页读取当前认证账号拥有的全部公有和私有仓库。
// 输入：ctx 控制所有 GitHub API 请求。
// 输出：返回按完整名称排序且去重的账号自有仓库；鉴权或响应失败时返回错误。
// 副作用：调用 GitHub REST API，不修改远端数据。
func (c *Client) ListRepositories(ctx context.Context) ([]Repository, error) {
	// 1. 校验客户端配置，避免发送缺少身份的发现请求。
	if c == nil || c.httpClient == nil || c.baseURL == "" || c.token == "" {
		return nil, fmt.Errorf("GitHub 仓库发现配置不完整")
	}

	// 2. 逐页读取 affiliation=owner 仓库并按完整名称去重。
	byName := make(map[string]Repository)
	for page := 1; ; page++ {
		repositories, err := c.listPage(ctx, page)
		if err != nil {
			return nil, err
		}
		for _, repository := range repositories {
			if repository.FullName == "" || repository.CloneURL == "" {
				continue
			}
			byName[repository.FullName] = repository
		}
		if len(repositories) < githubPageSize {
			break
		}
	}

	// 3. 返回稳定顺序，便于日志、清单和测试保持一致。
	result := make([]Repository, 0, len(byName))
	for _, repository := range byName {
		result = append(result, repository)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].FullName < result[j].FullName })
	return result, nil
}

// listPage 读取一页当前账号拥有的仓库。
// 输入：ctx 控制请求，page 是从 1 开始的页码。
// 输出：返回本页仓库；网络、状态码或 JSON 无效时返回错误。
// 副作用：发起一次 GitHub API GET 请求。
func (c *Client) listPage(ctx context.Context, page int) ([]Repository, error) {
	// 1. 使用结构化参数限定账号自有仓库和全部可见性。
	endpoint, err := url.Parse(c.baseURL + "/user/repos")
	if err != nil {
		return nil, fmt.Errorf("解析 GitHub API 地址: %w", err)
	}
	query := endpoint.Query()
	query.Set("affiliation", "owner")
	query.Set("visibility", "all")
	query.Set("sort", "full_name")
	query.Set("direction", "asc")
	query.Set("per_page", strconv.Itoa(githubPageSize))
	query.Set("page", strconv.Itoa(page))
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("创建 GitHub 仓库请求: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "aowugong-go-github-backup")

	// 2. 拒绝任何非成功状态并限制错误响应体大小。
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("读取 GitHub 仓库第 %d 页: %w", page, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return nil, fmt.Errorf("读取 GitHub 仓库第 %d 页 HTTP %d: %s",
			page, response.StatusCode, strings.TrimSpace(string(body)))
	}

	// 3. 只解码代码备份所需字段，避免绑定无关 GitHub 模型。
	var payload []struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("解析 GitHub 仓库第 %d 页: %w", page, err)
	}
	result := make([]Repository, 0, len(payload))
	for _, item := range payload {
		result = append(result, Repository{
			FullName: strings.TrimSpace(item.FullName), CloneURL: strings.TrimSpace(item.CloneURL),
		})
	}
	return result, nil
}
