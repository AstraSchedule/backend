package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/middleware"
	"AstraScheduleServerGo/model"
	"AstraScheduleServerGo/model/dbTable"
	"AstraScheduleServerGo/service"
	"AstraScheduleServerGo/testutil"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	return router
}

var testDBInitialized = false

// doRequest 在 router 上执行 JSON 请求并返回 recorder，减少测试样板重复
func doRequest(router *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	var reqBody *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}
	req, _ := http.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w
}

func ensureTestDB() {
	if testDBInitialized {
		return
	}
	testutil.InitTestDB()
	db.GetDB().AutoMigrate(
		&dbTable.User{},
		&dbTable.Schedule{},
		&dbTable.ClientConfig{},
		&dbTable.Timetable{},
		&dbTable.Subject{},
		&dbTable.DataVersion{},
		&dbTable.AutorunRecord{},
		&dbTable.CountdownRecord{},
	)
	testDBInitialized = true
}

// Menu handlers tests

func TestGetStatistic_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/statistic", GetStatistic)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/statistic", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, false, resp["serverless"])
	// 响应应包含完整统计字段（数值不在此处断言：与 client 包测试并行运行时采集器可能被写入）
	assert.Contains(t, resp, "weather_error")
	assert.Contains(t, resp, "websocket_disconnect")
	assert.Contains(t, resp, "websocket_disconnect_count")
	assert.Contains(t, resp, "clients")
	assert.Contains(t, resp, "clients_count")
	assert.IsType(t, map[string]interface{}{}, resp["websocket_disconnect"])
	assert.IsType(t, []interface{}{}, resp["clients"])
}

func TestGetStatistic_Auth(t *testing.T) {
	ensureTestDB()
	router := setupTestRouter()
	// 模拟 main.go 的 jwtAuth 组：/web/statistic 需 JWT 认证
	router.GET("/web/statistic", middleware.JWTAuthMiddleware(), GetStatistic)

	// 无 token -> 401
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/statistic", nil)
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// 无效 token -> 401
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/web/statistic", nil)
	req2.Header.Set("Authorization", "Bearer invalid.token.here")
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusUnauthorized, w2.Code)

	// 有效 token -> 200 且字段完整
	token, err := service.GenerateToken(model.Configs.Secret.Token, 1, "admin", "admin", "", 1)
	assert.NoError(t, err)
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/web/statistic", nil)
	req3.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusOK, w3.Code)
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w3.Body.Bytes(), &resp))
	assert.Contains(t, resp, "weather_error")
	assert.Contains(t, resp, "clients_count")
}

func TestGetMenu_Empty(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/menu", GetMenu)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/menu", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data, ok := resp["data"].([]interface{})
	assert.True(t, ok, "data 应为数组")
	assert.GreaterOrEqual(t, len(data), 4) // At least 4 base menu items
	// 每个菜单项必须具备 text/key，供 usr-dashboard 构建 scope tree（autorun.js 依赖）
	for _, item := range data {
		m, ok := item.(map[string]interface{})
		assert.True(t, ok, "menu item 应为对象")
		assert.Contains(t, m, "text")
		assert.Contains(t, m, "key")
	}
}

func TestGetStructure_Empty(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/structure", GetStructure)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/structure", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp []interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NotNil(t, resp)
}

// Config handlers tests

