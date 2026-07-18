package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

// OpenDatabase 封装当前模块的业务处理逻辑。
func OpenDatabase(cfg Config) (*sql.DB, error) {
	switch cfg.Database.Driver {
	case "sqlite":
		dir := filepath.Dir(cfg.Database.SQLitePath)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("create sqlite directory: %w", err)
			}
		}
		db, err := sql.Open("sqlite", cfg.Database.SQLitePath)
		if err != nil {
			return nil, err
		}
		applyDBPoolSettings(db, cfg.Database)
		if err := PingDB(db, cfg.Database.PingTimeoutSec); err != nil {
			_ = db.Close()
			return nil, err
		}
		return db, nil

	case "mysql":
		if cfg.Database.MySQLDSN == "" {
			return nil, errors.New("database.mysql_dsn is required when database.driver=mysql")
		}
		log.Println("connecting to mysql database")
		db, err := sql.Open("mysql", cfg.Database.MySQLDSN)
		if err != nil {
			return nil, err
		}
		applyDBPoolSettings(db, cfg.Database)
		if err := checkMySQLStartupConnection(db, cfg.Database); err != nil {
			_ = db.Close()
			return nil, err
		}
		return db, nil

	default:
		return nil, fmt.Errorf("unsupported database.driver: %s", cfg.Database.Driver)
	}
}

// PingDB 封装当前模块的业务处理逻辑。
func PingDB(db *sql.DB, timeoutSec int) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	return db.PingContext(ctx)
}

// InitSchema 封装当前模块的业务处理逻辑。
func InitSchema(initializer interface{ InitSchema(context.Context) error }) error {
	// Release and GitOps schema migration can be slower on remote MySQL during
	// startup. Old databases can require multiple ALTER TABLE statements, so the
	// startup migration window must be wider than the MySQL advisory-lock wait.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return initializer.InitSchema(ctx)
}

// applyDBPoolSettings 封装当前模块的业务处理逻辑。
func applyDBPoolSettings(db *sql.DB, cfg DatabaseConfig) {
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetimeSec) * time.Second)
	db.SetConnMaxIdleTime(time.Duration(cfg.ConnMaxIdleTimeSec) * time.Second)
}

// checkMySQLStartupConnection 检查业务状态并返回校验结果。
func checkMySQLStartupConnection(db *sql.DB, cfg DatabaseConfig) error {
	var lastErr error
	for attempt := 1; attempt <= cfg.StartupMaxRetries; attempt++ {
		lastErr = PingDB(db, cfg.PingTimeoutSec)
		if lastErr == nil {
			return nil
		}
		if attempt < cfg.StartupMaxRetries {
			time.Sleep(time.Duration(cfg.StartupRetryIntervalSec) * time.Second)
		}
	}

	return fmt.Errorf(
		"mysql startup connection check failed after %d attempts (addr=%s): %w",
		cfg.StartupMaxRetries,
		mysqlAddrForLog(cfg.MySQLDSN),
		lastErr,
	)
}

// mysqlAddrForLog 封装当前模块的业务处理逻辑。
func mysqlAddrForLog(dsn string) string {
	parsed, err := mysqlDriver.ParseDSN(dsn)
	if err != nil || parsed == nil || parsed.Addr == "" {
		return "unknown"
	}
	return parsed.Addr
}
