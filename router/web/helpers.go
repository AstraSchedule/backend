package web

import (
	"AstraScheduleServerGo/model/dbTable"
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

// makeHashID 生成自动任务规则的稳定哈希 ID。
// 安全修复：哈希输入加入 namespace，避免不同租户的相同规则产生相同 ID，
// 防止 upsert 时跨租户互相覆盖（主键 hash_id 不含 namespace 的隔离缺陷）
func makeHashID(ns string, etype int, scope []string, level int, parameters map[string]interface{}) string {
	sum := sha256.Sum256([]byte(ns + "|" + strconv.Itoa(etype) + "|" + strconv.Itoa(level) + "|" + stringsFromScope(scope) + "|" + stableMapString(parameters)))
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
