package dbTable

import "time"

// 自动任务规则类型（与 API 契约中的 type 数值一致，禁止改动数值）
const (
	AutorunTypeCompensation = 0 // 调休
	AutorunTypeTimetable    = 1 // 作息表调整
	AutorunTypeSchedule     = 2 // 课程表调整
	AutorunTypeAll          = 3 // 全部调整
)

type AutorunRecord struct {
	HashID     string                 `gorm:"primaryKey;not null;size:64" json:"hashid"`
	EType      int                    `gorm:"not null;index" json:"etype"`
	Scope      []string               `gorm:"type:json;not null;serializer:json" json:"scope"`
	Parameters map[string]interface{} `gorm:"type:json;not null;serializer:json" json:"parameters"`
	Level      int                    `gorm:"not null" json:"level"`
	Status     int                    `gorm:"not null" json:"status"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}
