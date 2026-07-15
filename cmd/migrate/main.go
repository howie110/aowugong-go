package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	_ "github.com/go-sql-driver/mysql"
	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
	"github.com/howiedata/aowugong-go/internal/database/mysqlmigration"
)

type options struct {
	mode          string
	mysqlURL      string
	mysqlDSN      string
	envFile       string
	sqlitePath    string
	migrationsDir string
	reportPath    string
	tables        string
	batchSize     int
}

type commandReport struct {
	Mode                 string                               `json:"mode"`
	SQLitePath           string                               `json:"sqlite_path"`
	Inventory            mysqlmigration.Inventory             `json:"inventory"`
	UnknownSourceTables  []string                             `json:"unknown_source_tables"`
	Migration            *mysqlmigration.MigrationReport      `json:"migration,omitempty"`
	Verifications        []mysqlmigration.TableVerification   `json:"verifications"`
	ForeignKeyViolations []mysqlmigration.ForeignKeyViolation `json:"foreign_key_violations"`
	Error                string                               `json:"error,omitempty"`
}

// main 运行一次性 MySQL 盘点、迁移或核验命令。
// 输入：读取命令行参数、环境变量和可选旧项目 env 文件。
// 输出：标准输出写 JSON 报告；失败时以非零状态结束。
// 副作用：只读 MySQL，迁移模式创建并重写 SQLite，写出核验报告文件。
func main() {
	// 1. 建立可响应终止信号的根上下文并执行命令。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr, os.LookupEnv); err != nil {
		log.Fatal(err)
	}
}

