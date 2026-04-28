package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func GetDocumentAutoSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var s models.DocumentAutoSettings
	if err := database.DB.Where("tenant_id = ?", tenantID).First(&s).Error; err != nil {
		// 默認值
		return c.JSON(models.DocumentAutoSettings{
			TenantID:                           tenantID,
			AutoGenerateCommission:             true,
			AutoGenerateOrderCommission:        true,
			AutoGenerateServiceOrderCommission: true,
			AutoGenerateOrderTaxes:             true,
			AutoGenerateServiceTaxes:           true,
			AutoGenerateOrderPayment:           true,
			AutoGenerateServiceOrderPayment:    true,
			AutoGeneratePurchaseOrderExpense:   true,
			AutoUseLastSupplierQuotationPrice:  true,
			AutoCalculateOrderShipping:         false,
		})
	}
	return c.JSON(s)
}

func CreateOrUpdateDocumentAutoSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var req struct {
		// 新欄位
		AutoGenerateOrderCommission        *bool `json:"auto_generate_order_commission"`
		AutoGenerateServiceOrderCommission *bool `json:"auto_generate_service_order_commission"`
		AutoGenerateOrderTaxes             *bool `json:"auto_generate_order_taxes"`
		AutoGenerateServiceTaxes           *bool `json:"auto_generate_service_taxes"`
		AutoGenerateOrderPayment           *bool `json:"auto_generate_order_payment"`
		AutoGenerateServiceOrderPayment    *bool `json:"auto_generate_service_order_payment"`
		AutoGeneratePurchaseOrderExpense   *bool `json:"auto_generate_purchase_order_expense"`
		AutoUseLastSupplierQuotationPrice  *bool `json:"auto_use_last_supplier_quotation_price"`
		AutoCalculateOrderShipping         *bool `json:"auto_calculate_order_shipping"`
		// 舊欄位（向後兼容）
		AutoGenerateCommission *bool `json:"auto_generate_commission"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	var s models.DocumentAutoSettings
	err := database.DB.Where("tenant_id = ?", tenantID).First(&s).Error
	now := time.Now()

	// Resolve final values (compat: if new fields missing, fall back to old)
	resolveBool := func(p *bool, fallback bool) bool {
		if p == nil {
			return fallback
		}
		return *p
	}
	// base defaults: if record doesn't exist yet, default should be TRUE (per requirement)
	base := s
	if err != nil {
		base = models.DocumentAutoSettings{
			TenantID:                           tenantID,
			AutoGenerateCommission:             true,
			AutoGenerateOrderCommission:        true,
			AutoGenerateServiceOrderCommission: true,
			AutoGenerateOrderTaxes:             true,
			AutoGenerateServiceTaxes:           true,
			AutoGenerateOrderPayment:           true,
			AutoGenerateServiceOrderPayment:    true,
			AutoGeneratePurchaseOrderExpense:   true,
			AutoUseLastSupplierQuotationPrice:  true,
			AutoCalculateOrderShipping:         false,
		}
	}

	// old fallback: if auto_generate_commission provided, apply to both new fields (unless new provided)
	oldCommissionFallback := base.AutoGenerateCommission
	if req.AutoGenerateCommission != nil {
		oldCommissionFallback = *req.AutoGenerateCommission
	}
	orderCommission := resolveBool(req.AutoGenerateOrderCommission, base.AutoGenerateOrderCommission)
	serviceOrderCommission := resolveBool(req.AutoGenerateServiceOrderCommission, base.AutoGenerateServiceOrderCommission)
	if req.AutoGenerateOrderCommission == nil && req.AutoGenerateCommission != nil {
		orderCommission = oldCommissionFallback
	}
	if req.AutoGenerateServiceOrderCommission == nil && req.AutoGenerateCommission != nil {
		serviceOrderCommission = oldCommissionFallback
	}
	orderTaxes := resolveBool(req.AutoGenerateOrderTaxes, base.AutoGenerateOrderTaxes)
	serviceTaxes := resolveBool(req.AutoGenerateServiceTaxes, base.AutoGenerateServiceTaxes)
	orderPayment := resolveBool(req.AutoGenerateOrderPayment, base.AutoGenerateOrderPayment)
	serviceOrderPayment := resolveBool(req.AutoGenerateServiceOrderPayment, base.AutoGenerateServiceOrderPayment)
	purchaseExpense := resolveBool(req.AutoGeneratePurchaseOrderExpense, base.AutoGeneratePurchaseOrderExpense)
	autoLastQuote := resolveBool(req.AutoUseLastSupplierQuotationPrice, base.AutoUseLastSupplierQuotationPrice)
	autoShip := resolveBool(req.AutoCalculateOrderShipping, base.AutoCalculateOrderShipping)

	if err != nil {
		s = models.DocumentAutoSettings{
			TenantID:                           tenantID,
			AutoGenerateCommission:             oldCommissionFallback,
			AutoGenerateOrderCommission:        orderCommission,
			AutoGenerateServiceOrderCommission: serviceOrderCommission,
			AutoGenerateOrderTaxes:             orderTaxes,
			AutoGenerateServiceTaxes:           serviceTaxes,
			AutoGenerateOrderPayment:           orderPayment,
			AutoGenerateServiceOrderPayment:    serviceOrderPayment,
			AutoGeneratePurchaseOrderExpense:   purchaseExpense,
			AutoUseLastSupplierQuotationPrice:  autoLastQuote,
			AutoCalculateOrderShipping:         autoShip,
			CreatedAt:                          now,
			UpdatedAt:                          now,
		}
		if err := database.DB.Create(&s).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create auto settings"})
		}
	} else {
		// keep legacy field in sync (best-effort)
		s.AutoGenerateCommission = oldCommissionFallback
		s.AutoGenerateOrderCommission = orderCommission
		s.AutoGenerateServiceOrderCommission = serviceOrderCommission
		s.AutoGenerateOrderTaxes = orderTaxes
		s.AutoGenerateServiceTaxes = serviceTaxes
		s.AutoGenerateOrderPayment = orderPayment
		s.AutoGenerateServiceOrderPayment = serviceOrderPayment
		s.AutoGeneratePurchaseOrderExpense = purchaseExpense
		s.AutoUseLastSupplierQuotationPrice = autoLastQuote
		s.AutoCalculateOrderShipping = autoShip
		s.UpdatedAt = now
		if err := database.DB.Save(&s).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update auto settings"})
		}
	}

	return c.JSON(s)
}

// best-effort helper（給訂單/服務單保存時使用）
func getDocumentAutoSettingsForTenant(tenantID uuid.UUID) models.DocumentAutoSettings {
	var s models.DocumentAutoSettings
	if err := database.DB.Where("tenant_id = ?", tenantID).First(&s).Error; err != nil {
		return models.DocumentAutoSettings{
			TenantID:                           tenantID,
			AutoGenerateCommission:             true,
			AutoGenerateOrderCommission:        true,
			AutoGenerateServiceOrderCommission: true,
			AutoGenerateOrderTaxes:             true,
			AutoGenerateServiceTaxes:           true,
			AutoGenerateOrderPayment:           true,
			AutoGenerateServiceOrderPayment:    true,
			AutoGeneratePurchaseOrderExpense:   true,
			AutoUseLastSupplierQuotationPrice:  true,
			AutoCalculateOrderShipping:         false,
		}
	}
	return s
}
