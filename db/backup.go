package db

import (
	"AstraScheduleServerGo/model/dbTable"
	"reflect"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BackupMeta struct {
	SchemaVersion int    `json:"schema_version"`
	GeneratedAt   string `json:"generated_at"`
}

type BackupPayload struct {
	Meta            BackupMeta                `json:"meta"`
	Schedules       []dbTable.Schedule        `json:"schedules"`
	ClientConfigs   []dbTable.ClientConfig    `json:"client_configs"`
	Timetables      []dbTable.Timetable       `json:"timetables"`
	Subjects        []dbTable.Subject         `json:"subjects"`
	DataVersions    []dbTable.DataVersion     `json:"data_versions"`
	AutorunRecords  []dbTable.AutorunRecord   `json:"autorun_records"`
	CountdownRecord []dbTable.CountdownRecord `json:"countdown_records"`
	Users           []dbTable.User            `json:"users"`
}

type BackupImportResult struct {
	Imported map[string]int `json:"imported"`
	Total    int            `json:"total"`
}

const (
	orderByIDAsc        = "id ASC"
	orderByCreatedAtAsc = "created_at ASC"
)

// resetIDsToZero 将切片中所有元素的 ID 字段重置为 0，让数据库自动分配新 ID
func resetIDsToZero(slice any) {
	v := reflect.ValueOf(slice)
	if v.Kind() != reflect.Slice {
		return
	}
	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i)
		if elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
		}
		if elem.Kind() != reflect.Struct {
			continue
		}
		idField := elem.FieldByName("ID")
		if !idField.IsValid() || !idField.CanSet() {
			continue
		}
		switch idField.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			idField.SetInt(0)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			idField.SetUint(0)
		}
	}
}

func ExportBackup() (*BackupPayload, error) {
	dbConn := GetDB()
	payload := &BackupPayload{
		Meta: BackupMeta{
			SchemaVersion: 1,
			GeneratedAt:   time.Now().Format(time.RFC3339),
		},
	}

	if err := dbConn.Order(orderByIDAsc).Find(&payload.Schedules).Error; err != nil {
		return nil, err
	}
	if err := dbConn.Order(orderByIDAsc).Find(&payload.ClientConfigs).Error; err != nil {
		return nil, err
	}
	if err := dbConn.Order(orderByIDAsc).Find(&payload.Timetables).Error; err != nil {
		return nil, err
	}
	if err := dbConn.Order(orderByIDAsc).Find(&payload.Subjects).Error; err != nil {
		return nil, err
	}
	if err := dbConn.Order(orderByIDAsc).Find(&payload.DataVersions).Error; err != nil {
		return nil, err
	}
	if err := dbConn.Order(orderByCreatedAtAsc).Find(&payload.AutorunRecords).Error; err != nil {
		return nil, err
	}
	if err := dbConn.Order(orderByCreatedAtAsc).Find(&payload.CountdownRecord).Error; err != nil {
		return nil, err
	}
	if err := dbConn.Order(orderByIDAsc).Find(&payload.Users).Error; err != nil {
		return nil, err
	}

	return payload, nil
}

func ImportBackup(payload *BackupPayload, mode string) (*BackupImportResult, error) {
	if payload == nil {
		return nil, gorm.ErrInvalidData
	}

	// 默认 overwrite 模式
	if mode == "" {
		mode = "overwrite"
	}

	tx := GetDB().Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}

	result := &BackupImportResult{Imported: map[string]int{}}

	rollback := func(err error) (*BackupImportResult, error) {
		tx.Rollback()
		return nil, err
	}

	type importStep struct {
		name string
		run  func(*gorm.DB) (int, error)
	}

	steps := []importStep{
		{name: "schedules", run: func(tx *gorm.DB) (int, error) { return importSchedules(tx, payload.Schedules, mode) }},
		{name: "client_configs", run: func(tx *gorm.DB) (int, error) { return importClientConfigs(tx, payload.ClientConfigs, mode) }},
		{name: "timetables", run: func(tx *gorm.DB) (int, error) { return importTimetables(tx, payload.Timetables, mode) }},
		{name: "subjects", run: func(tx *gorm.DB) (int, error) { return importSubjects(tx, payload.Subjects, mode) }},
		{name: "data_versions", run: func(tx *gorm.DB) (int, error) { return importDataVersions(tx, payload.DataVersions, mode) }},
		{name: "autorun_records", run: func(tx *gorm.DB) (int, error) { return importAutorunRecords(tx, payload.AutorunRecords, mode) }},
		{name: "countdown_records", run: func(tx *gorm.DB) (int, error) { return importCountdownRecords(tx, payload.CountdownRecord, mode) }},
		{name: "users", run: func(tx *gorm.DB) (int, error) { return importUsers(tx, payload.Users, mode) }},
	}

	for _, step := range steps {
		n, err := step.run(tx)
		if err != nil {
			return rollback(err)
		}
		result.Imported[step.name] = n
		result.Total += n
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	return result, nil
}

