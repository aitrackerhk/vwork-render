package utils

import (
	"encoding/json"
	"nwork/internal/database"
	"nwork/internal/models"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// LogActivity 記錄活動
func LogActivity(tenantID, userID uuid.UUID, action, resourceType string, resourceID *uuid.UUID, description string, changes map[string]interface{}, c *fiber.Ctx) error {
	log := models.ActivityLog{
		TenantID:     tenantID,
		UserID:       userID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Description:  description,
		CreatedAt:    time.Now(),
	}

	// 記錄變更資料
	if changes != nil && len(changes) > 0 {
		log.Changes = models.JSONB(changes)
	} else {
		log.Changes = make(models.JSONB)
	}

	// 記錄 IP 地址和 User Agent（如果有的話）
	if c != nil {
		log.IPAddress = c.IP()
		log.UserAgent = c.Get("User-Agent")
	}

	// 異步記錄，避免影響主流程
	go func() {
		if err := database.DB.Create(&log).Error; err != nil {
			// 記錄錯誤但不中斷主流程
			// 這裡可以使用日誌系統記錄錯誤
		}
	}()

	return nil
}

// LogActivitySync 同步記錄活動（用於需要確保記錄成功的場景）
func LogActivitySync(tenantID, userID uuid.UUID, action, resourceType string, resourceID *uuid.UUID, description string, changes map[string]interface{}, c *fiber.Ctx) error {
	log := models.ActivityLog{
		TenantID:     tenantID,
		UserID:       userID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Description:  description,
		CreatedAt:    time.Now(),
	}

	// 記錄變更資料
	if changes != nil && len(changes) > 0 {
		log.Changes = models.JSONB(changes)
	} else {
		log.Changes = make(models.JSONB)
	}

	// 記錄 IP 地址和 User Agent（如果有的話）
	if c != nil {
		log.IPAddress = c.IP()
		log.UserAgent = c.Get("User-Agent")
	}

	return database.DB.Create(&log).Error
}

// GetChangesMap 從舊值和新值創建變更映射
func GetChangesMap(oldData interface{}, newData interface{}) map[string]interface{} {
	changes := make(map[string]interface{})

	oldJSON, err := json.Marshal(oldData)
	if err == nil {
		var oldMap map[string]interface{}
		if err := json.Unmarshal(oldJSON, &oldMap); err == nil {
			changes["old"] = oldMap
		}
	}

	newJSON, err := json.Marshal(newData)
	if err == nil {
		var newMap map[string]interface{}
		if err := json.Unmarshal(newJSON, &newMap); err == nil {
			changes["new"] = newMap
		}
	}

	return changes
}

// GetFieldChangesMap 創建字段級別的變更映射
func GetFieldChangesMap(fieldChanges map[string]interface{}) map[string]interface{} {
	changes := make(map[string]interface{})
	changes["fields"] = fieldChanges
	return changes
}