// run 解析参数、执行迁移流程并输出最终报告。
// 输入：ctx 控制执行，args 是参数，stdout/stderr 是输出，lookup 查询环境变量。
// 输出：成功返回 nil，参数、连接、迁移或核验失败时返回错误。
// 副作用：连接数据库并按模式读写 SQLite 和报告文件。
func run(ctx context.Context, args []string, stdout, stderr io.Writer, lookup config.LookupEnv) error {
	// 1. 解析命令参数和 MySQL 源配置。
	opts, err := parseOptions(args, stderr)
	if err != nil {
		return err
	}
	sourceConfig, err := resolveSourceConfig(opts, lookup)
	if err != nil {
		return err
	}

	// 2. 在只读可重复读事务中执行源库盘点，确保迁移和核验看到同一快照。
	sourceDB, err := sql.Open("mysql", sourceConfig.DSN)
	if err != nil {
		return fmt.Errorf("打开 MySQL: %w", err)
	}
	defer sourceDB.Close()
	sourceDB.SetMaxOpenConns(2)
	sourceDB.SetMaxIdleConns(2)
	if err := sourceDB.PingContext(ctx); err != nil {
		return fmt.Errorf("连接 MySQL: %w", err)
	}
	sourceTransaction, err := sourceDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return fmt.Errorf("开始 MySQL 只读一致快照: %w", err)
	}
	defer sourceTransaction.Rollback()
	inventory, err := mysqlmigration.InspectSource(ctx, sourceTransaction, sourceConfig.Schema)
	if err != nil {
		return err
	}
	report := commandReport{
		Mode: opts.mode, SQLitePath: opts.sqlitePath, Inventory: inventory,
		UnknownSourceTables:  mysqlmigration.UnknownSourceTables(inventory),
		Verifications:        []mysqlmigration.TableVerification{},
		ForeignKeyViolations: []mysqlmigration.ForeignKeyViolation{},
	}
	if len(report.UnknownSourceTables) > 0 {
		report.Error = "存在尚未审计的源表: " + strings.Join(report.UnknownSourceTables, ",")
		return finalizeReport(stdout, opts, report, errors.New(report.Error))
	}
	if opts.mode == "inventory" {
		if err := sourceTransaction.Commit(); err != nil {
			return fmt.Errorf("提交 MySQL 只读盘点事务: %w", err)
		}
		return finalizeReport(stdout, opts, report, nil)
	}

	// 3. 迁移模式先在同目录临时库工作，核验模式直接打开现有目标库。
	migrationsDir, err := resolveMigrationsDirectory(opts.migrationsDir)
	if err != nil {
		report.Error = err.Error()
		return finalizeReport(stdout, opts, report, err)
	}
	targetPath := opts.sqlitePath
	stagedPath := ""
	published := false
	selectedTables := splitTables(opts.tables)
	if opts.mode == "migrate" {
		stagedPath, err = createStagedSQLitePath(opts.sqlitePath)
		if err != nil {
			report.Error = err.Error()
			return finalizeReport(stdout, opts, report, err)
		}
		if len(selectedTables) > 0 {
			if err := snapshotExistingSQLite(ctx, opts.sqlitePath, stagedPath); err != nil {
				cleanupSQLiteFiles(stagedPath)
				report.Error = err.Error()
				return finalizeReport(stdout, opts, report, err)
			}
		}
		targetPath = stagedPath
	}
	target, err := database.OpenSQLite(ctx, config.Database{Path: targetPath})
	if err != nil {
		cleanupSQLiteFiles(stagedPath)
		report.Error = err.Error()
		return finalizeReport(stdout, opts, report, err)
	}
	targetOpen := true
	defer func() {
		if targetOpen {
			_ = target.Close()
		}
		if stagedPath != "" && !published {
			cleanupSQLiteFiles(stagedPath)
		}
	}()
	if err := database.Migrate(ctx, target, migrationsDir); err != nil {
		report.Error = err.Error()
		return finalizeReport(stdout, opts, report, err)
	}
	plans, err := mysqlmigration.BuildPlans(
		ctx, target, inventory, mysqlmigration.DefaultTableSpecs(), selectedTables,
	)
	if err != nil {
		report.Error = err.Error()
		return finalizeReport(stdout, opts, report, err)
	}

	// 4. 按模式执行只读核验或完整复制，并保留失败报告。
	if opts.mode == "verify" {
		report.Verifications, report.ForeignKeyViolations, err = mysqlmigration.VerifyOnly(ctx, sourceTransaction, target, plans)
		if err == nil {
			err = verificationError(report.Verifications, report.ForeignKeyViolations)
		}
	} else {
		lastProgress := make(map[string]int64)
		progress := func(table string, copied int64) {
			// 5. 每十万行输出一次不含敏感信息的迁移进度。
			if copied-lastProgress[table] >= 100000 {
				fmt.Fprintf(stderr, "%s: 已复制 %d 行\n", table, copied)
				lastProgress[table] = copied
			}
		}
		var migrationReport mysqlmigration.MigrationReport
		migrationReport, err = mysqlmigration.Migrate(ctx, sourceTransaction, target, plans, opts.batchSize, progress)
		report.Migration = &migrationReport
	}
	if err != nil {
		report.Error = err.Error()
		return finalizeReport(stdout, opts, report, err)
	}
	if err := sourceTransaction.Commit(); err != nil {
		report.Error = err.Error()
		return finalizeReport(stdout, opts, report, fmt.Errorf("提交 MySQL 只读事务: %w", err))
	}

	// 5. 完整迁移必须 checkpoint、关闭并同步临时库后才原子替换最终文件。
	if opts.mode == "migrate" {
		closeErr := checkpointAndCloseSQLite(ctx, target)
		targetOpen = false
		if closeErr != nil {
			report.Error = closeErr.Error()
			return finalizeReport(stdout, opts, report, closeErr)
		}
		if err := publishStagedSQLite(stagedPath, opts.sqlitePath); err != nil {
			report.Error = err.Error()
			return finalizeReport(stdout, opts, report, err)
		}
		published = true
	} else {
		if err := target.Close(); err != nil {
			targetOpen = false
			report.Error = err.Error()
			return finalizeReport(stdout, opts, report, fmt.Errorf("关闭核验 SQLite: %w", err))
		}
		targetOpen = false
	}
	return finalizeReport(stdout, opts, report, nil)
}

