package service

import (
	"AstraScheduleServerGo/model/dbTable"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalcWeekNumber_EmptyStartDate(t *testing.T) {
	now := time.Date(2025, 10, 15, 0, 0, 0, 0, time.UTC)
	result := CalcWeekNumber("", now)
	assert.Equal(t, 1, result)
}

func TestCalcWeekNumber_InvalidDate(t *testing.T) {
	now := time.Date(2025, 10, 15, 0, 0, 0, 0, time.UTC)
	result := CalcWeekNumber("invalid-date", now)
	assert.Equal(t, 1, result)
}

func TestCalcWeekNumber_FirstWeek(t *testing.T) {
	start := time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC)
	now := start.Add(3 * 24 * time.Hour) // 3 days later
	result := CalcWeekNumber("2025-09-01", now)
	assert.Equal(t, 1, result)
}

func TestCalcWeekNumber_SecondWeek(t *testing.T) {
	start := time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC)
	now := start.Add(8 * 24 * time.Hour) // 8 days later
	result := CalcWeekNumber("2025-09-01", now)
	assert.Equal(t, 2, result)
}

func TestCalcWeekNumber_SixthWeek(t *testing.T) {
	start := time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC)
	now := start.Add(35 * 24 * time.Hour) // 35 days later
	result := CalcWeekNumber("2025-09-01", now)
	// days/7 + 1 = 35/7 + 1 = 5 + 1 = 6
	assert.Equal(t, 6, result)
}

func TestCalcWeekNumber_BeforeStartDate(t *testing.T) {
	start := time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC)
	now := start.Add(-3 * 24 * time.Hour) // 3 days before
	result := CalcWeekNumber("2025-09-01", now)
	assert.Equal(t, 1, result)
}

func TestCalcWeekNumber_ExactSevenDays(t *testing.T) {
	start := time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC)
	now := start.Add(7 * 24 * time.Hour) // exactly 7 days
	result := CalcWeekNumber("2025-09-01", now)
	// days/7 + 1 = 7/7 + 1 = 1 + 1 = 2
	assert.Equal(t, 2, result)
}

func TestResolveClassList_Empty(t *testing.T) {
	result := ResolveClassList(dbTable.ClassList{}, 1)
	assert.Equal(t, []string{}, result)
}

func TestResolveClassList_SingleWeek(t *testing.T) {
	cl := dbTable.ClassList{{"数"}, {"语"}, {"英"}}
	result := ResolveClassList(cl, 1)
	assert.Equal(t, []string{"数", "语", "英"}, result)
}

func TestResolveClassList_MultiWeek_Rotating(t *testing.T) {
	// Week 1: "数", Week 2: "代", Week 3: "几"
	cl := dbTable.ClassList{{"数", "代", "几"}, {"语"}}
	result1 := ResolveClassList(cl, 1)
	assert.Equal(t, []string{"数", "语"}, result1)

	result2 := ResolveClassList(cl, 2)
	assert.Equal(t, []string{"代", "语"}, result2)

	result3 := ResolveClassList(cl, 3)
	assert.Equal(t, []string{"几", "语"}, result3)

	// Week 4 wraps back to week 1
	result4 := ResolveClassList(cl, 4)
	assert.Equal(t, []string{"数", "语"}, result4)
}

func TestResolveClassList_EmptyItem(t *testing.T) {
	cl := dbTable.ClassList{{}, {"语"}}
	result := ResolveClassList(cl, 1)
	assert.Equal(t, []string{"", "语"}, result)
}

func TestFixWrongTimetable_InvalidTimetableFallback(t *testing.T) {
	timetable := map[string]map[string]interface{}{
		"常日": {"早上1": 1, "早上2": 2},
	}
	schedule := [7]dbTable.DailyClass{
		{Timetable: "不存在", ClassList: dbTable.ClassList{{"数"}, {"语"}}},
	}

	FixWrongTimetable(&schedule, timetable)
	assert.Equal(t, "常日", schedule[0].Timetable)
}

func TestFixWrongTimetable_PadClassList(t *testing.T) {
	timetable := map[string]map[string]interface{}{
		"常日": {"早上1": 1, "早上2": 2, "早上3": 3},
	}
	schedule := [7]dbTable.DailyClass{
		{Timetable: "常日", ClassList: dbTable.ClassList{{"数"}}},
	}

	FixWrongTimetable(&schedule, timetable)
	// timetableNeedCount finds max value (3) and returns 3+1=4
	assert.Equal(t, 4, len(schedule[0].ClassList))
	assert.Equal(t, []string{"数"}, schedule[0].ClassList[0])
	assert.Equal(t, []string{"课"}, schedule[0].ClassList[1])
	assert.Equal(t, []string{"课"}, schedule[0].ClassList[2])
	assert.Equal(t, []string{"课"}, schedule[0].ClassList[3])
}

