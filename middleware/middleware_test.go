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
	t.Helper()
	hash, err := service.HashPassword(password)
	require.NoError(t, err)
	require.NoError(t, db.GetDB().Where("username = ?", username).Delete(&dbTable.User{}).Error)
	user := &dbTable.User{Username: username, PasswordHash: hash, Role: role, Scope: "ALL"}
	require.NoError(t, db.GetDB().Create(user).Error)
	return user
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

// AdminOrToken

func TestAdminOrToken_NoAuth(t *testing.T) {
	setupMwEnv(t)
	router := gin.New()
	router.POST("/test", AdminOrToken(), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := doMwRequest(router, "POST", "/test", "{}", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminOrToken_NoPassword(t *testing.T) {
	setupMwEnv(t)
	user := createMwUser(t, "mwadmin", "test123", "admin")
	token := mwToken(t, user)

	router := gin.New()
	router.POST("/test", AdminOrToken(), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := doMwRequest(router, "POST", "/test", "{}", map[string]string{"Authorization": "Bearer " + token})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminOrToken_WrongPassword(t *testing.T) {
	setupMwEnv(t)
	user := createMwUser(t, "mwadmin", "test123", "admin")
	token := mwToken(t, user)

	router := gin.New()
	router.POST("/test", AdminOrToken(), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := doMwRequest(router, "POST", "/test", "{}", map[string]string{
		"Authorization": "Bearer " + token, "X-Verify-Password": "wrong",
	})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminOrToken_ValidPasswordHeader(t *testing.T) {
	setupMwEnv(t)
	user := createMwUser(t, "mwadmin", "test123", "admin")
	token := mwToken(t, user)

	router := gin.New()
	router.POST("/test", AdminOrToken(), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := doMwRequest(router, "POST", "/test", "{}", map[string]string{
		"Authorization": "Bearer " + token, "X-Verify-Password": "test123",
	})
	assert.Equal(t, http.StatusOK, w.Code)
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
