package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/model"
	"AstraScheduleServerGo/model/dbTable"
	"AstraScheduleServerGo/service"
	"AstraScheduleServerGo/testutil"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var contractDBInitialized = false

// setupContractEnv 初始化测试配置与内存库，并返回挂载了完整路由表与中间件链的 router。
// 与 web/client 包的 handler 级测试不同，这里走的是 main.go buildRouter() 的真实注册结果，
// 任何对路由、路径或认证中间件的误改都会导致本文件的测试失败。
func setupContractEnv(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	if !contractDBInitialized {
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
		contractDBInitialized = true
	}
	return buildRouter()
}

// contractRequest 通过完整 router 执行请求；body 为 nil 时发送空请求体。
func contractRequest(t *testing.T, router *gin.Engine, method, path string, body interface{}, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var payload string
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		payload = string(b)
	}
	req, err := http.NewRequest(method, path, strings.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func decodeJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

// createContractUser 创建测试用户并返回登录 token（默认 admin/test123）
func createContractUser(t *testing.T, username, password, role string) string {
	t.Helper()
	hash, err := service.HashPassword(password)
	require.NoError(t, err)
	require.NoError(t, db.GetDB().Where("username = ?", username).Delete(&dbTable.User{}).Error)
	require.NoError(t, db.GetDB().Create(&dbTable.User{Namespace: "default",
		Username: username, PasswordHash: hash, Role: role, Scope: "ALL",
	}).Error)

	w := contractRequest(t, buildRouter(), "POST", "/web/auth/login",
		map[string]string{"username": username, "password": password}, nil)
	require.Equal(t, http.StatusOK, w.Code)
	resp := decodeJSON(t, w)
	token, ok := resp["token"].(string)
	require.True(t, ok, "登录响应应包含 token")
	require.NotEmpty(t, token)
	return token
}

// TestRouteTable_AnonymousMatrix 无凭据时按真实路由表逐条断言状态码。
// 若某条路由被删除（404）、被改宽（404 变 200）或认证中间件被移除（401 变 200），本用例必红。
func TestRouteTable_AnonymousMatrix(t *testing.T) {
	router := setupContractEnv(t)

	// 天气路由断言需要无凭据环境（403 不发起上游请求），避免测试触碰真实网络
	origAPIKey := model.Configs.APIKey
	model.Configs.APIKey = model.APIKeyConfig{}
	t.Cleanup(func() { model.Configs.APIKey = origAPIKey })

	cases := []struct {
		name   string
		method string
		path   string
		body   interface{}
		want   int
	}{
		{"根路由", "GET", "/", nil, http.StatusOK},
		{"客户端课表读取", "GET", "/s1/g1/c1", nil, http.StatusOK},
		{"客户端课表写入需认证", "PUT", "/s1/g1/c1", nil, http.StatusUnauthorized},
		{"外部广播入口已废弃", "POST", "/api/broadcast/s1/g1/c1", nil, http.StatusNotFound},
		{"WebSocket 无升级头", "GET", "/ws/s1/g1/c1", nil, http.StatusBadRequest},
		{"天气省市区查询无凭据", "GET", "/api/weather/shanghai/pudong", nil, http.StatusForbidden},
		{"天气城市查询无凭据", "GET", "/api/weather/shanghai", nil, http.StatusForbidden},
		{"天气 CF 头查询无头", "GET", "/api/weather/", nil, http.StatusBadRequest},
		{"菜单", "GET", "/web/menu", nil, http.StatusOK},
		{"结构树", "GET", "/web/structure", nil, http.StatusOK},
		{"备份导出需认证", "GET", "/web/backup/export", nil, http.StatusUnauthorized},
		{"备份导入需认证", "POST", "/web/backup/import", nil, http.StatusUnauthorized},
		{"完整备份导出需认证", "POST", "/web/backup/full-export", nil, http.StatusUnauthorized},
		{"完整备份导入需认证", "POST", "/web/backup/full-import", nil, http.StatusUnauthorized},
		{"创建学校需认证", "POST", "/web/schools", map[string]string{"name": "x"}, http.StatusUnauthorized},
		{"删除学校需认证", "DELETE", "/web/schools/x", nil, http.StatusUnauthorized},
		{"创建年级需认证", "POST", "/web/schools/x/grades", map[string]string{"name": "g"}, http.StatusUnauthorized},
		{"删除年级需认证", "DELETE", "/web/schools/x/grades/g", nil, http.StatusUnauthorized},
		{"创建班级需认证", "POST", "/web/schools/x/grades/g/classes", map[string]string{"name": "c"}, http.StatusUnauthorized},
		{"删除班级需认证", "DELETE", "/web/schools/x/grades/g/classes/c", nil, http.StatusUnauthorized},
		{"科目选项", "GET", "/web/config/s1/g1/subjects/options", nil, http.StatusOK},
		{"科目读取", "GET", "/web/config/s1/g1/subjects", nil, http.StatusOK},
		{"科目写入需认证", "PUT", "/web/config/s1/g1/subjects", map[string]interface{}{}, http.StatusUnauthorized},
		{"作息选项", "GET", "/web/config/s1/g1/timetable/options", nil, http.StatusOK},
		{"作息读取", "GET", "/web/config/s1/g1/timetable", nil, http.StatusOK},
		{"作息写入需认证", "PUT", "/web/config/s1/g1/timetable", map[string]interface{}{}, http.StatusUnauthorized},
		{"课表配置读取", "GET", "/web/config/s1/g1/c1/schedule", nil, http.StatusOK},
		{"课表配置写入需认证", "PUT", "/web/config/s1/g1/c1/schedule", map[string]interface{}{}, http.StatusUnauthorized},
		{"通用设置读取", "GET", "/web/config/s1/g1/c1/settings", nil, http.StatusOK},
		{"通用设置写入需认证", "PUT", "/web/config/s1/g1/c1/settings", map[string]interface{}{}, http.StatusUnauthorized},
		{"复制配置需认证", "POST", "/web/config/copy", map[string]interface{}{}, http.StatusUnauthorized},
		{"自动任务列表", "GET", "/web/autorun", nil, http.StatusOK},
		{"自动任务详情", "GET", "/web/autorun/hash/nope", nil, http.StatusOK},
		{"删除自动任务需认证", "DELETE", "/web/autorun/nope", nil, http.StatusUnauthorized},
		{"写入调休规则需认证", "PUT", "/web/autorun/compensation", map[string]interface{}{}, http.StatusUnauthorized},
		{"写入作息规则需认证", "PUT", "/web/autorun/timetable", map[string]interface{}{}, http.StatusUnauthorized},
		{"写入课表规则需认证", "PUT", "/web/autorun/schedule", map[string]interface{}{}, http.StatusUnauthorized},
		{"写入组合规则需认证", "PUT", "/web/autorun/all", map[string]interface{}{}, http.StatusUnauthorized},
		{"倒数日列表", "GET", "/web/countdown", nil, http.StatusOK},
		{"倒数日详情", "GET", "/web/countdown/nope", nil, http.StatusOK},
		{"写入倒数日需认证", "PUT", "/web/countdown", map[string]interface{}{}, http.StatusUnauthorized},
		{"删除倒数日需认证", "DELETE", "/web/countdown/nope", nil, http.StatusUnauthorized},
		{"节假日调休查询", "GET", "/web/autorun/compensation/holiday/2025/10/01", nil, http.StatusOK},
		{"工作日调休查询", "GET", "/web/autorun/compensation/workday/2025/10/13", nil, http.StatusOK},
		{"年度调休对查询", "GET", "/web/autorun/compensation/year/2025", nil, http.StatusOK},
		{"按日期出课节", "GET", "/web/schedule/by-date?scope=s1/g1/c1&date=2025-10-13", nil, http.StatusOK},
		{"按日期出课节日期非法", "GET", "/web/schedule/by-date?scope=s1/g1/c1&date=x", nil, http.StatusBadRequest},
		{"按日期出课节 scope 非法", "GET", "/web/schedule/by-date?scope=s1&date=2025-10-13", nil, http.StatusBadRequest},
		{"删表接口需内部密钥", "DELETE", "/web/admin/drop-table/schedules", nil, http.StatusUnauthorized},
		{"租户注册需注册令牌", "POST", "/web/admin/register-tenant", nil, http.StatusUnauthorized},
		{"子域名检查需内部密钥", "GET", "/web/admin/check-subdomain/foo", nil, http.StatusUnauthorized},
		{"登录非法 JSON", "POST", "/web/auth/login", nil, http.StatusBadRequest},
		{"当前用户需认证", "GET", "/web/auth/me", nil, http.StatusUnauthorized},
		{"统计需 JWT 认证", "GET", "/web/statistic", nil, http.StatusUnauthorized},
		{"改密需认证", "POST", "/web/auth/change-password", nil, http.StatusUnauthorized},
		{"验证密码需认证", "POST", "/web/auth/verify-password", nil, http.StatusUnauthorized},
		{"用户列表需认证", "GET", "/web/users", nil, http.StatusUnauthorized},
		{"创建用户需认证", "POST", "/web/users", nil, http.StatusUnauthorized},
		{"更新用户需认证", "PUT", "/web/users/1", nil, http.StatusUnauthorized},
		{"删除用户需认证", "DELETE", "/web/users/1", nil, http.StatusUnauthorized},
		{"未注册路径返回 404", "GET", "/web/definitely-not-exists", nil, http.StatusNotFound},
		{"未注册方法返回 404", "PATCH", "/web/menu", nil, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := contractRequest(t, router, tc.method, tc.path, tc.body, nil)
			assert.Equal(t, tc.want, w.Code, "%s %s 期望 %d 实际 %d", tc.method, tc.path, tc.want, w.Code)
		})
	}
}

