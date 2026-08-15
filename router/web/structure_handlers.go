package web

import (
	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/model/dbTable"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

const (
	whereSchool          = "school = ?"
	whereSchoolGrade     = "school = ? AND grade = ?"
	whereSchoolGradeClass = "school = ? AND grade = ? AND class = ?"
)

func CreateSchool(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "学校名称不能为空"})
		return
	}

	// 学校是否存在以关联的科目行判断（学校本身没有独立表行，
	// 按 Schedule 行判断会在“学校尚无班级”时漏判，导致重复创建返回 200）
	var count int64
	db.GetDB().Model(&dbTable.Subject{}).Where(whereSchool, req.Name).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"detail": "学校已存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "学校创建成功"})
}

func DeleteSchool(c *gin.Context) {
	school := c.Param("school")
	if school == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "学校名称不能为空"})
		return
	}

	tx := db.GetDB().Begin()
	defer func() { if recover() != nil { tx.Rollback() } }()

	tx.Where(whereSchool, school).Delete(&dbTable.Schedule{})
	tx.Where(whereSchool, school).Delete(&dbTable.ClientConfig{})
	tx.Where(whereSchool, school).Delete(&dbTable.DataVersion{})
	tx.Where(whereSchool, school).Delete(&dbTable.Subject{})
	tx.Where(whereSchool, school).Delete(&dbTable.Timetable{})
	tx.Where(whereSchool, school).Delete(&dbTable.AutorunRecord{})

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "学校已删除"})
}

func CreateGrade(c *gin.Context) {
	school := c.Param("school")
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "年级名称不能为空"})
		return
	}

	// 年级是否存在以默认科目/作息行判断（学校与年级本身没有独立表行，
	// 按 Schedule 行判断会在“年级尚无班级”时漏判，导致重复创建返回 200）
	var count int64
	db.GetDB().Model(&dbTable.Subject{}).Where(whereSchoolGrade, school, req.Name).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"detail": "年级已存在"})
		return
	}

	// 创建默认科目和作息表
	subject := dbTable.Subject{
		School: school,
		Grade:  req.Name,
		SubjectConfig: dbTable.SubjectConfig{
			SubjectName: map[string]string{
				"课": "课程", "自": "自习", "英": "英语", "语": "语文",
				"数": "数学", "物": "物理", "化": "化学", "体": "体育",
				"史": "历史", "政": "政治", "班": "班会",
			},
		},
	}
	db.GetDB().Clauses(clause.OnConflict{DoNothing: true}).Create(&subject)

	timetable := dbTable.Timetable{
		School: school,
		Grade:  req.Name,
		TimetableConfig: dbTable.TimetableConfig{
			Timetable: map[string]map[string]interface{}{
				"常日": {"00:00-00:00": 0, "00:01-23:59": "常日"},
				"没课": {"00:00-00:00": 0, "00:01-23:59": "没课"},
			},
			Divider: map[string][]int{"常日": {}, "没课": {}},
			Start:   time.Now().Format("2006-01-02"),
		},
	}
	db.GetDB().Clauses(clause.OnConflict{DoNothing: true}).Create(&timetable)

	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "年级创建成功"})
}

func DeleteGrade(c *gin.Context) {
	school := c.Param("school")
	grade := c.Param("grade")

	tx := db.GetDB().Begin()
	defer func() { if recover() != nil { tx.Rollback() } }()

	tx.Where(whereSchoolGrade, school, grade).Delete(&dbTable.Schedule{})
	tx.Where(whereSchoolGrade, school, grade).Delete(&dbTable.ClientConfig{})
	tx.Where(whereSchoolGrade, school, grade).Delete(&dbTable.DataVersion{})
	tx.Where(whereSchoolGrade, school, grade).Delete(&dbTable.Subject{})
	tx.Where(whereSchoolGrade, school, grade).Delete(&dbTable.Timetable{})
	tx.Where(whereSchoolGrade, school, grade).Delete(&dbTable.AutorunRecord{})

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "年级已删除"})
}

