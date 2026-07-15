package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	_ "time/tzdata"

	"github.com/howiedata/aowugong-go/internal/app"
	"github.com/howiedata/aowugong-go/internal/config"
)

type commandMode string

const (
	modeServer commandMode = "server"
	modeJob    commandMode = "job"
)

type parsedCommand struct {
	mode    commandMode
	jobName string
}

// main 解析命令、加载配置并运行 HTTP 服务或统一任务入口。
// 输入：读取进程参数和环境变量。
// 输出：无；不可恢复错误以非零状态结束进程。
// 副作用：启动应用服务或执行任务，并写标准输出和日志。
func main() {
	// 1. 使用统一命令执行函数，错误交给进程日志和退出码。
	if err := executeCommand(os.Args, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

// executeCommand 加载配置并执行服务模式或单任务 CLI 模式。
// 输入：args 是进程参数，output 接收任务 JSON 结果。
// 输出：执行成功返回 nil，参数、配置或运行失败时返回错误。
// 副作用：读取环境变量，监听终止信号，启动服务或执行任务。
func executeCommand(args []string, output io.Writer) error {
	// 1. 解析命令并从环境变量加载最终运行配置。
	command, err := parseCommand(args)
	if err != nil {
		return err
	}
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("加载配置: %w", err)
	}

	// 2. 建立服务和任务共用的终止信号上下文。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if command.mode == modeServer {
		return app.Run(ctx, cfg)
	}

	// 3. CLI 任务调用同一 Registry.Run 并输出机器可读结果。
	result, err := app.RunJob(ctx, cfg, command.jobName)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(output).Encode(result); err != nil {
		return fmt.Errorf("输出任务结果: %w", err)
	}
	return nil
}

// parseCommand 解析默认服务模式和 job 子命令。
// 输入：args 包含可执行文件名及可选参数。
// 输出：返回 server 或带任务名的 job 命令；格式无效时返回用法错误。
// 副作用：无。
func parseCommand(args []string) (parsedCommand, error) {
	// 1. 无额外参数时保持正式 HTTP 服务入口行为。
	if len(args) == 1 {
		return parsedCommand{mode: modeServer}, nil
	}

	// 2. job 模式必须且只能提供一个非空注册任务名。
	if len(args) == 3 && args[1] == "job" && strings.TrimSpace(args[2]) != "" {
		return parsedCommand{mode: modeJob, jobName: strings.TrimSpace(args[2])}, nil
	}
	return parsedCommand{}, fmt.Errorf("用法: aowugong [job <任务名>]")
}
