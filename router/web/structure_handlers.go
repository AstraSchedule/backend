package web

import (
	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/model/dbTable"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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
	if rejectReservedSchoolName(c, req.Name) {
		return
	}

	// 学校是否存在以关联的科目行判断（学校本身没有独立表行，
	// 按 Schedule 行判断会在“学校尚无班级”时漏判，导致重复创建返回 200）
	var count int64
	countRes := db.GetDB().Model(&dbTable.Subject{}).Where(whereSchool, req.Name).Count(&count)
	if countRes.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": countRes.Error.Error()})
		return
	}
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"detail": "学校已存在"})
		return
	}

	// 学校本身没有持久化表行（存在性由科目行推导），此处无数据写入，
	// 因此不广播：客户端没有任何需要刷新的数据变更
	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "学校创建成功"})
}

// rejectReservedSchoolName 拒绝保留值 "ALL"：学校名 ALL 会与自动任务全局规则的作用域冲突，
// 删除学校 "ALL" 会按前缀匹配误删全局规则。返回 true 表示已写入 400 响应，调用方直接 return。
func rejectReservedSchoolName(c *gin.Context, school string) bool {
	if school != "ALL" {
		return false
	}
	c.JSON(http.StatusBadRequest, gin.H{"detail": "学校名称不能为保留值 ALL"})
	return true
}

// rollbackAnd500 回滚事务并写入 500 响应，供删除流程统一处理错误。
func rollbackAnd500(c *gin.Context, tx *gorm.DB, err error) {
	tx.Rollback()
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

// commitOr500 提交事务；失败时写入 500 响应并返回 false。
func commitOr500(c *gin.Context, tx *gorm.DB) bool {
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	return true
}

// deleteRecordsTx 在事务内按作用域条件删除多张表，任一条语句失败即返回错误。
// 此前各 Delete 语句的返回值被忽略，SQL 错误（如列不存在）会被静默吞掉。
func deleteRecordsTx(tx *gorm.DB, where string, args []interface{}, models ...interface{}) error {
	for _, m := range models {
		if err := tx.Where(where, args...).Delete(m).Error; err != nil {
			return err
		}
	}
	return nil
}

// deleteAutorunRecordsByScopePrefix 在事务连接上移除作用域前缀匹配的自动任务规则作用域。
// AutorunRecord 没有 school 列，作用域存于 Scope JSON 字段，需逐条过滤：
// 仅移除匹配的作用域；无剩余作用域时删除整条记录，否则回写剩余作用域，
// 避免整行删除误伤同一记录中的无关作用域。scope "ALL" 的全局规则不受影响
//（调用方已拒绝保留值 ALL）。
// 已过期规则（status=2）不会再被规则引擎命中，删除与否无差异：由 SQL WHERE 下推过滤
//（过期数据通常远多于待生效/生效中，下推可减少载入行数），保留历史数据。
// 注意必须复用 tx 连接：内存库单连接池下在事务未提交时调用 db.GetDB() 会死锁。
func deleteAutorunRecordsByScopePrefix(tx *gorm.DB, prefix string) error {
	var rows []dbTable.AutorunRecord
	// status：0 待生效 / 1 生效中 / 2 已过期（与 db.RefreshAutorunStatuses 维护值一致）
	if err := tx.Where("status <> ?", 2).Find(&rows).Error; err != nil {
		return err
	}
	for _, r := range rows {
		remaining := make([]string, 0, len(r.Scope))
		for _, s := range r.Scope {
			if s == prefix || strings.HasPrefix(s, prefix+"/") {
				continue
			}
			remaining = append(remaining, s)
		}
		if len(remaining) == len(r.Scope) {
			continue // 无匹配作用域，记录不动
		}
		if len(remaining) == 0 {
			if err := tx.Where("hash_id = ?", r.HashID).Delete(&dbTable.AutorunRecord{}).Error; err != nil {
				return err
			}
			continue
		}
		// 取出整条记录回写剩余作用域（Save 走 Scope 字段的 JSON serializer；
		// Update("scope", []string) 不经过 serializer 会写入非法 JSON）
		var rec dbTable.AutorunRecord
		if err := tx.Where("hash_id = ?", r.HashID).First(&rec).Error; err != nil {
			return err
		}
		rec.Scope = remaining
		if err := tx.Save(&rec).Error; err != nil {
			return err
		}
	}
	return nil
}

func DeleteSchool(c *gin.Context) {
	school := c.Param("school")
	if school == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "学校名称不能为空"})
		return
	}
	if rejectReservedSchoolName(c, school) {
		return
	}

	tx := db.GetDB().Begin()
	defer func() { if recover() != nil { tx.Rollback() } }()

	if err := deleteRecordsTx(tx, whereSchool, []interface{}{school},
		&dbTable.Schedule{}, &dbTable.ClientConfig{}, &dbTable.DataVersion{},
		&dbTable.Subject{}, &dbTable.Timetable{}); err != nil {
		rollbackAnd500(c, tx, err)
		return
	}
	if err := deleteAutorunRecordsByScopePrefix(tx, school); err != nil {
		rollbackAnd500(c, tx, err)
		return
	}

	if !commitOr500(c, tx) {
		return
	}
	broadcastScopes([]string{school})
	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "学校已删除"})
}

