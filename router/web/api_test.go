package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/middleware"
	"AstraScheduleServerGo/model"
	"AstraScheduleServerGo/model/dbTable"
	"AstraScheduleServerGo/service"

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
	model.Configs = model.SrvConfig{
		Server: model.ServerConfig{
			Host:   "127.0.0.1",
			Port:   9000,
			Domain: []string{"http://localhost:5173"},
		},
		Secret: model.SecretConfig{
			Token: "test-token-123",
		},
		Db: model.DbConfig{
			Type: "sqlite",
			Path: ":memory:",
		},
		APIKey: model.APIKeyConfig{
			APIHost: "https://geoapi.qweather.com",
			Weather: "test-weather-key",
		},
		Log: model.LogConfig{
			Debug: true,
		},
		Run: model.RunConfig{
			Serverless: false,
		},
	}
	database := db.GetDB()
	database.AutoMigrate(
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
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	assert.GreaterOrEqual(t, len(data), 4) // At least 4 base menu items
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

	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusBadRequest)
}

func TestCompensationFromWorkday_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/autorun/compensation/workday/:year/:month/:day", CompensationFromWorkday)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/autorun/compensation/workday/2025/10/13", nil)
	router.ServeHTTP(w, req)

	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusBadRequest)
}

func TestCompensationFromYear_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/autorun/compensation/year/:year", CompensationFromYear)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/autorun/compensation/year/2025", nil)
	router.ServeHTTP(w, req)

	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusBadRequest)
}

func TestGetScheduleByDate_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/schedule/by-date", GetScheduleByDate)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/schedule/by-date?school=school1&grade=grade1&class=class1&date=2025-10-13", nil)
	router.ServeHTTP(w, req)

	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusBadRequest)
}
