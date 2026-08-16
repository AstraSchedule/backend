package web

import (
	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/middleware"
	"AstraScheduleServerGo/model/dbTable"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	invalidArgPrefix = "无效参数: "
	dateLayout       = "2006-01-02"
)

func badRequestDetail(c *gin.Context, detail string) {
	c.JSON(http.StatusBadRequest, gin.H{"detail": detail})
}

func badRequestInvalidArg(c *gin.Context, detail string) {
	badRequestDetail(c, invalidArgPrefix+detail)
}

func validateDateField(c *gin.Context, fieldName string, value string) bool {
	if _, err := time.Parse(dateLayout, value); err != nil {
		badRequestInvalidArg(c, fieldName+" 格式错误")
		return false
	}
	return true
}

func persistAutorunRule(c *gin.Context, payload autorunPayload, params map[string]interface{}, hashID string) {
	ns := middleware.GetNamespace(c)
	claims := middleware.GetUserClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}
	scope := parseScopeInput(payload.Scope)
	// 校验作用域权限：非 admin 用户的操作范围不能超过自身 scope
	if claims.Role != "admin" {
		for _, s := range scope {
			if s == "ALL" {
				continue
			}
			if !db.CheckScopePermission(&dbTable.User{Role: claims.Role, Scope: claims.Scope}, s, "", "") {
				c.JSON(http.StatusForbidden, gin.H{"error": "无权操作该作用域"})
				return
			}
		}
	}

	if hashID == "" {
		hashID = makeHashID(ns, payload.Type, scope, payload.Priority, params)
	}
	// 更新前取回旧作用域：scope 变更时，旧作用域的客户端同样需要刷新通知
	var oldScopes []string
	if rows, err := db.FetchAutorunRecordsNs(ns, hashID); err == nil && len(rows) > 0 {
		oldScopes = rows[0].Scope
	}
	record := dbTable.AutorunRecord{HashID: hashID, Namespace: ns, EType: payload.Type, Scope: scope, Parameters: params, Level: payload.Priority, Status: 0}
	if err := db.UpsertAutorunRecord(&record); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, _ = db.RefreshAutorunStatusesNs(ns, time.Now())
	// 规则变更影响课表解析，按新旧作用域并集广播刷新
	broadcastScopes(ns, mergeScopes(oldScopes, scope))
	c.JSON(http.StatusOK, gin.H{"status": 200, "id": hashID})
}

func mapAutorunRecord(r dbTable.AutorunRecord) gin.H {
	content := map[string]interface{}{}
	if r.Parameters != nil {
		if rule, ok := r.Parameters["rule"].(map[string]interface{}); ok {
			content = rule
		} else {
			content = r.Parameters
		}
	}

	typeName := strconv.Itoa(r.EType)
	switch r.EType {
	case dbTable.AutorunTypeCompensation:
		typeName = "COMPENSATION"
	case dbTable.AutorunTypeTimetable:
		typeName = "TIMETABLE"
	case dbTable.AutorunTypeSchedule:
		typeName = "SCHEDULE"
	case dbTable.AutorunTypeAll:
		typeName = "ALL"
	}

	statusText := "未知"
	switch r.Status {
	case 0:
		statusText = "待生效"
	case 1:
		statusText = "生效中"
	case 2:
		statusText = "已过期"
	}

	return gin.H{
		"id":       r.HashID,
		"type":     typeName,
		"priority": r.Level,
		"status":   statusText,
		"scope":    r.Scope,
		"content":  content,
	}
}

