package web

import (
	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/model/dbTable"
	"AstraScheduleServerGo/router/client"
	"AstraScheduleServerGo/service"
	"errors"
	"fmt"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func syncTimetableDividerKeys(cfg *dbTable.TimetableConfig) {
	if cfg.Divider == nil {
		cfg.Divider = map[string][]int{}
	}
	for name := range cfg.Timetable {
		if _, ok := cfg.Divider[name]; !ok {
			cfg.Divider[name] = []int{}
		}
	}
	for name := range cfg.Divider {
		if _, ok := cfg.Timetable[name]; !ok {
			delete(cfg.Divider, name)
		}
	}
}

func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func cloneTimetableMap(src map[string]map[string]interface{}) map[string]map[string]interface{} {
	if src == nil {
		return map[string]map[string]interface{}{}
	}
	out := make(map[string]map[string]interface{}, len(src))
	for name, seg := range src {
		segCopy := make(map[string]interface{}, len(seg))
		for k, v := range seg {
			segCopy[k] = v
		}
		out[name] = segCopy
	}
	return out
}

func cloneDividerMap(src map[string][]int) map[string][]int {
	if src == nil {
		return map[string][]int{}
	}
	out := make(map[string][]int, len(src))
	for name, arr := range src {
		arrCopy := make([]int, len(arr))
		copy(arrCopy, arr)
		out[name] = arrCopy
	}
	return out
}

func cloneDailyClasses(src [7]dbTable.DailyClass) [7]dbTable.DailyClass {
	var out [7]dbTable.DailyClass
	for i := 0; i < 7; i++ {
		out[i] = dbTable.DailyClass{
			Chinese:   src[i].Chinese,
			English:   src[i].English,
			Timetable: src[i].Timetable,
		}
		if src[i].ClassList != nil {
			out[i].ClassList = make([][]string, len(src[i].ClassList))
			copy(out[i].ClassList, src[i].ClassList)
		} else {
			out[i].ClassList = [][]string{}
		}
	}
	return out
}

func GetSubjectsOptions(c *gin.Context) {
	school := c.Param("school")
	grade := c.Param("grade")
	subject := db.GetSubject(school, grade)
	options := make([]gin.H, 0)
	for abbr, full := range subject.SubjectName {
		options = append(options, gin.H{"label": abbr + "（" + full + "）", "value": abbr})
	}
	c.JSON(http.StatusOK, gin.H{"options": options})
}

func GetSubjects(c *gin.Context) {
	school := c.Param("school")
	grade := c.Param("grade")
	subject := db.GetSubject(school, grade)
	abbr := make([]gin.H, 0)
	fullName := make([]gin.H, 0)
	for k, v := range subject.SubjectName {
		abbr = append(abbr, gin.H{"text": k})
		fullName = append(fullName, gin.H{"text": v})
	}
	c.JSON(http.StatusOK, gin.H{"abbr": abbr, "fullName": fullName})
}

func PutSubjects(c *gin.Context) {
	school := c.Param("school")
	grade := c.Param("grade")
	var raw map[string]interface{}
	if err := c.ShouldBindJSON(&raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	bodyMap := raw
	if modelVal, ok := raw["model"].(map[string]interface{}); ok {
		bodyMap = modelVal
	}
	body := subjectsPayload{}
	if arr, ok := bodyMap["abbr"].([]interface{}); ok {
		for _, item := range arr {
			if obj, ok := item.(map[string]interface{}); ok {
				if text, ok := obj["text"].(string); ok {
					body.Abbr = append(body.Abbr, textItem{Text: text})
				}
			}
		}
	}
	if arr, ok := bodyMap["fullName"].([]interface{}); ok {
		for _, item := range arr {
			if obj, ok := item.(map[string]interface{}); ok {
				if text, ok := obj["text"].(string); ok {
					body.FullName = append(body.FullName, textItem{Text: text})
				}
			}
		}
	}
	m := map[string]string{}
	limit := len(body.Abbr)
	if len(body.FullName) < limit {
		limit = len(body.FullName)
	}
	for i := 0; i < limit; i++ {
		m[body.Abbr[i].Text] = body.FullName[i].Text
	}
	record := dbTable.Subject{School: school, Grade: grade, SubjectConfig: dbTable.SubjectConfig{SubjectName: m}}
	if err := db.GetDB().Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "school"}, {Name: "grade"}}, UpdateAll: true}).Create(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func GetTimetableOptions(c *gin.Context) {
	school := c.Param("school")
	grade := c.Param("grade")
	timetable := db.GetTimetable(school, grade)
	options := make([]gin.H, 0)
	keys := make([]string, 0, len(timetable.Timetable))
	for name := range timetable.Timetable {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		for i, k := range keys {
			if k == "常日" {
				keys = append([]string{"常日"}, append(keys[:i], keys[i+1:]...)...)
				break
			}
		}
	}

	for _, name := range keys {
		config := timetable.Timetable[name]
		need := 0
		for _, v := range config {
			i, ok := serviceAsInt(v)
			if !ok {
				continue
			}
			if i > need {
				need = i
			}
		}
		options = append(options, gin.H{"label": name, "value": name, "need": need})
	}
	c.JSON(http.StatusOK, gin.H{"options": options})
}

func GetTimetable(c *gin.Context) {
	school := c.Param("school")
	grade := c.Param("grade")
	timetable := db.GetTimetable(school, grade)
	c.JSON(http.StatusOK, timetable.TimetableConfig)
}

func PutTimetable(c *gin.Context) {
	school := c.Param("school")
	grade := c.Param("grade")
	var body dbTable.TimetableConfig
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(body.Timetable) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "timetable 不能为空"})
		return
	}
	if _, ok := body.Timetable["常日"]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不允许删除“常日”作息表，且必须包含“常日”"})
		return
	}
	syncTimetableDividerKeys(&body)
	record := dbTable.Timetable{School: school, Grade: grade, TimetableConfig: body}
	if err := db.GetDB().Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "school"}, {Name: "grade"}}, UpdateAll: true}).Create(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func CopyConfig(c *gin.Context) {
	var payload copyConfigPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效参数: " + err.Error()})
		return
	}

	fromClass := payload.From.ClassValue()
	toClass := payload.To.ClassValue()
	if payload.From.School == "" || payload.From.Grade == "" || fromClass == "" ||
		payload.To.School == "" || payload.To.Grade == "" || toClass == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "from/to 的 school、grade、class 均不能为空"})
		return
	}
	if payload.From.School == payload.To.School && payload.From.Grade == payload.To.Grade && fromClass == toClass {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "来源与目标完全一致，无需复制"})
		return
	}

	dbConn := db.GetDB()

	var srcSubject dbTable.Subject
	if err := dbConn.Where("school = ? AND grade = ?", payload.From.School, payload.From.Grade).Take(&srcSubject).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"detail": "未找到来源科目配置"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var srcTimetable dbTable.Timetable
	if err := dbConn.Where("school = ? AND grade = ?", payload.From.School, payload.From.Grade).Take(&srcTimetable).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"detail": "未找到来源作息配置"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var srcSchedule dbTable.Schedule
	if err := dbConn.Where("school = ? AND grade = ? AND class = ?", payload.From.School, payload.From.Grade, fromClass).Take(&srcSchedule).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"detail": "未找到来源课程表配置"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var srcSettings dbTable.ClientConfig
	if err := dbConn.Where("school = ? AND grade = ? AND class = ?", payload.From.School, payload.From.Grade, fromClass).Take(&srcSettings).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"detail": "未找到来源通用设置配置"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	targetSubject := dbTable.Subject{
		School: payload.To.School,
		Grade:  payload.To.Grade,
		SubjectConfig: dbTable.SubjectConfig{
			SubjectName: cloneStringMap(srcSubject.SubjectName),
		},
	}

	targetTimetable := dbTable.Timetable{
		School: payload.To.School,
		Grade:  payload.To.Grade,
		TimetableConfig: dbTable.TimetableConfig{
			Start:     srcTimetable.Start,
			Timetable: cloneTimetableMap(srcTimetable.Timetable),
			Divider:   cloneDividerMap(srcTimetable.Divider),
		},
	}
	syncTimetableDividerKeys(&targetTimetable.TimetableConfig)

	targetSchedule := dbTable.Schedule{
		School:       payload.To.School,
		Grade:        payload.To.Grade,
		Class:        toClass,
		DailyClasses: cloneDailyClasses(srcSchedule.DailyClasses),
	}

	targetSettings := dbTable.ClientConfig{
		School: payload.To.School,
		Grade:  payload.To.Grade,
		Class:  toClass,
		ClientConfigItems: dbTable.ClientConfigItems{
			CountdownTarget:      srcSettings.CountdownTarget,
			WeatherAlertOverride: srcSettings.WeatherAlertOverride,
			WeatherAlertBrief:    srcSettings.WeatherAlertBrief,
			WeekDisplay:          srcSettings.WeekDisplay,
			BannerText:           srcSettings.BannerText,
			CSSStyle:             cloneStringMap(srcSettings.CSSStyle),
			TemperatureColors:    srcSettings.TemperatureColors,
			StartupBehavior:      srcSettings.StartupBehavior,
		},
	}

	tx := dbConn.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": tx.Error.Error()})
		return
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "school"}, {Name: "grade"}}, UpdateAll: true}).Create(&targetSubject).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "school"}, {Name: "grade"}}, UpdateAll: true}).Create(&targetTimetable).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "school"}, {Name: "grade"}, {Name: "class"}}, UpdateAll: true}).Create(&targetSchedule).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "school"}, {Name: "grade"}, {Name: "class"}}, UpdateAll: true}).Create(&targetSettings).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	client.BroadcastSyncConfig(c)
	c.JSON(http.StatusOK, gin.H{
		"status": 200,
		"from": gin.H{
			"school": payload.From.School,
			"grade":  payload.From.Grade,
			"class":  fromClass,
		},
		"to": gin.H{
			"school": payload.To.School,
			"grade":  payload.To.Grade,
			"class":  toClass,
		},
	})
}

