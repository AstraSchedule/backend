package main

import (
	"AstraScheduleServerGo/middleware"
	"AstraScheduleServerGo/model"
	"AstraScheduleServerGo/router/client"
	"AstraScheduleServerGo/router/web"
	"AstraScheduleServerGo/startup"
	"fmt"
	"time"

	"github.com/gin-contrib/cache"
	"github.com/gin-contrib/cache/persistence"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func main() {
	startup.StartInit()

	logrus.Infof("程序初始化流程结束，即将启动 HTTP 服务：%+v", model.Configs)

	router := buildRouter()

	err := router.Run(fmt.Sprintf("%s:%d", model.Configs.Server.Host, model.Configs.Server.Port))
	if err != nil {
		logrus.Fatal(err.Error())
		return
	}
}

// buildRouter 组装完整的路由与中间件链，供 main 启动与契约回归测试共用。
// 任何对路由表、路径或认证中间件的误改都应由 main_test.go 的契约测试捕获。
func buildRouter() *gin.Engine {
	router := gin.Default()
	router.Use(cors.New(cors.Config{
		AllowOrigins:     model.Configs.Server.Domain,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type", "Accept", "Origin", "X-Requested-With", "X-Verify-Password"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	weatherCacheStore := persistence.NewInMemoryStore(10 * time.Minute)

	// 认证接口（无需 JWT）
	router.POST("/web/auth/login", web.Login)

	// JWT 认证路由组
	jwtAuth := router.Group("/", middleware.JWTAuthMiddleware())

	// 用户信息与改密（需 JWT）
	jwtAuth.GET("/web/auth/me", web.GetMe)
	jwtAuth.GET("/web/statistic", web.GetStatistic)
	jwtAuth.POST("/web/auth/change-password", web.ChangePassword)
	jwtAuth.POST("/web/auth/verify-password", web.VerifyPassword)

	// 用户管理（需 JWT + admin 角色）
	adminGroup := jwtAuth.Group("/", middleware.RequireRole("admin"))
	adminGroup.GET("/web/users", web.ListUsers)
	adminGroup.POST("/web/users", web.CreateUser)
	adminGroup.PUT("/web/users/:id", web.UpdateUser)
	adminGroup.DELETE("/web/users/:id", web.DeleteUser)

	// 管理员或密码验证可操作的写接口
	secureWrite := router.Group("/", middleware.AdminOrToken())

	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Hello World",
		})
	})

	// 完整更新课表（兼容 BasicAuth 客户端）
	secureWrite.PUT("/:school/:grade/:class", client.PutSchedule)
	// 获取完整课表
	router.GET("/:school/:grade/:class", client.GetSchedule)
	// 通过省份和城市查询天气
	router.GET("/api/weather/:name1/:name2", cache.CachePage(weatherCacheStore, 10*time.Minute, client.GetWeatherWithProvince))
	// 通过省份和城市查询天气
	router.GET("/api/weather/:name1", cache.CachePage(weatherCacheStore, 10*time.Minute, client.GetWeatherWithCity))
	// 通过 CF 头查询天气
	router.GET("/api/weather/", client.GetWeatherWithCFHeader)
	// WebSocket
	router.Any("/ws/:school/:grade/:class_number", client.WebSocketPlaceholder)
	// 广播
	secureWrite.POST("/api/broadcast/:school/:grade/:class_number", client.BroadcastSyncConfig)

	// 菜单/结构（读接口，与既有模式一致）；statistic 需 JWT 认证（防跨租户泄露）
	router.GET("/web/menu", web.GetMenu)
	router.GET("/web/structure", web.GetStructure)
	secureWrite.GET("/web/backup/export", web.ExportBackup)
	secureWrite.POST("/web/backup/import", web.ImportBackup)
	// 完整备份导出/导入（支持 overwrite/skip 模式）
	secureWrite.POST("/web/backup/full-export", web.FullExportBackup)
	secureWrite.POST("/web/backup/full-import", web.FullImportBackup)

	// 学校/年级/班级管理
	secureWrite.POST("/web/schools", web.CreateSchool)
	secureWrite.DELETE("/web/schools/:school", web.DeleteSchool)
	secureWrite.POST("/web/schools/:school/grades", web.CreateGrade)
	secureWrite.DELETE("/web/schools/:school/grades/:grade", web.DeleteGrade)
	secureWrite.POST("/web/schools/:school/grades/:grade/classes", web.CreateClass)
	secureWrite.DELETE("/web/schools/:school/grades/:grade/classes/:class_number", web.DeleteClass)

	// 配置接口
	router.GET("/web/config/:school/:grade/subjects/options", web.GetSubjectsOptions)
	router.GET("/web/config/:school/:grade/subjects", web.GetSubjects)
	secureWrite.PUT("/web/config/:school/:grade/subjects", web.PutSubjects)

	router.GET("/web/config/:school/:grade/timetable/options", web.GetTimetableOptions)
	router.GET("/web/config/:school/:grade/timetable", web.GetTimetable)
	secureWrite.PUT("/web/config/:school/:grade/timetable", web.PutTimetable)

	router.GET("/web/config/:school/:grade/:class_number/schedule", web.GetScheduleConfig)
	secureWrite.PUT("/web/config/:school/:grade/:class_number/schedule", web.PutScheduleConfig)

	router.GET("/web/config/:school/:grade/:class_number/settings", web.GetSettings)
	secureWrite.PUT("/web/config/:school/:grade/:class_number/settings", web.PutSettings)
	secureWrite.POST("/web/config/copy", web.CopyConfig)

	// 自动任务
	router.GET("/web/autorun", web.GetAutorunStatus)
	router.GET("/web/autorun/hash/:hashid", web.GetAutorunHashStatus)
	secureWrite.DELETE("/web/autorun/:hashid", web.DeleteAutorunRecord)
	secureWrite.PUT("/web/autorun/compensation", web.PutCompensationRule)
	secureWrite.PUT("/web/autorun/timetable", web.PutTimetableRule)
	secureWrite.PUT("/web/autorun/schedule", web.PutScheduleRule)
	secureWrite.PUT("/web/autorun/all", web.PutAllRule)

	// 倒数日配置
	router.GET("/web/countdown", web.GetCountdownStatus)
	router.GET("/web/countdown/:id", web.GetCountdownByID)
	secureWrite.PUT("/web/countdown", web.PutCountdownRule)
	secureWrite.DELETE("/web/countdown/:id", web.DeleteCountdownRecord)

	// 调休计算
	router.GET("/web/autorun/compensation/holiday/:year/:month/:day", web.CompensationFromHoliday)
	router.GET("/web/autorun/compensation/workday/:year/:month/:day", web.CompensationFromWorkday)
	router.GET("/web/autorun/compensation/year/:year", web.CompensationFromYear)

	// 按日期出课节
	router.GET("/web/schedule/by-date", web.GetScheduleByDate)

	// Admin: DROP table (仅内部调用，需 BasicAuth + password 验证)
	secureWrite.DELETE("/web/admin/drop-table/:table", web.DropAstraTable)

	return router
}
