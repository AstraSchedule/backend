package web

import (
	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/model"
	"AstraScheduleServerGo/model/dbTable"
	"AstraScheduleServerGo/router/client"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
)

const (
	configPathPrefix = "/config/"
	keySchoolPrefix  = "school-"
	keyGradeInfix    = "-grade-"
	keyClassInfix    = "-class-"
)

func schoolKey(school string) string {
	return keySchoolPrefix + school
}

func gradeKey(school, grade string) string {
	return schoolKey(school) + keyGradeInfix + grade
}

func classKey(school, grade, classNumber string) string {
	return gradeKey(school, grade) + keyClassInfix + classNumber
}

func configBasePath(school, grade string) string {
	return configPathPrefix + school + "/" + grade
}

func GetStatistic(c *gin.Context) {
	ns := "default" // main 分支无多租户概念；saas/main 分支改为 middleware.GetNamespace(c)
	weatherErr, disconnects := client.StatSnapshot(ns)
	totalDisconnect := 0
	for _, v := range disconnects {
		totalDisconnect += v
	}
	clients := client.StatOnlineClasses(ns)
	c.JSON(http.StatusOK, gin.H{
		"weather_error":              weatherErr,
		"websocket_disconnect":       disconnects,
		"websocket_disconnect_count": totalDisconnect,
		"clients":                    clients,
		"clients_count":              len(clients),
		"serverless":                 model.Configs.Run.Serverless,
	})
}

func listSchools() ([]string, error) {
	type row struct{ School string }
	rows := make([]row, 0)
	err := db.GetDB().Model(&dbTable.Schedule{}).Distinct("school").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.School)
	}
	sort.Strings(out)
	return out, nil
}

func listGrades(school string) ([]string, error) {
	type row struct{ Grade string }
	rows := make([]row, 0)
	err := db.GetDB().Model(&dbTable.Schedule{}).Where("school = ?", school).Distinct("grade").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Grade)
	}
	sort.Strings(out)
	return out, nil
}

func listClasses(school, grade string) ([]string, error) {
	type row struct{ Class string }
	rows := make([]row, 0)
	err := db.GetDB().Model(&dbTable.Schedule{}).Where("school = ? AND grade = ?", school, grade).Distinct("class").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Class)
	}
	sort.Strings(out)
	return out, nil
}

func GetMenu(c *gin.Context) {
	menu := gin.H{"data": []gin.H{{"to": "/", "text": "总览", "key": "go-back-home", "children": nil}, {"to": "/autorun", "text": "自动任务", "key": "autorun", "children": nil}, {"to": "/countdown", "text": "倒数日", "key": "countdown", "children": nil}, {"to": "/tools", "text": "实用工具", "key": "tools", "children": nil}}}
	data := menu["data"].([]gin.H)
	schools, err := listSchools()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, school := range schools {
		grades, _ := listGrades(school)
		gradeChildren := make([]gin.H, 0)
		for _, grade := range grades {
			classes, _ := listClasses(school, grade)
			gradeNodeKey := gradeKey(school, grade)
			classConfigBasePath := configBasePath(school, grade)
			children := []gin.H{
				{"to": classConfigBasePath + "/subjects", "text": "课程设置", "key": gradeNodeKey + "-subjects", "children": nil},
				{"to": classConfigBasePath + "/timetable", "text": "作息设置", "key": gradeNodeKey + "-timetable", "children": nil},
			}
			for _, classNumber := range classes {
				classNodeKey := classKey(school, grade, classNumber)
				children = append(children, gin.H{
					"text": classNumber + " 班",
					"key":  classNodeKey,
					"raw":  classNumber,
					"children": []gin.H{
						{"to": classConfigBasePath + "/" + classNumber + "/schedule", "text": "课表设置", "key": classNodeKey + "-schedule", "children": nil},
						{"to": classConfigBasePath + "/" + classNumber + "/settings", "text": "通用设置", "key": classNodeKey + "-settings", "children": nil},
					},
				})
			}
			gradeChildren = append(gradeChildren, gin.H{"text": grade + " 级", "key": gradeNodeKey, "raw": grade, "children": children})
		}
		data = append(data, gin.H{"text": school + " 学校", "key": schoolKey(school), "raw": school, "children": gradeChildren})
	}
	menu["data"] = data
	c.JSON(http.StatusOK, menu)
}

func GetStructure(c *gin.Context) {
	schools, err := listSchools()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]gin.H, 0)
	for _, school := range schools {
		grades, _ := listGrades(school)
		gradeNodes := make([]gin.H, 0)
		for _, grade := range grades {
			classes, _ := listClasses(school, grade)
			classNodes := make([]gin.H, 0)
			for _, classNumber := range classes {
				classNodes = append(classNodes, gin.H{"text": classNumber, "children": nil})
			}
			gradeNodes = append(gradeNodes, gin.H{"text": grade, "children": classNodes})
		}
		out = append(out, gin.H{"text": school, "children": gradeNodes})
	}
	c.JSON(http.StatusOK, out)
}
