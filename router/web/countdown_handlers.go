package web

import (
	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/middleware"
	"AstraScheduleServerGo/model/dbTable"
	"AstraScheduleServerGo/service"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func normalizeCountdownSchedules(items []countdownScheduleInput) []dbTable.CountdownScheduleItem {
	out := make([]dbTable.CountdownScheduleItem, 0, len(items))
	for _, it := range items {
		name := strings.TrimSpace(it.Name)
		date := strings.TrimSpace(it.Date)
		if name == "" || date == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", date); err != nil {
			continue
		}
		out = append(out, dbTable.CountdownScheduleItem{
			Name:     name,
			Date:     date,
			Priority: it.Priority,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].Date < out[j].Date
	})
	return out
}

// makeCountdownID 生成倒数日记录的稳定哈希 ID。
// 安全修复：哈希输入加入 namespace，避免不同租户的相同配置产生相同 ID，
// 防止 upsert 时跨租户互相覆盖（主键 id 不含 namespace 的隔离缺陷）
func makeCountdownID(ns string, scope []string, schedules []dbTable.CountdownScheduleItem) string {
	parts := append([]string(nil), scope...)
	sort.Strings(parts)
	buf := ns + "|" + strings.Join(parts, ";") + "|"
	for _, s := range schedules {
		buf += s.Name + "," + s.Date + "," + strconv.Itoa(s.Priority) + ";"
	}
	sum := sha256.Sum256([]byte(buf))
	return hex.EncodeToString(sum[:])[:16]
}

func computeCountdownStatus(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "未知"
	}
	today := time.Now().Truncate(24 * time.Hour)
	daysLeft := int(t.Sub(today).Hours() / 24)
	if daysLeft < 0 {
		return "已过期"
	} else if daysLeft == 0 {
		return "就是今天"
	}
	return "生效中"
}

func mapCountdownScheduleItem(it dbTable.CountdownScheduleItem) gin.H {
	return gin.H{
		"name":     it.Name,
		"date":     it.Date,
		"priority": it.Priority,
		"status":   computeCountdownStatus(it.Date),
	}
}

func mapCountdownRecord(r dbTable.CountdownRecord) gin.H {
	schedules := make([]gin.H, 0, len(r.Schedules))
	allExpired := len(r.Schedules) > 0
	for _, s := range r.Schedules {
		schedules = append(schedules, mapCountdownScheduleItem(s))
		if computeCountdownStatus(s.Date) != "已过期" {
			allExpired = false
		}
	}
	status := "已过期"
	if !allExpired {
		status = "生效中"
	}
	return gin.H{
		"id":        r.ID,
		"scope":     r.Scope,
		"schedules": schedules,
		"status":    status,
	}
}

func GetCountdownStatus(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	scope := strings.TrimSpace(c.Query("scope"))
	rows, err := db.FetchCountdownRecordsNs(ns, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(rows) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"loading":   false,
			"hasConfig": false,
			"data":      []gin.H{},
		})
		return
	}

	rows = service.FilterCountdownByScope(rows, scope)
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapCountdownRecord(r))
	}

	c.JSON(http.StatusOK, gin.H{
		"loading":   false,
		"hasConfig": true,
		"data":      out,
	})
}

func GetCountdownByID(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	id := c.Param("id")
	rows, err := db.FetchCountdownRecordsNs(ns, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(rows) == 0 {
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": mapCountdownRecord(rows[0])})
}

func PutCountdownRule(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	claims := middleware.GetUserClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "未认证"})
		return
	}

	var payload countdownPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效参数: " + err.Error()})
		return
	}
	scope := parseScopeInput(payload.Scope)
	// 校验作用域权限
	if claims.Role != "admin" {
		for _, s := range scope {
			if s == "ALL" {
				continue
			}
			if !db.CheckScopePermission(&dbTable.User{Role: claims.Role, Scope: claims.Scope}, s, "", "") {
				c.JSON(http.StatusForbidden, gin.H{"detail": "无权操作该作用域"})
				return
			}
		}
	}
	schedules := normalizeCountdownSchedules(payload.Schedules)
	if len(schedules) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "schedules 不能为空，且每项需要合法 name/date(YYYY-MM-DD)"})
		return
	}

	recordID := strings.TrimSpace(payload.ID)
	if recordID == "" {
		recordID = makeCountdownID(ns, scope, schedules)
	}
	record := dbTable.CountdownRecord{
		ID:        recordID,
		Namespace: ns,
		Scope:     scope,
		Schedules: schedules,
	}
	if err := db.UpsertCountdownRecord(&record); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "id": recordID})
}

func DeleteCountdownRecord(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	id := c.Param("id")
	affected, err := db.DeleteCountdownRecordNs(ns, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"detail": "记录不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "deleted": affected, "id": id})
}
