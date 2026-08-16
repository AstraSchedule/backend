package middleware

import (
	"net/http"
	"strings"

	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/model/dbTable"

	"github.com/gin-gonic/gin"
)

// CurrentDBUser 从上下文 claims 读取用户 ID 并加载数据库当前用户。
// 返回 nil 表示未认证或用户已不存在。角色与作用域判定必须以此为准，
// 防止用户被降级或收窄 scope 后旧 token 继续越权操作（JWT 内嵌值只在签发时有效）。
func CurrentDBUser(c *gin.Context) *dbTable.User {
	claims := GetUserClaims(c)
	if claims == nil {
		return nil
	}
	user, err := db.GetUserByID(claims.UserID)
	if err != nil {
		return nil
	}
	return user
}

// CheckUserScope 以数据库当前用户校验对 school/grade/class 的写权限。
// 校验失败时已写入响应（401/403），调用方直接 return。
func CheckUserScope(c *gin.Context, school, grade, class string) bool {
	user := CurrentDBUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "未认证"})
		return false
	}
	if !db.CheckScopePermission(user, school, grade, class) {
		c.JSON(http.StatusForbidden, gin.H{"detail": "无权操作该作用域"})
		return false
	}
	return true
}

// CheckUserScopeString 解析 1~3 段作用域串（school[/grade[/class]]）后按数据库当前用户校验写权限。
// admin 恒通过。ALL 级规则仅 admin 可写，必须显式按角色拒绝：
// school_w 且 Scope=="ALL" 会把 "ALL" 解析成 school 前缀而通过 CheckScopePermission，造成越权下发全局规则。
func CheckUserScopeString(c *gin.Context, scope string) bool {
	if scope == "ALL" {
		user := CurrentDBUser(c)
		if user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"detail": "未认证"})
			return false
		}
		if user.Role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"detail": "无权操作该作用域"})
			return false
		}
		return true
	}
	parts := strings.Split(scope, "/")
	school := parts[0]
	grade, class := "", ""
	if len(parts) > 1 {
		grade = parts[1]
	}
	if len(parts) > 2 {
		class = parts[2]
	}
	return CheckUserScope(c, school, grade, class)
}

// RequireScope 校验用户对请求路径 school/grade/class 的写权限。
// 角色与作用域以数据库当前值为准（防降级/收窄后的旧 token 继续越权写）。
// 客户端路由的班级参数名为 :class，web 路由为 :class_number，两者兼容（防参数名错配漏检）。
func RequireScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		school := c.Param("school")
		grade := c.Param("grade")
		class := c.Param("class_number")
		if class == "" {
			class = c.Param("class")
		}
		if !CheckUserScope(c, school, grade, class) {
			c.Abort()
			return
		}
		c.Next()
	}
}
