package testutil

import (
	"AstraScheduleServerGo/model"
	"AstraScheduleServerGo/model/dbTable"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	gormsqlite "github.com/libtnb/sqlite"
)

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
