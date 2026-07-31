package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestParseHostToNamespace_SimpleDomain(t *testing.T) {
	result := ParseHostToNamespace("aaa-do.getastra.cn")
	assert.Equal(t, "cn/getastra/aaa-do", result)
}

func TestParseHostToNamespace_Localhost(t *testing.T) {
	result := ParseHostToNamespace("localhost")
	assert.Equal(t, "default", result)
}

func TestParseHostToNamespace_IP(t *testing.T) {
	// 安全修复后：IP 地址返回 default，不再反转成伪 namespace（避免 IP 直连产生无意义租户标识）
	result := ParseHostToNamespace("127.0.0.1")
	assert.Equal(t, "default", result)
}

func TestParseHostToNamespace_Empty(t *testing.T) {
	result := ParseHostToNamespace("")
	assert.Equal(t, "default", result)
}

func TestParseHostToNamespace_WithPort(t *testing.T) {
	result := ParseHostToNamespace("localhost:8080")
	assert.Equal(t, "default", result)
}

func TestParseHostToNamespace_ComplexDomain(t *testing.T) {
	result := ParseHostToNamespace("app.example.com")
	assert.Equal(t, "com/example/app", result)
}

func TestParseHostToNamespace_SingleLabel(t *testing.T) {
	result := ParseHostToNamespace("myhost")
	assert.Equal(t, "default", result)
}

func TestNamespaceMiddleware_SetsNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(NamespaceMiddleware())

	var capturedNS interface{}
	router.GET("/test", func(c *gin.Context) {
		capturedNS, _ = c.Get(NamespaceKey)
		c.JSON(200, gin.H{"ns": capturedNS})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Host = "aaa-do.getastra.cn"
	router.ServeHTTP(w, req)

	assert.Equal(t, "cn/getastra/aaa-do", capturedNS)
}

func TestNamespaceMiddleware_LocalhostDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(NamespaceMiddleware())

	var capturedNS interface{}
	router.GET("/test", func(c *gin.Context) {
		capturedNS, _ = c.Get(NamespaceKey)
		c.JSON(200, gin.H{"ns": capturedNS})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Host = "localhost:8080"
	router.ServeHTTP(w, req)

	assert.Equal(t, "default", capturedNS)
}

func TestGetNamespace_FromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := &gin.Context{}
	c.Set(NamespaceKey, "test-namespace")

	ns := GetNamespace(c)
	assert.Equal(t, "test-namespace", ns)
}

func TestGetNamespace_EmptyContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := &gin.Context{}

	ns := GetNamespace(c)
	// In non-release mode, returns "default"
	assert.Equal(t, "default", ns)
}

func TestGetUserClaims_NoClaims(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := &gin.Context{}

	claims := GetUserClaims(c)
	assert.Nil(t, claims)
}

func TestExtractPasswordFromRequest_FromHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/test", nil)
	c.Request.Header.Set("X-Verify-Password", "test-password")

	password := extractPasswordFromRequest(c)
	assert.Equal(t, "test-password", password)
}

func TestExtractPasswordFromRequest_FromBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := `{"password":"body-password"}`
	c.Request, _ = http.NewRequest("POST", "/test", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Body = &mockReadCloser{data: []byte(body)}

	password := extractPasswordFromRequest(c)
	assert.Equal(t, "body-password", password)
}

type mockReadCloser struct {
	data   []byte
	offset int
}

func (m *mockReadCloser) Read(p []byte) (n int, err error) {
	if m.offset >= len(m.data) {
		return 0, io.EOF
	}
	n = copy(p, m.data[m.offset:])
	m.offset += n
	return n, nil
}

func (m *mockReadCloser) Close() error {
	return nil
}