// onConflictSpec 定义导入数据时的 OnConflict 配置
type onConflictSpec struct {
	columns    []clause.Column
	updateCols []string
	mode       string // "overwrite" 或 "skip"，默认 "overwrite"
}

// importRows 通用的批量导入函数，重置 ID 后通过 OnConflict 进行 upsert
func importRows[T any](tx *gorm.DB, rows []T, spec onConflictSpec) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	// 使用 reflect 将所有行的 ID 字段重置为 0，让数据库自动分配新 ID
	resetIDsToZero(rows)

	// 根据 mode 构建 OnConflict 子句
	var onConflict clause.OnConflict
	if spec.mode == "skip" {
		onConflict = clause.OnConflict{
			Columns:   spec.columns,
			DoNothing: true,
		}
	} else {
		// 默认 overwrite 模式
		onConflict = clause.OnConflict{
			Columns:   spec.columns,
			DoUpdates: clause.AssignmentColumns(spec.updateCols),
		}
	}

	if err := tx.Clauses(onConflict).Create(&rows).Error; err != nil {
		return 0, err
	}
	return len(rows), nil
}

func importSchedules(tx *gorm.DB, rows []dbTable.Schedule, mode string) (int, error) {
	return importRows(tx, rows, onConflictSpec{
		columns:    []clause.Column{{Name: "school"}, {Name: "grade"}, {Name: "class"}},
		updateCols: []string{"daily_classes"},
		mode:       mode,
	})
}

func importClientConfigs(tx *gorm.DB, rows []dbTable.ClientConfig, mode string) (int, error) {
	return importRows(tx, rows, onConflictSpec{
		columns: []clause.Column{{Name: "school"}, {Name: "grade"}, {Name: "class"}},
		updateCols: []string{
			"countdown_target",
			"weather_alert_override",
			"weather_alert_brief",
			"week_display",
			"banner_text",
			"css_style",
			"startup_behavior",
		},
		mode: mode,
	})
}

func importTimetables(tx *gorm.DB, rows []dbTable.Timetable, mode string) (int, error) {
	return importRows(tx, rows, onConflictSpec{
		columns:    []clause.Column{{Name: "school"}, {Name: "grade"}},
		updateCols: []string{"timetable", "divider", "start_date"},
		mode:       mode,
	})
}

func importSubjects(tx *gorm.DB, rows []dbTable.Subject, mode string) (int, error) {
	return importRows(tx, rows, onConflictSpec{
		columns:    []clause.Column{{Name: "school"}, {Name: "grade"}},
		updateCols: []string{"subject_name"},
		mode:       mode,
	})
}

func importDataVersions(tx *gorm.DB, rows []dbTable.DataVersion, mode string) (int, error) {
	return importRows(tx, rows, onConflictSpec{
		columns:    []clause.Column{{Name: "school"}, {Name: "grade"}, {Name: "class"}},
		updateCols: []string{"version", "updated_at"},
		mode:       mode,
	})
}

func importAutorunRecords(tx *gorm.DB, rows []dbTable.AutorunRecord, mode string) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}

	var onConflict clause.OnConflict
	if mode == "skip" {
		onConflict = clause.OnConflict{
			Columns:   []clause.Column{{Name: "hash_id"}},
			DoNothing: true,
		}
	} else {
		onConflict = clause.OnConflict{
			Columns: []clause.Column{{Name: "hash_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"e_type",
				"scope",
				"parameters",
				"level",
				"status",
				"created_at",
				"updated_at",
			}),
		}
	}

	err := tx.Clauses(onConflict).Create(&rows).Error
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}

func importCountdownRecords(tx *gorm.DB, rows []dbTable.CountdownRecord, mode string) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}

	var onConflict clause.OnConflict
	if mode == "skip" {
		onConflict = clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoNothing: true,
		}
	} else {
		onConflict = clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"scope",
				"schedules",
				"created_at",
				"updated_at",
			}),
		}
	}

	err := tx.Clauses(onConflict).Create(&rows).Error
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}

// importUsers 导入用户数据，主分支按 username 唯一索引做 upsert
//（saas/main 有 namespace+username 联合唯一索引，冲突目标不同，合并时注意适配）
func importUsers(tx *gorm.DB, rows []dbTable.User, mode string) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}

	var onConflict clause.OnConflict
	if mode == "skip" {
		onConflict = clause.OnConflict{
			Columns:   []clause.Column{{Name: "username"}},
			DoNothing: true,
		}
	} else {
		onConflict = clause.OnConflict{
			Columns: []clause.Column{{Name: "username"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"password_hash",
				"role",
				"scope",
				"must_change_pwd",
				"must_change_username",
				"updated_at",
			}),
		}
	}

	err := tx.Clauses(onConflict).Create(&rows).Error
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}
