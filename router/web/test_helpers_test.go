package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
)

// doRawRequest 以原始字符串作为请求体执行请求并返回 recorder，
// 供非法 JSON 等场景使用，避免在调用侧产生重复代码块被 SonarQube 计入 new code duplication。
func doRawRequest(router *gin.Engine, method, path, rawBody string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, bytes.NewBufferString(rawBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w
}

// doRequest 在 router 上执行 JSON 请求并返回 recorder，供 auth_test.go 和 api_test.go 共用；
// body 为 nil 时发送空请求体（用于 GET 等无体请求），否则序列化为 JSON。
func doRequest(router *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	if body == nil {
		return doRawRequest(router, method, path, "")
	}
	b, _ := json.Marshal(body)
	return doRawRequest(router, method, path, string(b))
}
