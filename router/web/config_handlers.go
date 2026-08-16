package web

import (
	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/middleware"
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

// sortedTimetableKeys 返回排序后的作息表名称列表，"常日" 始终排在首位
func sortedTimetableKeys(timetable map[string]map[string]interface{}) []string {
	keys := make([]string, 0, len(timetable))
	for name := range timetable {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for i, k := range keys {
		if k == "常日" {
			keys = append([]string{"常日"}, append(keys[:i], keys[i+1:]...)...)
			break
		}
	}
	return keys
}

func respondCopyError(c *gin.Context, err error, resource string) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"detail": "未找到来源" + resource})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

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
	ns := middleware.GetNamespace(c)
	school := c.Param("school")
	grade := c.Param("grade")
	subject := db.GetSubjectNs(ns, school, grade)
	options := make([]gin.H, 0)
	for abbr, full := range subject.SubjectName {
		options = append(options, gin.H{"label": abbr + "（" + full + "）", "value": abbr})
	}
	c.JSON(http.StatusOK, gin.H{"options": options})
}

func GetSubjects(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	school := c.Param("school")
	grade := c.Param("grade")
	subject := db.GetSubjectNs(ns, school, grade)
	abbr := make([]gin.H, 0)
	fullName := make([]gin.H, 0)
	for k, v := range subject.SubjectName {
		abbr = append(abbr, gin.H{"text": k})
		fullName = append(fullName, gin.H{"text": v})
	}
	c.JSON(http.StatusOK, gin.H{"abbr": abbr, "fullName": fullName})
}

// parseTextItems 从 JSON 数组中提取文本项
func parseTextItems(arr []interface{}) []textItem {
	var items []textItem
	for _, item := range arr {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		text, ok := obj["text"].(string)
		if !ok {
			continue
		}
		items = append(items, textItem{Text: text})
	}
	return items
}

func parseSubjectsPayload(raw map[string]interface{}) subjectsPayload {
	bodyMap := raw
	if modelVal, ok := raw["model"].(map[string]interface{}); ok {
		bodyMap = modelVal
	}

	body := subjectsPayload{}
	if arr, ok := bodyMap["abbr"].([]interface{}); ok {
		body.Abbr = parseTextItems(arr)
	}
	if arr, ok := bodyMap["fullName"].([]interface{}); ok {
		body.FullName = parseTextItems(arr)
	}
	return body
}

func subjectsNameMap(body subjectsPayload) map[string]string {
	m := map[string]string{}
	limit := len(body.Abbr)
	if len(body.FullName) < limit {
		limit = len(body.FullName)
	}
	for i := 0; i < limit; i++ {
		m[body.Abbr[i].Text] = body.FullName[i].Text
	}
	return m
}

