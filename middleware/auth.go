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

		// 安全修复：校验 JWT 绑定的 namespace 与请求 Host 解析的 namespace 一致，
		// 防止单实例多租户部署下通过伪造 Host 头跨租户访问
		if !validateNamespaceBinding(c, claims) {
			c.JSON(http.StatusForbidden, gin.H{"detail": "令牌与当前租户不匹配"})
			c.Abort()
			return
		}

		c.Set(UserClaimsKey, claims)
		c.Next()
	}
}

// RequireRole 要求用户具有指定角色之一
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
		for _, role := range roles {
			if jwtClaims.Role == role {
				c.Next()
				return
			}
		}
		c.JSON(http.StatusForbidden, gin.H{"detail": "权限不足"})
		c.Abort()
	}
}

// AdminOrToken 密码验证可操作
// 需要 JWT + 请求头 X-Verify-Password 匹配用户密码
func AdminOrToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := parseJWTFromHeader(c)

		if claims == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"detail": "未认证"})
			c.Abort()
			return
		}

		// 安全修复：校验 JWT 绑定的 namespace 与请求 Host 解析的 namespace 一致
		if !validateNamespaceBinding(c, claims) {
			c.JSON(http.StatusForbidden, gin.H{"detail": "令牌与当前租户不匹配"})
			c.Abort()
			return
		}

		password := extractPasswordFromRequest(c)
		if password == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"detail": "需要提供密码"})
			c.Abort()
			return
		}

		user, err := db.GetUserByID(claims.UserID)
		if err != nil || !service.CheckPassword(password, user.PasswordHash) {
			c.JSON(http.StatusUnauthorized, gin.H{"detail": "你寻思寻思这密码它对吗？"})
			c.Abort()
			return
		}

		c.Set(UserClaimsKey, claims)
		c.Next()
	}
}

// validateNamespaceBinding 校验 JWT 绑定的 namespace 与请求 Host 解析的 namespace 一致。
// 单实例多租户共享库部署下，租户身份来源于 Host 头；若 token 的租户与当前 Host 租户
// 不一致，说明攻击者在用其他租户的 token 伪造 Host，直接拒绝。
// Host 未解析出租户（release 模式 localhost/IP 等运维通道，ns=""）时跳过校验。
func validateNamespaceBinding(c *gin.Context, claims *service.JWTClaims) bool {
	ns := GetNamespace(c)
	if ns == "" {
		return true
	}
	return claims.Namespace == ns
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
