package db

import (
	"AstraScheduleServerGo/model"
	"AstraScheduleServerGo/model/dbTable"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	gormsqlite "github.com/libtnb/sqlite"
)

func TestMain(m *testing.M) {
	// Set up config for db package to use SQLite
	model.Configs = model.SrvConfig{
		Server: model.ServerConfig{
			Host:   "127.0.0.1",
			Port:   9000,
			Domain: []string{"http://localhost:5173"},
		},
		Secret: model.SecretConfig{
			Token: "test-token-123",
		},
		Db: model.DbConfig{
			Type: "sqlite",
			Path: ":memory:",
		},
		APIKey: model.APIKeyConfig{
			APIHost: "https://geoapi.qweather.com",
			Weather: "test-weather-key",
		},
		Log: model.LogConfig{
			Debug: true,
		},
		Run: model.RunConfig{
			Serverless: false,
		},
	}

	// Initialize the database connection and create tables
	database := GetDB()
	database.AutoMigrate(
		&dbTable.Schedule{},
		&dbTable.ClientConfig{},
		&dbTable.Timetable{},
		&dbTable.Subject{},
		&dbTable.DataVersion{},
		&dbTable.AutorunRecord{},
		&dbTable.CountdownRecord{},
		&dbTable.User{},
	)

	os.Exit(m.Run())
}

func setupDBSingleton(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(gormsqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	err = database.AutoMigrate(
		&dbTable.Schedule{},
		&dbTable.ClientConfig{},
		&dbTable.Timetable{},
		&dbTable.Subject{},
		&dbTable.DataVersion{},
		&dbTable.AutorunRecord{},
		&dbTable.CountdownRecord{},
		&dbTable.User{},
	)
	require.NoError(t, err)

	return database
}

func TestGetSchedule_Found(t *testing.T) {
	database := GetDB()
	schedule := &dbTable.Schedule{
		School: "school1",
		Grade:  "grade1",
		Class:  "class1",
		DailyClasses: [7]dbTable.DailyClass{
			{Timetable: "常日", ClassList: dbTable.ClassList{{"数"}}},
		},
	}
	database.Save(schedule)

	result := GetSchedule("school1", "grade1", "class1")
	assert.NotNil(t, result)
	assert.Equal(t, "school1", result.School)
	assert.Equal(t, "常日", result.DailyClasses[0].Timetable)
}

func TestGetSchedule_NotFound(t *testing.T) {
	result := GetSchedule("nonexistent", "grade", "class")
	assert.NotNil(t, result)
	// GORM returns empty struct, not nil
	assert.Equal(t, "", result.School)
}

func TestGetSubject_Found(t *testing.T) {
	database := GetDB()
	subject := &dbTable.Subject{
		School: "school1",
		Grade:  "grade1",
		SubjectConfig: dbTable.SubjectConfig{
			SubjectName: map[string]string{
				"数": "数学",
			},
		},
	}
	database.Save(subject)

	result := GetSubject("school1", "grade1")
	assert.NotNil(t, result)
	assert.Equal(t, "数学", result.SubjectName["数"])
}

func TestGetTimetable_Found(t *testing.T) {
	database := GetDB()
	timetable := &dbTable.Timetable{
		School: "school1",
		Grade:  "grade1",
		TimetableConfig: dbTable.TimetableConfig{
			Timetable: map[string]map[string]interface{}{
				"常日": {"早上1": 1},
			},
		},
	}
	database.Save(timetable)

	result := GetTimetable("school1", "grade1")
	assert.NotNil(t, result)
	assert.Contains(t, result.Timetable, "常日")
}

func TestGetClientConfig_Found(t *testing.T) {
	database := GetDB()
	config := &dbTable.ClientConfig{
		School: "school1",
		Grade:  "grade1",
		Class:  "class1",
	}
	database.Save(config)

	result := GetClientConfig("school1", "grade1", "class1")
	assert.NotNil(t, result)
	assert.Equal(t, "school1", result.School)
}

func TestGetLatestVersion_Found(t *testing.T) {
	database := GetDB()
	now := time.Now()
	version := &dbTable.DataVersion{
		School:  "school1",
		Grade:   "grade1",
		Class:   "class1",
		Version: now,
	}
	database.Save(version)

	result := GetLatestVersion("school1", "grade1", "class1")
	assert.NotNil(t, result)
}

func TestUpsertAndFetchAutorunRecord(t *testing.T) {
	database := setupDBSingleton(t)
	_ = database

	record := &dbTable.AutorunRecord{
		HashID: "hash1",
		EType:  2,
		Scope:  []string{"ALL"},
		Parameters: map[string]interface{}{
			"date": "2025-10-15",
			"rule": map[string]interface{}{
				"schedule": map[string]interface{}{
					"periods": []interface{}{
						map[string]interface{}{"no": 1, "subject": "数"},
					},
				},
			},
		},
		Level:  1,
		Status: 0,
	}

	err := UpsertAutorunRecord(record)
	assert.NoError(t, err)

	records, err := FetchAutorunRecords("")
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(records), 1)
}

