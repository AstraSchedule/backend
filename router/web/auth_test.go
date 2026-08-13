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

	w := doRawRequest(router, "POST", "/web/auth/login", "invalid")

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestLogin_UserNotFound(t *testing.T) {
	router := setupTestEnv(t)
	router.POST("/web/auth/login", Login)

	body := map[string]string{"username": "nonexistent", "password": "test123"}
	w := doRequest(router, "POST", "/web/auth/login", body)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestLogin_WrongPassword(t *testing.T) {
	ensureTestDB()
	setupTestUser(t)

	router := setupTestRouter()
	router.POST("/web/auth/login", Login)

	body := map[string]string{"username": "testuser", "password": "wrongpassword"}
	w := doRequest(router, "POST", "/web/auth/login", body)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestLogin_Success(t *testing.T) {
	ensureTestDB()
	setupTestUser(t)

	router := setupTestRouter()
	router.POST("/web/auth/login", Login)

	body := map[string]string{"username": "testuser", "password": "test123"}
	w := doRequest(router, "POST", "/web/auth/login", body)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NotEmpty(t, resp["token"])
}

// GetMe tests

func TestGetMe_NoAuth(t *testing.T) {
	router := setupTestEnv(t)
	router.GET("/web/auth/me", GetMe)

	w := doRequest(router, "GET", "/web/auth/me", nil)

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

	w := doRequest(router, "GET", "/web/auth/me", nil)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "testuser", resp["username"])
}

// VerifyPassword tests

func TestVerifyPassword_NoAuth(t *testing.T) {
	router := setupTestEnv(t)
	router.POST("/web/auth/verify-password", VerifyPassword)

	w := doRequest(router, "POST", "/web/auth/verify-password", nil)

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

	w := doRawRequest(router, "POST", "/web/auth/verify-password", "invalid")

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
	w := doRequest(router, "POST", "/web/auth/verify-password", body)

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
	w := doRequest(router, "POST", "/web/auth/verify-password", body)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ChangePassword tests

func TestChangePassword_NoAuth(t *testing.T) {
	router := setupTestEnv(t)
	router.POST("/web/auth/change-password", ChangePassword)

	w := doRequest(router, "POST", "/web/auth/change-password", nil)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestChangePassword_ShortPassword(t *testing.T) {
	ensureTestDB()
	user := setupTestUser(t)

	router := setupTestRouter()
	router.POST("/web/auth/change-password", func(c *gin.Context) {
		c.Set("user_claims", &service.JWTClaims{
			UserID:   user.ID,
			Username: user.Username,
			Role:     user.Role,
		})
		ChangePassword(c)
	})

	body := map[string]string{"old_password": "test123", "new_password": "123"}
	w := doRequest(router, "POST", "/web/auth/change-password", body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestChangePassword_WrongOldPassword(t *testing.T) {
	ensureTestDB()
	user := setupTestUser(t)

	router := setupTestRouter()
	router.POST("/web/auth/change-password", func(c *gin.Context) {
		c.Set("user_claims", &service.JWTClaims{
			UserID:   user.ID,
			Username: user.Username,
			Role:     user.Role,
		})
		ChangePassword(c)
	})

	body := map[string]string{"old_password": "wrongpassword", "new_password": "newpassword123"}
	w := doRequest(router, "POST", "/web/auth/change-password", body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestChangePassword_Success(t *testing.T) {
	ensureTestDB()
	user := setupTestUser(t)

	router := setupTestRouter()
	router.POST("/web/auth/change-password", func(c *gin.Context) {
		c.Set("user_claims", &service.JWTClaims{
			UserID:   user.ID,
			Username: user.Username,
			Role:     user.Role,
		})
		ChangePassword(c)
	})

	body := map[string]string{"old_password": "test123", "new_password": "newpassword123"}
	w := doRequest(router, "POST", "/web/auth/change-password", body)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ListUsers tests

func TestListUsers_Success(t *testing.T) {
	ensureTestDB()
	setupTestUser(t)

	router := setupTestRouter()
	router.GET("/web/users", ListUsers)

	w := doRequest(router, "GET", "/web/users", nil)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	assert.GreaterOrEqual(t, len(data), 1)
}

// CreateUser tests

func TestCreateUser_InvalidJSON(t *testing.T) {
	router := setupTestEnv(t)
	router.POST("/web/users", CreateUser)

	w := doRawRequest(router, "POST", "/web/users", "invalid")

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateUser_EmptyFields(t *testing.T) {
	router := setupTestEnv(t)
	router.POST("/web/users", CreateUser)

	body := map[string]string{"username": "", "password": ""}
	w := doRequest(router, "POST", "/web/users", body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateUser_ShortPassword(t *testing.T) {
	router := setupTestEnv(t)
	router.POST("/web/users", CreateUser)

	body := map[string]string{"username": "newuser", "password": "123"}
	w := doRequest(router, "POST", "/web/users", body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateUser_InvalidRole(t *testing.T) {
	router := setupTestEnv(t)
	router.POST("/web/users", CreateUser)

	body := map[string]string{"username": "newuser", "password": "password123", "role": "invalid"}
	w := doRequest(router, "POST", "/web/users", body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateUser_Success(t *testing.T) {
	router := setupTestEnv(t)
	router.POST("/web/users", CreateUser)

	body := map[string]string{"username": "newuser", "password": "password123", "role": "readonly"}
	w := doRequest(router, "POST", "/web/users", body)

	assert.Equal(t, http.StatusOK, w.Code)
}

// UpdateUser tests

func TestUpdateUser_InvalidID(t *testing.T) {
	router := setupTestEnv(t)
	router.PUT("/web/users/:id", UpdateUser)

	w := doRequest(router, "PUT", "/web/users/invalid", nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateUser_NotFound(t *testing.T) {
	router := setupTestEnv(t)
	router.PUT("/web/users/:id", UpdateUser)

	body := map[string]string{"username": "updated"}
	w := doRequest(router, "PUT", "/web/users/99999", body)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateUser_Success(t *testing.T) {
	ensureTestDB()
	user := setupTestUser(t)

	router := setupTestRouter()
	router.PUT("/web/users/:id", UpdateUser)

	body := map[string]string{"username": "updateduser"}
	w := doRequest(router, "PUT", "/web/users/"+strconv.Itoa(int(user.ID)), body)

	assert.Equal(t, http.StatusOK, w.Code)
}

// DeleteUser tests

func TestDeleteUser_InvalidID(t *testing.T) {
	router := setupTestEnv(t)
	router.DELETE("/web/users/:id", DeleteUser)

	w := doRequest(router, "DELETE", "/web/users/invalid", nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDeleteUser_NotFound(t *testing.T) {
	router := setupTestEnv(t)
	router.DELETE("/web/users/:id", DeleteUser)

	w := doRequest(router, "DELETE", "/web/users/99999", nil)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteUser_Success(t *testing.T) {
	ensureTestDB()
	user := setupTestUser(t)

	router := setupTestRouter()
	router.DELETE("/web/users/:id", DeleteUser)

	w := doRequest(router, "DELETE", "/web/users/"+strconv.Itoa(int(user.ID)), nil)

	assert.Equal(t, http.StatusOK, w.Code)
}
