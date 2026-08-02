package web

import (
	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/middleware"
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
	whereNamespace   = "namespace = ?"
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
	ns := middleware.GetNamespace(c)
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

// listSchools 列出学校（安全修复：namespace 为空时返回空，避免跨租户枚举）
func listSchools(namespace string) ([]string, error) {
	type row struct{ School string }
	rows := make([]row, 0)
	if namespace == "" {
		return []string{}, nil
	}
	err := db.GetDB().Model(&dbTable.Schedule{}).Where(whereNamespace, namespace).Distinct("school").Find(&rows).Error
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

// listGrades 列出年级（安全修复：namespace 为空时返回空，避免跨租户枚举）
func listGrades(namespace, school string) ([]string, error) {
	type row struct{ Grade string }
	rows := make([]row, 0)
	if namespace == "" {
		return []string{}, nil
	}
	err := db.GetDB().Model(&dbTable.Schedule{}).Where(whereNamespace, namespace).Where("school = ?", school).Distinct("grade").Find(&rows).Error
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

// listClasses 列出班级（安全修复：namespace 为空时返回空，避免跨租户枚举）
func listClasses(namespace, school, grade string) ([]string, error) {
	type row struct{ Class string }
	rows := make([]row, 0)
	if namespace == "" {
		return []string{}, nil
	}
	err := db.GetDB().Model(&dbTable.Schedule{}).Where(whereNamespace, namespace).Where("school = ? AND grade = ?", school, grade).Distinct("class").Find(&rows).Error
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
	ns := middleware.GetNamespace(c)
	menu := gin.H{"data": []gin.H{
		{"to": "/", "text": "总览", "key": "go-back-home", "children": nil},
		{"to": "/users", "text": "用户管理", "key": "users", "children": nil},
		{"to": "/structure", "text": "结构管理", "key": "structure", "children": nil},
		{"to": "/autorun", "text": "自动任务", "key": "autorun", "children": nil},
		{"to": "/countdown", "text": "倒数日", "key": "countdown", "children": nil},
		{"to": "/tools", "text": "实用工具", "key": "tools", "children": nil},
	}}
	data := menu["data"].([]gin.H)
	schools, err := listSchools(ns)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, school := range schools {
		grades, _ := listGrades(ns, school)
		gradeChildren := make([]gin.H, 0)
		for _, grade := range grades {
			classes, _ := listClasses(ns, school, grade)
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
	ns := middleware.GetNamespace(c)
	schools, err := listSchools(ns)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]gin.H, 0)
	for _, school := range schools {
		grades, _ := listGrades(ns, school)
		gradeNodes := make([]gin.H, 0)
		for _, grade := range grades {
			classes, _ := listClasses(ns, school, grade)
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
