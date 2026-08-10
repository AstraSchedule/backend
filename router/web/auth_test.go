package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/model/dbTable"
	"AstraScheduleServerGo/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func setupTestUser(t *testing.T) *dbTable.User {
	database := db.GetDB()
	// Delete existing test user first
	database.Where("namespace = ? AND username = ?", "default", "testuser").Delete(&dbTable.User{})

	hash, _ := service.HashPassword("test123")
	user := &dbTable.User{
		Username:     "testuser",
		PasswordHash: hash,
		Role:         "admin",
		Scope:        "ALL",
		Namespace:    "default",
	}
	database.Create(user)
	return user
}

// doRequestWithToken 执行带 Bearer JWT 的 JSON 请求，用于需认证的写接口测试。
// token 为空时不发送 Authorization 头，适用于无需认证的读接口测试。
func doRequestWithToken(router *gin.Engine, method, path string, body interface{}, token string) *httptest.ResponseRecorder {
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
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	router.ServeHTTP(w, req)
	return w
}

// doRequest 在 router 上执行请求并返回 recorder（无需认证）
func doRequest(router *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	return doRequestWithToken(router, method, path, body, "")
}

// Login tests

func TestLogin_InvalidJSON(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.POST("/web/auth/login", Login)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/web/auth/login", bytes.NewBufferString("invalid"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestLogin_UserNotFound(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.POST("/web/auth/login", Login)

	body := map[string]string{"username": "nonexistent", "password": "test123"}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/web/auth/login", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestLogin_WrongPassword(t *testing.T) {
	ensureTestDB()
	setupTestUser(t)

	router := setupTestRouter()
	router.POST("/web/auth/login", Login)

	body := map[string]string{"username": "testuser", "password": "wrongpassword"}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/web/auth/login", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestLogin_Success(t *testing.T) {
	ensureTestDB()
	setupTestUser(t)

	router := setupTestRouter()
	router.POST("/web/auth/login", Login)

	body := map[string]string{"username": "testuser", "password": "test123"}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/web/auth/login", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NotEmpty(t, resp["token"])
}

// GetMe tests

func TestGetMe_NoAuth(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.GET("/web/auth/me", GetMe)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/auth/me", nil)
	router.ServeHTTP(w, req)

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

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/auth/me", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "testuser", resp["username"])
}

// VerifyPassword tests

func TestVerifyPassword_NoAuth(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.POST("/web/auth/verify-password", VerifyPassword)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/web/auth/verify-password", nil)
	router.ServeHTTP(w, req)

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

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/web/auth/verify-password", bytes.NewBufferString("invalid"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

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

	w := doRequest(router, "POST", "/web/auth/verify-password", map[string]string{"password": "wrongpassword"})
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

	w := doRequest(router, "POST", "/web/auth/verify-password", map[string]string{"password": "test123"})
	assert.Equal(t, http.StatusOK, w.Code)
}

// ChangePassword tests

func TestChangePassword_NoAuth(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.POST("/web/auth/change-password", ChangePassword)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/web/auth/change-password", nil)
	router.ServeHTTP(w, req)

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
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/web/auth/change-password", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

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
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/web/auth/change-password", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

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
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/web/auth/change-password", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ListUsers tests

func TestListUsers_Success(t *testing.T) {
	ensureTestDB()
	setupTestUser(t)

	router := setupTestRouter()
	router.GET("/web/users", ListUsers)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/web/users", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	assert.GreaterOrEqual(t, len(data), 1)
}

// CreateUser tests

func TestCreateUser_InvalidJSON(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.POST("/web/users", CreateUser)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/web/users", bytes.NewBufferString("invalid"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateUser_EmptyFields(t *testing.T) {
	ensureTestDB()
	router := setupTestRouter()
	router.POST("/web/users", CreateUser)

	w := doRequest(router, "POST", "/web/users", map[string]string{"username": "", "password": ""})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateUser_ShortPassword(t *testing.T) {
	ensureTestDB()
	router := setupTestRouter()
	router.POST("/web/users", CreateUser)

	w := doRequest(router, "POST", "/web/users", map[string]string{"username": "newuser", "password": "123"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateUser_InvalidRole(t *testing.T) {
	ensureTestDB()
	router := setupTestRouter()
	router.POST("/web/users", CreateUser)

	w := doRequest(router, "POST", "/web/users", map[string]string{"username": "newuser", "password": "password123", "role": "invalid"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateUser_Success(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.POST("/web/users", CreateUser)

	body := map[string]string{"username": "newuser", "password": "password123", "role": "readonly"}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/web/users", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// UpdateUser tests

func TestUpdateUser_InvalidID(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/users/:id", UpdateUser)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/web/users/invalid", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateUser_NotFound(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.PUT("/web/users/:id", UpdateUser)

	body := map[string]string{"username": "updated"}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/web/users/99999", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateUser_Success(t *testing.T) {
	ensureTestDB()
	user := setupTestUser(t)

	router := setupTestRouter()
	router.PUT("/web/users/:id", UpdateUser)

	body := map[string]string{"username": "updateduser"}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/web/users/"+strconv.Itoa(int(user.ID)), bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// DeleteUser tests

func TestDeleteUser_InvalidID(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.DELETE("/web/users/:id", DeleteUser)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/web/users/invalid", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDeleteUser_NotFound(t *testing.T) {
	ensureTestDB()

	router := setupTestRouter()
	router.DELETE("/web/users/:id", DeleteUser)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/web/users/99999", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteUser_Success(t *testing.T) {
	ensureTestDB()
	user := setupTestUser(t)

	router := setupTestRouter()
	router.DELETE("/web/users/:id", DeleteUser)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/web/users/"+strconv.Itoa(int(user.ID)), nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
