package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/middleware"
	"AstraScheduleServerGo/model/dbTable"
	"AstraScheduleServerGo/service"
	"AstraScheduleServerGo/testutil"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// adminOnly 在请求上下文写入数据库内 admin 用户的 claims（模拟 JWTAndPassword 之后的已认证状态）。
// handler 级测试不挂完整认证中间件，直接挂本中间件补上下文，供 handler 内的作用域校验（CheckUserScope*）使用。
func adminOnly(t *testing.T) gin.HandlerFunc {
	t.Helper()
	user := testutil.CreateUser(t, db.GetDB(), "mw-web-admin", "test123", "admin", "ALL")
	claims := &service.JWTClaims{UserID: user.ID, Username: user.Username, Role: user.Role, Scope: user.Scope}
	return func(c *gin.Context) {
		c.Set(middleware.UserClaimsKey, claims)
		c.Next()
	}
}

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

// doRequest 在 router 上执行 JSON 请求并返回 recorder，供 auth_test.go 和 api_test.go 共用；
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

// fetchAutorunDetail 从 PUT 自动任务规则的响应中取 id，回读 /web/autorun/hash 详情并返回 data 对象。
// 供 timetable/schedule/all 三类规则的成功用例共用，避免重复的回读样板被 SonarQube 计入重复率。
func fetchAutorunDetail(t *testing.T, router *gin.Engine, putRecorder *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var putBody map[string]interface{}
	require.NoError(t, json.Unmarshal(putRecorder.Body.Bytes(), &putBody))
	hashID, ok := putBody["id"].(string)
	require.True(t, ok, "PUT 响应应包含 id")
	require.NotEmpty(t, hashID)

	w := doRequest(t, router, "GET", "/web/autorun/hash/"+hashID, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	item, ok := resp["data"].(map[string]interface{})
	require.True(t, ok, "详情 data 应为对象")
	return item
}

// autorunContent 从自动任务详情条目中提取 content 对象，供各规则详情断言共用
func autorunContent(t *testing.T, item map[string]interface{}) map[string]interface{} {
	t.Helper()
	content, ok := item["content"].(map[string]interface{})
	require.True(t, ok, "content 应为对象")
	return content
}

// fetchUser 按主键回读用户行，供用户 CRUD/改密成功用例验证持久化共用。
func fetchUser(t *testing.T, id uint) dbTable.User {
	t.Helper()
	var saved dbTable.User
	require.NoError(t, db.GetDB().First(&saved, id).Error)
	return saved
}
