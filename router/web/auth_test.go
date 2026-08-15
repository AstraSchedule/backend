package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/model/dbTable"
	"AstraScheduleServerGo/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestEnv 组合 ensureTestDB + setupTestRouter，消除每个测试开头的重复样板
func setupTestEnv(t *testing.T) *gin.Engine {
	t.Helper()
	ensureTestDB()
	return setupTestRouter()
}

func setupTestUser(t *testing.T) *dbTable.User {
	database := db.GetDB()
	// Delete existing test user first
	database.Where("username = ?", "testuser").Delete(&dbTable.User{})

	hash, _ := service.HashPassword("test123")
	user := &dbTable.User{
		Username:     "testuser",
		PasswordHash: hash,
		Role:         "admin",
		Scope:        "ALL",
	}
	database.Create(user)
	return user
}

// Login tests

func TestLogin_InvalidJSON(t *testing.T) {
	router := setupTestEnv(t)
	router.POST("/web/auth/login", Login)

	w := doRawRequest(t, router, "POST", "/web/auth/login", "invalid")

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestLogin_UserNotFound(t *testing.T) {
	router := setupTestEnv(t)
	router.POST("/web/auth/login", Login)

	body := map[string]string{"username": "nonexistent", "password": "test123"}
	w := doRequest(t, router, "POST", "/web/auth/login", body)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestLogin_WrongPassword(t *testing.T) {
	ensureTestDB()
	setupTestUser(t)

	router := setupTestRouter()
	router.POST("/web/auth/login", Login)

	body := map[string]string{"username": "testuser", "password": "wrongpassword"}
	w := doRequest(t, router, "POST", "/web/auth/login", body)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestLogin_Success(t *testing.T) {
	ensureTestDB()
	setupTestUser(t)

	router := setupTestRouter()
	router.POST("/web/auth/login", Login)

	body := map[string]string{"username": "testuser", "password": "test123"}
	w := doRequest(t, router, "POST", "/web/auth/login", body)

	assert.Equal(t, http.StatusOK, w.Code)

	// 契约：登录响应含 token/must_change_pwd/user{id,username,role,scope}（usr-dashboard Login.vue 依赖）
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["token"])
	assert.Contains(t, resp, "must_change_pwd")
	user, ok := resp["user"].(map[string]interface{})
	require.True(t, ok, "响应应包含 user 对象")
	assert.Equal(t, "testuser", user["username"])
	assert.Equal(t, "admin", user["role"])
	assert.Contains(t, user, "scope")
}

// GetMe tests

func TestGetMe_NoAuth(t *testing.T) {
	router := setupTestEnv(t)
	router.GET("/web/auth/me", GetMe)

	w := doRequest(t, router, "GET", "/web/auth/me", nil)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGetMe_Success(t *testing.T) {
	ensureTestDB()
	user := setupTestUser(t)

	router := setupTestRouter()
	router.GET("/web/auth/me", func(c *gin.Context) {
		c.Set("user_claims", &service.JWTClaims{
			UserID:   user.ID,
			Username: user.Username,
			Role:     user.Role,
		})
		GetMe(c)
	})

	w := doRequest(t, router, "GET", "/web/auth/me", nil)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "testuser", resp["username"])
}

// VerifyPassword tests

func TestVerifyPassword_NoAuth(t *testing.T) {
	router := setupTestEnv(t)
	router.POST("/web/auth/verify-password", VerifyPassword)

	w := doRequest(t, router, "POST", "/web/auth/verify-password", nil)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestVerifyPassword_InvalidJSON(t *testing.T) {
	ensureTestDB()
	user := setupTestUser(t)

	router := setupTestRouter()
	router.POST("/web/auth/verify-password", func(c *gin.Context) {
		c.Set("user_claims", &service.JWTClaims{
			UserID:   user.ID,
			Username: user.Username,
			Role:     user.Role,
		})
		VerifyPassword(c)
	})

	w := doRawRequest(t, router, "POST", "/web/auth/verify-password", "invalid")

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	ensureTestDB()
	user := setupTestUser(t)

	router := setupTestRouter()
	router.POST("/web/auth/verify-password", func(c *gin.Context) {
		c.Set("user_claims", &service.JWTClaims{
			UserID:   user.ID,
			Username: user.Username,
			Role:     user.Role,
		})
		VerifyPassword(c)
	})

	body := map[string]string{"password": "wrongpassword"}
	w := doRequest(t, router, "POST", "/web/auth/verify-password", body)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestVerifyPassword_Success(t *testing.T) {
	ensureTestDB()
	user := setupTestUser(t)

	router := setupTestRouter()
	router.POST("/web/auth/verify-password", func(c *gin.Context) {
		c.Set("user_claims", &service.JWTClaims{
			UserID:   user.ID,
			Username: user.Username,
			Role:     user.Role,
		})
		VerifyPassword(c)
	})

	body := map[string]string{"password": "test123"}
	w := doRequest(t, router, "POST", "/web/auth/verify-password", body)

	assert.Equal(t, http.StatusOK, w.Code)
}

// changePasswordRouter 挂载带测试用户 claims 的改密路由，供各改密用例共用，消除重复包装样板
func changePasswordRouter(t *testing.T, user *dbTable.User) *gin.Engine {
	t.Helper()
	router := setupTestRouter()
	router.POST("/web/auth/change-password", func(c *gin.Context) {
		c.Set("user_claims", &service.JWTClaims{
			UserID:   user.ID,
			Username: user.Username,
			Role:     user.Role,
		})
		ChangePassword(c)
	})
	return router
}

// ChangePassword tests

func TestChangePassword_NoAuth(t *testing.T) {
	router := setupTestEnv(t)
	router.POST("/web/auth/change-password", ChangePassword)

	w := doRequest(t, router, "POST", "/web/auth/change-password", nil)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestChangePassword_ShortPassword(t *testing.T) {
	ensureTestDB()
	user := setupTestUser(t)

	router := changePasswordRouter(t, user)

	body := map[string]string{"old_password": "test123", "new_password": "123"}
	w := doRequest(t, router, "POST", "/web/auth/change-password", body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestChangePassword_WrongOldPassword(t *testing.T) {
	ensureTestDB()
	user := setupTestUser(t)

	router := changePasswordRouter(t, user)

	body := map[string]string{"old_password": "wrongpassword", "new_password": "newpassword123"}
	w := doRequest(t, router, "POST", "/web/auth/change-password", body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestChangePassword_Success(t *testing.T) {
	ensureTestDB()
	user := setupTestUser(t)

	router := changePasswordRouter(t, user)

	body := map[string]string{"old_password": "test123", "new_password": "newpassword123"}
	w := doRequest(t, router, "POST", "/web/auth/change-password", body)

	assert.Equal(t, http.StatusOK, w.Code)

	// 回读验证密码确实更新，且 must_change_pwd 被清除
	saved := fetchUser(t, user.ID)
	assert.True(t, service.CheckPassword("newpassword123", saved.PasswordHash))
	assert.False(t, saved.MustChangePwd)
}

func TestChangePassword_ShortUsername(t *testing.T) {
	ensureTestDB()
	user := setupTestUser(t)

	router := changePasswordRouter(t, user)

	body := map[string]string{"old_password": "test123", "new_password": "newpassword123", "new_username": "ab"}
	w := doRequest(t, router, "POST", "/web/auth/change-password", body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestChangePassword_WithNewUsername(t *testing.T) {
	ensureTestDB()
	user := setupTestUser(t)

	router := changePasswordRouter(t, user)

	// usr-dashboard ChangePassword.vue 会同时提交 new_username
	body := map[string]string{"old_password": "test123", "new_password": "newpassword123", "new_username": "renameduser"}
	w := doRequest(t, router, "POST", "/web/auth/change-password", body)

	assert.Equal(t, http.StatusOK, w.Code)
	saved := fetchUser(t, user.ID)
	assert.Equal(t, "renameduser", saved.Username)
	assert.True(t, service.CheckPassword("newpassword123", saved.PasswordHash))
}

// ListUsers tests

func TestListUsers_Success(t *testing.T) {
	ensureTestDB()
	setupTestUser(t)

	router := setupTestRouter()
	router.GET("/web/users", ListUsers)

	w := doRequest(t, router, "GET", "/web/users", nil)

	assert.Equal(t, http.StatusOK, w.Code)

	// 契约：每项含 id/username/role/scope/must_change_pwd/must_change_username（usr-dashboard Users.vue 依赖）
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].([]interface{})
	assert.GreaterOrEqual(t, len(data), 1)
	found := false
	for _, item := range data {
		u, ok := item.(map[string]interface{})
		require.True(t, ok, "用户列表项应为对象")
		if u["username"] == "testuser" {
			found = true
			assert.Equal(t, "admin", u["role"])
			assert.Contains(t, u, "id")
			assert.Contains(t, u, "scope")
			assert.Contains(t, u, "must_change_pwd")
			assert.Contains(t, u, "must_change_username")
			assert.Contains(t, u, "created_at")
		}
	}
	assert.True(t, found, "testuser 应出现在用户列表")
}

// CreateUser tests

func TestCreateUser_InvalidJSON(t *testing.T) {
	router := setupTestEnv(t)
	router.POST("/web/users", CreateUser)

	w := doRawRequest(t, router, "POST", "/web/users", "invalid")

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateUser_EmptyFields(t *testing.T) {
	router := setupTestEnv(t)
	router.POST("/web/users", CreateUser)

	body := map[string]string{"username": "", "password": ""}
	w := doRequest(t, router, "POST", "/web/users", body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateUser_ShortPassword(t *testing.T) {
	router := setupTestEnv(t)
	router.POST("/web/users", CreateUser)

	body := map[string]string{"username": "newuser", "password": "123"}
	w := doRequest(t, router, "POST", "/web/users", body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateUser_InvalidRole(t *testing.T) {
	router := setupTestEnv(t)
	router.POST("/web/users", CreateUser)

	body := map[string]string{"username": "newuser", "password": "password123", "role": "invalid"}
	w := doRequest(t, router, "POST", "/web/users", body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateUser_Success(t *testing.T) {
	router := setupTestEnv(t)
	router.POST("/web/users", CreateUser)

	body := map[string]string{"username": "newuser", "password": "password123", "role": "readonly"}
	w := doRequest(t, router, "POST", "/web/users", body)

	assert.Equal(t, http.StatusOK, w.Code)

	// 回读验证持久化
	var saved dbTable.User
	assert.NoError(t, db.GetDB().Where("username = ?", "newuser").First(&saved).Error)
	assert.Equal(t, "readonly", saved.Role)
	assert.NotEmpty(t, saved.PasswordHash)
}

// UpdateUser tests

func TestUpdateUser_InvalidID(t *testing.T) {
	router := setupTestEnv(t)
	router.PUT("/web/users/:id", UpdateUser)

	w := doRequest(t, router, "PUT", "/web/users/invalid", nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateUser_NotFound(t *testing.T) {
	router := setupTestEnv(t)
	router.PUT("/web/users/:id", UpdateUser)

	body := map[string]string{"username": "updated"}
	w := doRequest(t, router, "PUT", "/web/users/99999", body)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateUser_Success(t *testing.T) {
	ensureTestDB()
	user := setupTestUser(t)

	router := setupTestRouter()
	router.PUT("/web/users/:id", UpdateUser)

	// 同时更新用户名、角色与作用域（usr-dashboard Users.vue 的编辑载荷）
	body := map[string]string{"username": "updateduser", "role": "readonly", "scope": "s1/g1/c1"}
	w := doRequest(t, router, "PUT", "/web/users/"+strconv.Itoa(int(user.ID)), body)

	assert.Equal(t, http.StatusOK, w.Code)

	// 回读验证持久化
	saved := fetchUser(t, user.ID)
	assert.Equal(t, "updateduser", saved.Username)
	assert.Equal(t, "readonly", saved.Role)
	assert.Equal(t, "s1/g1/c1", saved.Scope)
}

// DeleteUser tests

func TestDeleteUser_InvalidID(t *testing.T) {
	router := setupTestEnv(t)
	router.DELETE("/web/users/:id", DeleteUser)

	w := doRequest(t, router, "DELETE", "/web/users/invalid", nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDeleteUser_NotFound(t *testing.T) {
	router := setupTestEnv(t)
	router.DELETE("/web/users/:id", DeleteUser)

	w := doRequest(t, router, "DELETE", "/web/users/99999", nil)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteUser_Success(t *testing.T) {
	ensureTestDB()
	user := setupTestUser(t)

	router := setupTestRouter()
	router.DELETE("/web/users/:id", DeleteUser)

	w := doRequest(t, router, "DELETE", "/web/users/"+strconv.Itoa(int(user.ID)), nil)

	assert.Equal(t, http.StatusOK, w.Code)

	// 回读验证用户确实被删除
	assert.Error(t, db.GetDB().First(&dbTable.User{}, user.ID).Error)
}
