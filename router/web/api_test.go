package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/model/dbTable"
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

func TestGetMenu_Empty(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/menu", GetMenu)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/menu", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	assert.GreaterOrEqual(t, len(data), 4, "menu should contain at least 4 base items")
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
	req, _ := http.NewRequest("GET", "/web/schedule/by-date?scope=school1/grade1/class1&date=2025-10-13", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// PutSubjects tests

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
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/web/config/school1/grade1/subjects", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// PutTimetable tests

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
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/web/config/school1/grade1/timetable", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// PutScheduleConfig tests

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
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/web/config/school1/grade1/class1/schedule", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// PutSettings tests

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
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/web/config/school1/grade1/class1/settings", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// CopyConfig tests

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

func TestCopyConfig_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.POST("/web/config/copy", CopyConfig)

	body := map[string]interface{}{
		"from": map[string]interface{}{
			"school": "school1",
			"grade":  "grade1",
			"class":  "class1",
		},
		"to": map[string]interface{}{
			"school": "school2",
			"grade":  "grade2",
			"class":  "class2",
		},
	}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/web/config/copy", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// Autorun handlers tests

func TestGetAutorunHashStatus_NotFound(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/autorun/hash/:hashid", GetAutorunHashStatus)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/autorun/hash/nonexistent", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// Countdown handlers tests

func TestGetCountdownByID_NotFound(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/countdown/:id", GetCountdownByID)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/countdown/nonexistent", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// Backup handlers tests

func TestExportBackup_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/backup/export", ExportBackup)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/backup/export", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestImportBackup_InvalidJSON(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.POST("/web/backup/import", ImportBackup)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/web/backup/import", bytes.NewBufferString("invalid"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
