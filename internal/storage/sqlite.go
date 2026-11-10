package storage

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/beego/beego/v2/client/orm"
	_ "modernc.org/sqlite"
)

// InitSQLite 使用同机共享文件；每个新连接都设置 WAL 和锁等待。
// InitSchema 仍由 main 在此函数之后调用。
func InitSQLite(dbPath string) error {
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return err
	}
	dsn := url.URL{Scheme: "file", Path: filepath.ToSlash(absPath)}
	query := dsn.Query()
	query.Add("_pragma", "busy_timeout(3000)")
	query.Add("_pragma", "journal_mode(WAL)")
	query.Add("_pragma", "foreign_keys(ON)")
	dsn.RawQuery = query.Encode()
	if err := orm.RegisterDriver("sqlite", orm.DRSqlite); err != nil {
		return err
	}
	if err := orm.RegisterDataBase("default", "sqlite", dsn.String()); err != nil {
		return fmt.Errorf("注册 SQLite: %w", err)
	}
	db, err := orm.GetDB("default")
	if err != nil {
		return err
	}
	// 单进程内限制连接数量，跨进程写入仍由 SQLite 文件锁串行化。
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		return fmt.Errorf("连接 SQLite: %w", err)
	}
	return nil
}