func GetScheduleConfig(c *gin.Context) {
	school := c.Param("school")
	grade := c.Param("grade")
	classNumber := c.Param("class_number")
	schedule := db.GetSchedule(school, grade, classNumber)
	timetable := db.GetTimetable(school, grade)
	maxSubjects := 0
	for _, v := range timetable.Timetable {
		for _, item := range v {
			i, ok := serviceAsInt(item)
			if ok && i+1 > maxSubjects {
				maxSubjects = i + 1
			}
		}
	}
	out := make([]gin.H, 0, 7)
	for i := 0; i < 7; i++ {
		day := schedule.DailyClasses[i]
		if _, ok := timetable.Timetable[day.Timetable]; !ok {
			day.Timetable = "常日"
		}
		classList := make([][]string, 0, len(day.ClassList))
		for _, s := range day.ClassList {
			// 直接使用数据库格式 [["物"], ["数"]] 或 [["物", "化"], ["数"]]
			classList = append(classList, s)
		}
		for len(classList) < maxSubjects {
			classList = append(classList, []string{"课"})
		}
		out = append(out, gin.H{
			"Chinese":   day.Chinese,
			"English":   day.English,
			"classList": classList,
			"timetable": day.Timetable,
		})
	}
	c.JSON(http.StatusOK, gin.H{"daily_class": out})
}

