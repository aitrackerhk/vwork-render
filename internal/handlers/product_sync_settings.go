package handlers

import (
	"fmt"
	"strings"

	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type hktvMallSyncSettings struct {
	Enabled       *bool   `json:"enabled"`
	MerchantID    *string `json:"merchant_id"`
	AppID         *string `json:"app_id"`
	AppSecret     *string `json:"app_secret"`
	APIBase       *string `json:"api_base"`
	ShopID        *string `json:"shop_id"`
	SyncProducts  *bool   `json:"sync_products"`
	SyncInventory *bool   `json:"sync_inventory"`
	SyncPrice     *bool   `json:"sync_price"`
}

// 通用電商同步設定結構（適用於 Shopee, Lazada, Amazon, Rakuten, PChome, Momo）
type marketplaceSyncSettings struct {
	Enabled       *bool   `json:"enabled"`
	ShopID        *string `json:"shop_id"`
	AppKey        *string `json:"app_key"`
	AppSecret     *string `json:"app_secret"`
	AccessToken   *string `json:"access_token"`
	RefreshToken  *string `json:"refresh_token"`
	Region        *string `json:"region"`
	APIBase       *string `json:"api_base"`
	SyncProducts  *bool   `json:"sync_products"`
	SyncInventory *bool   `json:"sync_inventory"`
	SyncPrice     *bool   `json:"sync_price"`
	SyncOrders    *bool   `json:"sync_orders"`
}

type productSyncSettingsRequest struct {
	Enabled  *bool                    `json:"enabled"`
	HktvMall *hktvMallSyncSettings    `json:"hktvmall"`
	Shopee   *marketplaceSyncSettings `json:"shopee"`
	Lazada   *marketplaceSyncSettings `json:"lazada"`
	Amazon   *marketplaceSyncSettings `json:"amazon"`
	Rakuten  *marketplaceSyncSettings `json:"rakuten"`
	PChome   *marketplaceSyncSettings `json:"pchome"`
	Momo     *marketplaceSyncSettings `json:"momo"`
}

func parseStringFromExtra(value interface{}, fallback string) string {
	if value == nil {
		return fallback
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []byte:
		return strings.TrimSpace(string(v))
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func buildDefaultProductSyncSettings() map[string]interface{} {
	return map[string]interface{}{
		"enabled": false,
		"hktvmall": map[string]interface{}{
			"enabled":        false,
			"merchant_id":    "",
			"app_id":         "",
			"app_secret":     "",
			"api_base":       "",
			"shop_id":        "",
			"sync_products":  true,
			"sync_inventory": true,
			"sync_price":     true,
		},
		"shopee":  buildDefaultMarketplaceSettings(),
		"lazada":  buildDefaultMarketplaceSettings(),
		"amazon":  buildDefaultMarketplaceSettings(),
		"rakuten": buildDefaultMarketplaceSettings(),
		"pchome":  buildDefaultMarketplaceSettings(),
		"momo":    buildDefaultMarketplaceSettings(),
	}
}

func buildDefaultMarketplaceSettings() map[string]interface{} {
	return map[string]interface{}{
		"enabled":        false,
		"shop_id":        "",
		"app_key":        "",
		"app_secret":     "",
		"access_token":   "",
		"refresh_token":  "",
		"region":         "",
		"api_base":       "",
		"sync_products":  true,
		"sync_inventory": true,
		"sync_price":     true,
		"sync_orders":    true,
	}
}

func normalizeProductSyncSettings(raw interface{}) map[string]interface{} {
	settings := buildDefaultProductSyncSettings()
	rawMap, ok := raw.(map[string]interface{})
	if !ok {
		return settings
	}

	settings["enabled"] = parseBoolFromExtra(rawMap["enabled"], false)

	// HKTVmall
	hktvRaw, ok := rawMap["hktvmall"].(map[string]interface{})
	if ok {
		hktv := settings["hktvmall"].(map[string]interface{})
		hktv["enabled"] = parseBoolFromExtra(hktvRaw["enabled"], false)
		hktv["merchant_id"] = parseStringFromExtra(hktvRaw["merchant_id"], "")
		hktv["app_id"] = parseStringFromExtra(hktvRaw["app_id"], "")
		hktv["app_secret"] = parseStringFromExtra(hktvRaw["app_secret"], "")
		hktv["api_base"] = parseStringFromExtra(hktvRaw["api_base"], "")
		hktv["shop_id"] = parseStringFromExtra(hktvRaw["shop_id"], "")
		hktv["sync_products"] = parseBoolFromExtra(hktvRaw["sync_products"], true)
		hktv["sync_inventory"] = parseBoolFromExtra(hktvRaw["sync_inventory"], true)
		hktv["sync_price"] = parseBoolFromExtra(hktvRaw["sync_price"], true)
	}

	// 其他電商平台
	marketplaces := []string{"shopee", "lazada", "amazon", "rakuten", "pchome", "momo"}
	for _, mp := range marketplaces {
		if mpRaw, ok := rawMap[mp].(map[string]interface{}); ok {
			mpSettings := settings[mp].(map[string]interface{})
			mpSettings["enabled"] = parseBoolFromExtra(mpRaw["enabled"], false)
			mpSettings["shop_id"] = parseStringFromExtra(mpRaw["shop_id"], "")
			mpSettings["app_key"] = parseStringFromExtra(mpRaw["app_key"], "")
			mpSettings["app_secret"] = parseStringFromExtra(mpRaw["app_secret"], "")
			mpSettings["access_token"] = parseStringFromExtra(mpRaw["access_token"], "")
			mpSettings["refresh_token"] = parseStringFromExtra(mpRaw["refresh_token"], "")
			mpSettings["region"] = parseStringFromExtra(mpRaw["region"], "")
			mpSettings["api_base"] = parseStringFromExtra(mpRaw["api_base"], "")
			mpSettings["sync_products"] = parseBoolFromExtra(mpRaw["sync_products"], true)
			mpSettings["sync_inventory"] = parseBoolFromExtra(mpRaw["sync_inventory"], true)
			mpSettings["sync_price"] = parseBoolFromExtra(mpRaw["sync_price"], true)
			mpSettings["sync_orders"] = parseBoolFromExtra(mpRaw["sync_orders"], true)
		}
	}

	return settings
}

// GetProductSyncSettings 取得租戶的產品同步設定
func GetProductSyncSettings(c *fiber.Ctx) error {
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

	settings := normalizeProductSyncSettings(tenant.ExtraFields["product_sync"])

	return c.JSON(fiber.Map{
		"data": settings,
	})
}

// UpdateProductSyncSettings 更新租戶的產品同步設定
func UpdateProductSyncSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var req productSyncSettingsRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.Enabled == nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "enabled is required",
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

	settings := buildDefaultProductSyncSettings()
	settings["enabled"] = *req.Enabled

	// HKTVmall
	hktv := settings["hktvmall"].(map[string]interface{})
	if req.HktvMall != nil {
		if req.HktvMall.Enabled != nil {
			hktv["enabled"] = *req.HktvMall.Enabled
		}
		if req.HktvMall.MerchantID != nil {
			hktv["merchant_id"] = strings.TrimSpace(*req.HktvMall.MerchantID)
		}
		if req.HktvMall.AppID != nil {
			hktv["app_id"] = strings.TrimSpace(*req.HktvMall.AppID)
		}
		if req.HktvMall.AppSecret != nil {
			hktv["app_secret"] = strings.TrimSpace(*req.HktvMall.AppSecret)
		}
		if req.HktvMall.APIBase != nil {
			hktv["api_base"] = strings.TrimSpace(*req.HktvMall.APIBase)
		}
		if req.HktvMall.ShopID != nil {
			hktv["shop_id"] = strings.TrimSpace(*req.HktvMall.ShopID)
		}
		if req.HktvMall.SyncProducts != nil {
			hktv["sync_products"] = *req.HktvMall.SyncProducts
		}
		if req.HktvMall.SyncInventory != nil {
			hktv["sync_inventory"] = *req.HktvMall.SyncInventory
		}
		if req.HktvMall.SyncPrice != nil {
			hktv["sync_price"] = *req.HktvMall.SyncPrice
		}
	}

	// 處理其他電商平台
	applyMarketplaceSettings := func(mp map[string]interface{}, req *marketplaceSyncSettings) {
		if req == nil {
			return
		}
		if req.Enabled != nil {
			mp["enabled"] = *req.Enabled
		}
		if req.ShopID != nil {
			mp["shop_id"] = strings.TrimSpace(*req.ShopID)
		}
		if req.AppKey != nil {
			mp["app_key"] = strings.TrimSpace(*req.AppKey)
		}
		if req.AppSecret != nil {
			mp["app_secret"] = strings.TrimSpace(*req.AppSecret)
		}
		if req.AccessToken != nil {
			mp["access_token"] = strings.TrimSpace(*req.AccessToken)
		}
		if req.RefreshToken != nil {
			mp["refresh_token"] = strings.TrimSpace(*req.RefreshToken)
		}
		if req.Region != nil {
			mp["region"] = strings.TrimSpace(*req.Region)
		}
		if req.APIBase != nil {
			mp["api_base"] = strings.TrimSpace(*req.APIBase)
		}
		if req.SyncProducts != nil {
			mp["sync_products"] = *req.SyncProducts
		}
		if req.SyncInventory != nil {
			mp["sync_inventory"] = *req.SyncInventory
		}
		if req.SyncPrice != nil {
			mp["sync_price"] = *req.SyncPrice
		}
		if req.SyncOrders != nil {
			mp["sync_orders"] = *req.SyncOrders
		}
	}

	applyMarketplaceSettings(settings["shopee"].(map[string]interface{}), req.Shopee)
	applyMarketplaceSettings(settings["lazada"].(map[string]interface{}), req.Lazada)
	applyMarketplaceSettings(settings["amazon"].(map[string]interface{}), req.Amazon)
	applyMarketplaceSettings(settings["rakuten"].(map[string]interface{}), req.Rakuten)
	applyMarketplaceSettings(settings["pchome"].(map[string]interface{}), req.PChome)
	applyMarketplaceSettings(settings["momo"].(map[string]interface{}), req.Momo)

	tenant.ExtraFields["product_sync"] = settings

	if err := database.DB.Model(&tenant).Update("extra_fields", tenant.ExtraFields).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to update product sync settings",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Product sync settings updated successfully",
		"data":    settings,
	})
}
