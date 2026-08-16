package client

import (
	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/middleware"
	"AstraScheduleServerGo/model"
	"AstraScheduleServerGo/model/dbTable"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm/clause"
)

func PutSchedule(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	school := c.Param("school")
	grade := c.Param("grade")
	class := c.Param("class")
	fullCC := &model.FullClientConfig{}
	err := c.ShouldBind(fullCC)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()}) // 400
		return
	}
	logrus.Infof("收到如下请求：%+v", fullCC)
	cC := dbTable.ClientConfig{
		Namespace:         ns,
		School:            school,
		Grade:             grade,
		Class:             class,
		ClientConfigItems: fullCC.ClientConfigItems,
	}
	schedule := dbTable.Schedule{
		Namespace:    ns,
		School:       school,
		Grade:        grade,
		Class:        class,
		DailyClasses: fullCC.DailyClasses,
	}
	subject := dbTable.Subject{
		Namespace:     ns,
		School:        school,
		Grade:         grade,
		SubjectConfig: fullCC.SubjectConfig,
	}
	timetable := dbTable.Timetable{
		Namespace:       ns,
		School:          school,
		Grade:           grade,
		TimetableConfig: fullCC.TimetableConfig,
	}
	dataVersion := dbTable.DataVersion{
		Namespace: ns,
		School:    school,
		Grade:     grade,
		Class:     class,
		Version:   time.Now(),
	}
	tx := db.GetDB().Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "事务开启失败：" + tx.Error.Error()}) // 500
		return
	}
	defer func() {
		if recover() != nil {
			tx.Rollback()
		}
	}()

	tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "namespace"}, {Name: "school"}, {Name: "grade"}, {Name: "class"}},
		UpdateAll: true,
	}).Create(&cC)
	if tx.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存客户端配置失败：" + tx.Error.Error()})
		return
	}
	tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "namespace"}, {Name: "school"}, {Name: "grade"}, {Name: "class"}},
		UpdateAll: true,
	}).Create(&schedule)
	if tx.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存课表失败：" + tx.Error.Error()})
		return
	}
	tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "namespace"}, {Name: "school"}, {Name: "grade"}, {Name: "class"}},
		UpdateAll: true,
	}).Create(&dataVersion)
	if tx.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存版本号失败：" + tx.Error.Error()})
		return
	}
	tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "namespace"}, {Name: "school"}, {Name: "grade"}},
		UpdateAll: true,
	}).Create(&subject)
	if tx.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存科目配置失败：" + tx.Error.Error()})
		return
	}
	tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "namespace"}, {Name: "school"}, {Name: "grade"}},
		UpdateAll: true,
	}).Create(&timetable)
	if tx.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存作息配置失败：" + tx.Error.Error()})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "事务提交失败：" + err.Error()}) // 500
		return
	}

	// 数据更新后内部广播，通知当前租户同年级在线客户端刷新（serverless 模式下自动跳过）
	BroadcastSync(requestNamespace(c), school, grade)

	c.JSON(http.StatusOK, gin.H{ // 200
		"message": "课表更新成功",
		"version": strconv.FormatInt(dataVersion.Version.Unix(), 10),
	})
}