func PutScheduleConfig(c *gin.Context) {
	school := c.Param("school")
	grade := c.Param("grade")
	classNumber := c.Param("class_number")

	var raw map[string]interface{}
	if err := c.ShouldBindJSON(&raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	bodyMap := raw
	if modelVal, ok := raw["model"].(map[string]interface{}); ok {
		bodyMap = modelVal
	}
	// 契约校验：请求体必须携带完整的 7 天 daily_class 数组，
	// 否则会导致该班课表被静默清空（防坏请求覆盖既有数据）。
	dailyClassRaw, ok := bodyMap["daily_class"].([]interface{})
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "daily_class 必须为数组"})
		return
	}
	if len(dailyClassRaw) != 7 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "daily_class 必须包含 7 天（日一二三四五六）"})
		return
	}
	body := schedulePayload{}
	for index, one := range dailyClassRaw {
		obj, ok := one.(map[string]interface{})
		if !ok {
			// 契约校验：非对象条目会导致对应日期写入零值课表（部分清空），必须拒绝
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("daily_class[%d] 必须为对象", index)})
			return
		}
		item := dailyClassInput{}
		item.Chinese, _ = obj["Chinese"].(string)
		item.English, _ = obj["English"].(string)
		item.Timetable, _ = obj["timetable"].(string)
		if classListRaw, ok := obj["classList"].([]interface{}); ok {
			for _, classItem := range classListRaw {
				if arr2, ok := classItem.([]interface{}); ok {
					line := make([]string, 0, len(arr2))
					for _, x := range arr2 {
						if s, ok := x.(string); ok {
							line = append(line, s)
						}
					}
					item.ClassList = append(item.ClassList, line)
				}
			}
		}
		body.DailyClass = append(body.DailyClass, item)
	}

	var daily [7]dbTable.DailyClass
	for i := 0; i < 7 && i < len(body.DailyClass); i++ {
		daily[i] = dbTable.DailyClass{
			Chinese:   body.DailyClass[i].Chinese,
			English:   body.DailyClass[i].English,
			ClassList: parseClassList(body.DailyClass[i].ClassList),
			Timetable: body.DailyClass[i].Timetable,
		}
	}
	timetable := db.GetTimetable(school, grade)
	service.FixWrongTimetable(&daily, timetable.Timetable)

	record := dbTable.Schedule{School: school, Grade: grade, Class: classNumber, DailyClasses: daily}
	if err := db.GetDB().Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "school"}, {Name: "grade"}, {Name: "class"}}, UpdateAll: true}).Create(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func GetSettings(c *gin.Context) {
	school := c.Param("school")
	grade := c.Param("grade")
	classNumber := c.Param("class_number")
	config := db.GetClientConfig(school, grade, classNumber)
	c.JSON(http.StatusOK, config.ClientConfigItems)
}

func PutSettings(c *gin.Context) {
	school := c.Param("school")
	grade := c.Param("grade")
	classNumber := c.Param("class_number")
	var body dbTable.ClientConfigItems
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	record := dbTable.ClientConfig{School: school, Grade: grade, Class: classNumber, ClientConfigItems: body}
	if err := db.GetDB().Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "school"}, {Name: "grade"}, {Name: "class"}}, UpdateAll: true}).Create(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	client.BroadcastSyncConfig(c)
	c.JSON(http.StatusOK, gin.H{"status": 200})
}
