package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
)

// doRequest 在 router 上执行 JSON 请求并返回 recorder，供 auth_test.go 和 api_test.go 共用，
// 避免在调用侧产生重复代码块被 SonarQube 计入 new code duplication。
func doRequest(router *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	var reqBody *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}
	req, _ := http.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w
}