// parseOptions 解析迁移模式、源连接、目标路径、报告和批次参数。
// 输入：args 是命令参数，errorsOutput 接收 flag 用法错误。
// 输出：返回规范化选项；模式或批次无效时返回错误。
// 副作用：可能向 errorsOutput 写入参数说明。
func parseOptions(args []string, errorsOutput io.Writer) (options, error) {
	// 1. 定义默认可直接建库的迁移参数。
	opts := options{}
	flags := flag.NewFlagSet("aowugong-migrate", flag.ContinueOnError)
	flags.SetOutput(errorsOutput)
	flags.StringVar(&opts.mode, "mode", "migrate", "inventory、migrate 或 verify")
	flags.StringVar(&opts.mysqlURL, "mysql-url", "", "SQLAlchemy/MySQL URL，默认读取环境变量")
	flags.StringVar(&opts.mysqlDSN, "mysql-dsn", "", "Go MySQL DSN，优先于 URL")
	flags.StringVar(&opts.envFile, "env-file", "", "可选旧项目 .env 文件，仅读取 DATABASE_URL/MYSQL_DSN")
	flags.StringVar(&opts.sqlitePath, "sqlite", "storage/data/aowugong.db", "目标 SQLite 文件")
	flags.StringVar(&opts.migrationsDir, "migrations", "", "SQLite migration 目录")
	flags.StringVar(&opts.reportPath, "report", "", "JSON 报告路径")
	flags.StringVar(&opts.tables, "tables", "", "可选逗号分隔有效表")
	flags.IntVar(&opts.batchSize, "batch-size", 1000, "每批复制行数")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}

	// 2. 校验模式、批次和目标路径并应用默认报告路径。
	opts.mode = strings.ToLower(strings.TrimSpace(opts.mode))
	if opts.mode != "inventory" && opts.mode != "migrate" && opts.mode != "verify" {
		return options{}, fmt.Errorf("mode 必须是 inventory、migrate 或 verify")
	}
	if opts.batchSize < 1 || opts.batchSize > 10000 {
		return options{}, fmt.Errorf("batch-size 必须在 1 到 10000 之间")
	}
	if strings.TrimSpace(opts.sqlitePath) == "" {
		return options{}, fmt.Errorf("sqlite 路径不能为空")
	}
	opts.sqlitePath = filepath.Clean(opts.sqlitePath)
	if opts.reportPath == "" {
		opts.reportPath = filepath.Join("storage", "exports", "mysql-sqlite-"+time.Now().Format("20060102-150405")+".json")
	}
	opts.reportPath = filepath.Clean(opts.reportPath)
	return opts, nil
}

// resolveSourceConfig 按 flag、env 文件和环境变量优先级解析 MySQL 源。
// 输入：opts 提供显式值和 env 文件，lookup 查询进程环境。
// 输出：返回不对外暴露密码的源配置；缺失或格式错误时返回错误。
// 副作用：可选只读打开 env 文件。
func resolveSourceConfig(opts options, lookup config.LookupEnv) (mysqlmigration.SourceConfig, error) {
	// 1. 显式原生 DSN 和 URL 优先。
	if strings.TrimSpace(opts.mysqlDSN) != "" {
		return mysqlmigration.ParseSourceDSN(opts.mysqlDSN)
	}
	if strings.TrimSpace(opts.mysqlURL) != "" {
		return mysqlmigration.ParseSourceURL(opts.mysqlURL)
	}

	// 2. 可选旧项目 env 文件只读取数据库连接字段。
	if strings.TrimSpace(opts.envFile) != "" {
		values, err := readDatabaseEnv(opts.envFile)
		if err != nil {
			return mysqlmigration.SourceConfig{}, err
		}
		if value := values["MYSQL_DSN"]; value != "" {
			return mysqlmigration.ParseSourceDSN(value)
		}
		if value := values["DATABASE_URL"]; value != "" {
			return mysqlmigration.ParseSourceURL(value)
		}
	}

	// 3. 依次兼容新旧环境变量名称。
	for _, key := range []string{"AOWUGONG_MYSQL_DSN", "MYSQL_DSN"} {
		if value, ok := lookup(key); ok && strings.TrimSpace(value) != "" {
			return mysqlmigration.ParseSourceDSN(value)
		}
	}
	for _, key := range []string{"AOWUGONG_MYSQL_URL", "DATABASE_URL"} {
		if value, ok := lookup(key); ok && strings.TrimSpace(value) != "" {
			return mysqlmigration.ParseSourceURL(value)
		}
	}
	return mysqlmigration.SourceConfig{}, fmt.Errorf("缺少 MySQL 源，请设置 -mysql-url、-mysql-dsn、-env-file 或 DATABASE_URL")
}

