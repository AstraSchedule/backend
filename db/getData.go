package db

import (
	"AstraScheduleServerGo/model/dbTable"

	"github.com/dromara/carbon/v2"
)

const (
	scopeClassWhere   = "school = ? AND grade = ? AND class = ?"
	scopeGradeWhere   = "school = ? AND grade = ?"
	scopeNsClassWhere = "namespace = ? AND school = ? AND grade = ? AND class = ?"
	scopeNsGradeWhere = "namespace = ? AND school = ? AND grade = ?"
)

// GetLatestVersion 获取最新版本（无命名空间，向后兼容）
func GetLatestVersion(school, grade, class string) *carbon.Carbon {
	return GetLatestVersionNs("", school, grade, class)
}

// GetLatestVersionNs 获取最新版本（带命名空间）
// 安全修复：namespace 为空（release 模式 localhost/IP 等）时不查询，避免跨租户返回数据
func GetLatestVersionNs(namespace, school, grade, class string) *carbon.Carbon {
	if namespace == "" {
		return carbon.CreateFromTimestamp(0)
	}
	latestVersion := &dbTable.DataVersion{}
	GetDB().Where(scopeNsClassWhere, namespace, school, grade, class).Take(latestVersion)
	return carbon.CreateFromTimestamp(latestVersion.Version.Unix())
}

// GetClientConfig 获取客户端配置（无命名空间，向后兼容）
func GetClientConfig(school, grade, class string) *dbTable.ClientConfig {
	return GetClientConfigNs("", school, grade, class)
}

// GetClientConfigNs 获取客户端配置（带命名空间）
// 安全修复：namespace 为空时不查询，避免跨租户返回数据
func GetClientConfigNs(namespace, school, grade, class string) *dbTable.ClientConfig {
	clientConfig := &dbTable.ClientConfig{}
	if namespace != "" {
		GetDB().Where(scopeNsClassWhere, namespace, school, grade, class).Take(clientConfig)
	}
	return clientConfig
}

// GetSchedule 获取课表（无命名空间，向后兼容）
func GetSchedule(school, grade, class string) *dbTable.Schedule {
	return GetScheduleNs("", school, grade, class)
}

// GetScheduleNs 获取课表（带命名空间）
// 安全修复：namespace 为空时不查询，避免跨租户返回数据
func GetScheduleNs(namespace, school, grade, class string) *dbTable.Schedule {
	schedule := &dbTable.Schedule{}
	if namespace != "" {
		GetDB().Where(scopeNsClassWhere, namespace, school, grade, class).Take(schedule)
	}
	return schedule
}

// GetSubject 获取科目配置（无命名空间，向后兼容）
func GetSubject(school, grade string) *dbTable.Subject {
	return GetSubjectNs("", school, grade)
}

// GetSubjectNs 获取科目配置（带命名空间）
// 安全修复：namespace 为空时不查询，避免跨租户返回数据
func GetSubjectNs(namespace, school, grade string) *dbTable.Subject {
	subject := &dbTable.Subject{}
	if namespace != "" {
		GetDB().Where(scopeNsGradeWhere, namespace, school, grade).Take(subject)
	}
	return subject
}

// GetTimetable 获取作息表（无命名空间，向后兼容）
func GetTimetable(school, grade string) *dbTable.Timetable {
	return GetTimetableNs("", school, grade)
}

// GetTimetableNs 获取作息表（带命名空间）
// 安全修复：namespace 为空时不查询，避免跨租户返回数据
func GetTimetableNs(namespace, school, grade string) *dbTable.Timetable {
	timetable := &dbTable.Timetable{}
	if namespace != "" {
		GetDB().Where(scopeNsGradeWhere, namespace, school, grade).Take(timetable)
	}
	return timetable
}
