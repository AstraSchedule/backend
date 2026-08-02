package middleware

import (
	"net/http/httptest"
	"testing"

	"AstraScheduleServerGo/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func newTestClaims(namespace string) *service.JWTClaims {
	return &service.JWTClaims{UserID: 1, Namespace: namespace, Username: "admin", Role: "admin", Scope: ""}
}

func TestValidateNamespaceBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 1. 请求注入 ns-a 且 token claims 一致 -> 放行
	c1, _ := gin.CreateTestContext(httptest.NewRecorder())
	c1.Set(NamespaceKey, "ns-a")
	assert.True(t, validateNamespaceBinding(c1, newTestClaims("ns-a")))

	// 2. 请求注入 ns-a 但 token claims 为 ns-b -> 跨租户拒绝
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Set(NamespaceKey, "ns-a")
	assert.False(t, validateNamespaceBinding(c2, newTestClaims("ns-b")))

	// 3. 无注入（非 release 默认 "default"）但 claims 为 ns-a -> 拒绝
	c3, _ := gin.CreateTestContext(httptest.NewRecorder())
	assert.False(t, validateNamespaceBinding(c3, newTestClaims("ns-a")))

	// 4. 无注入（默认 "default"）与 claims "default" 一致 -> 放行
	c4, _ := gin.CreateTestContext(httptest.NewRecorder())
	assert.True(t, validateNamespaceBinding(c4, newTestClaims("default")))

	// 5. release 模式 Host 未解析出租户（ns=""）-> 跳过绑定校验（运维通道设计）
	t.Setenv("GIN_MODE", "release")
	c5, _ := gin.CreateTestContext(httptest.NewRecorder())
	assert.True(t, validateNamespaceBinding(c5, newTestClaims("anything")))
}