func CreateGrade(c *gin.Context) {
	school := c.Param("school")
	if rejectReservedSchoolName(c, school) {
		return
	}
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
	countRes := db.GetDB().Model(&dbTable.Subject{}).Where(whereSchoolGrade, school, req.Name).Count(&count)
	if countRes.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": countRes.Error.Error()})
		return
	}
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
	// 事务内原子创建默认科目与作息：以 Subject 行的插入结果作为年级是否已存在的最终裁决，
	// 并发重复请求只会有一个成功插入（另一个 RowsAffected==0 -> 409）
	tx := db.GetDB().Begin()
	defer func() { if recover() != nil { tx.Rollback() } }()

	subjectRes := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&subject)
	if subjectRes.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": subjectRes.Error.Error()})
		return
	}
	if subjectRes.RowsAffected == 0 {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"detail": "年级已存在"})
		return
	}

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
	timetableRes := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&timetable)
	if timetableRes.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": timetableRes.Error.Error()})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 年级创建生成默认科目/作息，广播该年级客户端刷新
	broadcastScopes([]string{school + "/" + req.Name})
	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "年级创建成功"})
}

func DeleteGrade(c *gin.Context) {
	school := c.Param("school")
	grade := c.Param("grade")
	if rejectReservedSchoolName(c, school) {
		return
	}

	tx := db.GetDB().Begin()
	defer func() { if recover() != nil { tx.Rollback() } }()

	if err := deleteRecordsTx(tx, whereSchoolGrade, []interface{}{school, grade},
		&dbTable.Schedule{}, &dbTable.ClientConfig{}, &dbTable.DataVersion{},
		&dbTable.Subject{}, &dbTable.Timetable{}); err != nil {
		rollbackAnd500(c, tx, err)
		return
	}
	if err := deleteAutorunRecordsByScopePrefix(tx, school+"/"+grade); err != nil {
		rollbackAnd500(c, tx, err)
		return
	}

	if !commitOr500(c, tx) {
		return
	}
	broadcastScopes([]string{school + "/" + grade})
	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "年级已删除"})
}

func CreateClass(c *gin.Context) {
	school := c.Param("school")
	grade := c.Param("grade")
	if rejectReservedSchoolName(c, school) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "班级名称不能为空"})
		return
	}

	var count int64
	countRes := db.GetDB().Model(&dbTable.Schedule{}).Where(whereSchoolGradeClass, school, grade, req.Name).Count(&count)
	if countRes.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": countRes.Error.Error()})
		return
	}
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
	// 事务内原子创建默认课表与客户端配置，任一失败回滚
	tx := db.GetDB().Begin()
	defer func() { if recover() != nil { tx.Rollback() } }()

	scheduleRes := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&schedule)
	if scheduleRes.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": scheduleRes.Error.Error()})
		return
	}

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
	configRes := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&clientConfig)
	if configRes.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": configRes.Error.Error()})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 班级创建生成默认课表与客户端配置，提交成功后才广播该年级客户端刷新
	broadcastScopes([]string{school + "/" + grade})
	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "班级创建成功"})
}

func DeleteClass(c *gin.Context) {
	school := c.Param("school")
	grade := c.Param("grade")
	classNumber := c.Param("class_number")
	if rejectReservedSchoolName(c, school) {
		return
	}

	tx := db.GetDB().Begin()
	defer func() { if recover() != nil { tx.Rollback() } }()

	if err := deleteRecordsTx(tx, whereSchoolGradeClass, []interface{}{school, grade, classNumber},
		&dbTable.Schedule{}, &dbTable.ClientConfig{}, &dbTable.DataVersion{}); err != nil {
		rollbackAnd500(c, tx, err)
		return
	}
	if err := deleteAutorunRecordsByScopePrefix(tx, school+"/"+grade+"/"+classNumber); err != nil {
		rollbackAnd500(c, tx, err)
		return
	}

	if !commitOr500(c, tx) {
		return
	}
	broadcastScopes([]string{school + "/" + grade})
	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "班级已删除"})
}
