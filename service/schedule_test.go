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
	timetable := map[string]map[string]interface{}{
		"常日": {"早上1": 1, "早上2": 2, "下午1": 3},
	}
	// 2025-10-13 is a Monday (index 1)
	date := time.Date(2025, 10, 13, 0, 0, 0, 0, time.Local)
	schedule := [7]dbTable.DailyClass{
		{}, // Sunday
		{Timetable: "常日", ClassList: dbTable.ClassList{{"数"}, {"语"}, {"英"}}}, // Monday
		{}, {}, {}, {}, {},
	}

	periods := BuildPeriodsForDate(schedule, timetable, date)
	// The function extracts indices from timetable values: 1, 2, 3
	// Then creates Period structs with No = index + 1
	// So periods should be: {No:2, Subject:"数"}, {No:3, Subject:"语"}, {No:4, Subject:"英"}
	// Wait, that doesn't match. Let me re-read the function.
	// The function uses indices as array indices into ClassList
	// indices = [1, 2, 3] (sorted)
	// For index 1: ClassList[1] = "语"
	// For index 2: ClassList[2] = "英"
	// For index 3: ClassList[3] = "" (out of bounds)
	// So the result should be: {No:2, "语"}, {No:3, "英"}, {No:4, ""}
	assert.Equal(t, 3, len(periods))
	assert.Equal(t, 2, periods[0].No)
	assert.Equal(t, "语", periods[0].Subject)
	assert.Equal(t, 3, periods[1].No)
	assert.Equal(t, "英", periods[1].Subject)
	assert.Equal(t, 4, periods[2].No)
	assert.Equal(t, "", periods[2].Subject)
}
