package db

import (
	"AstraScheduleServerGo/model/dbTable"

	"gorm.io/gorm/clause"
)

// FetchCountdownRecords 获取倒数日记录（无命名空间，向后兼容）
func FetchCountdownRecords(id string) ([]dbTable.CountdownRecord, error) {
	return FetchCountdownRecordsNs("", id)
}

// FetchCountdownRecordsNs 获取倒数日记录（带命名空间）
// 安全修复：namespace 为空时不查询，避免跨租户返回数据
func FetchCountdownRecordsNs(namespace, id string) ([]dbTable.CountdownRecord, error) {
	records := make([]dbTable.CountdownRecord, 0)
	if namespace == "" {
		return records, nil
	}
	q := GetDB().Model(&dbTable.CountdownRecord{}).Where("namespace = ?", namespace)
	if id != "" {
		q = q.Where("id = ?", id)
	}
	err := q.Order("updated_at DESC").Find(&records).Error
	return records, err
}

// DeleteCountdownRecord 删除倒数日记录（无命名空间，向后兼容）
func DeleteCountdownRecord(id string) (int64, error) {
	return DeleteCountdownRecordNs("", id)
}

// DeleteCountdownRecordNs 删除倒数日记录（带命名空间）
// 安全修复：namespace 为空时不删除，避免跨租户删除
func DeleteCountdownRecordNs(namespace, id string) (int64, error) {
	if namespace == "" {
		return 0, nil
	}
	resp := GetDB().Where("id = ?", id).Where("namespace = ?", namespace).Delete(&dbTable.CountdownRecord{})
	return resp.RowsAffected, resp.Error
}

func UpsertCountdownRecord(record *dbTable.CountdownRecord) error {
	return GetDB().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(record).Error
}