func PutSubjects(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	school := c.Param("school")
	grade := c.Param("grade")
	var raw map[string]interface{}
	if err := c.ShouldBindJSON(&raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	m := subjectsNameMap(parseSubjectsPayload(raw))
	record := dbTable.Subject{Namespace: ns, School: school, Grade: grade, SubjectConfig: dbTable.SubjectConfig{SubjectName: m}}
	if err := db.GetDB().Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "namespace"}, {Name: "school"}, {Name: "grade"}}, UpdateAll: true}).Create(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	client.BroadcastSync(ns, school, grade)
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func GetTimetableOptions(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	school := c.Param("school")
	grade := c.Param("grade")
	timetable := db.GetTimetableNs(ns, school, grade)
	options := make([]gin.H, 0)
	keys := sortedTimetableKeys(timetable.Timetable)

	for _, name := range keys {
		config := timetable.Timetable[name]
		need := 0
		for _, v := range config {
			i, ok := serviceAsInt(v)
			if !ok {
				continue
			}
			if i+1 > need {
				need = i + 1
			}
		}
		options = append(options, gin.H{"label": name, "value": name, "need": need})
	}
	c.JSON(http.StatusOK, gin.H{"options": options})
}

func GetTimetable(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	school := c.Param("school")
	grade := c.Param("grade")
	timetable := db.GetTimetableNs(ns, school, grade)
	c.JSON(http.StatusOK, timetable.TimetableConfig)
}

func PutTimetable(c *gin.Context) {
	ns := middleware.GetNamespace(c)
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "不允许删除\"常日\"作息表，且必须包含\"常日\""})
		return
	}
	syncTimetableDividerKeys(&body)
	record := dbTable.Timetable{Namespace: ns, School: school, Grade: grade, TimetableConfig: body}
	if err := db.GetDB().Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "namespace"}, {Name: "school"}, {Name: "grade"}}, UpdateAll: true}).Create(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	client.BroadcastSync(ns, school, grade)
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

// validateCopyPayload 校验复制请求参数，返回 from/to class 和错误
func validateCopyPayload(payload *copyConfigPayload) (fromClass, toClass string, err *handlerError) {
	fromClass = payload.From.ClassValue()
	toClass = payload.To.ClassValue()
	if payload.From.School == "" || payload.From.Grade == "" || fromClass == "" ||
		payload.To.School == "" || payload.To.Grade == "" || toClass == "" {
		return "", "", &handlerError{http.StatusBadRequest, "from/to 的 school、grade、class 均不能为空"}
	}
	if payload.From.School == payload.To.School && payload.From.Grade == payload.To.Grade && fromClass == toClass {
		return "", "", &handlerError{http.StatusBadRequest, "来源与目标完全一致，无需复制"}
	}
	return fromClass, toClass, nil
}

type copySourceConfig struct {
	Subject   dbTable.Subject
	Timetable dbTable.Timetable
	Schedule  dbTable.Schedule
	Settings  dbTable.ClientConfig
}

type copyTargetConfig struct {
	Subject   dbTable.Subject
	Timetable dbTable.Timetable
	Schedule  dbTable.Schedule
	Settings  dbTable.ClientConfig
}

func loadCopySourceConfig(c *gin.Context, dbConn *gorm.DB, ns string, payload copyConfigPayload, fromClass string) (copySourceConfig, bool) {
	var src copySourceConfig
	if err := dbConn.Where("namespace = ? AND school = ? AND grade = ?", ns, payload.From.School, payload.From.Grade).Take(&src.Subject).Error; err != nil {
		respondCopyError(c, err, "科目配置")
		return src, false
	}
	if err := dbConn.Where("namespace = ? AND school = ? AND grade = ?", ns, payload.From.School, payload.From.Grade).Take(&src.Timetable).Error; err != nil {
		respondCopyError(c, err, "作息配置")
		return src, false
	}
	if err := dbConn.Where("namespace = ? AND school = ? AND grade = ? AND class = ?", ns, payload.From.School, payload.From.Grade, fromClass).Take(&src.Schedule).Error; err != nil {
		respondCopyError(c, err, "课程表配置")
		return src, false
	}
	if err := dbConn.Where("namespace = ? AND school = ? AND grade = ? AND class = ?", ns, payload.From.School, payload.From.Grade, fromClass).Take(&src.Settings).Error; err != nil {
		respondCopyError(c, err, "通用设置配置")
		return src, false
	}
	return src, true
}

func buildCopyTargetConfig(ns string, payload copyConfigPayload, toClass string, src copySourceConfig) copyTargetConfig {
	targetTimetable := dbTable.Timetable{
		Namespace: ns,
		School:    payload.To.School,
		Grade:     payload.To.Grade,
		TimetableConfig: dbTable.TimetableConfig{
			Start:     src.Timetable.Start,
			Timetable: cloneTimetableMap(src.Timetable.Timetable),
			Divider:   cloneDividerMap(src.Timetable.Divider),
		},
	}
	syncTimetableDividerKeys(&targetTimetable.TimetableConfig)

	return copyTargetConfig{
		Subject: dbTable.Subject{
			Namespace: ns,
			School:    payload.To.School,
			Grade:     payload.To.Grade,
			SubjectConfig: dbTable.SubjectConfig{
				SubjectName: cloneStringMap(src.Subject.SubjectName),
			},
		},
		Timetable: targetTimetable,
		Schedule: dbTable.Schedule{
			Namespace:    ns,
			School:       payload.To.School,
			Grade:        payload.To.Grade,
			Class:        toClass,
			DailyClasses: cloneDailyClasses(src.Schedule.DailyClasses),
		},
		Settings: dbTable.ClientConfig{
			Namespace: ns,
			School:    payload.To.School,
			Grade:     payload.To.Grade,
			Class:     toClass,
			ClientConfigItems: dbTable.ClientConfigItems{
				CountdownTarget:      src.Settings.CountdownTarget,
				WeatherAlertOverride: src.Settings.WeatherAlertOverride,
				WeatherAlertBrief:    src.Settings.WeatherAlertBrief,
				WeekDisplay:          src.Settings.WeekDisplay,
				BannerText:           src.Settings.BannerText,
				CSSStyle:             cloneStringMap(src.Settings.CSSStyle),
				TemperatureColors:    src.Settings.TemperatureColors,
				StartupBehavior:      src.Settings.StartupBehavior,
			},
		},
	}
}

func saveCopyTargetConfig(c *gin.Context, tx *gorm.DB, target copyTargetConfig) bool {
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "namespace"}, {Name: "school"}, {Name: "grade"}}, UpdateAll: true}).Create(&target.Subject).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "namespace"}, {Name: "school"}, {Name: "grade"}}, UpdateAll: true}).Create(&target.Timetable).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "namespace"}, {Name: "school"}, {Name: "grade"}, {Name: "class"}}, UpdateAll: true}).Create(&target.Schedule).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "namespace"}, {Name: "school"}, {Name: "grade"}, {Name: "class"}}, UpdateAll: true}).Create(&target.Settings).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	return true
}

