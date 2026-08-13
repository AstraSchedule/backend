package client

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/model"
	"AstraScheduleServerGo/model/dbTable"
	"AstraScheduleServerGo/testutil"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
)

var testDBInitialized = false

func ensureTestDB() {
	if testDBInitialized {
		return
	}
	testutil.InitTestDB()
	db.GetDB().AutoMigrate(
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

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	return router
}

// doClientRequest 在 router 上执行无请求体的请求并返回 recorder，消除重复样板。
func doClientRequest(t *testing.T, router *gin.Engine, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req, err := http.NewRequest(method, path, nil)
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
		return nil
	}
	router.ServeHTTP(w, req)
	return w
}

// GetSchedule tests

func TestGetSchedule_Empty(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/:school/:grade/:class", GetSchedule)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/school1/grade1/class1", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// 契约：响应必须包含客户端消费的全部顶层字段（desktop/js/index.js 与 renderer.js 依赖）
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp, "supportWebSocket")
	assert.Contains(t, resp, "version")
	assert.Contains(t, resp, "daily_class")
	assert.Contains(t, resp, "countdown_target")
	assert.Contains(t, resp, "startup_behavior")
	assert.Contains(t, resp, "banner_text")
	assert.Contains(t, resp, "css_style")
	assert.Contains(t, resp, "temperature_colors")
	assert.Contains(t, resp, "timetable")
	assert.Contains(t, resp, "divider")
	assert.Contains(t, resp, "subject_name")
	assert.Contains(t, resp, "countdown_records")

	// daily_class 必须为 7 项的扁平化数组（desktop 按 index 取星期几）
	dailyClass, ok := resp["daily_class"].([]interface{})
	assert.True(t, ok, "daily_class 应为数组")
	assert.Equal(t, 7, len(dailyClass))
	for _, day := range dailyClass {
		d, ok := day.(map[string]interface{})
		assert.True(t, ok)
		assert.Contains(t, d, "Chinese")
		assert.Contains(t, d, "English")
		assert.Contains(t, d, "classList")
		assert.Contains(t, d, "timetable")
	}

	// 空数据时嵌套 map 应为空对象而非 null（客户端解构不崩溃）
	assert.IsType(t, map[string]interface{}{}, resp["timetable"])
	assert.IsType(t, map[string]interface{}{}, resp["divider"])
	assert.IsType(t, map[string]interface{}{}, resp["subject_name"])
	assert.IsType(t, []interface{}{}, resp["countdown_records"])
}

func TestGetSchedule_DataContract(t *testing.T) {
	ensureTestDB()

	database := db.GetDB()
	// 使用独立 scope，避免与其它测试共享数据
	database.Save(&dbTable.Schedule{
		Namespace: "default",
		School:    "contract", Grade: "2024", Class: "1",
		DailyClasses: [7]dbTable.DailyClass{
			{Timetable: "常日", ClassList: dbTable.ClassList{{"数", "代"}, {"语"}, {"英"}}},
		},
	})
	database.Save(&dbTable.Subject{
		Namespace: "default",
		School:    "contract", Grade: "2024",
		SubjectConfig: dbTable.SubjectConfig{SubjectName: map[string]string{"数": "数学", "语": "语文"}},
	})
	database.Save(&dbTable.Timetable{
		Namespace: "default",
		School:    "contract", Grade: "2024",
		TimetableConfig: dbTable.TimetableConfig{
			Timetable: map[string]map[string]interface{}{"常日": {"早上1": 1, "早上2": 2}},
			Divider:   map[string][]int{"常日": {1}},
		},
	})
	database.Save(&dbTable.CountdownRecord{
		Namespace: "default",
		ID:        "contract-cd", Scope: []string{"ALL"},
		Schedules: []dbTable.CountdownScheduleItem{{Name: "期末", Date: "2026-01-01", Priority: 1}},
	})

	router := setupTestRouter()
	router.GET("/:school/:grade/:class", GetSchedule)

	w := doClientRequest(t, router, "GET", "/contract/2024/1")

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// 多周轮换：week 1 取第一个课程，扁平化 classList
	dailyClass := resp["daily_class"].([]interface{})
	day0 := dailyClass[0].(map[string]interface{})
	assert.Equal(t, "常日", day0["timetable"])
	assert.Equal(t, []interface{}{"数", "语", "英"}, day0["classList"])

	// subject_name / timetable / divider 映射
	assert.Equal(t, "数学", resp["subject_name"].(map[string]interface{})["数"])
	assert.Contains(t, resp["timetable"].(map[string]interface{}), "常日")
	assert.Equal(t, []interface{}{float64(1)}, resp["divider"].(map[string]interface{})["常日"])

	// 倒数日按 scope 过滤后返回
	countdowns := resp["countdown_records"].([]interface{})
	assert.Equal(t, 1, len(countdowns))
}