func CreateClass(c *gin.Context) {
	school := c.Param("school")
	grade := c.Param("grade")
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "班级名称不能为空"})
		return
	}

	var count int64
	db.GetDB().Model(&dbTable.Schedule{}).Where(whereSchoolGradeClass, school, grade, req.Name).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"detail": "班级已存在"})
		return
	}

	schedule := dbTable.Schedule{
		School: school,
		Grade:  grade,
		Class:  req.Name,
		DailyClasses: [7]dbTable.DailyClass{
			{Chinese: "日", English: "SUN", Timetable: "没课", ClassList: dbTable.ClassList{[]string{"课"}}},
			{Chinese: "一", English: "MON", Timetable: "常日", ClassList: dbTable.ClassList{[]string{"课"}, []string{"课"}}},
			{Chinese: "二", English: "TUE", Timetable: "常日", ClassList: dbTable.ClassList{[]string{"课"}, []string{"课"}}},
			{Chinese: "三", English: "WED", Timetable: "常日", ClassList: dbTable.ClassList{[]string{"课"}, []string{"课"}}},
			{Chinese: "四", English: "THR", Timetable: "常日", ClassList: dbTable.ClassList{[]string{"课"}, []string{"课"}}},
			{Chinese: "五", English: "FRI", Timetable: "常日", ClassList: dbTable.ClassList{[]string{"课"}, []string{"课"}}},
			{Chinese: "六", English: "SAT", Timetable: "没课", ClassList: dbTable.ClassList{[]string{"课"}}},
		},
	}
	db.GetDB().Clauses(clause.OnConflict{DoNothing: true}).Create(&schedule)

	// 创建默认客户端配置（含 CSS 变量）
	clientConfig := dbTable.ClientConfig{
		School: school,
		Grade:  grade,
		Class:  req.Name,
		ClientConfigItems: dbTable.ClientConfigItems{
			CountdownTarget:      "hidden",
			WeatherAlertOverride: true,
			WeatherAlertBrief:    true,
			WeekDisplay:          true,
			BannerText:           "",
			CSSStyle: map[string]string{
				"--center-font-size":       "30px",
				"--corner-font-size":       "14px",
				"--countdown-font-size":    "28px",
				"--global-border-radius":   "16px",
				"--global-bg-opacity":      "0.3",
				"--container-bg-padding":   "8px 14px",
				"--countdown-bg-padding":   "5px 12px",
				"--container-space":        "16px",
				"--top-space":              "16px",
				"--main-horizontal-space":  "8px",
				"--divider-width":          "2px",
				"--divider-margin":         "6px",
				"--triangle-size":          "16px",
				"--sub-font-size":          "20px",
				"--banner-height":          "30px",
			},
			TemperatureColors: dbTable.TemperatureColorsConfig{
				UseGradient: false,
				Stops: []dbTable.TemperatureStop{
					{Temp: 20, Color: "#66CCFF"},
					{Temp: 30, Color: "#5FBC21"},
					{Temp: 36, Color: "#FF8C00"},
					{Temp: 100, Color: "#EE0000"},
				},
			},
			StartupBehavior: "normal",
		},
	}
	db.GetDB().Clauses(clause.OnConflict{DoNothing: true}).Create(&clientConfig)

	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "班级创建成功"})
}

func DeleteClass(c *gin.Context) {
	school := c.Param("school")
	grade := c.Param("grade")
	classNumber := c.Param("class_number")

	tx := db.GetDB().Begin()
	defer func() { if recover() != nil { tx.Rollback() } }()

	tx.Where(whereSchoolGradeClass, school, grade, classNumber).Delete(&dbTable.Schedule{})
	tx.Where(whereSchoolGradeClass, school, grade, classNumber).Delete(&dbTable.ClientConfig{})
	tx.Where(whereSchoolGradeClass, school, grade, classNumber).Delete(&dbTable.DataVersion{})

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "班级已删除"})
}
