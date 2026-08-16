package middleware

import (
	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/model"
	"AstraScheduleServerGo/service"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

const UserClaimsKey = "user_claims"

// JWTAuthMiddleware 验证 JWT 令牌，将 claims 注入 Context
func JWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"detail": "缺少认证令牌"})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{"detail": "认证格式应为 Bearer <token>"})
			c.Abort()
			return
		}

		claims, err := service.ParseToken(model.Configs.Secret.Token, tokenString)
		if err != nil {
			logrus.Debugf("JWT 验证失败: %v", err)
			c.JSON(http.StatusUnauthorized, gin.H{"detail": "认证令牌无效或已过期"})
			c.Abort()
			return
		}

		c.Set(UserClaimsKey, claims)
		c.Next()
	}
}

// RequireRole 要求用户具有指定角色之一
// 角色以数据库当前值为准（而非 JWT 内嵌角色），防止用户被降级后旧 token 仍可访问管理接口
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, exists := c.Get(UserClaimsKey)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"detail": "未认证"})
			c.Abort()
			return
		}
		jwtClaims, ok := claims.(*service.JWTClaims)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"detail": "内部错误"})
			c.Abort()
			return
		}
		user, err := db.GetUserByID(jwtClaims.UserID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"detail": "未认证"})
			c.Abort()
			return
		}
		for _, role := range roles {
			if user.Role == role {
				c.Next()
				return
			}
		}
		c.JSON(http.StatusForbidden, gin.H{"detail": "权限不足"})
		c.Abort()
	}
}

// JWTAndPassword 写接口认证：需要有效 JWT + 密码验证，只读用户（readonly）禁止写操作。
// 密码来源与优先级：先取请求头 X-Verify-Password，为空时回退到 JSON 请求体 password 字段
// （见 extractPasswordFromRequest）。
// 角色判定以数据库当前值为准（而非 JWT 内嵌角色），防止用户被降级后旧 token 继续写入。
// 历史遗留说明：原函数名 AdminOrToken 暗示"管理员或令牌二选一"，但万能密码早在
// 多用户 JWT 改造时已移除，实际语义一直是 JWT + 密码验证，本次改名并补角色校验。
func JWTAndPassword() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := parseJWTFromHeader(c)

		if claims == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"detail": "未认证"})
			c.Abort()
			return
		}

		// 先取数据库当前用户：角色判定与后续密码校验共用同一次查询，不增加额外 DB 调用
		user, err := db.GetUserByID(claims.UserID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"detail": "未认证"})
			c.Abort()
			return
		}

		// 只读用户禁止写操作（此前任何角色只要知道密码都能写，且信任 token 内嵌角色
		// 会让被降级用户的旧 token 继续写入）
		if user.Role == "readonly" {
			c.JSON(http.StatusForbidden, gin.H{"detail": "只读用户无写权限"})
			c.Abort()
			return
		}

		password := extractPasswordFromRequest(c)
		if password == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"detail": "需要提供密码"})
			c.Abort()
			return
		}

		if !service.CheckPassword(password, user.PasswordHash) {
			c.JSON(http.StatusUnauthorized, gin.H{"detail": "你寻思寻思这密码它对吗？"})
			c.Abort()
			return
		}

		c.Set(UserClaimsKey, claims)
		c.Next()
	}
}

func parseJWTFromHeader(c *gin.Context) *service.JWTClaims {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return nil
	}
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		return nil
	}
	claims, err := service.ParseToken(model.Configs.Secret.Token, tokenString)
	if err != nil {
		return nil
	}
	return claims
}

func extractPasswordFromRequest(c *gin.Context) string {
	// 优先从自定义头读取
	if pw := c.GetHeader("X-Verify-Password"); pw != "" {
		return pw
	}
	// 回退到请求体
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return ""
	}
	c.Request.Body = io.NopCloser(strings.NewReader(string(body)))
	var req struct {
		Password string `json:"password"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}
	return req.Password
}

// GetUserClaims 从 Context 获取当前用户的 JWT claims
func GetUserClaims(c *gin.Context) *service.JWTClaims {
	claims, exists := c.Get(UserClaimsKey)
	if !exists {
		return nil
	}
	jwtClaims, ok := claims.(*service.JWTClaims)
	if !ok {
		return nil
	}
	return jwtClaims
}
