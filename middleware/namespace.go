package middleware

import (
	"net"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

const NamespaceKey = "namespace"

// ParseHostToNamespace 将 Host 头转换为命名空间
// 例: aaa-do.getastra.cn -> cn/getastra/aaa-do
// 单段（如 localhost、IP） -> default
func ParseHostToNamespace(host string) string {
	// 去掉端口号
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return "default"
	}
	// 安全修复：IP 地址（如 127.0.0.1、[::1]）不应反转成伪 namespace，
	// 否则 IP 直连会得到一个无意义的租户标识并绕过 release 模式的 default 兜底
	if net.ParseIP(host) != nil {
		return "default"
	}
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return "default"
	}
	// 反转域名段
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, "/")
}

// GetNamespace 从 gin.Context 获取当前请求的命名空间
// release 模式下，如果 namespace 为空或 default，返回空字符串
func GetNamespace(c *gin.Context) string {
	ns, ok := c.Get(NamespaceKey)
	if !ok || ns == "" {
		if os.Getenv("GIN_MODE") == "release" {
			return ""
		}
		return "default"
	}
	s, ok := ns.(string)
	if !ok || s == "" {
		if os.Getenv("GIN_MODE") == "release" {
			return ""
		}
		return "default"
	}
	// release 模式下禁止 default namespace
	if os.Getenv("GIN_MODE") == "release" && s == "default" {
		return ""
	}
	return s
}

// NamespaceMiddleware 从请求 Host 头解析命名空间并注入 Context
func NamespaceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ns := ParseHostToNamespace(c.Request.Host)
		// release 模式下，localhost/IP 请求不设置 namespace
		if os.Getenv("GIN_MODE") == "release" && ns == "default" {
			ns = ""
		}
		c.Set(NamespaceKey, ns)
		if ns != "" {
			logrus.Debugf("请求命名空间: %s (host=%s)", ns, c.Request.Host)
		}
		c.Next()
	}
}