func TestGetSubjectsOptions_Empty(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/config/:school/:grade/subjects/options", GetSubjectsOptions)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/config/school1/grade1/subjects/options", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetSubjects_Empty(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/config/:school/:grade/subjects", GetSubjects)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/config/school1/grade1/subjects", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetTimetableOptions_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/config/:school/:grade/timetable/options", GetTimetableOptions)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/config/school1/grade1/timetable/options", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetTimetable_Empty(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/config/:school/:grade/timetable", GetTimetable)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/config/school1/grade1/timetable", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetScheduleConfig_Empty(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/config/:school/:grade/:class_number/schedule", GetScheduleConfig)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/config/school1/grade1/class1/schedule", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetSettings_Empty(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/config/:school/:grade/:class_number/settings", GetSettings)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/config/school1/grade1/class1/settings", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// Autorun handlers tests

func TestGetAutorunStatus_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/autorun", GetAutorunStatus)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/autorun", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// Countdown handlers tests

func TestGetCountdownStatus_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/countdown", GetCountdownStatus)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/countdown", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// Compensation handlers tests

func TestCompensationFromHoliday_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/autorun/compensation/holiday/:year/:month/:day", CompensationFromHoliday)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/autorun/compensation/holiday/2025/10/01", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCompensationFromWorkday_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/autorun/compensation/workday/:year/:month/:day", CompensationFromWorkday)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/autorun/compensation/workday/2025/10/13", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCompensationFromYear_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/autorun/compensation/year/:year", CompensationFromYear)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/autorun/compensation/year/2025", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetScheduleByDate_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/schedule/by-date", GetScheduleByDate)

	w := httptest.NewRecorder()
	// 契约：handler 读取 scope 参数（school/grade/class），而不是三个独立参数
	req, _ := http.NewRequest("GET", "/web/schedule/by-date?scope=school1/grade1/class1&date=2025-10-13", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data, ok := resp["data"].(map[string]interface{})
	assert.True(t, ok, "响应应包含 data 对象")
	assert.Contains(t, data, "periods")
}

func TestGetScheduleByDate_InvalidDate(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/schedule/by-date", GetScheduleByDate)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/schedule/by-date?scope=school1/grade1/class1&date=invalid", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetScheduleByDate_InvalidScope(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/schedule/by-date", GetScheduleByDate)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/schedule/by-date?scope=school1&date=2025-10-13", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPutSubjects_InvalidJSON(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/config/:school/:grade/subjects", PutSubjects)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/web/config/school1/grade1/subjects", bytes.NewBufferString("invalid"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPutSubjects_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/config/:school/:grade/subjects", PutSubjects)

	body := map[string]interface{}{
		"abbr":     []map[string]interface{}{{"text": "数"}},
		"fullName": []map[string]interface{}{{"text": "数学"}},
	}
	w := doRequest(router, "PUT", "/web/config/school1/grade1/subjects", body)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPutTimetable_InvalidJSON(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/config/:school/:grade/timetable", PutTimetable)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/web/config/school1/grade1/timetable", bytes.NewBufferString("invalid"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPutTimetable_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/config/:school/:grade/timetable", PutTimetable)

	body := map[string]interface{}{
		"timetable": map[string]interface{}{
			"常日": map[string]interface{}{"早上1": 1},
		},
		"divider": map[string]interface{}{},
	}
	w := doRequest(router, "PUT", "/web/config/school1/grade1/timetable", body)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPutTimetable_MissingChangri(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/config/:school/:grade/timetable", PutTimetable)

	body := map[string]interface{}{
		"timetable": map[string]interface{}{
			"考试周": map[string]interface{}{"早上1": 1},
		},
	}
	w := doRequest(router, "PUT", "/web/config/school1/grade1/timetable", body)

	// 契约：必须保留“常日”作息表
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPutScheduleConfig_InvalidJSON(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/config/:school/:grade/:class_number/schedule", PutScheduleConfig)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/web/config/school1/grade1/class1/schedule", bytes.NewBufferString("invalid"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPutScheduleConfig_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/config/:school/:grade/:class_number/schedule", PutScheduleConfig)

	body := map[string]interface{}{
		"Chinese":   "周一",
		"English":   "Monday",
		"classList": []interface{}{[]interface{}{"数"}},
		"timetable": "常日",
	}
	w := doRequest(router, "PUT", "/web/config/school1/grade1/class1/schedule", body)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPutSettings_InvalidJSON(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/config/:school/:grade/:class_number/settings", PutSettings)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/web/config/school1/grade1/class1/settings", bytes.NewBufferString("invalid"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPutSettings_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/config/:school/:grade/:class_number/settings", PutSettings)

	body := map[string]interface{}{
		"countdown_target": "2025-12-31",
		"banner_text":      "欢迎",
	}
	w := doRequest(router, "PUT", "/web/config/school1/grade1/class1/settings", body)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCopyConfig_InvalidJSON(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.POST("/web/config/copy", CopyConfig)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/web/config/copy", bytes.NewBufferString("invalid"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCopyConfig_MissingSource(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.POST("/web/config/copy", CopyConfig)

	body := map[string]interface{}{
		"from": map[string]interface{}{"school": "nosrc", "grade": "g", "class": "c"},
		"to":   map[string]interface{}{"school": "dst", "grade": "g", "class": "c"},
	}
	w := doRequest(router, "POST", "/web/config/copy", body)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCopyConfig_Success(t *testing.T) {
	ensureTestDB()

	database := db.GetDB()
	database.Save(&dbTable.Subject{
		School: "src", Grade: "g",
		SubjectConfig: dbTable.SubjectConfig{SubjectName: map[string]string{"数": "数学"}},
	})
	database.Save(&dbTable.Timetable{
		School: "src", Grade: "g",
		TimetableConfig: dbTable.TimetableConfig{
			Timetable: map[string]map[string]interface{}{"常日": {"早上1": 1}},
			Divider:   map[string][]int{"常日": {1}},
		},
	})
	database.Save(&dbTable.Schedule{
		School: "src", Grade: "g", Class: "c",
		DailyClasses: [7]dbTable.DailyClass{{Timetable: "常日", ClassList: dbTable.ClassList{{"数"}}}},
	})
	database.Save(&dbTable.ClientConfig{
		School: "src", Grade: "g", Class: "c",
		ClientConfigItems: dbTable.ClientConfigItems{BannerText: "来自源班级"},
	})

	router := setupTestRouter()
	router.POST("/web/config/copy", CopyConfig)

	body := map[string]interface{}{
		"from": map[string]interface{}{"school": "src", "grade": "g", "class": "c"},
		"to":   map[string]interface{}{"school": "dst", "grade": "g", "class": "c"},
	}
	w := doRequest(router, "POST", "/web/config/copy", body)

	assert.Equal(t, http.StatusOK, w.Code)

	// 验证目标班级已复制科目配置
	var dstSubject dbTable.Subject
	assert.NoError(t, db.GetDB().Where("school = ? AND grade = ?", "dst", "g").First(&dstSubject).Error)
	assert.Equal(t, "数学", dstSubject.SubjectName["数"])
}

// Autorun 规则写接口测试

func TestPutCompensationRule_InvalidDate(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/autorun/compensation", PutCompensationRule)

	body := map[string]interface{}{
		"type": 0, "scope": []string{"ALL"}, "priority": 1,
		"content": map[string]interface{}{"date": "invalid", "useDate": "2025-09-29"},
	}
	w := doRequest(router, "PUT", "/web/autorun/compensation", body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPutCompensationRule_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/autorun/compensation", PutCompensationRule)

	body := map[string]interface{}{
		"type": 0, "scope": []string{"ALL"}, "priority": 1,
		"content": map[string]interface{}{"date": "2025-10-01", "useDate": "2025-09-29"},
	}
	w := doRequest(router, "PUT", "/web/autorun/compensation", body)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPutTimetableRule_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/autorun/timetable", PutTimetableRule)

	body := map[string]interface{}{
		"type": 1, "scope": []string{"ALL"}, "priority": 1,
		"content": map[string]interface{}{"date": "2025-10-08", "timetableId": "exam"},
	}
	w := doRequest(router, "PUT", "/web/autorun/timetable", body)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPutScheduleRule_MissingPeriods(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/autorun/schedule", PutScheduleRule)

	body := map[string]interface{}{
		"type": 2, "scope": []string{"ALL"}, "priority": 1,
		"content": map[string]interface{}{"date": "2025-10-09", "schedule": map[string]interface{}{}},
	}
	w := doRequest(router, "PUT", "/web/autorun/schedule", body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPutScheduleRule_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/autorun/schedule", PutScheduleRule)

	body := map[string]interface{}{
		"type": 2, "scope": []string{"ALL"}, "priority": 1,
		"content": map[string]interface{}{
			"date":     "2025-10-09",
			"schedule": map[string]interface{}{"periods": []interface{}{map[string]interface{}{"no": 1, "subject": "数"}}},
		},
	}
	w := doRequest(router, "PUT", "/web/autorun/schedule", body)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPutAllRule_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/autorun/all", PutAllRule)

	body := map[string]interface{}{
		"type": 3, "scope": []string{"ALL"}, "priority": 1,
		"content": map[string]interface{}{
			"date":        "2025-10-10",
			"timetableId": "exam",
			"schedule":    map[string]interface{}{"periods": []interface{}{map[string]interface{}{"no": 1, "subject": "班会"}}},
		},
	}
	w := doRequest(router, "PUT", "/web/autorun/all", body)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestDeleteAutorunRecord_NotFound(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.DELETE("/web/autorun/:hashid", DeleteAutorunRecord)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/web/autorun/nonexistent-hash", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteAutorunRecord_Success(t *testing.T) {
	ensureTestDB()

	// 先创建一条规则
	router := setupTestRouter()
	router.PUT("/web/autorun/compensation", PutCompensationRule)

	body := map[string]interface{}{
		"type": 0, "scope": []string{"ALL"}, "priority": 1,
		"content": map[string]interface{}{"date": "2025-11-01", "useDate": "2025-10-31"},
	}
	w := doRequest(router, "PUT", "/web/autorun/compensation", body)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	hashID, _ := resp["id"].(string)
	assert.NotEmpty(t, hashID)

	// 再删除
	router.DELETE("/web/autorun/:hashid", DeleteAutorunRecord)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("DELETE", "/web/autorun/"+hashID, nil)
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
}

// Countdown 写接口测试

func TestPutCountdownRule_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/countdown", PutCountdownRule)

	body := map[string]interface{}{
		"scope": []string{"ALL"},
		"schedules": []map[string]interface{}{
			{"name": "期末考试", "date": "2026-01-01", "priority": 1},
		},
	}
	w := doRequest(router, "PUT", "/web/countdown", body)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestDeleteCountdownRecord_NotFound(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.DELETE("/web/countdown/:id", DeleteCountdownRecord)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/web/countdown/nonexistent-id", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// Backup 接口测试

func TestExportBackup_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/backup/export", ExportBackup)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/backup/export", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// 响应应为有效 JSON 备份（meta + 各表数据）
	var payload map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Contains(t, payload, "meta")
	assert.Contains(t, payload, "schedules")
	assert.Contains(t, payload, "timetables")
	assert.Contains(t, payload, "subjects")
}

func TestImportBackup_InvalidBody(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.POST("/web/backup/import", ImportBackup)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/web/backup/import", bytes.NewBufferString("invalid"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestImportBackup_RoundTrip(t *testing.T) {
	ensureTestDB()

	// 先导出
	router := setupTestRouter()
	router.GET("/web/backup/export", ExportBackup)
	router.POST("/web/backup/import", ImportBackup)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/backup/export", nil)
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 回填导入
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/web/backup/import", bytes.NewBuffer(w.Body.Bytes()))
	req2.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
	assert.Equal(t, float64(200), resp["status"])
}

func TestFullExportBackup_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.POST("/web/backup/full-export", FullExportBackup)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/web/backup/full-export", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var payload map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Contains(t, payload, "meta")
}

func TestFullImportBackup_InvalidMode(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.POST("/web/backup/full-import", FullImportBackup)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/web/backup/full-import?mode=invalid", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestFullImportBackup_RoundTrip(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.POST("/web/backup/full-export", FullExportBackup)
	router.POST("/web/backup/full-import", FullImportBackup)

	// 先 full-export 拿备份内容
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/web/backup/full-export", nil)
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// multipart 回填导入
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "backup.json")
	part.Write(w.Body.Bytes())
	writer.WriteField("mode", "overwrite")
	writer.Close()

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/web/backup/full-import", body)
	req2.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
	assert.Equal(t, float64(200), resp["status"])
	assert.Equal(t, "overwrite", resp["mode"])
}
