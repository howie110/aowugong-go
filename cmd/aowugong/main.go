package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/howiedata/aowugong-go/internal/app"
	"github.com/howiedata/aowugong-go/internal/config"
)

// main 加载配置并运行支持终止信号的应用服务。
func main() {
	// 1. 从环境变量加载运行配置。
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		log.Fatal(err)
	}

	// 2. 创建响应终止信号的根上下文。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 3. 启动应用并报告不可恢复的运行错误。
	if err := app.Run(ctx, cfg); err != nil {
		log.Fatal(err)
	}
}
