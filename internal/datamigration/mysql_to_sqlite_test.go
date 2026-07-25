package datamigration

import (
	"context"
	"database/sql"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
)

// TestCopyTableStreamsMySQLValuesIntoSQLite 验证单表复制保留主键、文本和时间值。
// 输入：模拟 MySQL 用户行和真实临时 SQLite 目标表。
// 输出：复制数量与目标行数一致，字节文本被正确写入。
// 副作用：创建并写入测试临时 SQLite 文件。
func TestCopyTableStreamsMySQLValuesIntoSQLite(t *testing.T) {
	// 1. 创建最小 SQLite 目标表和目标事务。
	ctx := context.Background()
	targetDB, err := database.OpenSQLite(ctx, config.Database{
		Path: filepath.Join(t.TempDir(), "target.db"), MaxOpenConns: 1,
		MaxIdleConns: 1, BusyTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	defer targetDB.Close()
	if _, err := targetDB.Exec(`CREATE TABLE sample(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create target table: %v", err)
	}
	targetTx, err := targetDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("target BeginTx() error = %v", err)
	}
	defer targetTx.Rollback()

	// 2. 用 sqlmock 模拟 MySQL 按目标字段顺序返回字节文本。
	sourceDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer sourceDB.Close()
	mock.ExpectBegin()
	sourceTx, err := sourceDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("source BeginTx() error = %v", err)
	}
	defer sourceTx.Rollback()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `id`,`name`,`created_at` FROM `sample`")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "created_at"}).
			AddRow(int64(7), []byte("迁移记录"), []byte("2026-07-25 10:00:00")))

	// 3. 执行复制并核对目标事务中的数据。
	mock.ExpectQuery(regexp.QuoteMeta("SELECT MIN(`created_at`),MAX(`created_at`) FROM `sample`")).
		WillReturnRows(sqlmock.NewRows([]string{"min", "max"}).AddRow(
			[]byte("2026-07-25 10:00:00"), []byte("2026-07-25 10:00:00"),
		))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `id`,`name`,`created_at` FROM `sample` ORDER BY `id` LIMIT 1")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "created_at"}).
			AddRow(int64(7), []byte("迁移记录"), []byte("2026-07-25 10:00:00")))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `id`,`name`,`created_at` FROM `sample` ORDER BY `id` DESC LIMIT 1")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "created_at"}).
			AddRow(int64(7), []byte("迁移记录"), []byte("2026-07-25 10:00:00")))
	result, err := copyTable(ctx, sourceTx, targetTx, tableSpec{
		name: "sample", keyColumns: []string{"id"}, dateColumn: "created_at",
	})
	if err != nil {
		t.Fatalf("copyTable() error = %v", err)
	}
	if result.SourceRows != 1 || result.TargetRows != 1 {
		t.Errorf("result = %#v", result)
	}
	if result.Samples != 2 || result.DateMin != "2026-07-25 10:00:00" {
		t.Errorf("audit result = %#v", result)
	}
	var id int64
	var name, createdAt string
	if err := targetTx.QueryRow(`SELECT id,name,created_at FROM sample`).Scan(&id, &name, &createdAt); err != nil {
		t.Fatalf("query copied row: %v", err)
	}
	if id != 7 || name != "迁移记录" || createdAt != "2026-07-25 10:00:00" {
		t.Errorf("copied row = %d %q %q", id, name, createdAt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations = %v", err)
	}
}

// TestCountMySQLRowsUsesMySQLIdentifierRules 验证源表核对使用反引号而非 SQLite 双引号。
// 输入：模拟 MySQL tushare_daily 计数查询。
// 输出：返回固定行数并满足反引号 SQL 预期。
// 副作用：只操作模拟数据库。
func TestCountMySQLRowsUsesMySQLIdentifierRules(t *testing.T) {
	// 1. 建立只接受 MySQL 反引号查询的模拟事务。
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()
	mock.ExpectBegin()
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer tx.Rollback()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM `tushare_daily`")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4596173))

	// 2. 核对跳过行数读取正确。
	count, err := countMySQLRows(context.Background(), tx, "tushare_daily")
	if err != nil {
		t.Fatalf("countMySQLRows() error = %v", err)
	}
	if count != 4596173 {
		t.Errorf("count = %d, want 4596173", count)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations = %v", err)
	}
}

// TestMigrationTableListExcludesDailyHistory 验证一次性迁移清单不复制个股日线大表。
// 输入：代码维护的固定迁移表清单。
// 输出：tushare_daily 不在复制清单，三个小型 Tushare 元数据表仍保留。
// 副作用：无。
func TestMigrationTableListExcludesDailyHistory(t *testing.T) {
	// 1. 把固定清单转换为便于断言的集合。
	names := make(map[string]bool, len(migrationTables))
	for _, table := range migrationTables {
		names[table.name] = true
	}

	// 2. 大表必须排除，小型基础数据必须迁移。
	if names["tushare_daily"] {
		t.Fatal("migrationTables contains tushare_daily")
	}
	for _, name := range []string{"tushare_etf_basic", "tushare_stock_basic", "tushare_trade_cal"} {
		if !names[name] {
			t.Errorf("migrationTables missing %s", name)
		}
	}
}

// TestMigrationTableListCoversSQLiteBusinessSchema 验证迁移清单覆盖全部持久业务表。
// 输入：正式 SQLite baseline 和代码维护的迁移表清单。
// 输出：除日线和临时任务锁外，每张业务表恰好出现在迁移清单一次。
// 副作用：创建并迁移临时 SQLite 文件。
func TestMigrationTableListCoversSQLiteBusinessSchema(t *testing.T) {
	// 1. 建立正式 SQLite 结构并读取所有非框架数据表。
	ctx := context.Background()
	db, err := database.OpenSQLite(ctx, config.Database{
		Path: filepath.Join(t.TempDir(), "schema.db"), MaxOpenConns: 1,
		MaxIdleConns: 1, BusyTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	defer db.Close()
	if err := database.MigrateSQLite(ctx, db, filepath.Join("..", "..", "migrations", "sqlite")); err != nil {
		t.Fatalf("MigrateSQLite() error = %v", err)
	}
	rows, err := db.Query(`
		SELECT name
		FROM sqlite_schema
		WHERE type = 'table'
		  AND name NOT LIKE 'sqlite_%'
		  AND name <> 'goose_db_version'
		ORDER BY name
	`)
	if err != nil {
		t.Fatalf("query schema tables: %v", err)
	}
	defer rows.Close()
	schemaTables := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan schema table: %v", err)
		}
		schemaTables[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate schema tables: %v", err)
	}

	// 2. 去掉有意不复制的日线和运行时锁，再与迁移清单双向核对。
	delete(schemaTables, "tushare_daily")
	delete(schemaTables, "job_execution_lock")
	migrationSet := make(map[string]bool, len(migrationTables))
	for _, spec := range migrationTables {
		if migrationSet[spec.name] {
			t.Fatalf("migrationTables contains duplicate %s", spec.name)
		}
		migrationSet[spec.name] = true
	}
	var missing, unknown []string
	for name := range schemaTables {
		if !migrationSet[name] {
			missing = append(missing, name)
		}
	}
	for name := range migrationSet {
		if !schemaTables[name] {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(unknown)
	if len(missing) > 0 || len(unknown) > 0 {
		t.Fatalf("migration schema mismatch: missing=%v unknown=%v", missing, unknown)
	}
}

// TestClearSQLiteTargetRemovesCanaryRowsAndResetsSequences 验证最终迁移清理测试写入和旧自增序号。
// 输入：包含高位用户主键、日线和任务锁的正式临时 SQLite。
// 输出：全部业务行被删除，新用户主键从 1 重新开始。
// 副作用：创建、迁移并写入测试临时 SQLite 文件。
func TestClearSQLiteTargetRemovesCanaryRowsAndResetsSequences(t *testing.T) {
	// 1. 建立正式结构并模拟 canary 写入高位主键、日线和运行锁。
	ctx := context.Background()
	db, err := database.OpenSQLite(ctx, config.Database{
		Path: filepath.Join(t.TempDir(), "clear-target.db"), MaxOpenConns: 1,
		MaxIdleConns: 1, BusyTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	defer db.Close()
	if err := database.MigrateSQLite(ctx, db, filepath.Join("..", "..", "migrations", "sqlite")); err != nil {
		t.Fatalf("MigrateSQLite() error = %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO aowugong_fastapi_users(id,username,email,password)
		VALUES(42,'canary','canary@example.com','hash');
		INSERT INTO tushare_daily(id,ts_code,trade_date) VALUES(99,'000001.SZ','20260725');
		INSERT INTO job_execution_lock(lock_name,owner_token,acquired_at,expires_at)
		VALUES('aowugong:job:test','owner','2026-07-25T00:00:00Z','2026-07-25T01:00:00Z')
	`); err != nil {
		t.Fatalf("seed canary rows: %v", err)
	}

	// 2. 在迁移事务内清空目标，再插入无主键用户核对序号和排除表。
	transaction, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer transaction.Rollback()
	if err := clearSQLiteTarget(ctx, transaction); err != nil {
		t.Fatalf("clearSQLiteTarget() error = %v", err)
	}
	result, err := transaction.Exec(`
		INSERT INTO aowugong_fastapi_users(username,email,password)
		VALUES('source','source@example.com','hash')
	`)
	if err != nil {
		t.Fatalf("insert source row: %v", err)
	}
	userID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId() error = %v", err)
	}
	if userID != 1 {
		t.Errorf("new user id = %d, want 1", userID)
	}
	for _, table := range []string{"tushare_daily", "job_execution_lock"} {
		var count int
		if err := transaction.QueryRow("SELECT COUNT(*) FROM " + quoteSQLiteIdentifier(table)).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Errorf("%s count = %d, want 0", table, count)
		}
	}
}
