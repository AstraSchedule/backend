package db

import (
	"AstraScheduleServerGo/model/dbTable"
	"time"

	"gorm.io/gorm/clause"
)

const hashIDWhere = "hash_id = ?"

// FetchAutorunRecords 获取自动任务记录（无命名空间，向后兼容）
func FetchAutorunRecords(hashid string) ([]dbTable.AutorunRecord, error) {
	return FetchAutorunRecordsNs("", hashid)
}

// FetchAutorunRecordsNs 获取自动任务记录（带命名空间）
// 安全修复：namespace 为空时不查询，避免跨租户返回数据
func FetchAutorunRecordsNs(namespace, hashid string) ([]dbTable.AutorunRecord, error) {
	records := make([]dbTable.AutorunRecord, 0)
	if namespace == "" {
		return records, nil
	}
	q := GetDB().Model(&dbTable.AutorunRecord{}).Where("namespace = ?", namespace)
	if hashid != "" {
		q = q.Where(hashIDWhere, hashid)
	}
	err := q.Find(&records).Error
	return records, err
}

// DeleteAutorunRecord 删除自动任务记录（无命名空间，向后兼容）
func DeleteAutorunRecord(hashid string) (int64, error) {
	return DeleteAutorunRecordNs("", hashid)
}

// DeleteAutorunRecordNs 删除自动任务记录（带命名空间）
// 安全修复：namespace 为空时不删除，避免跨租户删除
func DeleteAutorunRecordNs(namespace, hashid string) (int64, error) {
	if namespace == "" {
		return 0, nil
	}
	resp := GetDB().Where(hashIDWhere, hashid).Where("namespace = ?", namespace).Delete(&dbTable.AutorunRecord{})
	return resp.RowsAffected, resp.Error
}

func UpsertAutorunRecord(record *dbTable.AutorunRecord) error {
	return GetDB().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "hash_id"}},
		UpdateAll: true,
	}).Create(record).Error
}

func deriveStatusForRecord(etype int, parameters map[string]interface{}, today time.Time) int {
	if etype < 0 || etype > 3 {
		return 0
	}
	var rule map[string]interface{}
	if rv, ok := parameters["rule"].(map[string]interface{}); ok {
		rule = rv
	} else {
		rule = parameters
	}
	dateStr, _ := rule["date"].(string)
	if dateStr == "" {
		return 0
	}
	day, err := time.ParseInLocation("2006-01-02", dateStr, today.Location())
	if err != nil {
		return 0
	}
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	ruleDate := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	if todayDate.Before(ruleDate) {
		return 0
	}
	if todayDate.Equal(ruleDate) {
		return 1
	}
	return 2
}

// RefreshAutorunStatuses 刷新自动任务状态（无命名空间，向后兼容）
func RefreshAutorunStatuses(today time.Time) (int64, error) {
	return RefreshAutorunStatusesNs("", today)
}

// RefreshAutorunStatusesNs 刷新自动任务状态（带命名空间）
func RefreshAutorunStatusesNs(namespace string, today time.Time) (int64, error) {
	records, err := FetchAutorunRecordsNs(namespace, "")
	if err != nil {
		return 0, err
	}
	updated := int64(0)
	for i := range records {
		newStatus := deriveStatusForRecord(records[i].EType, records[i].Parameters, today)
		if newStatus == records[i].Status {
			continue
		}
		if err := GetDB().Model(&dbTable.AutorunRecord{}).
			Where(hashIDWhere, records[i].HashID).
			Update("status", newStatus).Error; err != nil {
			return updated, err
		}
		updated++
	}
	return updated, nil
}
