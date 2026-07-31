package web

import (
	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/middleware"
	"AstraScheduleServerGo/model"
	"AstraScheduleServerGo/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

func Login(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效参数"})
		return
	}

	// 登录限流：按 IP+用户名维度限制失败尝试次数，防止暴力破解
	if !middleware.LoginLimiter.Allow(c.ClientIP(), req.Username) {
		c.JSON(http.StatusTooManyRequests, gin.H{"detail": "尝试次数过多，请稍后再试"})
		return
	}

	user, err := db.GetUserByUsername(middleware.GetNamespace(c), req.Username)
	if err != nil {
		middleware.LoginLimiter.Fail(c.ClientIP(), req.Username)
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "用户名或密码错误"})
		return
	}

	if !service.CheckPassword(req.Password, user.PasswordHash) {
		middleware.LoginLimiter.Fail(c.ClientIP(), req.Username)
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "用户名或密码错误"})
		return
	}

	// 登录成功，清空失败计数
	middleware.LoginLimiter.Reset(c.ClientIP(), req.Username)

	token, err := service.GenerateToken(
		model.Configs.Secret.Token,
		user.ID, user.Namespace, user.Username, user.Role, user.Scope,
		24,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "生成令牌失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":           token,
		"must_change_pwd": user.MustChangePwd,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
			"scope":    user.Scope,
		},
	})
}

func ChangePassword(c *gin.Context) {
	claims := middleware.GetUserClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "未认证"})
		return
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
		NewUsername string `json:"new_username"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效参数"})
		return
	}

	if len(req.NewPassword) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "新密码长度不能少于 6 位"})
		return
	}

	user, err := db.GetUserByID(claims.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "用户不存在"})
		return
	}

	if !service.CheckPassword(req.OldPassword, user.PasswordHash) {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "旧密码错误"})
		return
	}

	// 更新用户名
	if req.NewUsername != "" && req.NewUsername != user.Username {
		if len(req.NewUsername) < 3 {
			c.JSON(http.StatusBadRequest, gin.H{"detail": "用户名长度不能少于 3 位"})
			return
		}
		user.Username = req.NewUsername
	}

	// 更新密码
	hash, err := service.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "密码哈希失败"})
		return
	}
	user.PasswordHash = hash
	user.MustChangePwd = false

	if err := db.UpdateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "修改成功"})
}

func GetMe(c *gin.Context) {
	claims := middleware.GetUserClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "未认证"})
		return
	}

	user, err := db.GetUserByID(claims.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "用户不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":              user.ID,
		"username":        user.Username,
		"role":            user.Role,
		"scope":           user.Scope,
		"must_change_pwd": user.MustChangePwd,
	})
}

func VerifyPassword(c *gin.Context) {
	claims := middleware.GetUserClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "未认证"})
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效参数"})
		return
	}

	user, err := db.GetUserByID(claims.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "用户不存在"})
		return
	}

	if !service.CheckPassword(req.Password, user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "密码错误"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "验证通过"})
}