func TestGetSchedule_NotModified(t *testing.T) {
	ensureTestDB()

	database := db.GetDB()
	database.Save(&dbTable.DataVersion{
		Namespace: "default",
		School:    "nms", Grade: "2024", Class: "1",
		Version: time.Now(),
	})

	router := setupTestRouter()
	router.GET("/:school/:grade/:class", GetSchedule)

	// 不带 version -> 200
	w := doClientRequest(t, router, "GET", "/nms/2024/1")
	assert.Equal(t, http.StatusOK, w.Code)

	// 带与服务器一致的 version -> 304（desktop 依赖增量同步）
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	v := resp["version"].(string)

	w2 := doClientRequest(t, router, "GET", "/nms/2024/1?version="+v)
	assert.Equal(t, http.StatusNotModified, w2.Code)
}

func TestGetSchedule_WithVersion(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/:school/:grade/:class", GetSchedule)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/school1/grade1/class1?version=0", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetSchedule_InvalidVersion(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/:school/:grade/:class", GetSchedule)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/school1/grade1/class1?version=invalid", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// GetWeather tests — 使用 mock HTTP server 替代真实和风天气 API

func setupMockWeatherServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	// 城市查询
	mux.HandleFunc("/geo/v2/city/lookup", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": "200",
			"location": []map[string]interface{}{
				{"id": "101010100", "lat": "39.904", "lon": "116.407", "name": "北京"},
			},
		})
	})

	// 实时天气
	mux.HandleFunc("/v7/weather/now", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"now": map[string]string{
				"temp": "25", "text": "晴", "windDir": "北风", "windScale": "3",
			},
		})
	})

	// 天气预警
	mux.HandleFunc("/weatheralert/v1/current/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"alerts": []interface{}{},
		})
	})

	// TLS mock server — 生产代码硬编码 https://
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)

	// 注入跳过 TLS 验证的 resty 客户端
	origFactory := newRestyClient
	newRestyClient = func() *resty.Client {
		c := resty.New().SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
		return c
	}
	t.Cleanup(func() { newRestyClient = origFactory })

	return srv
}

func TestGetWeatherWithProvince_Success(t *testing.T) {
	ensureTestDB()
	mock := setupMockWeatherServer(t)

	// 指向 mock server（去掉 scheme）
	origHost := model.Configs.APIKey.APIHost
	model.Configs.APIKey.APIHost = strings.TrimPrefix(mock.URL, "https://")
	t.Cleanup(func() { model.Configs.APIKey.APIHost = origHost })

	router := setupTestRouter()
	router.GET("/api/weather/:name1/:name2", GetWeatherWithProvince)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/weather/北京/朝阳", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp model.WeatherResponse
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "北京", resp.Where)
	assert.Equal(t, "25", resp.Temp)
	assert.Equal(t, "晴", resp.Weat)
}

func TestGetWeatherWithCity_Success(t *testing.T) {
	ensureTestDB()
	mock := setupMockWeatherServer(t)

	origHost := model.Configs.APIKey.APIHost
	model.Configs.APIKey.APIHost = strings.TrimPrefix(mock.URL, "https://")
	t.Cleanup(func() { model.Configs.APIKey.APIHost = origHost })

	router := setupTestRouter()
	router.GET("/api/weather/:name1", GetWeatherWithCity)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/weather/北京", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp model.WeatherResponse
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "北京", resp.Where)
	assert.Equal(t, "25", resp.Temp)
}

func TestGetWeatherWithCFHeader_NoCFHeader(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/api/weather/", GetWeatherWithCFHeader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/weather/", nil)
	router.ServeHTTP(w, req)

	// 没有 CF-IPCity 头时应返回 400
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetWeatherWithCFHeader_Success(t *testing.T) {
	ensureTestDB()
	mock := setupMockWeatherServer(t)

	origHost := model.Configs.APIKey.APIHost
	model.Configs.APIKey.APIHost = strings.TrimPrefix(mock.URL, "https://")
	t.Cleanup(func() { model.Configs.APIKey.APIHost = origHost })

	router := setupTestRouter()
	router.GET("/api/weather/", GetWeatherWithCFHeader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/weather/", nil)
	req.Header.Set("CF-IPCity", "北京")
	req.Header.Set("CF-Region", "北京")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp model.WeatherResponse
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "北京", resp.Where)
	assert.Equal(t, "25", resp.Temp)
}

// WebSocket tests

func TestWebSocketPlaceholder(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.Any("/ws/:school/:grade/:class_number", WebSocketPlaceholder)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/ws/school1/grade1/class1", nil)
	router.ServeHTTP(w, req)

	// WebSocket upgrade will fail in test, but handler should not panic
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusUpgradeRequired || w.Code == http.StatusBadRequest)
}

// BroadcastSyncConfig tests

func TestBroadcastSyncConfig_NoAuth(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.POST("/api/broadcast/:school/:grade/:class_number", BroadcastSyncConfig)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/broadcast/school1/grade1/class1", nil)
	router.ServeHTTP(w, req)

	// Without auth, should fail or succeed depending on middleware
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusUnauthorized)
}
