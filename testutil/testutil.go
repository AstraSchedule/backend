package testutil

import (
	"testing"

	"AstraScheduleServerGo/model"
	"AstraScheduleServerGo/model/dbTable"
	"AstraScheduleServerGo/service"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	gormsqlite "github.com/libtnb/sqlite"
)

// CreateUser 在指定连接上创建指定角色与作用域的测试用户（密码哈希化，先删同名旧用户）。
// 供多个测试包共享，避免重复的用户创建样板被 SonarQube 计入新代码重复率。
// 连接由调用方传入（testutil 不能 import db：db 包自身测试依赖 testutil，会造成导入环）。
func CreateUser(t *testing.T, conn *gorm.DB, username, password, role, scope string) *dbTable.User {
	t.Helper()
	hash, err := service.HashPassword(password)
	require.NoError(t, err)
	require.NoError(t, conn.Where("username = ?", username).Delete(&dbTable.User{}).Error)
	user := &dbTable.User{Username: username, PasswordHash: hash, Role: role, Scope: scope}
	require.NoError(t, conn.Create(user).Error)
	return user
}

// InitTestDB 初始化测试用的模型配置（SQLite 内存库），返回数据库连接
// 注意：业务代码统一通过 db.GetDB() 使用全局单例连接，这里返回的连接仅用于
// 建表等辅助工作，避免在多个测试包中重复粘贴 model.Configs 配置。
func InitTestDB() *gorm.DB {
	model.Configs = model.SrvConfig{
		Server: model.ServerConfig{
			Host:   "127.0.0.1",
			Port:   9000,
			Domain: []string{"http://localhost:5173"},
		},
		Secret: model.SecretConfig{
			Token: "test-token-123",
		},
		Db: model.DbConfig{
			Type: "sqlite",
			Path: ":memory:",
		},
		APIKey: model.APIKeyConfig{
			APIHost: "geoapi.qweather.com",
			Weather: "test-weather-key",
		},
		Log: model.LogConfig{
			Debug: true,
		},
		Run: model.RunConfig{
			Serverless: false,
		},
	}

	db, err := gorm.Open(gormsqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		panic("testutil: failed to open sqlite memory db: " + err.Error())
	}
	// SQLite :memory: 每个连接是独立空库；锁为单连接，保证并发测试共享同一套表与数据
	sqlDB, err := db.DB()
	if err != nil {
		panic("testutil: failed to get underlying sql db: " + err.Error())
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&dbTable.Schedule{},
		&dbTable.ClientConfig{},
		&dbTable.Timetable{},
		&dbTable.Subject{},
		&dbTable.DataVersion{},
		&dbTable.AutorunRecord{},
		&dbTable.CountdownRecord{},
	); err != nil {
		panic("testutil: failed to migrate test db: " + err.Error())
	}
	return db
}