func CopyConfig(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	var payload copyConfigPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效参数: " + err.Error()})
		return
	}

	fromClass, toClass, verr := validateCopyPayload(&payload)
	if verr != nil {
		c.JSON(verr.status, gin.H{"detail": verr.msg})
		return
	}

	dbConn := db.GetDB()
	src, ok := loadCopySourceConfig(c, dbConn, ns, payload, fromClass)
	if !ok {
		return
	}
	target := buildCopyTargetConfig(ns, payload, toClass, src)

	tx := dbConn.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": tx.Error.Error()})
		return
	}
	defer func() {
		if recover() != nil {
			tx.Rollback()
		}
	}()

	if !saveCopyTargetConfig(c, tx, target) {
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 复制后通知目标班级所在年级的在线客户端刷新（来源班级数据未变，无需广播）
	client.BroadcastSync(ns, payload.To.School, payload.To.Grade)
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

func maxTimetableSubjects(timetable dbTable.TimetableConfig) int {
	maxSubjects := 0
	for _, v := range timetable.Timetable {
		for _, item := range v {
			i, ok := serviceAsInt(item)
			if ok && i+1 > maxSubjects {
				maxSubjects = i + 1
			}
		}
	}
	return maxSubjects
}

func scheduleDayResponse(day dbTable.DailyClass, timetable dbTable.TimetableConfig, maxSubjects int) gin.H {
	if _, ok := timetable.Timetable[day.Timetable]; !ok {
		day.Timetable = "常日"
	}
	classList := make([][]string, 0, len(day.ClassList))
	for _, s := range day.ClassList {
		classList = append(classList, s)
	}
	for len(classList) < maxSubjects {
		classList = append(classList, []string{"课"})
	}
	return gin.H{
		"Chinese":   day.Chinese,
		"English":   day.English,
		"classList": classList,
		"timetable": day.Timetable,
	}
}

func GetScheduleConfig(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	school := c.Param("school")
	grade := c.Param("grade")
	classNumber := c.Param("class_number")
	schedule := db.GetScheduleNs(ns, school, grade, classNumber)
	timetable := db.GetTimetableNs(ns, school, grade)
	maxSubjects := maxTimetableSubjects(timetable.TimetableConfig)
	out := make([]gin.H, 0, 7)
	for i := 0; i < 7; i++ {
		out = append(out, scheduleDayResponse(schedule.DailyClasses[i], timetable.TimetableConfig, maxSubjects))
	}
	c.JSON(http.StatusOK, gin.H{"daily_class": out})
}

// parseClassListRaw 从嵌套的 interface{} 中解析 classList
func parseClassListRaw(raw interface{}) [][]string {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	var result [][]string
	for _, classItem := range arr {
		arr2, ok := classItem.([]interface{})
		if !ok {
			continue
		}
		line := make([]string, 0, len(arr2))
		for _, x := range arr2 {
			if s, ok := x.(string); ok {
				line = append(line, s)
			}
		}
		result = append(result, line)
	}
	return result
}

// parseSchedulePayload 从原始 JSON 中解析课表请求体
func parseSchedulePayload(raw map[string]interface{}) schedulePayload {
	bodyMap := raw
	if modelVal, ok := raw["model"].(map[string]interface{}); ok {
		bodyMap = modelVal
	}
	body := schedulePayload{}
	arr, ok := bodyMap["daily_class"].([]interface{})
	if !ok {
		return body
	}
	for _, one := range arr {
		obj, ok := one.(map[string]interface{})
		if !ok {
			continue
		}
		item := dailyClassInput{}
		item.Chinese, _ = obj["Chinese"].(string)
		item.English, _ = obj["English"].(string)
		item.Timetable, _ = obj["timetable"].(string)
		item.ClassList = parseClassListRaw(obj["classList"])
		body.DailyClass = append(body.DailyClass, item)
	}
	return body
}

func PutScheduleConfig(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	school := c.Param("school")
	grade := c.Param("grade")
	classNumber := c.Param("class_number")

	var raw map[string]interface{}
	if err := c.ShouldBindJSON(&raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 契约校验：请求体必须携带完整的 7 天 daily_class 数组，
	// 否则会导致该班课表被静默清空（防坏请求覆盖既有数据）
	bodyMap := raw
	if modelVal, ok := raw["model"].(map[string]interface{}); ok {
		bodyMap = modelVal
	}
	dailyClassRaw, ok := bodyMap["daily_class"].([]interface{})
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "daily_class 必须为数组"})
		return
	}
	if len(dailyClassRaw) != 7 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "daily_class 必须包含 7 天（日一二三四五六）"})
		return
	}
	for index, one := range dailyClassRaw {
		if _, isObj := one.(map[string]interface{}); !isObj {
			// 契约校验：非对象条目会导致对应日期写入零值课表（部分清空），必须拒绝
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("daily_class[%d] 必须为对象", index)})
			return
		}
	}
	body := parseSchedulePayload(raw)

	var daily [7]dbTable.DailyClass
	for i := 0; i < 7 && i < len(body.DailyClass); i++ {
		daily[i] = dbTable.DailyClass{
			Chinese:   body.DailyClass[i].Chinese,
			English:   body.DailyClass[i].English,
			ClassList: parseClassList(body.DailyClass[i].ClassList),
			Timetable: body.DailyClass[i].Timetable,
		}
	}
	timetable := db.GetTimetableNs(ns, school, grade)
	service.FixWrongTimetable(&daily, timetable.Timetable)

	record := dbTable.Schedule{Namespace: ns, School: school, Grade: grade, Class: classNumber, DailyClasses: daily}
	if err := db.GetDB().Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "namespace"}, {Name: "school"}, {Name: "grade"}, {Name: "class"}}, UpdateAll: true}).Create(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	client.BroadcastSync(ns, school, grade)
	c.JSON(http.StatusOK, gin.H{"status": 200})
}

func GetSettings(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	school := c.Param("school")
	grade := c.Param("grade")
	classNumber := c.Param("class_number")
	config := db.GetClientConfigNs(ns, school, grade, classNumber)
	c.JSON(http.StatusOK, config.ClientConfigItems)
}

func PutSettings(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	school := c.Param("school")
	grade := c.Param("grade")
	classNumber := c.Param("class_number")
	var body dbTable.ClientConfigItems
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	record := dbTable.ClientConfig{Namespace: ns, School: school, Grade: grade, Class: classNumber, ClientConfigItems: body}
	if err := db.GetDB().Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "namespace"}, {Name: "school"}, {Name: "grade"}, {Name: "class"}}, UpdateAll: true}).Create(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 通用设置影响桌面端渲染，广播刷新
	client.BroadcastSync(ns, school, grade)
	c.JSON(http.StatusOK, gin.H{"status": 200})
}
