package handlers

import (
	"nwork/internal/database"
	"nwork/internal/models"

	"github.com/google/uuid"
)

func getInventorySettingsForTenant(tenantID uuid.UUID) models.InventorySettings {
	var settings models.InventorySettings
	if err := database.DB.Where("tenant_id = ?", tenantID).First(&settings).Error; err != nil {
		settings = models.InventorySettings{
			TenantID:             tenantID,
			RequiresOutbound:     true,
			RequiresInbound:      true,
			AutoCompleteIfNoNeed: true,
		}
	}
	return settings
}

func applyAutoCompleteInventoryNotes(settings models.InventorySettings, extraFields map[string]interface{}, noteKey string) {
	if !settings.AutoCompleteIfNoNeed || extraFields == nil {
		return
	}

	rawNotes, ok := extraFields[noteKey]
	if !ok || rawNotes == nil {
		return
	}

	switch notes := rawNotes.(type) {
	case []interface{}:
		for _, note := range notes {
			if m, ok := note.(map[string]interface{}); ok {
				if status, _ := m["status"].(string); status != "completed" {
					m["status"] = "completed"
				}
			}
		}
		extraFields[noteKey] = notes
	case []map[string]interface{}:
		for i := range notes {
			if status, _ := notes[i]["status"].(string); status != "completed" {
				notes[i]["status"] = "completed"
			}
		}
		extraFields[noteKey] = notes
	}
}
