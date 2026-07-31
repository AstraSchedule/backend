package web

import (
	"AstraScheduleServerGo/db"
	"AstraScheduleServerGo/middleware"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

const maxBackupImportSize = 50 << 20 // 50MB

func ExportBackup(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	payload, err := db.ExportBackupNs(ns)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	content, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := "astra-backup-" + time.Now().Format("20060102-150405") + ".json"
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "application/json", content)
}

// handleImportBackup 统一的备份导入处理逻辑
func handleImportBackup(c *gin.Context, mode string) {
	if c.Request.ContentLength > maxBackupImportSize {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "上传文件过大（限制 50MB）"})
		return
	}

	payload, err := parseBackupPayload(c)
	if err != nil {
		if errors.Is(err, gorm.ErrInvalidData) {
			c.JSON(http.StatusBadRequest, gin.H{"detail": "备份文件内容无效"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}

	// 安全修复：不再接受请求参数指定的目标 namespace（query/postForm 均可伪造），
	// 目标租户一律取当前请求 Host 解析出的 namespace，防止跨租户覆盖数据/接管账号
	overrideNs := middleware.GetNamespace(c)

	// 自动检测自部署备份：如果备份数据没有 namespace，用请求来源的 namespace 填充
	if overrideNs == "" && len(payload.Schedules) > 0 && payload.Schedules[0].Namespace == "" {
		if ns := middleware.GetNamespace(c); ns != "" {
			overrideNs = ns
			logrus.Infof("自部署备份检测：自动填充 namespace=%s", overrideNs)
		}
	}

	result, err := db.ImportBackupNs(payload, mode, overrideNs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := gin.H{"status": 200, "message": "备份导入完成", "data": result}
	if mode != "overwrite" {
		resp["mode"] = mode
	}
	c.JSON(http.StatusOK, resp)
}

func ImportBackup(c *gin.Context) {
	handleImportBackup(c, "overwrite")
}

func parseBackupPayload(c *gin.Context) (*db.BackupPayload, error) {
	file, err := c.FormFile("file")
	if err == nil {
		ext := strings.ToLower(filepath.Ext(file.Filename))
		if ext != ".json" {
			return nil, errors.New("仅支持 .json 备份文件")
		}
		if file.Size > maxBackupImportSize {
			return nil, errors.New("上传文件过大（限制 50MB）")
		}
		src, openErr := file.Open()
		if openErr != nil {
			return nil, openErr
		}
		defer func(src multipart.File) {
			err := src.Close()
			if err != nil {
				logrus.Error(err)
			}
		}(src)

		decoder := json.NewDecoder(src)
		payload := &db.BackupPayload{}
		if decodeErr := decoder.Decode(payload); decodeErr != nil {
			return nil, decodeErr
		}
		return payload, nil
	}

	payload := &db.BackupPayload{}
	if err := c.ShouldBindJSON(payload); err != nil {
		return nil, errors.New("请通过 multipart/form-data 上传字段 file，或直接提交 JSON 请求体")
	}
	return payload, nil
}

// FullExportBackup 完整备份导出（使用 BasicAuth 验证）
func FullExportBackup(c *gin.Context) {
	ns := middleware.GetNamespace(c)
	payload, err := db.ExportBackupNs(ns)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	content, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := "astra-full-backup-" + time.Now().Format("20060102-150405") + ".json"
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "application/json", content)
}

// FullImportBackup 完整备份导入（使用 BasicAuth 验证，支持 overwrite/skip 模式）
func FullImportBackup(c *gin.Context) {
	mode := c.DefaultPostForm("mode", "overwrite")
	if mode != "overwrite" && mode != "skip" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效的 mode 参数，仅支持 overwrite 或 skip"})
		return
	}
	handleImportBackup(c, mode)
}
