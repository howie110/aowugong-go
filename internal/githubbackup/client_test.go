package githubbackup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// TestClientListsOnlyOwnedRepositoriesAcrossPages 验证账号仓库发现使用 owner 过滤并正确分页。
// 输入：第一页一百项、第二页一项的模拟 GitHub API。
// 输出：返回一百零一个账号自有仓库且请求携带正确鉴权和过滤参数。
// 副作用：启动本机临时 HTTP 服务。
func TestClientListsOnlyOwnedRepositoriesAcrossPages(t *testing.T) {
	// 1. 启动严格校验请求参数的两页模拟 GitHub API。
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/user/repos" || request.URL.Query().Get("affiliation") != "owner" ||
			request.URL.Query().Get("visibility") != "all" || request.URL.Query().Get("per_page") != "100" {
			t.Errorf("request URL = %s", request.URL.String())
		}
		if request.Header.Get("Authorization") != "Bearer test-token" ||
			request.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("request headers = %#v", request.Header)
		}
		page, _ := strconv.Atoi(request.URL.Query().Get("page"))
		count := 100
		start := 0
		if page == 2 {
			count = 1
			start = 100
		}
		payload := make([]map[string]string, 0, count)
		for index := 0; index < count; index++ {
			name := fmt.Sprintf("howie/repository-%03d", start+index)
			payload = append(payload, map[string]string{
				"full_name": name, "clone_url": "https://github.com/" + name + ".git",
			})
		}
		writer.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(writer).Encode(payload); err != nil {
			t.Errorf("Encode() error = %v", err)
		}
	}))
	defer server.Close()

	// 2. 执行发现并核对跨页数量和排序边界。
	client := newClient(server.URL, "test-token", server.Client())
	repositories, err := client.ListRepositories(context.Background())
	if err != nil {
		t.Fatalf("ListRepositories() error = %v", err)
	}
	if len(repositories) != 101 || repositories[0].FullName != "howie/repository-000" ||
		repositories[100].FullName != "howie/repository-100" {
		t.Errorf("repositories length/bounds = %d/%+v/%+v", len(repositories), repositories[0], repositories[len(repositories)-1])
	}
}

// TestClientRejectsGitHubAPIError 验证鉴权或 API 错误不会被当成空仓库列表。
// 输入：返回 401 的模拟 GitHub API。
// 输出：返回包含状态码的错误。
// 副作用：启动本机临时 HTTP 服务。
func TestClientRejectsGitHubAPIError(t *testing.T) {
	// 1. 启动始终拒绝鉴权的模拟 API 并发起发现请求。
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "bad credentials", http.StatusUnauthorized)
	}))
	defer server.Close()
	client := newClient(server.URL, "invalid-token", server.Client())
	_, err := client.ListRepositories(context.Background())

	// 2. API 失败必须显式返回，避免把历史仓库误标为全部失联。
	if err == nil || !containsAll(err.Error(), "HTTP 401", "bad credentials") {
		t.Fatalf("ListRepositories() error = %v", err)
	}
}

// containsAll 判断文本是否同时包含全部片段。
// 输入：text 是待检查文本，fragments 是必要片段。
// 输出：全部命中返回 true。
// 副作用：无。
func containsAll(text string, fragments ...string) bool {
	// 1. 逐项检查任一缺失即返回 false。
	for _, fragment := range fragments {
		if !strings.Contains(text, fragment) {
			return false
		}
	}
	return true
}
