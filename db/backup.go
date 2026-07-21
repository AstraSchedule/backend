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
	Namespace     string `json:"namespace,omitempty"`
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

// setSliceNamespace 使用 reflect 将切片中所有元素的 Namespace 字段设为指定值
func setSliceNamespace(slice any, ns string) {
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
		nsField := elem.FieldByName("Namespace")
		if nsField.IsValid() && nsField.CanSet() && nsField.Kind() == reflect.String {
			nsField.SetString(ns)
		}
	}
}

// overrideBackupNamespace 将备份数据中所有记录的 Namespace 设为指定值
func overrideBackupNamespace(payload *BackupPayload, ns string) {
	setSliceNamespace(payload.Schedules, ns)
	setSliceNamespace(payload.ClientConfigs, ns)
	setSliceNamespace(payload.Timetables, ns)
	setSliceNamespace(payload.Subjects, ns)
	setSliceNamespace(payload.DataVersions, ns)
	setSliceNamespace(payload.AutorunRecords, ns)
	setSliceNamespace(payload.CountdownRecord, ns)
}

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

// ExportBackup 导出备份（无命名空间，向后兼容）
func ExportBackup() (*BackupPayload, error) {
	return ExportBackupNs("")
}

// ExportBackupNs 导出备份（带命名空间）
func ExportBackupNs(namespace string) (*BackupPayload, error) {
	dbConn := GetDB()
	payload := &BackupPayload{
		Meta: BackupMeta{
			SchemaVersion: 1,
			GeneratedAt:   time.Now().Format(time.RFC3339),
			Namespace:     namespace,
		},
	}

	base := dbConn
	if namespace != "" {
		base = base.Where("namespace = ?", namespace)
	}

	if err := base.Session(&gorm.Session{}).Order(orderByIDAsc).Find(&payload.Schedules).Error; err != nil {
		return nil, err
	}
	if err := base.Session(&gorm.Session{}).Order(orderByIDAsc).Find(&payload.ClientConfigs).Error; err != nil {
		return nil, err
	}
	if err := base.Session(&gorm.Session{}).Order(orderByIDAsc).Find(&payload.Timetables).Error; err != nil {
		return nil, err
	}
	if err := base.Session(&gorm.Session{}).Order(orderByIDAsc).Find(&payload.Subjects).Error; err != nil {
		return nil, err
	}
	if err := base.Session(&gorm.Session{}).Order(orderByIDAsc).Find(&payload.DataVersions).Error; err != nil {
		return nil, err
	}
	if err := base.Session(&gorm.Session{}).Order(orderByCreatedAtAsc).Find(&payload.AutorunRecords).Error; err != nil {
		return nil, err
	}
	if err := base.Session(&gorm.Session{}).Order(orderByCreatedAtAsc).Find(&payload.CountdownRecord).Error; err != nil {
		return nil, err
	}
	if err := dbConn.Order(orderByIDAsc).Find(&payload.Users).Error; err != nil {
		return nil, err
	}

	return payload, nil
}

// ImportBackup 导入备份（向后兼容，不覆盖命名空间）
func ImportBackup(payload *BackupPayload, mode string) (*BackupImportResult, error) {
	return ImportBackupNs(payload, mode, "")
}

// ImportBackupNs 导入备份，可指定目标命名空间覆盖导入数据的 namespace
// overrideNs 为空时不覆盖，保持备份数据原有的 namespace
func ImportBackupNs(payload *BackupPayload, mode string, overrideNs string) (*BackupImportResult, error) {
	if payload == nil {
		return nil, gorm.ErrInvalidData
	}

	// 如果指定了目标命名空间，覆盖所有记录的 namespace
	if overrideNs != "" {
		overrideBackupNamespace(payload, overrideNs)
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
		columns:    []clause.Column{{Name: "namespace"}, {Name: "school"}, {Name: "grade"}, {Name: "class"}},
		updateCols: []string{"daily_classes"},
		mode:       mode,
	})
}

func importClientConfigs(tx *gorm.DB, rows []dbTable.ClientConfig, mode string) (int, error) {
	return importRows(tx, rows, onConflictSpec{
		columns: []clause.Column{{Name: "namespace"}, {Name: "school"}, {Name: "grade"}, {Name: "class"}},
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
		columns:    []clause.Column{{Name: "namespace"}, {Name: "school"}, {Name: "grade"}},
		updateCols: []string{"timetable", "divider", "start_date"},
		mode:       mode,
	})
}

func importSubjects(tx *gorm.DB, rows []dbTable.Subject, mode string) (int, error) {
	return importRows(tx, rows, onConflictSpec{
		columns:    []clause.Column{{Name: "namespace"}, {Name: "school"}, {Name: "grade"}},
		updateCols: []string{"subject_name"},
		mode:       mode,
	})
}

func importDataVersions(tx *gorm.DB, rows []dbTable.DataVersion, mode string) (int, error) {
	return importRows(tx, rows, onConflictSpec{
		columns:    []clause.Column{{Name: "namespace"}, {Name: "school"}, {Name: "grade"}, {Name: "class"}},
		updateCols: []string{"version", "updated_at"},
		mode:       mode,
	})
}

// importIDRows 通用的基于主键导入逻辑，支持 overwrite/skip 模式
func importIDRows(tx *gorm.DB, rows interface{}, idCol string, updateCols []string, mode string) (int, error) {
	// 使用 reflect 检查切片长度
	switch v := rows.(type) {
	case []dbTable.AutorunRecord:
		if len(v) == 0 {
			return 0, nil
		}
	case []dbTable.CountdownRecord:
		if len(v) == 0 {
			return 0, nil
		}
	}

	onConflict := clause.OnConflict{
		Columns: []clause.Column{{Name: idCol}},
	}
	if mode == "skip" {
		onConflict.DoNothing = true
	} else {
		onConflict.DoUpdates = clause.AssignmentColumns(updateCols)
	}

	err := tx.Clauses(onConflict).Create(rows).Error
	if err != nil {
		return 0, err
	}
	switch v := rows.(type) {
	case []dbTable.AutorunRecord:
		return len(v), nil
	case []dbTable.CountdownRecord:
		return len(v), nil
	}
	return 0, nil
}

func importAutorunRecords(tx *gorm.DB, rows []dbTable.AutorunRecord, mode string) (int, error) {
	return importIDRows(tx, rows, "hash_id", []string{
		"namespace", "e_type", "scope", "parameters", "level", "status", "created_at", "updated_at",
	}, mode)
}

func importCountdownRecords(tx *gorm.DB, rows []dbTable.CountdownRecord, mode string) (int, error) {
	return importIDRows(tx, rows, "id", []string{
		"namespace", "scope", "schedules", "created_at", "updated_at",
	}, mode)
}

// importUsers 导入用户数据，按 namespace+username 做 upsert
// 主分支不带 namespace，但保留函数签名一致，方便后续 saas/main 合并
func importUsers(tx *gorm.DB, rows []dbTable.User, mode string) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}

	var onConflict clause.OnConflict
	if mode == "skip" {
		onConflict = clause.OnConflict{
			Columns:   []clause.Column{{Name: "namespace"}, {Name: "username"}},
			DoNothing: true,
		}
	} else {
		onConflict = clause.OnConflict{
			Columns: []clause.Column{{Name: "namespace"}, {Name: "username"}},
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