// readDatabaseEnv 从 dotenv 文本读取数据库连接字段。
// 输入：path 是旧项目 .env 文件路径。
// 输出：返回 MYSQL_DSN/DATABASE_URL 映射；读取失败时返回错误。
// 副作用：只读本地文件，不修改或输出其中的密钥。
func readDatabaseEnv(path string) (map[string]string, error) {
	// 1. 打开文件并逐行解析 KEY=VALUE，只保留两个允许字段。
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("打开数据库 env 文件: %w", err)
	}
	defer file.Close()
	result := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "MYSQL_DSN" || key == "DATABASE_URL" {
			result[key] = strings.Trim(strings.TrimSpace(value), "'\"")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取数据库 env 文件: %w", err)
	}
	return result, nil
}

// resolveMigrationsDirectory 查找显式目录、可执行文件同级目录或当前目录。
// 输入：configured 是可选显式路径。
// 输出：返回存在的 migration 目录；找不到时返回错误。
// 副作用：读取文件系统元数据。
func resolveMigrationsDirectory(configured string) (string, error) {
	// 1. 显式配置优先，随后检查发布产物和源码运行位置。
	candidates := make([]string, 0, 3)
	if strings.TrimSpace(configured) != "" {
		candidates = append(candidates, configured)
	}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), "migrations"))
	}
	candidates = append(candidates, "migrations")
	for _, candidate := range candidates {
		path, err := filepath.Abs(filepath.Clean(candidate))
		if err != nil {
			continue
		}
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("找不到 SQLite migrations 目录")
}

// splitTables 清理逗号分隔表名并去除空项。
// 输入：value 是 -tables 参数。
// 输出：返回表名切片，空输入返回 nil。
// 副作用：无。
func splitTables(value string) []string {
	// 1. 保持调用方顺序清理表名。
	if strings.TrimSpace(value) == "" {
		return nil
	}
	result := make([]string, 0)
	for _, item := range strings.Split(value, ",") {
		if name := strings.TrimSpace(item); name != "" {
			result = append(result, name)
		}
	}
	return result
}

// createStagedSQLitePath 在最终库同目录预留唯一临时路径。
// 输入：finalPath 是最终 SQLite 路径。
// 输出：返回不存在且与最终库同文件系统的临时路径。
// 副作用：创建目标目录并短暂创建后删除一个空临时文件。
func createStagedSQLitePath(finalPath string) (string, error) {
	// 1. 创建最终目录并让系统生成不可碰撞的同目录名称。
	directory := filepath.Dir(filepath.Clean(finalPath))
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return "", fmt.Errorf("创建 SQLite 目录: %w", err)
	}
	file, err := os.CreateTemp(directory, "."+filepath.Base(finalPath)+".migrating-*")
	if err != nil {
		return "", fmt.Errorf("创建 SQLite 临时路径: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("关闭 SQLite 临时占位文件: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("释放 SQLite 临时路径: %w", err)
	}
	return path, nil
}

