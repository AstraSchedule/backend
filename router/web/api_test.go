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
	"github.com/stretchr/testify/require"
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

func TestGetMenu_Contract(t *testing.T) {
	ensureTestDB()

	// 造一所带年级/班级的学校，验证菜单树节点契约
	db.GetDB().Save(&dbTable.Schedule{School: "menu-school", Grade: "2024", Class: "1"})

	router := setupTestRouter()
	router.GET("/web/menu", GetMenu)

	w := doRequest(t, router, "GET", "/web/menu", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data, ok := resp["data"].([]interface{})
	assert.True(t, ok, "data 应为数组")
	assert.GreaterOrEqual(t, len(data), 5) // 4 个基础项 + 1 所学校

	sawBase := 0
	for _, item := range data {
		m, ok := item.(map[string]interface{})
		assert.True(t, ok, "menu item 应为对象")
		assert.Contains(t, m, "text")
		assert.Contains(t, m, "key")
		// 基础页项必须带 to；scope 树节点必须带 raw（usr-dashboard buildScopeTreeFromMenu 依赖）
		if _, hasTo := m["to"]; hasTo {
			sawBase++
			continue
		}
		assert.Contains(t, m, "raw", "scope 树节点必须包含 raw 字段")
		if m["text"] == "menu-school 学校" {
			children := m["children"].([]interface{})
			assert.NotEmpty(t, children, "学校节点应包含年级")
			grade, ok := children[0].(map[string]interface{})
			require.True(t, ok, "年级节点应为对象")
			assert.Contains(t, grade, "raw")
			gradeChildren := grade["children"].([]interface{})
			assert.NotEmpty(t, gradeChildren, "年级节点应包含班级")
			classNode, ok := gradeChildren[len(gradeChildren)-1].(map[string]interface{})
			require.True(t, ok, "班级节点应为对象")
			assert.Contains(t, classNode, "raw")
		}
	}
	assert.Equal(t, 4, sawBase, "菜单应包含 4 个基础页项")
}

func TestGetStructure_Contract(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/structure", GetStructure)

	w := doRequest(t, router, "GET", "/web/structure", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// 契约：返回数组，每项含 text/children（usr-dashboard Users.vue/Structure.vue 依赖）
	var resp []map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	for _, s := range resp {
		assert.Contains(t, s, "text")
		assert.Contains(t, s, "children")
	}
}

// Config handlers tests

func TestGetSubjectsOptions_Contract(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/config/:school/:grade/subjects/options", GetSubjectsOptions)

	// 空数据：options 必须为空数组（usr-dashboard fetchSubjectsOptions 依赖数组形状）
	w := doRequest(t, router, "GET", "/web/config/school1/grade1/subjects/options", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.IsType(t, []interface{}{}, resp["options"])

	// 有数据：每项必须包含 label/value
	db.GetDB().Save(&dbTable.Subject{
		School: "school1", Grade: "grade1",
		SubjectConfig: dbTable.SubjectConfig{SubjectName: map[string]string{"数": "数学"}},
	})
	w = doRequest(t, router, "GET", "/web/config/school1/grade1/subjects/options", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	options := resp["options"].([]interface{})
	assert.Len(t, options, 1)
	opt, ok := options[0].(map[string]interface{})
	require.True(t, ok, "options 项应为对象")
	assert.Contains(t, opt, "label")
	assert.Contains(t, opt, "value")
	assert.Equal(t, "数", opt["value"])
}

func TestGetSubjects_Contract(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/config/:school/:grade/subjects", GetSubjects)

	w := doRequest(t, router, "GET", "/web/config/school1/grade1/subjects", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	// 契约：abbr/fullName 均为 [{text}] 数组（usr-dashboard SubjectsConfig.vue 依赖）
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.IsType(t, []interface{}{}, resp["abbr"])
	assert.IsType(t, []interface{}{}, resp["fullName"])
}

func TestGetTimetableOptions_Contract(t *testing.T) {
	ensureTestDB()

	db.GetDB().Save(&dbTable.Timetable{
		School: "school1", Grade: "grade1",
		TimetableConfig: dbTable.TimetableConfig{
			Timetable: map[string]map[string]interface{}{"常日": {"08:00-08:40": 0, "08:50-09:30": 1}},
			Divider:   map[string][]int{"常日": {}},
		},
	})

	router := setupTestRouter()
	router.GET("/web/config/:school/:grade/timetable/options", GetTimetableOptions)

	w := doRequest(t, router, "GET", "/web/config/school1/grade1/timetable/options", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	// 契约：options 为 [{label,value,need}]（usr-dashboard fetchTimetableOptions 依赖）
	// need 语义：period 下标从 0 起，need = 最大下标 + 1（即所需课节行数）
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	options := resp["options"].([]interface{})
	assert.Len(t, options, 1)
	opt, ok := options[0].(map[string]interface{})
	require.True(t, ok, "options 项应为对象")
	assert.Equal(t, "常日", opt["label"])
	assert.Equal(t, "常日", opt["value"])
	assert.Equal(t, float64(2), opt["need"], "两个节次(下标0/1)应得出 need=2")
}

func TestGetTimetable_Contract(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/config/:school/:grade/timetable", GetTimetable)

	w := doRequest(t, router, "GET", "/web/config/school1/grade1/timetable", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	// 契约：顶层包含 start/timetable/divider（usr-dashboard TimetableConfig.vue 依赖）
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp, "timetable")
	assert.Contains(t, resp, "divider")
}

func TestGetScheduleConfig_Contract(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/config/:school/:grade/:class_number/schedule", GetScheduleConfig)

	w := doRequest(t, router, "GET", "/web/config/school1/grade1/class1/schedule", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	// 契约：daily_class 为 7 天数组，每天含 Chinese/English/classList(嵌套)/timetable
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	daily, ok := resp["daily_class"].([]interface{})
	assert.True(t, ok, "daily_class 应为数组")
	assert.Equal(t, 7, len(daily))
	for _, d := range daily {
		day, ok := d.(map[string]interface{})
		require.True(t, ok, "daily_class 项应为对象")
		assert.Contains(t, day, "Chinese")
		assert.Contains(t, day, "English")
		assert.Contains(t, day, "classList")
		assert.Contains(t, day, "timetable")
	}
}

func TestGetSettings_Contract(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/config/:school/:grade/:class_number/settings", GetSettings)

	w := doRequest(t, router, "GET", "/web/config/school1/grade1/class1/settings", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	// 契约：返回 ClientConfigItems 的完整字段（desktop 渲染与 SettingsConfig.vue 依赖）
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	for _, key := range []string{
		"countdown_target", "weather_alert_override", "weather_alert_brief",
		"week_display", "banner_text", "css_style", "startup_behavior", "temperature_colors",
	} {
		assert.Contains(t, resp, key)
	}
}

// Autorun handlers tests

func TestGetAutorunStatus_Contract(t *testing.T) {
	ensureTestDB()

	db.GetDB().Save(&dbTable.AutorunRecord{
		HashID: "contract-hash", EType: dbTable.AutorunTypeCompensation, Scope: []string{"ALL"}, Level: 1, Status: 0,
		Parameters: map[string]interface{}{"rule": map[string]interface{}{"date": "2025-10-01", "useDate": "2025-09-29"}},
	})

	router := setupTestRouter()
	router.GET("/web/autorun", GetAutorunStatus)

	w := doRequest(t, router, "GET", "/web/autorun", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	// 契约：data 为数组，每项含 id/type/priority/status/scope/content（usr-dashboard listTasks 依赖）
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].([]interface{})
	assert.NotEmpty(t, data)
	item, ok := data[0].(map[string]interface{})
	require.True(t, ok, "列表项应为对象")
	assert.Equal(t, "contract-hash", item["id"])
	assert.Equal(t, "COMPENSATION", item["type"])
	assert.Contains(t, item, "priority")
	assert.Contains(t, item, "status")
	assert.Contains(t, item, "scope")
	content, ok := item["content"].(map[string]interface{})
	require.True(t, ok, "content 应为对象")
	assert.Equal(t, "2025-10-01", content["date"])
}

func TestGetAutorunHashStatus_NotFound(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/autorun/hash/:hashid", GetAutorunHashStatus)

	w := doRequest(t, router, "GET", "/web/autorun/hash/nonexistent", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	// 契约：未命中返回空数组（usr-dashboard getTask 依赖数组/对象形状区分）
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.IsType(t, []interface{}{}, resp["data"])
}

// Countdown handlers tests

func TestGetCountdownStatus_Contract(t *testing.T) {
	ensureTestDB()

	db.GetDB().Save(&dbTable.CountdownRecord{
		ID: "cd-contract", Scope: []string{"ALL"},
		Schedules: []dbTable.CountdownScheduleItem{{Name: "期末", Date: "2026-01-01", Priority: 1}},
	})
	db.GetDB().Save(&dbTable.CountdownRecord{
		ID: "cd-scoped", Scope: []string{"other-school"},
		Schedules: []dbTable.CountdownScheduleItem{{Name: "其他", Date: "2026-02-01", Priority: 1}},
	})

	router := setupTestRouter()
	router.GET("/web/countdown", GetCountdownStatus)

	// 契约：loading/hasConfig/data 三键 + 记录形状（按 id 查找，不依赖返回顺序）
	w := doRequest(t, router, "GET", "/web/countdown", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp, "loading")
	assert.Equal(t, true, resp["hasConfig"])
	data := resp["data"].([]interface{})
	assert.Len(t, data, 2)
	var item map[string]interface{}
	for _, raw := range data {
		m, ok := raw.(map[string]interface{})
		if ok && m["id"] == "cd-contract" {
			item = m
		}
	}
	require.NotNil(t, item, "cd-contract 应出现在列表")
	assert.Contains(t, item, "scope")
	schedules := item["schedules"].([]interface{})
	first, ok := schedules[0].(map[string]interface{})
	require.True(t, ok, "schedules 项应为对象")
	assert.Equal(t, "期末", first["name"])

	// 契约：?scope= 过滤（usr-dashboard listCountdown 带 scope 查询）
	w = doRequest(t, router, "GET", "/web/countdown?scope=school1/grade1/class1", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data = resp["data"].([]interface{})
	assert.Len(t, data, 1)
	only, ok := data[0].(map[string]interface{})
	require.True(t, ok, "过滤结果应为对象")
	assert.Equal(t, "cd-contract", only["id"])
}

func TestGetCountdownByID_NotFound(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/countdown/:id", GetCountdownByID)

	w := doRequest(t, router, "GET", "/web/countdown/nonexistent", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	// 契约：未命中返回空数组（usr-dashboard getCountdown 依赖数组/对象形状区分）
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.IsType(t, []interface{}{}, resp["data"])
}

// Compensation handlers tests

func TestCompensationFromHoliday_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/autorun/compensation/holiday/:year/:month/:day", CompensationFromHoliday)

	// 2025-10-01 是法定节假日，应有调休结果
	w := doRequest(t, router, "GET", "/web/autorun/compensation/holiday/2025/10/01", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	// 契约：{date, compensation}（usr-dashboard fetchCompByHoliday 依赖）
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "2025-10-01", resp["date"])
	assert.Contains(t, resp, "compensation")

	// 非法日期 -> 400
	w = doRequest(t, router, "GET", "/web/autorun/compensation/holiday/2025/13/40", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCompensationFromWorkday_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/autorun/compensation/workday/:year/:month/:day", CompensationFromWorkday)

	w := doRequest(t, router, "GET", "/web/autorun/compensation/workday/2025/10/13", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	// 契约：{date, compensation}（usr-dashboard fetchCompByWorkday 依赖）
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "2025-10-13", resp["date"])
	assert.Contains(t, resp, "compensation")

	// 非法日期 -> 400
	w = doRequest(t, router, "GET", "/web/autorun/compensation/workday/2025/02/30", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCompensationFromYear_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/autorun/compensation/year/:year", CompensationFromYear)

	w := doRequest(t, router, "GET", "/web/autorun/compensation/year/2025", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	// 契约：{year, pairs:[{holiday, workday}]}（usr-dashboard fetchCompYearPairs 依赖）
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(2025), resp["year"])
	pairs := resp["pairs"].([]interface{})
	assert.NotEmpty(t, pairs, "2025 年应存在调休对")
	first, ok := pairs[0].(map[string]interface{})
	require.True(t, ok, "pairs 项应为对象")
	assert.Contains(t, first, "holiday")
	assert.Contains(t, first, "workday")

	// 非法年份 -> 400
	w = doRequest(t, router, "GET", "/web/autorun/compensation/year/abc", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetScheduleByDate_Success(t *testing.T) {
	ensureTestDB()

	// 造一条课表 + 作息，验证 by-date 输出真实的课节映射
	db.GetDB().Save(&dbTable.Timetable{
		School: "bydate", Grade: "2024",
		TimetableConfig: dbTable.TimetableConfig{
			Timetable: map[string]map[string]interface{}{"常日": {"08:00-08:40": 0, "08:50-09:30": 1}},
			Divider:   map[string][]int{"常日": {}},
		},
	})
	db.GetDB().Save(&dbTable.Schedule{
		School: "bydate", Grade: "2024", Class: "1",
		DailyClasses: [7]dbTable.DailyClass{
			{}, // Sunday
			{Timetable: "常日", ClassList: dbTable.ClassList{{"数"}, {"语"}}}, // Monday
			{}, {}, {}, {}, {},
		},
	})

	router := setupTestRouter()
	router.GET("/web/schedule/by-date", GetScheduleByDate)

	// 契约：handler 读取 scope 参数（school/grade/class），而不是三个独立参数
	w := doRequest(t, router, "GET", "/web/schedule/by-date?scope=bydate/2024/1&date=2025-10-13", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data, ok := resp["data"].(map[string]interface{})
	assert.True(t, ok, "响应应包含 data 对象")
	periods, ok := data["periods"].([]interface{})
	assert.True(t, ok, "data 应包含 periods 数组")
	assert.Len(t, periods, 2)
	p0, ok := periods[0].(map[string]interface{})
	require.True(t, ok, "periods 项应为对象")
	assert.Equal(t, float64(1), p0["no"])
	assert.Equal(t, "数", p0["subject"])
	p1, ok := periods[1].(map[string]interface{})
	require.True(t, ok, "periods 项应为对象")
	assert.Equal(t, "语", p1["subject"])
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

	w := doRawRequest(t, router, "PUT", "/web/config/school1/grade1/subjects", "invalid")

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPutSubjects_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/config/:school/:grade/subjects", PutSubjects)
	router.GET("/web/config/:school/:grade/subjects/options", GetSubjectsOptions)

	body := map[string]interface{}{
		"abbr":     []map[string]interface{}{{"text": "数"}, {"text": "语"}},
		"fullName": []map[string]interface{}{{"text": "数学"}, {"text": "语文"}},
	}
	w := doRequest(t, router, "PUT", "/web/config/school1/grade1/subjects", body)
	assert.Equal(t, http.StatusOK, w.Code)

	// 回读验证持久化
	w = doRequest(t, router, "GET", "/web/config/school1/grade1/subjects/options", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	options := resp["options"].([]interface{})
	assert.Len(t, options, 2)
	values := map[string]bool{}
	for _, o := range options {
		opt, ok := o.(map[string]interface{})
		require.True(t, ok, "options 项应为对象")
		value, ok := opt["value"].(string)
		require.True(t, ok, "options 项应含 value")
		values[value] = true
	}
	assert.True(t, values["数"])
	assert.True(t, values["语"])
}

func TestPutTimetable_InvalidJSON(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/config/:school/:grade/timetable", PutTimetable)

	w := doRawRequest(t, router, "PUT", "/web/config/school1/grade1/timetable", "invalid")

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPutTimetable_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/config/:school/:grade/timetable", PutTimetable)
	router.GET("/web/config/:school/:grade/timetable", GetTimetable)

	body := map[string]interface{}{
		"timetable": map[string]interface{}{
			"常日": map[string]interface{}{"08:00-08:40": 0},
			"考试": map[string]interface{}{"08:00-08:40": 0},
		},
		"divider": map[string]interface{}{},
	}
	w := doRequest(t, router, "PUT", "/web/config/school1/grade1/timetable", body)
	assert.Equal(t, http.StatusOK, w.Code)

	// 回读验证持久化与 divider key 同步
	w = doRequest(t, router, "GET", "/web/config/school1/grade1/timetable", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	timetable, ok := resp["timetable"].(map[string]interface{})
	require.True(t, ok, "timetable 应为对象")
	assert.Contains(t, timetable, "常日")
	assert.Contains(t, timetable, "考试")
	divider, ok := resp["divider"].(map[string]interface{})
	require.True(t, ok, "divider 应为对象")
	assert.Contains(t, divider, "常日", "divider 键应与 timetable 同步")
	assert.Contains(t, divider, "考试")
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
	w := doRequest(t, router, "PUT", "/web/config/school1/grade1/timetable", body)

	// 契约：必须保留“常日”作息表
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPutScheduleConfig_InvalidJSON(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/config/:school/:grade/:class_number/schedule", PutScheduleConfig)

	w := doRawRequest(t, router, "PUT", "/web/config/school1/grade1/class1/schedule", "invalid")

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPutScheduleConfig_MissingDailyClass(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/config/:school/:grade/:class_number/schedule", PutScheduleConfig)

	// 契约：缺 daily_class 或长度不为 7 必须 400，防止坏请求清空课表
	w := doRequest(t, router, "PUT", "/web/config/school1/grade1/class1/schedule",
		map[string]interface{}{"timetable": "常日"})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = doRequest(t, router, "PUT", "/web/config/school1/grade1/class1/schedule",
		map[string]interface{}{"daily_class": []interface{}{map[string]interface{}{"Chinese": "一"}}})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 7 项但存在非对象条目也必须 400，防止对应日期被写入零值课表
	w = doRequest(t, router, "PUT", "/web/config/school1/grade1/class1/schedule",
		map[string]interface{}{"daily_class": []interface{}{nil, "bad", 1, 2.5, true, []interface{}{}, map[string]interface{}{}}})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPutScheduleConfig_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/config/:school/:grade/:class_number/schedule", PutScheduleConfig)
	router.GET("/web/config/:school/:grade/:class_number/schedule", GetScheduleConfig)

	// 契约：请求体为 7 天的 daily_class 数组（usr-dashboard ScheduleConfig.vue 的提交格式）
	day := func(chinese string) map[string]interface{} {
		return map[string]interface{}{
			"Chinese":   chinese,
			"English":   "X",
			"timetable": "常日",
			"classList": []interface{}{[]interface{}{"数"}},
		}
	}
	days := []interface{}{}
	for _, name := range []string{"日", "一", "二", "三", "四", "五", "六"} {
		days = append(days, day(name))
	}
	body := map[string]interface{}{"daily_class": days}
	w := doRequest(t, router, "PUT", "/web/config/school1/grade1/class1/schedule", body)
	assert.Equal(t, http.StatusOK, w.Code)

	// 回读验证持久化：7 天结构与首日课程
	w = doRequest(t, router, "GET", "/web/config/school1/grade1/class1/schedule", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	daily, ok := resp["daily_class"].([]interface{})
	assert.True(t, ok, "daily_class 应为数组")
	assert.Equal(t, 7, len(daily))
	day0, ok := daily[0].(map[string]interface{})
	require.True(t, ok, "daily_class 项应为对象")
	assert.Equal(t, "日", day0["Chinese"])
	classList, ok := day0["classList"].([]interface{})
	require.True(t, ok, "classList 应为数组")
	slot, ok := classList[0].([]interface{})
	require.True(t, ok, "classList 项应为嵌套数组")
	assert.Equal(t, []interface{}{"数"}, slot)
}

func TestPutSettings_InvalidJSON(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/config/:school/:grade/:class_number/settings", PutSettings)

	w := doRawRequest(t, router, "PUT", "/web/config/school1/grade1/class1/settings", "invalid")

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPutSettings_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/config/:school/:grade/:class_number/settings", PutSettings)
	router.GET("/web/config/:school/:grade/:class_number/settings", GetSettings)

	body := map[string]interface{}{
		"countdown_target": "2025-12-31",
		"banner_text":      "欢迎",
	}
	w := doRequest(t, router, "PUT", "/web/config/school1/grade1/class1/settings", body)
	assert.Equal(t, http.StatusOK, w.Code)

	// 回读验证持久化
	w = doRequest(t, router, "GET", "/web/config/school1/grade1/class1/settings", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "2025-12-31", resp["countdown_target"])
	assert.Equal(t, "欢迎", resp["banner_text"])
}

func TestCopyConfig_InvalidJSON(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.POST("/web/config/copy", CopyConfig)

	w := doRawRequest(t, router, "POST", "/web/config/copy", "invalid")

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCopyConfig_MissingSource(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.POST("/web/config/copy", adminOnly(t), CopyConfig)

	body := map[string]interface{}{
		"from": map[string]interface{}{"school": "nosrc", "grade": "g", "class": "c"},
		"to":   map[string]interface{}{"school": "dst", "grade": "g", "class": "c"},
	}
	w := doRequest(t, router, "POST", "/web/config/copy", body)

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
	router.POST("/web/config/copy", adminOnly(t), CopyConfig)

	body := map[string]interface{}{
		"from": map[string]interface{}{"school": "src", "grade": "g", "class": "c"},
		"to":   map[string]interface{}{"school": "dst", "grade": "g", "class": "c"},
	}
	w := doRequest(t, router, "POST", "/web/config/copy", body)

	assert.Equal(t, http.StatusOK, w.Code)
	// 契约：响应为复制配置自身的 from/to 结构，而不是内部广播的 SyncConfig 载荷
	var copyResp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &copyResp))
	assert.Contains(t, copyResp, "from")
	assert.Contains(t, copyResp, "to")

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
		"type": dbTable.AutorunTypeCompensation, "scope": []string{"ALL"}, "priority": 1,
		"content": map[string]interface{}{"date": "invalid", "useDate": "2025-09-29"},
	}
	w := doRequest(t, router, "PUT", "/web/autorun/compensation", body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPutCompensationRule_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/autorun/compensation", adminOnly(t), PutCompensationRule)
	router.GET("/web/autorun", GetAutorunStatus)

	body := map[string]interface{}{
		"type": dbTable.AutorunTypeCompensation, "scope": []string{"ALL"}, "priority": 1,
		"content": map[string]interface{}{"date": "2025-10-01", "useDate": "2025-09-29"},
	}
	w := doRequest(t, router, "PUT", "/web/autorun/compensation", body)
	assert.Equal(t, http.StatusOK, w.Code)

	// 回读验证持久化：规则出现在 /web/autorun 列表且字段正确
	var putResp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &putResp))
	hashID := putResp["id"].(string)
	assert.NotEmpty(t, hashID)

	w = doRequest(t, router, "GET", "/web/autorun", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].([]interface{})
	found := false
	for _, item := range data {
		m, ok := item.(map[string]interface{})
		require.True(t, ok, "列表项应为对象")
		if m["id"] == hashID {
			found = true
			assert.Equal(t, "COMPENSATION", m["type"])
			content, ok := m["content"].(map[string]interface{})
			require.True(t, ok, "content 应为对象")
			assert.Equal(t, "2025-10-01", content["date"])
			assert.Equal(t, "2025-09-29", content["useDate"])
		}
	}
	assert.True(t, found, "写入的调休规则应出现在列表")
}

func TestPutTimetableRule_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/autorun/timetable", adminOnly(t), PutTimetableRule)
	router.GET("/web/autorun/hash/:hashid", GetAutorunHashStatus)

	body := map[string]interface{}{
		"type": dbTable.AutorunTypeTimetable, "scope": []string{"ALL"}, "priority": 1,
		"content": map[string]interface{}{"date": "2025-10-08", "timetableId": "exam"},
	}
	w := doRequest(t, router, "PUT", "/web/autorun/timetable", body)

	assert.Equal(t, http.StatusOK, w.Code)

	// 回读验证持久化
	item := fetchAutorunDetail(t, router, w)
	assert.Equal(t, "TIMETABLE", item["type"])
	assert.Equal(t, "exam", autorunContent(t, item)["timetableId"])
}

func TestPutScheduleRule_MissingPeriods(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/autorun/schedule", PutScheduleRule)

	body := map[string]interface{}{
		"type": dbTable.AutorunTypeSchedule, "scope": []string{"ALL"}, "priority": 1,
		"content": map[string]interface{}{"date": "2025-10-09", "schedule": map[string]interface{}{}},
	}
	w := doRequest(t, router, "PUT", "/web/autorun/schedule", body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPutScheduleRule_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/autorun/schedule", adminOnly(t), PutScheduleRule)
	router.GET("/web/autorun/hash/:hashid", GetAutorunHashStatus)

	body := map[string]interface{}{
		"type": dbTable.AutorunTypeSchedule, "scope": []string{"ALL"}, "priority": 1,
		"content": map[string]interface{}{
			"date":     "2025-10-09",
			"schedule": map[string]interface{}{"periods": []interface{}{map[string]interface{}{"no": 1, "subject": "数"}}},
		},
	}
	w := doRequest(t, router, "PUT", "/web/autorun/schedule", body)

	assert.Equal(t, http.StatusOK, w.Code)

	// 回读验证持久化
	item := fetchAutorunDetail(t, router, w)
	assert.Equal(t, "SCHEDULE", item["type"])
	schedule, ok := autorunContent(t, item)["schedule"].(map[string]interface{})
	require.True(t, ok, "content.schedule 应为对象")
	periods, ok := schedule["periods"].([]interface{})
	require.True(t, ok, "periods 应为数组")
	assert.Len(t, periods, 1)
	p0, ok := periods[0].(map[string]interface{})
	require.True(t, ok, "period 应为对象")
	assert.Equal(t, float64(1), p0["no"])
	assert.Equal(t, "数", p0["subject"])
}

func TestPutAllRule_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/autorun/all", adminOnly(t), PutAllRule)
	router.GET("/web/autorun/hash/:hashid", GetAutorunHashStatus)

	body := map[string]interface{}{
		"type": dbTable.AutorunTypeAll, "scope": []string{"ALL"}, "priority": 1,
		"content": map[string]interface{}{
			"date":        "2025-10-10",
			"timetableId": "exam",
			"schedule":    map[string]interface{}{"periods": []interface{}{map[string]interface{}{"no": 1, "subject": "班会"}}},
		},
	}
	w := doRequest(t, router, "PUT", "/web/autorun/all", body)

	assert.Equal(t, http.StatusOK, w.Code)

	// 回读验证持久化
	item := fetchAutorunDetail(t, router, w)
	assert.Equal(t, "ALL", item["type"])
	content := autorunContent(t, item)
	assert.Equal(t, "exam", content["timetableId"])
	schedule, ok := content["schedule"].(map[string]interface{})
	require.True(t, ok, "content.schedule 应为对象")
	periods, ok := schedule["periods"].([]interface{})
	require.True(t, ok, "periods 应为数组")
	assert.Len(t, periods, 1)
	p0, ok := periods[0].(map[string]interface{})
	require.True(t, ok, "period 应为对象")
	assert.Equal(t, float64(1), p0["no"])
	assert.Equal(t, "班会", p0["subject"])
}

func TestDeleteAutorunRecord_NotFound(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.DELETE("/web/autorun/:hashid", DeleteAutorunRecord)

	w := doRequest(t, router, "DELETE", "/web/autorun/nonexistent-hash", nil)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteAutorunRecord_Success(t *testing.T) {
	ensureTestDB()

	// 先创建一条规则
	router := setupTestRouter()
	router.PUT("/web/autorun/compensation", adminOnly(t), PutCompensationRule)
	router.DELETE("/web/autorun/:hashid", DeleteAutorunRecord)
	router.GET("/web/autorun/hash/:hashid", GetAutorunHashStatus)

	body := map[string]interface{}{
		"type": dbTable.AutorunTypeCompensation, "scope": []string{"ALL"}, "priority": 1,
		"content": map[string]interface{}{"date": "2025-11-01", "useDate": "2025-10-31"},
	}
	w := doRequest(t, router, "PUT", "/web/autorun/compensation", body)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	hashID, _ := resp["id"].(string)
	assert.NotEmpty(t, hashID)

	// 再删除并回读验证
	w2 := doRequest(t, router, "DELETE", "/web/autorun/"+hashID, nil)
	assert.Equal(t, http.StatusOK, w2.Code)
	w3 := doRequest(t, router, "GET", "/web/autorun/hash/"+hashID, nil)
	assert.Equal(t, http.StatusOK, w3.Code)
	assert.NoError(t, json.Unmarshal(w3.Body.Bytes(), &resp))
	assert.IsType(t, []interface{}{}, resp["data"], "删除后详情应为空数组")
}

// Countdown 写接口测试

func TestPutCountdownRule_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/countdown", adminOnly(t), PutCountdownRule)
	router.GET("/web/countdown", GetCountdownStatus)

	body := map[string]interface{}{
		"scope": []string{"ALL"},
		"schedules": []map[string]interface{}{
			{"name": "期末考试", "date": "2026-01-01", "priority": 1},
		},
	}
	w := doRequest(t, router, "PUT", "/web/countdown", body)
	assert.Equal(t, http.StatusOK, w.Code)

	// 回读验证持久化
	var putResp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &putResp))
	id := putResp["id"].(string)
	assert.NotEmpty(t, id)

	w = doRequest(t, router, "GET", "/web/countdown", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].([]interface{})
	found := false
	for _, item := range data {
		m, ok := item.(map[string]interface{})
		require.True(t, ok, "列表项应为对象")
		if m["id"] == id {
			found = true
			schedules := m["schedules"].([]interface{})
			assert.Len(t, schedules, 1)
			s, ok := schedules[0].(map[string]interface{})
			require.True(t, ok, "schedules 项应为对象")
			assert.Equal(t, "期末考试", s["name"])
		}
	}
	assert.True(t, found, "写入的倒数日应出现在列表")
}

func TestDeleteCountdownRecord_NotFound(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.DELETE("/web/countdown/:id", DeleteCountdownRecord)

	w := doRequest(t, router, "DELETE", "/web/countdown/nonexistent-id", nil)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// Backup 接口测试

func TestExportBackup_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/backup/export", ExportBackup)

	w := doRequest(t, router, "GET", "/web/backup/export", nil)

	assert.Equal(t, http.StatusOK, w.Code)

	// 契约：响应应为完整备份 JSON（meta + 全部 8 个表数据段），usr-dashboard 整包上传依赖
	var payload map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	for _, key := range []string{
		"meta", "schedules", "client_configs", "timetables", "subjects",
		"data_versions", "autorun_records", "countdown_records", "users",
	} {
		assert.Contains(t, payload, key)
	}
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