func TestFixWrongTimetable_TrimClassList(t *testing.T) {
	timetable := map[string]map[string]interface{}{
		"常日": {"早上1": 1},
	}
	schedule := [7]dbTable.DailyClass{
		{Timetable: "常日", ClassList: dbTable.ClassList{{"数"}, {"语"}, {"英"}}},
	}

	FixWrongTimetable(&schedule, timetable)
	// timetableNeedCount finds max value (1) and returns 1+1=2
	assert.Equal(t, 2, len(schedule[0].ClassList))
	assert.Equal(t, []string{"数"}, schedule[0].ClassList[0])
	assert.Equal(t, []string{"语"}, schedule[0].ClassList[1])
}

func TestBuildPeriodsForDate_NormalDay(t *testing.T) {
	// 契约：timetable 的 value 是 0 起始的 classList 下标（desktop scheduleConfig.js 注释明确），
	// 返回的 Period.No 为 1 起始的展示序号（下标 + 1）。
	timetable := map[string]map[string]interface{}{
		"常日": {"08:00-08:40": 0, "08:50-09:30": 1, "10:00-10:40": 2},
	}
	// 2025-10-13 is a Monday (index 1)
	date := time.Date(2025, 10, 13, 0, 0, 0, 0, time.Local)
	schedule := [7]dbTable.DailyClass{
		{}, // Sunday
		{Timetable: "常日", ClassList: dbTable.ClassList{{"数"}, {"语"}, {"英"}}}, // Monday
		{}, {}, {}, {}, {},
	}

	periods := BuildPeriodsForDate(schedule, timetable, date)
	assert.Equal(t, 3, len(periods))
	assert.Equal(t, 1, periods[0].No)
	assert.Equal(t, "数", periods[0].Subject)
	assert.Equal(t, 2, periods[1].No)
	assert.Equal(t, "语", periods[1].Subject)
	assert.Equal(t, 3, periods[2].No)
	assert.Equal(t, "英", periods[2].Subject)
}

func TestBuildPeriodsForDate_OutOfRangeIndex(t *testing.T) {
	// 下标超出 classList 长度时该节次科目为空串（不 panic）
	timetable := map[string]map[string]interface{}{
		"常日": {"08:00-08:40": 0, "08:50-09:30": 5},
	}
	date := time.Date(2025, 10, 13, 0, 0, 0, 0, time.Local)
	schedule := [7]dbTable.DailyClass{
		{}, // Sunday
		{Timetable: "常日", ClassList: dbTable.ClassList{{"数"}}}, // Monday
		{}, {}, {}, {}, {},
	}

	periods := BuildPeriodsForDate(schedule, timetable, date)
	assert.Equal(t, 2, len(periods))
	assert.Equal(t, "数", periods[0].Subject)
	assert.Equal(t, "", periods[1].Subject)
}

// ApplyScheduleRules 规则引擎测试（COMPENSATION → TIMETABLE → SCHEDULE → ALL 优先级叠加）

func mondayDate() time.Time {
	// 2025-10-13 为周一（weekday index 1）
	return time.Date(2025, 10, 13, 0, 0, 0, 0, time.Local)
}

func baseSchedule() [7]dbTable.DailyClass {
	return [7]dbTable.DailyClass{
		{}, // Sunday
		{Timetable: "常日", ClassList: dbTable.ClassList{{"数"}, {"语"}}}, // Monday
		{Timetable: "常日", ClassList: dbTable.ClassList{{"英"}}},        // Tuesday
		{}, {}, {}, {},
	}
}

func baseTimetable() map[string]map[string]interface{} {
	return map[string]map[string]interface{}{
		"常日": {"08:00-08:40": 0, "08:50-09:30": 1},
		"exam": {"09:00-10:00": 0},
	}
}

func makeRecord(etype int, scope []string, level int, rule map[string]interface{}) dbTable.AutorunRecord {
	return dbTable.AutorunRecord{
		HashID:     "h",
		EType:      etype,
		Scope:      scope,
		Level:      level,
		Parameters: map[string]interface{}{"rule": rule},
	}
}

func TestApplyScheduleRules_Compensation(t *testing.T) {
	records := []dbTable.AutorunRecord{
		makeRecord(0, []string{"ALL"}, 1, map[string]interface{}{"date": "2025-10-13", "useDate": "2025-10-14"}),
	}
	resolved := ApplyScheduleRules(baseSchedule(), baseTimetable(), records, "s", "g", "c", mondayDate())

	// 周一按调休规则使用周二的课；随后 FixWrongTimetable 按作息节数（2 节）补齐"课"占位
	assert.Equal(t, dbTable.ClassList{{"英"}, {"课"}}, resolved[1].ClassList)
	assert.Equal(t, "常日", resolved[1].Timetable)
	// 调休只改目标日，周二本身内容不变（同样被补齐占位）
	assert.Equal(t, dbTable.ClassList{{"英"}, {"课"}}, resolved[2].ClassList, "周二的课不应被改")
}