// snapshotExistingSQLite 为局部迁移创建包含全部原数据的一致临时快照。
// 输入：ctx 控制快照，finalPath 是现有完整库，stagedPath 是不存在的同目录临时路径。
// 输出：快照创建并关闭成功返回 nil。
// 副作用：只读现有 SQLite 一致视图并通过 VACUUM INTO 创建临时库。
func snapshotExistingSQLite(ctx context.Context, finalPath, stagedPath string) error {
	// 1. 局部迁移必须基于存在的正式库，不能从空库发布未选表。
	info, err := os.Stat(finalPath)
	if err != nil {
		return fmt.Errorf("局部迁移需要现有 SQLite %s: %w", finalPath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("局部迁移 SQLite 不是普通文件: %s", finalPath)
	}
	if _, err := os.Stat(stagedPath); err == nil {
		return fmt.Errorf("局部迁移临时 SQLite 已存在: %s", stagedPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查局部迁移临时 SQLite: %w", err)
	}

	// 2. 使用 SQLite 自身的一致快照能力复制全部未选表和运行表。
	source, err := database.OpenSQLite(ctx, config.Database{Path: finalPath})
	if err != nil {
		return fmt.Errorf("打开局部迁移源 SQLite: %w", err)
	}
	if _, err := source.ExecContext(ctx, "VACUUM INTO ?", stagedPath); err != nil {
		_ = source.Close()
		return fmt.Errorf("创建局部迁移 SQLite 快照: %w", err)
	}
	if err := source.Close(); err != nil {
		return fmt.Errorf("关闭局部迁移源 SQLite: %w", err)
	}
	return nil
}

// checkpointAndCloseSQLite 把 WAL 完整合入主文件并关闭迁移连接。
// 输入：ctx 控制 checkpoint，db 是只供本次迁移使用的 SQLite。
// 输出：checkpoint 无忙连接且关闭成功时返回 nil。
// 副作用：截断 WAL 并关闭数据库连接池。
func checkpointAndCloseSQLite(ctx context.Context, db *sql.DB) error {
	// 1. 执行 TRUNCATE checkpoint 并拒绝仍有忙连接的结果。
	if db == nil {
		return fmt.Errorf("迁移 SQLite 不能为空")
	}
	var busy, logFrames, checkpointedFrames int
	checkpointErr := db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(
		&busy, &logFrames, &checkpointedFrames,
	)
	closeErr := db.Close()
	if checkpointErr != nil {
		return fmt.Errorf("checkpoint 迁移 SQLite: %w", checkpointErr)
	}
	if busy != 0 || logFrames != checkpointedFrames {
		return fmt.Errorf("checkpoint 迁移 SQLite 未完成: busy=%d log=%d checkpointed=%d", busy, logFrames, checkpointedFrames)
	}
	if closeErr != nil {
		return fmt.Errorf("关闭迁移 SQLite: %w", closeErr)
	}
	return nil
}

// publishStagedSQLite 同步并原子替换最终 SQLite 主文件。
// 输入：stagedPath 是已关闭临时库，finalPath 是最终库路径。
// 输出：发布和目录同步成功返回 nil，存在任一 sidecar 或文件错误时返回错误。
// 副作用：同步文件并在同一目录原子替换最终路径。
func publishStagedSQLite(stagedPath, finalPath string) error {
	// 1. 任何源或目标 WAL/SHM 都表示数据库尚未安全独立，必须停止发布。
	for _, sidecar := range []string{stagedPath + "-wal", stagedPath + "-shm", finalPath + "-wal", finalPath + "-shm"} {
		if _, err := os.Stat(sidecar); err == nil {
			return fmt.Errorf("拒绝发布仍存在 SQLite sidecar 的数据库: %s", sidecar)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("检查 SQLite sidecar %s: %w", sidecar, err)
		}
	}
	if err := syncFile(stagedPath); err != nil {
		return err
	}

	// 2. 临时库与最终库位于同一目录，Rename 在生产 Linux 上原子替换目录项。
	if err := os.Rename(stagedPath, finalPath); err != nil {
		return fmt.Errorf("原子发布 SQLite: %w", err)
	}
	if err := syncDirectory(filepath.Dir(finalPath)); err != nil {
		return err
	}
	return nil
}

// syncFile 强制把 SQLite 主文件写入稳定存储。
// 输入：path 是已关闭 SQLite 主文件。
// 输出：打开、同步和关闭均成功时返回 nil。
// 副作用：调用操作系统文件同步。
func syncFile(path string) error {
	// 1. 以读写方式打开并执行 fsync，确保重命名前数据已落盘。
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("打开待同步 SQLite: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("同步 SQLite 文件: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("关闭已同步 SQLite: %w", err)
	}
	return nil
}

// syncDirectory 同步 SQLite 所在目录的重命名结果。
// 输入：path 是最终文件父目录。
// 输出：生产 Linux 同步成功返回 nil；Windows 不支持目录 fsync 时安全跳过。
// 副作用：调用操作系统目录同步。
func syncDirectory(path string) error {
	// 1. 打开目录并在支持的平台同步目录项。
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("打开 SQLite 目录: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil && runtime.GOOS != "windows" {
		return fmt.Errorf("同步 SQLite 目录: %w", err)
	}
	return nil
}

// cleanupSQLiteFiles 删除失败迁移留下的临时主文件和 sidecar。
// 输入：path 是临时 SQLite 主文件路径，空路径不操作。
// 输出：无。
// 副作用：尽力删除临时数据库文件，不触碰最终库。
func cleanupSQLiteFiles(path string) {
	// 1. 只清理本次唯一临时路径及其 WAL/SHM。
	if strings.TrimSpace(path) == "" {
		return
	}
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		_ = os.Remove(candidate)
	}
}

// verificationError 把核验失败表和外键异常转换为命令错误。
// 输入：tables 是逐表结果，violations 是外键异常。
// 输出：全部通过返回 nil，否则返回汇总错误。
// 副作用：无。
func verificationError(tables []mysqlmigration.TableVerification, violations []mysqlmigration.ForeignKeyViolation) error {
	// 1. 收集未通过表并同时检查外键异常。
	failed := make([]string, 0)
	for _, table := range tables {
		if !table.Passed {
			failed = append(failed, table.Name)
		}
	}
	if len(failed) == 0 && len(violations) == 0 {
		return nil
	}
	return fmt.Errorf("核验失败表=%s，外键异常=%d", strings.Join(failed, ","), len(violations))
}

// finalizeReport 写报告文件和标准输出，并保留原始执行错误。
// 输入：stdout 接收 JSON，opts 提供路径，report 是报告，runErr 是执行错误。
// 输出：报告输出成功时返回 runErr，否则返回报告写入错误。
// 副作用：创建父目录并写 JSON 报告文件和标准输出。
func finalizeReport(stdout io.Writer, opts options, report commandReport, runErr error) error {
	// 1. 创建报告目录并写入格式化 JSON。
	if err := os.MkdirAll(filepath.Dir(opts.reportPath), 0o750); err != nil {
		return fmt.Errorf("创建迁移报告目录: %w", err)
	}
	file, err := os.OpenFile(opts.reportPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return fmt.Errorf("创建迁移报告: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		_ = file.Close()
		return fmt.Errorf("写迁移报告: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("关闭迁移报告: %w", err)
	}

	// 2. 标准输出写一份相同 JSON，便于 systemd/SSH 留痕。
	outputEncoder := json.NewEncoder(stdout)
	outputEncoder.SetIndent("", "  ")
	if err := outputEncoder.Encode(report); err != nil {
		return fmt.Errorf("输出迁移报告: %w", err)
	}
	if runErr != nil {
		return runErr
	}
	return nil
}
