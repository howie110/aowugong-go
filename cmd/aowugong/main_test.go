package main

import "testing"

// TestParseCommandSupportsServerAndUnifiedJobCLI 验证默认服务模式和单任务 CLI 参数。
// 输入：无参数、job 加任务名以及缺少任务名三种参数。
// 输出：前两种正确解析，第三种返回用法错误。
// 副作用：无。
func TestParseCommandSupportsServerAndUnifiedJobCLI(t *testing.T) {
	// 1. 核对无额外参数时进入 HTTP 服务模式。
	server, err := parseCommand([]string{"aowugong"})
	if err != nil || server.mode != modeServer {
		t.Errorf("server command = %+v error = %v", server, err)
	}

	// 2. 核对 job 子命令保留任务名。
	job, err := parseCommand([]string{"aowugong", "job", "backup_mysql"})
	if err != nil || job.mode != modeJob || job.jobName != "backup_mysql" {
		t.Errorf("job command = %+v error = %v", job, err)
	}

	// 3. 缺少任务名必须返回错误而不是启动服务。
	if _, err := parseCommand([]string{"aowugong", "job"}); err == nil {
		t.Fatal("parseCommand(job) error = nil, want usage error")
	}
}