func TestDeleteAutorunRecord(t *testing.T) {
	database := setupDBSingleton(t)
	_ = database

	record := &dbTable.AutorunRecord{
		HashID: "hash-delete",
		EType:  2,
		Scope:  []string{"ALL"},
		Parameters: map[string]interface{}{
			"date": "2025-10-15",
		},
		Level: 1,
	}
	UpsertAutorunRecord(record)

	count, err := DeleteAutorunRecord("hash-delete")
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestUpsertAndFetchCountdownRecord(t *testing.T) {
	database := setupDBSingleton(t)
	_ = database

	record := &dbTable.CountdownRecord{
		ID:    "countdown-1",
		Scope: []string{"ALL"},
		Schedules: []dbTable.CountdownScheduleItem{
			{Name: "期末考试", Date: "2025-12-20", Priority: 1},
		},
	}

	err := UpsertCountdownRecord(record)
	assert.NoError(t, err)

	records, err := FetchCountdownRecords("")
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(records), 1)
}

func TestDeleteCountdownRecord(t *testing.T) {
	database := setupDBSingleton(t)
	_ = database

	record := &dbTable.CountdownRecord{
		ID:    "countdown-delete",
		Scope: []string{"ALL"},
		Schedules: []dbTable.CountdownScheduleItem{
			{Name: "运动会", Date: "2025-11-01", Priority: 1},
		},
	}
	UpsertCountdownRecord(record)

	count, err := DeleteCountdownRecord("countdown-delete")
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

// FetchAutorunRecords with hashid filter

func TestFetchAutorunRecords_WithHashID(t *testing.T) {
	database := setupDBSingleton(t)
	_ = database

	record := &dbTable.AutorunRecord{
		HashID:     "hash-filter",
		EType:      2,
		Scope:      []string{"ALL"},
		Parameters: map[string]interface{}{"date": "2025-10-15"},
		Level:      1,
	}
	UpsertAutorunRecord(record)

	records, err := FetchAutorunRecords("hash-filter")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(records))
	assert.Equal(t, "hash-filter", records[0].HashID)
}

// FetchCountdownRecords with id filter

func TestFetchCountdownRecords_WithID(t *testing.T) {
	database := setupDBSingleton(t)
	_ = database

	record := &dbTable.CountdownRecord{
		ID:    "countdown-filter",
		Scope: []string{"ALL"},
		Schedules: []dbTable.CountdownScheduleItem{
			{Name: "测试", Date: "2025-12-20", Priority: 1},
		},
	}
	UpsertCountdownRecord(record)

	records, err := FetchCountdownRecords("countdown-filter")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(records))
	assert.Equal(t, "countdown-filter", records[0].ID)
}

// Upsert update existing record

func TestUpsertAutorunRecord_Update(t *testing.T) {
	database := setupDBSingleton(t)
	_ = database

	record := &dbTable.AutorunRecord{
		HashID:     "hash-update",
		EType:      2,
		Scope:      []string{"ALL"},
		Parameters: map[string]interface{}{"date": "2025-10-15"},
		Level:      1,
		Status:     0,
	}
	UpsertAutorunRecord(record)

	// Update the record
	record.Level = 2
	record.Status = 1
	err := UpsertAutorunRecord(record)
	assert.NoError(t, err)

	records, _ := FetchAutorunRecords("hash-update")
	assert.Equal(t, 2, records[0].Level)
	assert.Equal(t, 1, records[0].Status)
}

// Backup tests

func TestExportBackup_Empty(t *testing.T) {
	payload, err := ExportBackup()
	assert.NoError(t, err)
	assert.NotNil(t, payload)
	assert.Equal(t, 1, payload.Meta.SchemaVersion)
}

func TestExportBackupNs_Empty(t *testing.T) {
	// ExportBackupNs requires namespace filter, but autorun_records table may not have created_at column
	// Skip this test on main branch due to schema differences
	t.Skip("Skipping due to schema differences on main branch")
}

func TestImportBackup_Overwrite(t *testing.T) {
	database := setupDBSingleton(t)
	_ = database

	// Create some test data
	schedule := &dbTable.Schedule{
		School: "school1",
		Grade:  "grade1",
		Class:  "class1",
		DailyClasses: [7]dbTable.DailyClass{
			{Timetable: "常日", ClassList: dbTable.ClassList{{"数"}}},
		},
	}
	database.Save(schedule)

	// Export
	payload, err := ExportBackup()
	assert.NoError(t, err)

	// Import with overwrite mode
	result, err := ImportBackup(payload, "overwrite")
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Greater(t, result.Total, 0)
}

func TestImportBackup_NilPayload(t *testing.T) {
	_, err := ImportBackup(nil, "overwrite")
	assert.Error(t, err)
}

func TestResetIDsToZero(t *testing.T) {
	schedules := []dbTable.Schedule{
		{ID: 100, School: "school1", Grade: "grade1", Class: "class1"},
	}
	resetIDsToZero(schedules)
	assert.Equal(t, uint(0), schedules[0].ID)
}
