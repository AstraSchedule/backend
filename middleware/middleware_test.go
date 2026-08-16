package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
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

var mwDBInitialized = false

func setupMwEnv(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	if !mwDBInitialized {
		testutil.InitTestDB()
		db.GetDB().AutoMigrate(&dbTable.User{})
		mwDBInitialized = true
	}
}

func createMwUser(t *testing.T, username, password, role string) *dbTable.User {
	return createMwUserScoped(t, username, password, role, "ALL")
}

// createMwUserScoped 创建指定角色与作用域的测试用户（复用 testutil.CreateUser 避免重复样板）
func createMwUserScoped(t *testing.T, username, password, role, scope string) *dbTable.User {
	return testutil.CreateUser(t, db.GetDB(), username, password, role, scope)
}

func mwToken(t *testing.T, user *dbTable.User) string {
	t.Helper()
	token, err := service.GenerateToken(model.Configs.Secret.Token, user.ID, user.Username, user.Role, "ALL", 1)
	require.NoError(t, err)
	return token
}

func doMwRequest(router *gin.Engine, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, err := http.NewRequest(method, path, bytes.NewBufferString(body))
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	router.ServeHTTP(w, req)
	return w
}

// JWTAuthMiddleware

func TestJWTAuthMiddleware_NoHeader(t *testing.T) {
	router := gin.New()
	router.GET("/test", JWTAuthMiddleware(), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := doMwRequest(router, "GET", "/test", "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJWTAuthMiddleware_BadFormat(t *testing.T) {
	router := gin.New()
	router.GET("/test", JWTAuthMiddleware(), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := doMwRequest(router, "GET", "/test", "", map[string]string{"Authorization": "Token abc"})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJWTAuthMiddleware_InvalidToken(t *testing.T) {
	router := gin.New()
	router.GET("/test", JWTAuthMiddleware(), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := doMwRequest(router, "GET", "/test", "", map[string]string{"Authorization": "Bearer invalid.token.here"})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJWTAuthMiddleware_ValidToken(t *testing.T) {
	setupMwEnv(t)
	token := mwToken(t, createMwUser(t, "mwuser", "test123", "admin"))

	router := gin.New()
	router.GET("/test", JWTAuthMiddleware(), func(c *gin.Context) {
		claims := GetUserClaims(c)
		assert.Equal(t, "mwuser", claims.Username)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := doMwRequest(router, "GET", "/test", "", map[string]string{"Authorization": "Bearer " + token})
	assert.Equal(t, http.StatusOK, w.Code)
}

// RequireRole

func TestRequireRole_NoClaims(t *testing.T) {
	router := gin.New()
	router.GET("/test", RequireRole("admin"), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := doMwRequest(router, "GET", "/test", "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireRole_Forbidden(t *testing.T) {
	setupMwEnv(t)
	token := mwToken(t, createMwUser(t, "mwreadonly", "test123", "readonly"))

	router := gin.New()
	router.GET("/test", JWTAuthMiddleware(), RequireRole("admin"), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := doMwRequest(router, "GET", "/test", "", map[string]string{"Authorization": "Bearer " + token})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireRole_Allowed(t *testing.T) {
	setupMwEnv(t)
	token := mwToken(t, createMwUser(t, "mwadmin", "test123", "admin"))

	router := gin.New()
	router.GET("/test", JWTAuthMiddleware(), RequireRole("admin"), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := doMwRequest(router, "GET", "/test", "", map[string]string{"Authorization": "Bearer " + token})
	assert.Equal(t, http.StatusOK, w.Code)
}

// JWTAndPassword

func TestJWTAndPassword_NoAuth(t *testing.T) {
	setupMwEnv(t)
	router := gin.New()
	router.POST("/test", JWTAndPassword(), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := doMwRequest(router, "POST", "/test", "{}", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJWTAndPassword_NoPassword(t *testing.T) {
	setupMwEnv(t)
	user := createMwUser(t, "mwadmin", "test123", "admin")
	token := mwToken(t, user)

	router := gin.New()
	router.POST("/test", JWTAndPassword(), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := doMwRequest(router, "POST", "/test", "{}", map[string]string{"Authorization": "Bearer " + token})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJWTAndPassword_WrongPassword(t *testing.T) {
	setupMwEnv(t)
	user := createMwUser(t, "mwadmin", "test123", "admin")
	token := mwToken(t, user)

	router := gin.New()
	router.POST("/test", JWTAndPassword(), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := doMwRequest(router, "POST", "/test", "{}", map[string]string{
		"Authorization": "Bearer " + token, "X-Verify-Password": "wrong",
	})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJWTAndPassword_ValidPasswordHeader(t *testing.T) {
	setupMwEnv(t)
	user := createMwUser(t, "mwadmin", "test123", "admin")
	token := mwToken(t, user)

	router := gin.New()
	router.POST("/test", JWTAndPassword(), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := doMwRequest(router, "POST", "/test", "{}", map[string]string{
		"Authorization": "Bearer " + token, "X-Verify-Password": "test123",
	})
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestJWTAndPassword_ReadonlyForbidden(t *testing.T) {
	setupMwEnv(t)
	user := createMwUser(t, "mwreadonlyuser", "test123", "readonly")
	token := mwToken(t, user)

	router := gin.New()
	router.POST("/test", JWTAndPassword(), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	// 只读用户无论密码正确、错误还是缺失，都必须 403：拒绝发生在密码校验之前
	cases := map[string]map[string]string{
		"密码正确": {"Authorization": "Bearer " + token, "X-Verify-Password": "test123"},
		"密码错误": {"Authorization": "Bearer " + token, "X-Verify-Password": "wrong"},
		"无密码":   {"Authorization": "Bearer " + token},
	}
	for name, headers := range cases {
		t.Run(name, func(t *testing.T) {
			w := doMwRequest(router, "POST", "/test", "{}", headers)
			assert.Equal(t, http.StatusForbidden, w.Code)
		})
	}
}

func TestJWTAndPassword_DowngradedAdminForbidden(t *testing.T) {
	setupMwEnv(t)
	user := createMwUser(t, "mwdowngradeduser", "test123", "admin")
	token := mwToken(t, user)

	// 用户被降级为 readonly 后，旧 token 内嵌角色仍为 admin，但必须按数据库当前角色拒绝写操作
	require.NoError(t, db.GetDB().Model(user).Update("role", "readonly").Error)

	router := gin.New()
	router.POST("/test", JWTAndPassword(), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := doMwRequest(router, "POST", "/test", "{}", map[string]string{
		"Authorization": "Bearer " + token, "X-Verify-Password": "test123",
	})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireRole_DowngradedAdminForbidden(t *testing.T) {
	setupMwEnv(t)
	user := createMwUser(t, "mwrruser", "test123", "admin")
	token := mwToken(t, user)

	// 用户被降级为 readonly 后，旧 token 不得再访问 RequireRole 管理接口
	require.NoError(t, db.GetDB().Model(user).Update("role", "readonly").Error)

	router := gin.New()
	router.GET("/test", JWTAuthMiddleware(), RequireRole("admin"), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := doMwRequest(router, "GET", "/test", "", map[string]string{"Authorization": "Bearer " + token})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestJWTAndPassword_PasswordInBody(t *testing.T) {
	setupMwEnv(t)
	user := createMwUser(t, "mwbodyuser", "test123", "admin")
	token := mwToken(t, user)

	// 走完整 JWTAndPassword 链：密码放在 JSON 请求体（中间件读取后必须完整回填 body 供后续 handler 使用）
	router := gin.New()
	router.POST("/test", JWTAndPassword(), func(c *gin.Context) {
		var body map[string]interface{}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false})
			return
		}
		// 密码字段与普通字段都必须可读，验证中间件回填的是完整请求体而非仅密码
		if body["password"] != "test123" || body["note"] != "keep-me" {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "detail": "body 回填不完整"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := doMwRequest(router, "POST", "/test", `{"password":"test123","note":"keep-me"}`, map[string]string{
		"Authorization": "Bearer " + token,
	})
	assert.Equal(t, http.StatusOK, w.Code)

	// 错误 body 密码 -> 401
	w2 := doMwRequest(router, "POST", "/test", `{"password":"wrong"}`, map[string]string{
		"Authorization": "Bearer " + token,
	})
	assert.Equal(t, http.StatusUnauthorized, w2.Code)
}

func TestExtractPasswordFromRequest_FromHeader(t *testing.T) {
	setupMwEnv(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/test", bytes.NewBufferString("{}"))
	c.Request.Header.Set("X-Verify-Password", "test-password")

	assert.Equal(t, "test-password", extractPasswordFromRequest(c))
}

func TestExtractPasswordFromRequest_FromBody(t *testing.T) {
	setupMwEnv(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/test", bytes.NewBufferString(`{"password":"body-password"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	assert.Equal(t, "body-password", extractPasswordFromRequest(c))
}

// LoginLimiter

func TestLoginLimiter_LocksAfterMaxFailures(t *testing.T) {
	limiter := &LoginRateLimiter{attempts: map[string]*loginAttempt{}}

	assert.True(t, limiter.Allow("127.0.0.1", "user"))
	for i := 0; i < loginMaxFailures; i++ {
		limiter.Fail("127.0.0.1", "user")
	}
	assert.False(t, limiter.Allow("127.0.0.1", "user"))
	// 其他用户名不受影响
	assert.True(t, limiter.Allow("127.0.0.1", "other"))
	// 重置后恢复
	limiter.Reset("127.0.0.1", "user")
	assert.True(t, limiter.Allow("127.0.0.1", "user"))
}

// RequireScope

func TestRequireScope_ScopeMatrix(t *testing.T) {
	setupMwEnv(t)
	build := func(role, scope, school, grade, class string) int {
		user := createMwUserScoped(t, "scopemx", "test123", role, scope)
		token := mwToken(t, user)
		router := gin.New()
		router.PUT("/t/:school/:grade/:class_number", JWTAuthMiddleware(), RequireScope(), func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})
		w := doMwRequest(router, "PUT", "/t/"+school+"/"+grade+"/"+class, "{}", map[string]string{
			"Authorization": "Bearer " + token,
		})
		return w.Code
	}
	cases := []struct {
		name, role, scope, school, grade, class string
		want                                    int
	}{
		{"admin 全通", "admin", "ALL", "s1", "g1", "c1", http.StatusOK},
		{"school_w 本校任意年级班级放行", "school_w", "s1", "s1", "g9", "c9", http.StatusOK},
		{"school_w 他校拒绝", "school_w", "s1", "s2", "g1", "c1", http.StatusForbidden},
		{"grade_w 本年级任意班级放行", "grade_w", "s1/g1", "s1", "g1", "c9", http.StatusOK},
		{"grade_w 他年级拒绝", "grade_w", "s1/g1", "s1", "g2", "c1", http.StatusForbidden},
		{"class_w 本班放行", "class_w", "s1/g1/c1", "s1", "g1", "c1", http.StatusOK},
		{"class_w 他班拒绝", "class_w", "s1/g1/c1", "s1", "g1", "c2", http.StatusForbidden},
		{"readonly 拒绝", "readonly", "s1", "s1", "g1", "c1", http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, build(tc.role, tc.scope, tc.school, tc.grade, tc.class))
		})
	}
}

func TestRequireScope_ClassParamFallback(t *testing.T) {
	setupMwEnv(t)
	user := createMwUserScoped(t, "scopefb", "test123", "class_w", "s1/g1/c1")
	token := mwToken(t, user)

	// 客户端路由使用 :class 参数名，RequireScope 需兼容两种参数名，防止漏检
	router := gin.New()
	router.PUT("/t/:school/:grade/:class", JWTAuthMiddleware(), RequireScope(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := doMwRequest(router, "PUT", "/t/s1/g1/c1", "{}", map[string]string{"Authorization": "Bearer " + token})
	assert.Equal(t, http.StatusOK, w.Code)
	w = doMwRequest(router, "PUT", "/t/s1/g1/c2", "{}", map[string]string{"Authorization": "Bearer " + token})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireScope_DowngradedUserForbidden(t *testing.T) {
	setupMwEnv(t)
	user := createMwUserScoped(t, "scopedg", "test123", "admin", "ALL")
	token := mwToken(t, user)

	// 用户被收窄为 class_w(s1/g1/c1) 后，旧 token 内嵌 admin/ALL，仍必须按数据库当前值拒绝越界写
	require.NoError(t, db.GetDB().Model(user).Updates(map[string]interface{}{
		"role": "class_w", "scope": "s1/g1/c1",
	}).Error)

	router := gin.New()
	router.PUT("/t/:school/:grade/:class_number", JWTAuthMiddleware(), RequireScope(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := doMwRequest(router, "PUT", "/t/s1/g1/c2", "{}", map[string]string{"Authorization": "Bearer " + token})
	assert.Equal(t, http.StatusForbidden, w.Code)
	w = doMwRequest(router, "PUT", "/t/s1/g1/c1", "{}", map[string]string{"Authorization": "Bearer " + token})
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCheckUserScopeString_NonAdminALLForbidden(t *testing.T) {
	setupMwEnv(t)

	// school_w 且 Scope=="ALL"：把 "ALL" 解析成 school 前缀会通过 CheckScopePermission，
	// 必须显式按角色拒绝，防止越权写全局规则
	user := createMwUserScoped(t, "scopewall", "test123", "school_w", "ALL")
	token := mwToken(t, user)
	claims, err := service.ParseToken(model.Configs.Secret.Token, token)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(UserClaimsKey, claims)
	assert.False(t, CheckUserScopeString(c, "ALL"))
	assert.Equal(t, http.StatusForbidden, w.Code)

	// admin 写 ALL 放行
	admin := createMwUserScoped(t, "scopealladmin", "test123", "admin", "ALL")
	adminToken := mwToken(t, admin)
	adminClaims, err := service.ParseToken(model.Configs.Secret.Token, adminToken)
	require.NoError(t, err)
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Set(UserClaimsKey, adminClaims)
	assert.True(t, CheckUserScopeString(c2, "ALL"))
}
