package db

import (
	"AstraScheduleServerGo/model"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	gormsqlite "github.com/libtnb/sqlite"
	"github.com/sirupsen/logrus"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var (
	dbOnce sync.Once
	dbInst *gorm.DB
	dbErr  error
)

func ConnectDb() *gorm.DB {
	dbOnce.Do(func() {
		dbType := strings.ToLower(strings.TrimSpace(model.Configs.Db.Type))
		if dbType == "" {
			dbType = "mysql"
		}

		var dialector gorm.Dialector
		switch dbType {
		case "sqlite":
			dir := filepath.Dir(model.Configs.Db.Path)
			if dir != "" && dir != "." {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					dbErr = fmt.Errorf("failed to create sqlite directory: %w", err)
					return
				}
			}
			logrus.Infof("Connecting to SQLite database: %s", model.Configs.Db.Path)
			dialector = gormsqlite.Open(fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_busy_timeout=5000", model.Configs.Db.Path))
		default:

			cfg := mysql.NewConfig()
			cfg.User = model.Configs.Db.User
			cfg.Passwd = model.Configs.Db.Pass
			cfg.Net = "tcp"
			cfg.Addr = fmt.Sprintf("%s:%d", model.Configs.Db.Host, model.Configs.Db.Port)
			cfg.DBName = model.Configs.Db.Name
			cfg.ParseTime = true
			cfg.Loc = time.Local
			cfg.Params = map[string]string{
				"charset": "utf8mb4",
			}

			dsn := cfg.FormatDSN()

			logrus.Infof("Connecting to MySQL database: %s@%s:%d/%s",
				model.Configs.Db.User,
				model.Configs.Db.Host,
				model.Configs.Db.Port,
				model.Configs.Db.Name)
			dialector = gormmysql.Open(dsn)
		}

		db, err := gorm.Open(dialector, &gorm.Config{})
		if err != nil {
			logrus.Errorf("Failed to connect to database: %v", err)
			dbErr = fmt.Errorf("database connection failed: %w", err)
			return
		}
		// SQLite :memory: 的每个连接都是独立空库；锁为单连接，避免并发请求落到不同的库（测试/内存模式专用）
		if dbType == "sqlite" && strings.HasPrefix(model.Configs.Db.Path, ":memory:") {
			if sqlDB, err := db.DB(); err == nil {
				sqlDB.SetMaxOpenConns(1)
			}
		}
		dbInst = db
		logrus.Info("Database connected successfully")
	})
	return dbInst
}

func GetDB() *gorm.DB {
	db := ConnectDb()
	if dbErr != nil {
		panic(dbErr)
	}
	return db
}
