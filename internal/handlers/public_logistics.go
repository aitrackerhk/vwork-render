package handlers

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"nwork/internal/database"
	"nwork/internal/models"
)

// PublicGetCustomerAddresses returns the logged-in webstore customer's addresses (requires customer_id cookie).
func PublicGetCustomerAddresses(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}
	customerID, err := getCustomerIDFromCookie(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	var addrs []models.CustomerAddress
	if err := database.DB.
		Where("tenant_id = ? AND customer_id = ?", tenant.ID, *customerID).
		Order("is_default DESC, created_at DESC").
		Find(&addrs).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to fetch addresses"})
	}
	return c.JSON(fiber.Map{"data": addrs})
}

// PublicCalculateBestLogisticsFee calculates best logistics fee for webstore customer (requires customer_id cookie).
// Uses:
// - logistics_companies fee fields
// - logistics_companies.extra_fields.allowed_country_codes (optional)
// - logistics_companies.extra_fields.allowed_region_keys (optional, values like "US-CA")
func PublicCalculateBestLogisticsFee(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}
	_, err = getCustomerIDFromCookie(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	var req struct {
		CountryCode string `json:"country_code"`
		RegionCode  string `json:"region_code"`
		OrderItems  []struct {
			ProductID string  `json:"product_id"`
			Quantity  float64 `json:"quantity"`
		} `json:"order_items"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	// Load active logistics companies
	var companies []models.LogisticsCompany
	if err := database.DB.Where("tenant_id = ? AND status = ?", tenant.ID, "active").Find(&companies).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to fetch logistics companies"})
	}
	if len(companies) == 0 {
		return c.JSON(fiber.Map{
			"best_company": nil,
			"best_fee":     0.0,
			"all_fees":     []interface{}{},
		})
	}

	// Totals from products
	totalWeight := 0.0
	totalArea := 0.0
	totalItems := 0
	for _, item := range req.OrderItems {
		pid, err := uuid.Parse(strings.TrimSpace(item.ProductID))
		if err != nil {
			continue
		}
		var product models.Product
		if err := database.DB.Where("id = ? AND tenant_id = ? AND status = ?", pid, tenant.ID, "active").First(&product).Error; err == nil {
			totalItems += int(item.Quantity)
			totalWeight += product.Weight * item.Quantity
			totalArea += product.Area * item.Quantity
		}
	}

	type CompanyFee struct {
		CompanyID   uuid.UUID `json:"company_id"`
		CompanyName string    `json:"company_name"`
		TotalFee    float64   `json:"total_fee"`
	}
	allFees := make([]CompanyFee, 0)

	for _, company := range companies {
		if !logisticsCompanyMatchesLocation(company, req.CountryCode, req.RegionCode) {
			continue
		}
		totalFee := company.BaseFee
		totalFee += company.PerItemFee * float64(totalItems)
		totalFee += company.PerWeightFee * totalWeight
		totalFee += company.PerAreaFee * totalArea
		allFees = append(allFees, CompanyFee{
			CompanyID:   company.ID,
			CompanyName: company.Name,
			TotalFee:    totalFee,
		})
	}

	if len(allFees) == 0 {
		return c.JSON(fiber.Map{
			"best_company": nil,
			"best_fee":     0.0,
			"all_fees":     []interface{}{},
		})
	}

	bestFee := allFees[0]
	for _, fee := range allFees {
		if fee.TotalFee < bestFee.TotalFee {
			bestFee = fee
		}
	}

	return c.JSON(fiber.Map{
		"best_company": fiber.Map{
			"id":   bestFee.CompanyID,
			"name": bestFee.CompanyName,
		},
		"best_fee": bestFee.TotalFee,
		"all_fees": allFees,
	})
}


