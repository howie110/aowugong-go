package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// CronScheduler 把统一任务注册表装载到指定时区的进程内 Cron。
type CronScheduler struct {
	registry *Registry
	location *time.Location
	mutex    sync.Mutex
	cron     *cron.Cron
	cancel   context.CancelFunc
	started  bool
}

// NewCronScheduler 创建尚未启动的进程内调度器。
// 输入：registry 是唯一任务注册表，location 是调度时区。
// 输出：返回可启动和优雅停止的调度器。
// 副作用：无，不注册或执行 Cron 任务。
func NewCronScheduler(registry *Registry, location *time.Location) *CronScheduler {
	// 1. 缺少时区时使用系统本地时区兜底。
	if location == nil {
		location = time.Local
	}
	return &CronScheduler{registry: registry, location: location, cron: cron.New(cron.WithLocation(location))}
}

// Start 装载当前全部任务定义并启动 Cron。
// 输入：无。
// 输出：成功返回 nil，注册表、表达式或重复启动无效时返回错误。
// 副作用：启动进程内 Cron，触发时通过 Registry.Run 执行业务任务。
func (s *CronScheduler) Start() error {
	// 1. 在锁内拒绝无注册表和重复启动。
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.registry == nil {
		return fmt.Errorf("任务注册表不能为空")
	}
	if s.started {
		return fmt.Errorf("Cron 调度器已经启动")
	}

	// 2. 先向新 Cron 装载全部定义，任一表达式失败都不启动半套任务。
	loaded := cron.New(cron.WithLocation(s.location))
	runContext, cancel := context.WithCancel(context.Background())
	for _, definition := range s.registry.Definitions() {
		if definition.ManualOnly {
			continue
		}
		name := definition.Name
		if _, err := loaded.AddFunc(definition.Schedule, func() {
			_, _ = s.registry.Run(runContext, name, SourceScheduler)
		}); err != nil {
			cancel()
			return fmt.Errorf("装载任务 %s Cron 表达式 %q: %w", name, definition.Schedule, err)
		}
	}

	// 3. 全部定义有效后替换实例并启动。
	s.cron = loaded
	s.cancel = cancel
	s.cron.Start()
	s.started = true
	return nil
}

// Stop 停止接收新触发并等待正在运行的 Cron 调用退出。
// 输入：ctx 限制优雅停止等待时间。
// 输出：全部退出返回 nil，调用上下文超时或取消时返回错误。
// 副作用：停止进程内 Cron 调度器。
func (s *CronScheduler) Stop(ctx context.Context) error {
	// 1. 在锁内读取并切换启动状态，未启动时直接成功。
	s.mutex.Lock()
	if !s.started {
		s.mutex.Unlock()
		return nil
	}
	cronInstance := s.cron
	cancel := s.cancel
	s.cancel = nil
	s.started = false
	s.mutex.Unlock()

	// 2. 取消已派发任务，再等待 Cron 调用完成或外部关闭上下文结束。
	cancel()
	stopped := cronInstance.Stop()
	select {
	case <-stopped.Done():
		return nil
	case <-ctx.Done():
		return fmt.Errorf("等待 Cron 调度器停止: %w", ctx.Err())
	}
}