func TestApplyScheduleRules_Timetable(t *testing.T) {
	records := []dbTable.AutorunRecord{
		makeRecord(1, []string{"ALL"}, 1, map[string]interface{}{"date": "2025-10-13", "timetableId": "exam"}),
	}
	resolved := ApplyScheduleRules(baseSchedule(), baseTimetable(), records, "s", "g", "c", mondayDate())

	assert.Equal(t, "exam", resolved[1].Timetable)
}

func TestApplyScheduleRules_Schedule(t *testing.T) {
	records := []dbTable.AutorunRecord{
		makeRecord(2, []string{"ALL"}, 1, map[string]interface{}{
			"date": "2025-10-13",
			"schedule": map[string]interface{}{
				"periods": []interface{}{
					map[string]interface{}{"no": 1, "subject": "班会"},
					map[string]interface{}{"no": 2, "subject": "自习"},
				},
			},
		}),
	}
	resolved := ApplyScheduleRules(baseSchedule(), baseTimetable(), records, "s", "g", "c", mondayDate())

	assert.Equal(t, dbTable.ClassList{{"班会"}, {"自习"}}, resolved[1].ClassList)
	assert.Equal(t, "常日", resolved[1].Timetable)
}

func TestApplyScheduleRules_All(t *testing.T) {
	records := []dbTable.AutorunRecord{
		makeRecord(3, []string{"ALL"}, 1, map[string]interface{}{
			"date":        "2025-10-13",
			"timetableId": "exam",
			"schedule": map[string]interface{}{
				"periods": []interface{}{
					map[string]interface{}{"no": 1, "subject": "考试"},
				},
			},
		}),
	}
	resolved := ApplyScheduleRules(baseSchedule(), baseTimetable(), records, "s", "g", "c", mondayDate())

	assert.Equal(t, "exam", resolved[1].Timetable)
	assert.Equal(t, dbTable.ClassList{{"考试"}}, resolved[1].ClassList)
}

func TestApplyScheduleRules_PriorityOrdering(t *testing.T) {
	// 同日期两条 SCHEDULE 规则：priority 更高（Level 更大）者胜出
	records := []dbTable.AutorunRecord{
		makeRecord(2, []string{"ALL"}, 1, map[string]interface{}{
			"date":     "2025-10-13",
			"schedule": map[string]interface{}{"periods": []interface{}{map[string]interface{}{"no": 1, "subject": "低优先级"}}},
		}),
		makeRecord(2, []string{"ALL"}, 2, map[string]interface{}{
			"date":     "2025-10-13",
			"schedule": map[string]interface{}{"periods": []interface{}{map[string]interface{}{"no": 1, "subject": "高优先级"}}},
		}),
	}
	resolved := ApplyScheduleRules(baseSchedule(), baseTimetable(), records, "s", "g", "c", mondayDate())

	// 高优先级覆盖低优先级；FixWrongTimetable 补齐"课"占位
	assert.Equal(t, dbTable.ClassList{{"高优先级"}, {"课"}}, resolved[1].ClassList)
}

func TestApplyScheduleRules_ScopeSpecificity(t *testing.T) {
	// 同优先级下，更具体的 scope（班级）覆盖 ALL
	records := []dbTable.AutorunRecord{
		makeRecord(2, []string{"ALL"}, 1, map[string]interface{}{
			"date":     "2025-10-13",
			"schedule": map[string]interface{}{"periods": []interface{}{map[string]interface{}{"no": 1, "subject": "全校"}}},
		}),
		makeRecord(2, []string{"s/g/c"}, 1, map[string]interface{}{
			"date":     "2025-10-13",
			"schedule": map[string]interface{}{"periods": []interface{}{map[string]interface{}{"no": 1, "subject": "本班"}}},
		}),
		// 其它班级的规则不应生效
		makeRecord(2, []string{"s/g/other"}, 1, map[string]interface{}{
			"date":     "2025-10-13",
			"schedule": map[string]interface{}{"periods": []interface{}{map[string]interface{}{"no": 1, "subject": "他班"}}},
		}),
	}
	resolved := ApplyScheduleRules(baseSchedule(), baseTimetable(), records, "s", "g", "c", mondayDate())

	// 班级规则覆盖 ALL 规则；随后 FixWrongTimetable 按作息节数（2 节）补齐"课"占位
	assert.Equal(t, dbTable.ClassList{{"本班"}, {"课"}}, resolved[1].ClassList)
}

