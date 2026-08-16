package web

import (
	"AstraScheduleServerGo/model/dbTable"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseScopeInput_Nil(t *testing.T) {
	result := parseScopeInput(nil)
	assert.Equal(t, []string{"ALL"}, result)
}

func TestParseScopeInput_String(t *testing.T) {
	result := parseScopeInput("school/grade/class")
	assert.Equal(t, []string{"school/grade/class"}, result)
}

func TestParseScopeInput_StringArray(t *testing.T) {
	input := []interface{}{"scope1", "scope2"}
	result := parseScopeInput(input)
	assert.Equal(t, []string{"scope1", "scope2"}, result)
}

func TestParseScopeInput_GoStringArray(t *testing.T) {
	input := []string{"scope1", "scope2"}
	result := parseScopeInput(input)
	assert.Equal(t, []string{"scope1", "scope2"}, result)
}

func TestToString_Int(t *testing.T) {
	assert.Equal(t, "42", toString(42))
}

func TestToString_Float(t *testing.T) {
	assert.Equal(t, "3.14", toString(3.14))
}

func TestToString_String(t *testing.T) {
	assert.Equal(t, "hello", toString("hello"))
}

func TestToString_Bool(t *testing.T) {
	assert.Equal(t, "true", toString(true))
}

func TestToString_Nil(t *testing.T) {
	assert.Equal(t, "", toString(nil))
}

func TestParseScope_FullScope(t *testing.T) {
	school, grade, class, ok := parseScope("school1/grade1/class1")
	assert.True(t, ok)
	assert.Equal(t, "school1", school)
	assert.Equal(t, "grade1", grade)
	assert.Equal(t, "class1", class)
}

func TestParseScope_GradeScope(t *testing.T) {
	// parseScope requires exactly 3 parts, so grade-only scope returns false
	school, grade, class, ok := parseScope("school1/grade1")
	assert.False(t, ok)
	assert.Equal(t, "", school)
	assert.Equal(t, "", grade)
	assert.Equal(t, "", class)
}

func TestParseScope_SchoolScope(t *testing.T) {
	// parseScope requires exactly 3 parts, so school-only scope returns false
	school, grade, class, ok := parseScope("school1")
	assert.False(t, ok)
	assert.Equal(t, "", school)
	assert.Equal(t, "", grade)
	assert.Equal(t, "", class)
}

func TestParseScope_Empty(t *testing.T) {
	_, _, _, ok := parseScope("")
	assert.False(t, ok)
}

func TestParseClassList(t *testing.T) {
	input := dbTable.ClassList{{"数"}, {"语"}, {"英"}}
	result := parseClassList(input)
	assert.Equal(t, dbTable.ClassList{{"数"}, {"语"}, {"英"}}, result)
}

func TestMakeHashID(t *testing.T) {
	scope := []string{"ALL"}
	params := map[string]interface{}{
		"date": "2025-10-15",
	}
	hash1 := makeHashID("ns1", dbTable.AutorunTypeSchedule, scope, 1, params)
	hash2 := makeHashID("ns1", dbTable.AutorunTypeSchedule, scope, 1, params)
	assert.Equal(t, hash1, hash2)
	assert.NotEmpty(t, hash1)
}

// 安全修复验证：不同 namespace 的相同规则必须生成不同 ID，防止跨租户覆盖
func TestMakeHashID_SameContentDifferentNamespace(t *testing.T) {
	scope := []string{"ALL"}
	params := map[string]interface{}{
		"date": "2025-10-15",
	}
	hashA := makeHashID("cn/getastra/tenant-a", dbTable.AutorunTypeSchedule, scope, 1, params)
	hashB := makeHashID("cn/getastra/tenant-b", dbTable.AutorunTypeSchedule, scope, 1, params)
	assert.NotEqual(t, hashA, hashB)
}

func TestMakeHashID_DifferentInputs(t *testing.T) {
	scope1 := []string{"ALL"}
	scope2 := []string{"school"}
	params := map[string]interface{}{}

	hash1 := makeHashID("ns1", dbTable.AutorunTypeSchedule, scope1, 1, params)
	hash2 := makeHashID("ns1", dbTable.AutorunTypeSchedule, scope2, 1, params)
	assert.NotEqual(t, hash1, hash2)
}

func TestStringsFromScope(t *testing.T) {
	scope := []string{"school1", "grade1", "class1"}
	result := stringsFromScope(scope)
	assert.Contains(t, result, "school1")
	assert.Contains(t, result, "grade1")
	assert.Contains(t, result, "class1")
}

func TestStableMapString(t *testing.T) {
	m := map[string]interface{}{
		"b": 2,
		"a": 1,
	}
	result1 := stableMapString(m)
	result2 := stableMapString(m)
	assert.Equal(t, result1, result2)
	assert.NotEmpty(t, result1)
}

func TestServiceAsInt(t *testing.T) {
	val, ok := serviceAsInt(42)
	assert.True(t, ok)
	assert.Equal(t, 42, val)

	val, ok = serviceAsInt("123")
	assert.True(t, ok)
	assert.Equal(t, 123, val)

	val, ok = serviceAsInt("abc")
	assert.False(t, ok)

	val, ok = serviceAsInt(nil)
	assert.False(t, ok)
}

func TestMergeScopes_UnionDedupe(t *testing.T) {
	got := mergeScopes([]string{"s1/g1", "ALL"}, []string{"ALL", "s2/g2", " s1/g1 "})
	assert.Equal(t, []string{"s1/g1", "ALL", "s2/g2"}, got)
}

func TestMergeScopes_EmptyOld(t *testing.T) {
	assert.Equal(t, []string{"s1"}, mergeScopes(nil, []string{"s1"}))
	assert.Empty(t, mergeScopes(nil, nil))
}
