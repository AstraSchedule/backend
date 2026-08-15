package web

import (
	"AstraScheduleServerGo/model/dbTable"
	"AstraScheduleServerGo/router/client"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

func parseScopeInput(raw interface{}) []string {
	if raw == nil {
		return []string{"ALL"}
	}
	switch v := raw.(type) {
	case string:
		if v == "" {
			return []string{"ALL"}
		}
		return []string{v}
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if ok && s != "" {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return []string{"ALL"}
		}
		return out
	case []string:
		if len(v) == 0 {
			return []string{"ALL"}
		}
		return v
	default:
		return []string{"ALL"}
	}
}

func makeHashID(etype int, scope []string, level int, parameters map[string]interface{}) string {
	sum := sha256.Sum256([]byte(strconv.Itoa(etype) + "|" + strconv.Itoa(level) + "|" + stringsFromScope(scope) + "|" + stableMapString(parameters)))
	return hex.EncodeToString(sum[:])[:16]
}

func stringsFromScope(scope []string) string {
	copyScope := append([]string(nil), scope...)
	sort.Strings(copyScope)
	out := ""
	for _, s := range copyScope {
		out += s + ";"
	}
	return out
}

func stableMapString(m map[string]interface{}) string {
	if m == nil {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := ""
	for _, k := range keys {
		out += k + "=" + toString(m[k]) + "|"
	}
	return out
}

func toString(v interface{}) string {
	switch vv := v.(type) {
	case string:
		return vv
	case float64:
		return strconv.FormatFloat(vv, 'f', -1, 64)
	case int:
		return strconv.Itoa(vv)
	case bool:
		if vv {
			return "true"
		}
		return "false"
	case map[string]interface{}:
		return stableMapString(vv)
	case []interface{}:
		out := ""
		for _, x := range vv {
			out += toString(x) + ","
		}
		return out
	default:
		return ""
	}
}

func parseClassList(input dbTable.ClassList) dbTable.ClassList {
	if len(input) == 0 {
		return dbTable.ClassList{}
	}
	out := make(dbTable.ClassList, 0, len(input))
	for _, item := range input {
		if len(item) == 0 {
			out = append(out, []string{""})
			continue
		}
		out = append(out, item)
	}
	return out
}

func serviceAsInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case string:
		i, err := strconv.Atoi(n)
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

func parseScope(scope string) (string, string, string, bool) {
	parts := strings.Split(scope, "/")
	if len(parts) < 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

// broadcastScopes 按作用域列表向在线客户端广播 SyncConfig（仅 WebSocket 模式生效，serverless 自动跳过）。
// 支持 ALL / school / school/grade 粒度；返回成功发送条数。
func broadcastScopes(scopes []string) int {
	total := 0
	for _, raw := range scopes {
		parts := strings.Split(strings.TrimSpace(raw), "/")
		switch {
		case raw == "" || strings.EqualFold(raw, "ALL"):
			total += client.BroadcastSyncAll()
		case len(parts) >= 2 && parts[0] != "" && parts[1] != "":
			total += client.BroadcastSync(parts[0], parts[1])
		case len(parts) == 1 && parts[0] != "":
			total += client.BroadcastSyncSchool(parts[0])
		}
	}
	return total
}
