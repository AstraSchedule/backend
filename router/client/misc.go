package client

import (
	"AstraScheduleServerGo/model"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

type wsScope struct {
	School string
	Grade  string
}

type wsHub struct {
	mu      sync.RWMutex
	clients map[wsScope]map[*websocket.Conn]string // conn -> classNumber
}

var clientWsHub = &wsHub{
	clients: map[wsScope]map[*websocket.Conn]string{},
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// 安全修复：校验 Origin，防止跨站 WebSocket 劫持（CSWSH）。
		// - 无 Origin 头：非浏览器客户端（如 Electron 桌面端/Node ws 库），放行
		// - Origin 为 file://：Electron 本地页面，放行
		// - 其他：必须与 CORS 白名单（server.domain）中的条目协议+主机完全一致
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		originURL, err := url.Parse(origin)
		if err != nil || originURL.Host == "" {
			return false
		}
		if strings.EqualFold(originURL.Scheme, "file") {
			return true
		}
		for _, allowed := range model.Configs.Server.Domain {
			allowedURL, err := url.Parse(allowed)
			if err != nil || allowedURL.Host == "" {
				continue
			}
			if strings.EqualFold(originURL.Scheme, allowedURL.Scheme) &&
				strings.EqualFold(originURL.Host, allowedURL.Host) {
				return true
			}
		}
		return false
	},
}

func (h *wsHub) add(scope wsScope, classNumber string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[scope]; !ok {
		h.clients[scope] = map[*websocket.Conn]string{}
	}
	h.clients[scope][conn] = classNumber
}

func (h *wsHub) remove(scope wsScope, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	group, ok := h.clients[scope]
	if !ok {
		return
	}
	delete(group, conn)
	if len(group) == 0 {
		delete(h.clients, scope)
	}
}

func (h *wsHub) count(scope wsScope) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	group, ok := h.clients[scope]
	if !ok {
		return 0
	}
	return len(group)
}

func (h *wsHub) snapshot(scope wsScope) []*websocket.Conn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	group, ok := h.clients[scope]
	if !ok {
		return []*websocket.Conn{}
	}
	out := make([]*websocket.Conn, 0, len(group))
	for conn := range group {
		out = append(out, conn)
	}
	return out
}

// onlineClasses 返回在线客户端的班级标识集合（school/grade/class）。
// main 分支 wsScope 无 namespace 概念，namespace 参数保留以兼容 saas/main 的按租户过滤。
func (h *wsHub) onlineClasses(namespace string) map[string]bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := map[string]bool{}
	for scope, group := range h.clients {
		for _, classNumber := range group {
			out[fmt.Sprintf("%s/%s/%s", scope.School, scope.Grade, classNumber)] = true
		}
	}
	return out
}

func (h *wsHub) broadcast(scope wsScope, message string) int {
	conns := h.snapshot(scope)
	if len(conns) == 0 {
		return 0
	}
	sent := 0
	for _, conn := range conns {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
			logrus.Warnf("WebSocket 广播失败，将移除连接：scope=%s/%s err=%v", scope.School, scope.Grade, err)
			h.remove(scope, conn)
			_ = conn.Close()
			continue
		}
		sent++
	}
	return sent
}

func BroadcastSyncConfig(c *gin.Context) {
	school := c.Param("school")
	grade := c.Param("grade")
	classNumber := c.Param("class_number")
	if !model.Configs.WebSocketEnabled() {
		logrus.Infof("收到广播请求：%s 学校 %s 级 %s 班，但当前配置禁用 WebSocket（serverless=true）", school, grade, classNumber)
		c.JSON(http.StatusOK, gin.H{
			"status":            200,
			"message":           "SyncConfig",
			"sent":              0,
			"websocket_enabled": false,
		})
		return
	}
	scope := wsScope{School: school, Grade: grade}
	sent := clientWsHub.broadcast(scope, "SyncConfig")
	logrus.Infof("收到广播请求：%s 学校 %s 级 %s 班，已广播 SyncConfig，成功发送 %d 条", school, grade, classNumber, sent)
	if sent == 0 {
		logrus.Warnf("未找到可用 websocket 连接：%s %s", school, grade)
	}
	c.JSON(http.StatusOK, gin.H{
		"status":            200,
		"message":           "SyncConfig",
		"sent":              sent,
		"websocket_enabled": true,
	})
}

func WebSocketPlaceholder(c *gin.Context) {
	if !model.Configs.WebSocketEnabled() {
		c.JSON(http.StatusNotImplemented, gin.H{
			"detail": "当前配置禁用 WebSocket（run.serverless=true）",
		})
		return
	}

	school := c.Param("school")
	grade := c.Param("grade")
	classNumber := c.Param("class_number")
	scope := wsScope{School: school, Grade: grade}

	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logrus.Errorf("WebSocket 升级失败：%v", err)
		c.JSON(http.StatusBadRequest, gin.H{"detail": "WebSocket 升级失败"})
		return
	}

	clientWsHub.add(scope, classNumber, conn)
	logrus.Infof("WebSocket 连接建立：%s 学校 %s 级 %s 班，当前级部连接数=%d", school, grade, classNumber, clientWsHub.count(scope))

	defer func() {
		clientWsHub.remove(scope, conn)
		_ = conn.Close()
		recordWSDisconnect(requestNamespace(c), fmt.Sprintf("%s/%s/%s", school, grade, classNumber))
		logrus.Infof("WebSocket 连接断开：%s 学校 %s 级 %s 班，当前级部连接数=%d", school, grade, classNumber, clientWsHub.count(scope))
	}()

	for {
		_, data, readErr := conn.ReadMessage()
		if readErr != nil {
			return
		}
		logrus.Infof("收到 WebSocket 数据：%s 学校 %s 级 %s 班 -> %s", school, grade, classNumber, string(data))
	}
}