func GetAutorunStatus(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	_, _ = db.RefreshAutorunStatusesNs(ns, time.Now())
	rows, err := db.FetchAutorunRecordsNs(ns, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapAutorunRecord(r))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func GetAutorunHashStatus(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	hashid := c.Param("hashid")
	_, _ = db.RefreshAutorunStatusesNs(ns, time.Now())
	rows, err := db.FetchAutorunRecordsNs(ns, hashid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(rows) == 0 {
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": mapAutorunRecord(rows[0])})
}

func DeleteAutorunRecord(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	hashid := c.Param("hashid")
	// 删除前取回规则作用域，删除后按原作用域广播刷新
	var scopes []string
	if rows, err := db.FetchAutorunRecordsNs(ns, hashid); err == nil && len(rows) > 0 {
		scopes = rows[0].Scope
	}
	affected, err := db.DeleteAutorunRecordNs(ns, hashid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"detail": "记录不存在"})
		return
	}
	_, _ = db.RefreshAutorunStatusesNs(ns, time.Now())
	broadcastScopes(ns, scopes)
	c.JSON(http.StatusOK, gin.H{"status": 200, "deleted": affected, "id": hashid})
}

func PutCompensationRule(c *gin.Context) {
	var payload autorunPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		badRequestInvalidArg(c, err.Error())
		return
	}
	if payload.Type != 0 {
		payload.Type = 0
	}
	dateStr, _ := payload.Content["date"].(string)
	useDateStr, _ := payload.Content["useDate"].(string)
	if !validateDateField(c, "date", dateStr) {
		return
	}
	if !validateDateField(c, "useDate", useDateStr) {
		return
	}
	params := map[string]interface{}{"rule": map[string]interface{}{"date": dateStr, "useDate": useDateStr}}
	persistAutorunRule(c, payload, params, "")
}

func PutTimetableRule(c *gin.Context) {
	var payload autorunPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		badRequestInvalidArg(c, err.Error())
		return
	}
	payload.Type = 1
	dateStr, _ := payload.Content["date"].(string)
	timetableID, _ := payload.Content["timetableId"].(string)
	if timetableID == "" {
		badRequestInvalidArg(c, "timetableId 必须为非空字符串")
		return
	}
	if !validateDateField(c, "date", dateStr) {
		return
	}
	params := map[string]interface{}{"rule": map[string]interface{}{"date": dateStr, "timetableId": timetableID}}
	hashID := payload.ID
	if hashID == "" {
		if idInContent, ok := payload.Content["id"].(string); ok {
			hashID = idInContent
		}
	}
	persistAutorunRule(c, payload, params, hashID)
}

func PutScheduleRule(c *gin.Context) {
	var payload autorunPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		badRequestInvalidArg(c, err.Error())
		return
	}
	payload.Type = 2
	dateStr, _ := payload.Content["date"].(string)
	scheduleObj, ok := payload.Content["schedule"].(map[string]interface{})
	if !ok {
		badRequestDetail(c, "content.schedule 必须为对象")
		return
	}
	if _, ok := scheduleObj["periods"].([]interface{}); !ok {
		badRequestDetail(c, "content.schedule.periods 必须为数组")
		return
	}
	if !validateDateField(c, "date", dateStr) {
		return
	}
	params := map[string]interface{}{"rule": map[string]interface{}{"date": dateStr, "schedule": map[string]interface{}{"periods": scheduleObj["periods"]}}}
	persistAutorunRule(c, payload, params, "")
}

func PutAllRule(c *gin.Context) {
	var payload autorunPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		badRequestInvalidArg(c, err.Error())
		return
	}
	payload.Type = 3
	dateStr, _ := payload.Content["date"].(string)
	timetableID, _ := payload.Content["timetableId"].(string)
	if timetableID == "" {
		badRequestDetail(c, "content.timetableId 必须为非空字符串")
		return
	}
	scheduleObj, ok := payload.Content["schedule"].(map[string]interface{})
	if !ok {
		badRequestDetail(c, "content.schedule 必须为对象")
		return
	}
	if _, ok := scheduleObj["periods"].([]interface{}); !ok {
		badRequestDetail(c, "content.schedule.periods 必须为数组")
		return
	}
	if !validateDateField(c, "date", dateStr) {
		return
	}
	params := map[string]interface{}{"rule": map[string]interface{}{"date": dateStr, "timetableId": timetableID, "schedule": map[string]interface{}{"periods": scheduleObj["periods"]}}}
	persistAutorunRule(c, payload, params, "")
}