// TestRouteTable_AuthMatrix 覆盖 JWT、角色与密码验证组合的认证矩阵。
func TestRouteTable_AuthMatrix(t *testing.T) {
	router := setupContractEnv(t)
	adminToken := createContractUser(t, "admin1", "test123", "admin")
	readonlyToken := createContractUser(t, "reader1", "test123", "readonly")

	auth := func(token string) map[string]string {
		return map[string]string{"Authorization": "Bearer " + token}
	}
	authPwd := func(token, pwd string) map[string]string {
		return map[string]string{"Authorization": "Bearer " + token, "X-Verify-Password": pwd}
	}

	t.Run("admin 读取当前用户", func(t *testing.T) {
		w := contractRequest(t, router, "GET", "/web/auth/me", nil, auth(adminToken))
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "admin1", decodeJSON(t, w)["username"])
	})
	t.Run("readonly 读取当前用户", func(t *testing.T) {
		w := contractRequest(t, router, "GET", "/web/auth/me", nil, auth(readonlyToken))
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "reader1", decodeJSON(t, w)["username"])
	})
	t.Run("统计 JWT 认证后可用", func(t *testing.T) {
		w := contractRequest(t, router, "GET", "/web/statistic", nil, auth(adminToken))
		assert.Equal(t, http.StatusOK, w.Code)
		resp := decodeJSON(t, w)
		assert.Contains(t, resp, "weather_error")
		assert.Contains(t, resp, "clients_count")
		assert.Contains(t, resp, "serverless")
	})
	t.Run("admin 可列用户", func(t *testing.T) {
		w := contractRequest(t, router, "GET", "/web/users", nil, auth(adminToken))
		assert.Equal(t, http.StatusOK, w.Code)
	})
	t.Run("readonly 列用户被拒 403", func(t *testing.T) {
		w := contractRequest(t, router, "GET", "/web/users", nil, auth(readonlyToken))
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
	t.Run("readonly 创建用户被拒 403", func(t *testing.T) {
		w := contractRequest(t, router, "POST", "/web/users",
			map[string]string{"username": "x1", "password": "test123", "role": "readonly"}, auth(readonlyToken))
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
	t.Run("readonly 写接口被拒 403", func(t *testing.T) {
		// 只读用户即使密码正确也不能通过写接口认证
		w := contractRequest(t, router, "PUT", "/web/countdown", map[string]interface{}{}, authPwd(readonlyToken, "test123"))
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
	t.Run("写接口缺密码被拒 401", func(t *testing.T) {
		w := contractRequest(t, router, "PUT", "/web/countdown", map[string]interface{}{}, auth(adminToken))
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
	t.Run("写接口密码错误被拒 401", func(t *testing.T) {
		w := contractRequest(t, router, "PUT", "/web/countdown", map[string]interface{}{}, authPwd(adminToken, "wrong"))
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
	t.Run("写接口密码正确放行", func(t *testing.T) {
		body := map[string]interface{}{
			"scope": []string{"ALL"},
			"schedules": []map[string]interface{}{
				{"name": "期末", "date": "2026-01-01", "priority": 1},
			},
		}
		w := contractRequest(t, router, "PUT", "/web/countdown", body, authPwd(adminToken, "test123"))
		assert.Equal(t, http.StatusOK, w.Code)
	})
	t.Run("篡改 token 被拒 401", func(t *testing.T) {
		w := contractRequest(t, router, "GET", "/web/auth/me", nil, auth("invalid.token.here"))
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// TestRouteTable_StructureCRUDFlow 走真实路由验证学校/年级/班级的增删流程与默认数据。
func TestRouteTable_StructureCRUDFlow(t *testing.T) {
	router := setupContractEnv(t)
	token := createContractUser(t, "admin2", "test123", "admin")
	h := map[string]string{"Authorization": "Bearer " + token, "X-Verify-Password": "test123"}

	// 创建学校（仅登记，不落库）
	w := contractRequest(t, router, "POST", "/web/schools", map[string]string{"name": "测试学校"}, h)
	assert.Equal(t, http.StatusOK, w.Code)
	// 空名称 -> 400
	w = contractRequest(t, router, "POST", "/web/schools", map[string]string{"name": ""}, h)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 创建年级：应同时生成默认科目与两个默认作息表
	w = contractRequest(t, router, "POST", "/web/schools/测试学校/grades", map[string]string{"name": "2026"}, h)
	assert.Equal(t, http.StatusOK, w.Code)
	// 年级已存在（默认科目/作息已生成）时重复创建 -> 409
	w = contractRequest(t, router, "POST", "/web/schools/测试学校/grades", map[string]string{"name": "2026"}, h)
	assert.Equal(t, http.StatusConflict, w.Code)

	var subject dbTable.Subject
	assert.NoError(t, db.GetDB().Where("school = ? AND grade = ?", "测试学校", "2026").First(&subject).Error)
	assert.Contains(t, subject.SubjectName, "数")

	var timetable dbTable.Timetable
	assert.NoError(t, db.GetDB().Where("school = ? AND grade = ?", "测试学校", "2026").First(&timetable).Error)
	assert.Contains(t, timetable.Timetable, "常日")
	assert.Contains(t, timetable.Timetable, "没课")

	// 学校已有关联数据（默认科目/作息）时重复创建同名学校 -> 409
	w = contractRequest(t, router, "POST", "/web/schools", map[string]string{"name": "测试学校"}, h)
	assert.Equal(t, http.StatusConflict, w.Code)

	// 创建班级：应生成默认课表与客户端配置
	w = contractRequest(t, router, "POST", "/web/schools/测试学校/grades/2026/classes", map[string]string{"name": "1"}, h)
	assert.Equal(t, http.StatusOK, w.Code)
	// 重复创建同名班级 -> 409
	w = contractRequest(t, router, "POST", "/web/schools/测试学校/grades/2026/classes", map[string]string{"name": "1"}, h)
	assert.Equal(t, http.StatusConflict, w.Code)

	var schedule dbTable.Schedule
	assert.NoError(t, db.GetDB().Where("school = ? AND grade = ? AND class = ?", "测试学校", "2026", "1").First(&schedule).Error)
	assert.Equal(t, "常日", schedule.DailyClasses[1].Timetable)

	var clientConfig dbTable.ClientConfig
	assert.NoError(t, db.GetDB().Where("school = ? AND grade = ? AND class = ?", "测试学校", "2026", "1").First(&clientConfig).Error)
	assert.NotEmpty(t, clientConfig.CSSStyle)

	// 年级已有班级数据后，重复创建同名年级 -> 409
	w = contractRequest(t, router, "POST", "/web/schools/测试学校/grades", map[string]string{"name": "2026"}, h)
	assert.Equal(t, http.StatusConflict, w.Code)

	// 结构树应包含新学校/年级/班级
	w = contractRequest(t, router, "GET", "/web/structure", nil, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	var structure []map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &structure))
	foundSchool := false
	for _, s := range structure {
		if s["text"] == "测试学校" {
			foundSchool = true
			grades, _ := s["children"].([]interface{})
			assert.NotEmpty(t, grades)
			grade, _ := grades[0].(map[string]interface{})
			assert.Equal(t, "2026", grade["text"])
			classes, _ := grade["children"].([]interface{})
			assert.NotEmpty(t, classes)
			cls, _ := classes[0].(map[string]interface{})
			assert.Equal(t, "1", cls["text"])
		}
	}
	assert.True(t, foundSchool, "结构树应包含新建学校")

	// 级联删除：班级 -> 年级 -> 学校
	w = contractRequest(t, router, "DELETE", "/web/schools/测试学校/grades/2026/classes/1", nil, h)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Error(t, db.GetDB().Where("school = ? AND grade = ? AND class = ?", "测试学校", "2026", "1").First(&dbTable.Schedule{}).Error)
	w = contractRequest(t, router, "DELETE", "/web/schools/测试学校/grades/2026", nil, h)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Error(t, db.GetDB().Where("school = ? AND grade = ?", "测试学校", "2026").First(&dbTable.Subject{}).Error)
	w = contractRequest(t, router, "DELETE", "/web/schools/测试学校", nil, h)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestRouteTable_CreateGradeConcurrent 并发创建同名年级：恰好一个成功、一个 409，
// 防止预检查竞态导致两个请求都返回 200。
func TestRouteTable_CreateGradeConcurrent(t *testing.T) {
	router := setupContractEnv(t)
	token := createContractUser(t, "admin-grade-race", "test123", "admin")

	const workers = 2
	var wg sync.WaitGroup
	codes := make([]int, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			payload := `{"name":"2026"}`
			req, _ := http.NewRequest("POST", "/web/schools/race-school/grades", strings.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("X-Verify-Password", "test123")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			codes[idx] = w.Code
		}(i)
	}
	wg.Wait()

	oks, conflicts := 0, 0
	for _, code := range codes {
		switch code {
		case http.StatusOK:
			oks++
		case http.StatusConflict:
			conflicts++
		}
	}
	require.Equal(t, 1, oks, "并发重复创建年级应恰好一个成功，codes=%v", codes)
	require.Equal(t, 1, conflicts, "并发重复创建年级应恰好一个 409，codes=%v", codes)
}

// TestRouteTable_ClientPutScheduleFlow 走真实路由验证客户端课表写入（PUT /:school/:grade/:class）。
func TestRouteTable_ClientPutScheduleFlow(t *testing.T) {
	router := setupContractEnv(t)
	token := createContractUser(t, "admin3", "test123", "admin")
	h := map[string]string{"Authorization": "Bearer " + token, "X-Verify-Password": "test123"}

	payload := map[string]interface{}{
		"daily_class": []map[string]interface{}{
			{"Chinese": "日", "English": "SUN", "timetable": "常日", "classList": []interface{}{[]interface{}{"数"}}},
			{"Chinese": "一", "English": "MON", "timetable": "常日", "classList": []interface{}{[]interface{}{"数"}}},
			{"Chinese": "二", "English": "TUE", "timetable": "常日", "classList": []interface{}{[]interface{}{"语"}}},
			{"Chinese": "三", "English": "WED", "timetable": "常日", "classList": []interface{}{[]interface{}{"英"}}},
			{"Chinese": "四", "English": "THR", "timetable": "常日", "classList": []interface{}{[]interface{}{"物"}}},
			{"Chinese": "五", "English": "FRI", "timetable": "常日", "classList": []interface{}{[]interface{}{"化"}}},
			{"Chinese": "六", "English": "SAT", "timetable": "常日", "classList": []interface{}{[]interface{}{"体"}}},
		},
		"subject_name": map[string]string{"数": "数学", "语": "语文"},
		"timetable":    map[string]map[string]interface{}{"常日": {"08:00-08:40": 0}},
		"divider":      map[string][]int{"常日": {}},
		"banner_text":  "来自客户端",
	}

	w := contractRequest(t, router, "PUT", "/push/g1/c1", payload, h)
	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeJSON(t, w)
	assert.NotEmpty(t, resp["version"], "写入响应应包含 version（desktop 依赖）")

	// 回读验证持久化与契约字段
	w = contractRequest(t, router, "GET", "/push/g1/c1", nil, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	resp = decodeJSON(t, w)
	assert.Contains(t, resp, "daily_class")
	assert.Contains(t, resp, "timetable")
	assert.Contains(t, resp, "subject_name")
	assert.Contains(t, resp, "divider")
	assert.Contains(t, resp, "banner_text")
	assert.Contains(t, resp, "version")
	assert.Equal(t, "来自客户端", resp["banner_text"])
	assert.Equal(t, "数学", resp["subject_name"].(map[string]interface{})["数"])
}
