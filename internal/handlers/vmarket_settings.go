package handlers

import (
    "nwork/internal/database"
    "nwork/internal/middleware"
    "nwork/internal/models"
    "strings"

    "github.com/gofiber/fiber/v2"
    "github.com/google/uuid"
)

func parseBoolFromExtra(value interface{}, fallback bool) bool {
    if value == nil {
        return fallback
    }
    switch v := value.(type) {
    case bool:
        return v
    case string:
        s := strings.ToLower(strings.TrimSpace(v))
        return s == "true" || s == "1" || s == "on" || s == "yes"
    case float64:
        return int(v) == 1
    case int:
        return v == 1
    default:
        return fallback
    }
}

func getTenantVMarketJoined(tenantID uuid.UUID) (bool, error) {
    if tenantID == uuid.Nil {
        return false, nil
    }
    var tenant models.Tenant
    if err := database.DB.Select("extra_fields").Where("id = ?", tenantID).First(&tenant).Error; err != nil {
        return false, err
    }
    return parseBoolFromExtra(tenant.ExtraFields["vmarket_joined"], false), nil
}

// GetVMarketSettings 取得租戶的 VMarket 設定
func GetVMarketSettings(c *fiber.Ctx) error {
    tenantID := middleware.GetTenantID(c)
    if tenantID == uuid.Nil {
        return c.Status(400).JSON(fiber.Map{
            "error": "Tenant not found",
        })
    }

    var tenant models.Tenant
    if err := database.DB.Select("extra_fields").Where("id = ?", tenantID).First(&tenant).Error; err != nil {
        return c.Status(404).JSON(fiber.Map{
            "error": "Tenant not found",
        })
    }

    joined := parseBoolFromExtra(tenant.ExtraFields["vmarket_joined"], false)

    return c.JSON(fiber.Map{
        "data": fiber.Map{
            "vmarket_joined": joined,
        },
    })
}

// UpdateVMarketSettings 更新租戶的 VMarket 設定
func UpdateVMarketSettings(c *fiber.Ctx) error {
    tenantID := middleware.GetTenantID(c)
    if tenantID == uuid.Nil {
        return c.Status(400).JSON(fiber.Map{
            "error": "Tenant not found",
        })
    }

    var req struct {
        VMarketJoined *bool `json:"vmarket_joined"`
    }
    if err := c.BodyParser(&req); err != nil {
        return c.Status(400).JSON(fiber.Map{
            "error": "Invalid request body",
        })
    }

    if req.VMarketJoined == nil {
        return c.Status(400).JSON(fiber.Map{
            "error": "vmarket_joined is required",
        })
    }

    var tenant models.Tenant
    if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
        return c.Status(404).JSON(fiber.Map{
            "error": "Tenant not found",
        })
    }

    if tenant.ExtraFields == nil {
        tenant.ExtraFields = models.JSONB{}
    }
    tenant.ExtraFields["vmarket_joined"] = *req.VMarketJoined

    if err := database.DB.Model(&tenant).Update("extra_fields", tenant.ExtraFields).Error; err != nil {
        return c.Status(500).JSON(fiber.Map{
            "error": "Failed to update VMarket settings",
        })
    }

    return c.JSON(fiber.Map{
        "message": "VMarket settings updated successfully",
        "data": fiber.Map{
            "vmarket_joined": *req.VMarketJoined,
        },
    })
}