func TestApplyScheduleRules_DateMismatch(t *testing.T) {
	records := []dbTable.AutorunRecord{
		makeRecord(2, []string{"ALL"}, 1, map[string]interface{}{
			"date":     "2025-10-14",
			"schedule": map[string]interface{}{"periods": []interface{}{map[string]interface{}{"no": 1, "subject": "明天"}}},
		}),
	}
	resolved := ApplyScheduleRules(baseSchedule(), baseTimetable(), records, "s", "g", "c", mondayDate())

	assert.Equal(t, dbTable.ClassList{{"数"}, {"语"}}, resolved[1].ClassList)
}

func TestApplyScheduleRules_TypePrecedence(t *testing.T) {
	// 同一日期四种规则同时命中：应用顺序 COMPENSATION → TIMETABLE → SCHEDULE → ALL，后应用者覆盖前者
	records := []dbTable.AutorunRecord{
		makeRecord(0, []string{"ALL"}, 1, map[string]interface{}{"date": "2025-10-13", "useDate": "2025-10-15"}),
		makeRecord(1, []string{"ALL"}, 1, map[string]interface{}{"date": "2025-10-13", "timetableId": "exam"}),
		makeRecord(2, []string{"ALL"}, 1, map[string]interface{}{
			"date": "2025-10-13",
			"schedule": map[string]interface{}{"periods": []interface{}{
				map[string]interface{}{"no": 1, "subject": "班会"},
				map[string]interface{}{"no": 2, "subject": "自习"},
			}},
		}),
		makeRecord(3, []string{"ALL"}, 1, map[string]interface{}{
			"date":        "2025-10-13",
			"timetableId": "常日",
			"schedule": map[string]interface{}{"periods": []interface{}{
				map[string]interface{}{"no": 1, "subject": "考试"},
			}},
		}),
	}
	resolved := ApplyScheduleRules(baseSchedule(), baseTimetable(), records, "s", "g", "c", mondayDate())

	// ALL 最后应用覆盖前三种规则；FixWrongTimetable 按常日 2 节补齐占位
	assert.Equal(t, "常日", resolved[1].Timetable)
	assert.Equal(t, dbTable.ClassList{{"考试"}, {"课"}}, resolved[1].ClassList)
}

func TestCalcWeekNumber_SundayBoundary(t *testing.T) {
	// 开学日恰逢周日：当天为第 1 周，7 天后进入第 2 周
	start := time.Date(2025, 9, 7, 0, 0, 0, 0, time.UTC) // 2025-09-07 是周日
	assert.Equal(t, 1, CalcWeekNumber("2025-09-07", start))
	assert.Equal(t, 2, CalcWeekNumber("2025-09-07", start.Add(7*24*time.Hour)))
}

func TestApplyScheduleRules_TypePrecedence_Pairs(t *testing.T) {
	// 不含 ALL 的相邻规则组合，分别验证 COMPENSATION / TIMETABLE / SCHEDULE 两两之间的应用顺序边界
	t.Run("TIMETABLE 覆盖 COMPENSATION 的作息", func(t *testing.T) {
		records := []dbTable.AutorunRecord{
			makeRecord(0, []string{"ALL"}, 1, map[string]interface{}{"date": "2025-10-13", "useDate": "2025-10-15"}),
			makeRecord(1, []string{"ALL"}, 1, map[string]interface{}{"date": "2025-10-13", "timetableId": "exam"}),
		}
		resolved := ApplyScheduleRules(baseSchedule(), baseTimetable(), records, "s", "g", "c", mondayDate())

		// COMPENSATION 先拷入周三（空）课表，TIMETABLE 随后把作息改为 exam；exam 单节补“课”占位
		assert.Equal(t, "exam", resolved[1].Timetable)
		assert.Equal(t, dbTable.ClassList{{"课"}}, resolved[1].ClassList)
	})

	t.Run("SCHEDULE 覆盖 COMPENSATION 的课表", func(t *testing.T) {
		records := []dbTable.AutorunRecord{
			makeRecord(0, []string{"ALL"}, 1, map[string]interface{}{"date": "2025-10-13", "useDate": "2025-10-15"}),
			makeRecord(2, []string{"ALL"}, 1, map[string]interface{}{
				"date": "2025-10-13",
				"schedule": map[string]interface{}{"periods": []interface{}{
					map[string]interface{}{"no": 1, "subject": "班会"},
					map[string]interface{}{"no": 2, "subject": "自习"},
				}},
			}),
		}
		resolved := ApplyScheduleRules(baseSchedule(), baseTimetable(), records, "s", "g", "c", mondayDate())

		// SCHEDULE 后应用：若顺序颠倒，COMPENSATION 会把课表清回周三空课表，此处应保有班会/自习
		assert.Equal(t, dbTable.ClassList{{"班会"}, {"自习"}}, resolved[1].ClassList)
		assert.Equal(t, "常日", resolved[1].Timetable)
	})
}
