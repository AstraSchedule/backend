package web

import (
	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/middleware"
	"AstraScheduleServerGo/model/dbTable"
	"AstraScheduleServerGo/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func ListUsers(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	users, err := db.ListUsers(ns)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]gin.H, 0, len(users))
	for _, u := range users {
		out = append(out, gin.H{
			"id":                   u.ID,
			"username":             u.Username,
			"role":                 u.Role,
			"scope":                u.Scope,
			"must_change_pwd":      u.MustChangePwd,
			"must_change_username": u.MustChangeUsername,
			"created_at":           u.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func CreateUser(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	var req struct {
		Username           string `json:"username"`
		Password           string `json:"password"`
		Role               string `json:"role"`
		Scope              string `json:"scope"`
		MustChangePwd      *bool  `json:"must_change_pwd"`
		MustChangeUsername *bool  `json:"must_change_username"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效参数"})
		return
	}

	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "用户名和密码不能为空"})
		return
	}

	if len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "密码长度不能少于 6 位"})
		return
	}

	validRoles := map[string]bool{"admin": true, "school_w": true, "grade_w": true, "class_w": true, "readonly": true}
	if !validRoles[req.Role] {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效的角色，可选: admin, school_w, grade_w, class_w, readonly"})
		return
	}

	hash, err := service.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "密码哈希失败"})
		return
	}

	user := dbTable.User{
		Namespace:          ns,
		Username:           req.Username,
		PasswordHash:       hash,
		Role:               req.Role,
		Scope:              req.Scope,
		MustChangePwd:      req.MustChangePwd != nil && *req.MustChangePwd,
		MustChangeUsername: req.MustChangeUsername != nil && *req.MustChangeUsername,
	}

	if err := db.CreateUser(&user); err != nil {
		c.JSON(http.StatusConflict, gin.H{"detail": "用户名已存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": 200,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
			"scope":    user.Scope,
		},
	})
}

type handlerError struct {
	status int
	msg    string
}

type updateUserRequest struct {
	Username           *string `json:"username"`
	Password           *string `json:"password"`
	Role               *string `json:"role"`
	Scope              *string `json:"scope"`
	MustChangePwd      *bool   `json:"must_change_pwd"`
	MustChangeUsername *bool   `json:"must_change_username"`
}

var validRoles = map[string]bool{"admin": true, "school_w": true, "grade_w": true, "class_w": true, "readonly": true}

func applyPasswordUpdate(user *dbTable.User, password string) *handlerError {
	if len(password) < 6 {
		return &handlerError{http.StatusBadRequest, "密码长度不能少于 6 位"}
	}
	hash, err := service.HashPassword(password)
	if err != nil {
		return &handlerError{http.StatusInternalServerError, "密码哈希失败"}
	}
	user.PasswordHash = hash
	user.MustChangePwd = false
	return nil
}

func applyRoleUpdate(user *dbTable.User, role string) *handlerError {
	if !validRoles[role] {
		return &handlerError{http.StatusBadRequest, "无效的角色"}
	}
	user.Role = role
	return nil
}

func applyUserUpdates(user *dbTable.User, req updateUserRequest) *handlerError {
	if req.Username != nil {
		user.Username = *req.Username
	}
	if req.Password != nil {
		if err := applyPasswordUpdate(user, *req.Password); err != nil {
			return err
		}
	}
	if req.Role != nil {
		if err := applyRoleUpdate(user, *req.Role); err != nil {
			return err
		}
	}
	if req.Scope != nil {
		user.Scope = *req.Scope
	}
	if req.MustChangePwd != nil {
		user.MustChangePwd = *req.MustChangePwd
	}
	if req.MustChangeUsername != nil {
		user.MustChangeUsername = *req.MustChangeUsername
	}
	return nil
}

func UpdateUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效的用户 ID"})
		return
	}

	user, err := db.GetUserByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "用户不存在"})
		return
	}

	// 安全修复：用户管理必须限定当前请求的 namespace，防止跨租户修改用户
	if user.Namespace != middleware.GetNamespace(c) {
		c.JSON(http.StatusNotFound, gin.H{"detail": "用户不存在"})
		return
	}

	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效参数"})
		return
	}

	if err := applyUserUpdates(user, req); err != nil {
		c.JSON(err.status, gin.H{"detail": err.msg})
		return
	}

	if err := db.UpdateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": 200,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
			"scope":    user.Scope,
		},
	})
}

func DeleteUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效的用户 ID"})
		return
	}

	// 安全修复：删除用户前校验其 namespace 属于当前请求租户，防止跨租户删除用户
	user, err := db.GetUserByID(uint(id))
	if err != nil || user.Namespace != middleware.GetNamespace(c) {
		c.JSON(http.StatusNotFound, gin.H{"detail": "用户不存在"})
		return
	}

	affected, err := db.DeleteUser(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"detail": "用户不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": 200, "deleted": affected})
}
