package client

import (
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// requestNamespace 返回请求所属的 namespace。
// main 分支无多租户概念，恒为 "default"；saas/main 分支改为 middleware.GetNamespace(c)。
func requestNamespace(c *gin.Context) string {
	return "default"
}

// statCollector 采集实时统计指标：内存态、当日计数、跨天自动重置、按 namespace 隔离。
//
// main 分支无 namespace 多租户概念，namespace 恒为 middleware.GetNamespace 的返回值
// （非 release 模式默认 "default"）；saas/main 分支为真实租户命名空间。
type statCollector struct {
	mu            sync.Mutex
	day           string
	weatherErrors map[string]int            // namespace -> 当日天气上游错误次数
	wsDisconnects map[string]map[string]int // namespace -> 班级标识 -> 当日断连次数
}

var collector = &statCollector{
	weatherErrors: map[string]int{},
	wsDisconnects: map[string]map[string]int{},
}

func (s *statCollector) rolloverLocked(now time.Time) {
	day := now.Format("2006-01-02")
	if s.day != day {
		s.day = day
		s.weatherErrors = map[string]int{}
		s.wsDisconnects = map[string]map[string]int{}
	}
}

// recordWeatherError 记录一次天气上游请求错误（按 namespace 隔离）。
func recordWeatherError(namespace string) {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	collector.rolloverLocked(time.Now())
	collector.weatherErrors[namespace]++
}

// recordWSDisconnect 记录一次 WebSocket 断开（按 namespace + 班级隔离）。
// classKey 形如 school/grade/class。
func recordWSDisconnect(namespace, classKey string) {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	collector.rolloverLocked(time.Now())
	m := collector.wsDisconnects[namespace]
	if m == nil {
		m = map[string]int{}
		collector.wsDisconnects[namespace] = m
	}
	m[classKey]++
}

// statSnapshot 返回指定 namespace 的统计快照：天气错误总数与班级断连映射。
func statSnapshot(namespace string) (weatherError int, disconnects map[string]int) {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	collector.rolloverLocked(time.Now())
	weatherError = collector.weatherErrors[namespace]
	disconnects = map[string]int{}
	for k, v := range collector.wsDisconnects[namespace] {
		disconnects[k] = v
	}
	return weatherError, disconnects
}

// StatSnapshot 导出统计快照，供 web 管理端 /web/statistic 接口使用。
func StatSnapshot(namespace string) (int, map[string]int) {
	return statSnapshot(namespace)
}

// StatOnlineClasses 返回指定 namespace 下当前在线客户端的班级标识列表（school/grade/class）。
// main 分支 wsScope 无 namespace 概念，返回全部在线班级；saas/main 分支按 namespace 过滤。
func StatOnlineClasses(namespace string) []string {
	classes := clientWsHub.onlineClasses(namespace)
	out := make([]string, 0, len(classes))
	for k := range classes {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
