package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// doRawRequest 以原始字符串作为请求体执行请求并返回 recorder，
// 供非法 JSON 等场景使用，避免在调用侧产生重复代码块被 SonarQube 计入 new code duplication。
// 请求构造失败时直接终止测试，避免带着无效请求继续执行。
func doRawRequest(t *testing.T, router *gin.Engine, method, path, rawBody string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req, err := http.NewRequest(method, path, bytes.NewBufferString(rawBody))
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w
}

// doRequest 在 router 上执行 JSON 请求并返回 recorder，供各测试文件共用；
// body 为 nil 时发送空请求体（用于 GET 等无体请求），否则序列化为 JSON。
// 序列化或请求构造失败时直接终止测试。
func doRequest(t *testing.T, router *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	if body == nil {
		return doRawRequest(t, router, method, path, "")
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("序列化请求体失败: %v", err)
		return nil
	}
	return doRawRequest(t, router, method, path, string(b))
}

// doRequestWithToken 执行带 Bearer JWT 的 JSON 请求，用于需认证的写接口测试。
// token 为空时不发送 Authorization 头，适用于无需认证的读接口测试。
func doRequestWithToken(t *testing.T, router *gin.Engine, method, path string, body interface{}, token string) *httptest.ResponseRecorder {
	t.Helper()
	raw := ""
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("序列化请求体失败: %v", err)
			return nil
		}
		raw = string(b)
	}
	w := httptest.NewRecorder()
	req, err := http.NewRequest(method, path, bytes.NewBufferString(raw))
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	router.ServeHTTP(w, req)
	return w
}
