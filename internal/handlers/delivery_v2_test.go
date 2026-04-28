package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"nwork/internal/database"
	"nwork/internal/models"
	"nwork/internal/services/delivery"
)

// Define local structs for testing to avoid Postgres specific syntax in AutoMigrate
// SQLite doesn't support gen_random_uuid() or jsonb
type SQLiteOrder struct {
	ID               uuid.UUID `gorm:"type:uuid;primary_key"`
	TenantID         uuid.UUID
	OrderNumber      string
	CustomerID       *uuid.UUID
	OrderDate        time.Time
	Status           string
	TotalAmount      float64
	CouponID         *uuid.UUID
	PointsUsed       int
	PointsEarned     int
	PointsDiscount   float64
	CouponDiscount   float64
	ReferralCode     string
	ContactName      string
	ContactEmail     string
	ContactPhone     string
	ContactAddress   string
	ShippingMethodID *uuid.UUID
	SalespersonID    *uuid.UUID
	StoreID          *uuid.UUID
	CommissionAmount float64
	Notes            string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	CreatedBy        *uuid.UUID
	UpdatedBy        *uuid.UUID
	SourceType       string
	TrashedAt        *time.Time
	ExtraFields      models.JSONB `gorm:"type:text"`

	DeliveryPlatform *string
	PlatformOrderID  *string
}

func (SQLiteOrder) TableName() string {
	return "orders"
}

type SQLiteDeliveryIntegration struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key"`
	TenantID       uuid.UUID
	StoreID        *uuid.UUID
	Platform       models.DeliveryPlatform
	MerchantID     string
	MerchantName   string
	APIKey         string
	APISecret      string
	AccessToken    string
	RefreshToken   string
	TokenExpiresAt *time.Time
	WebhookSecret  string
	WebhookURL     string
	IsEnabled      bool
	IsConnected    bool
	LastSyncAt     *time.Time
	LastError      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CreatedBy      *uuid.UUID
	UpdatedBy      *uuid.UUID
	Settings       models.JSONB `gorm:"type:text"`
}

func (SQLiteDeliveryIntegration) TableName() string {
	return "delivery_integrations"
}

type SQLiteDeliveryOrderDetail struct {
	ID                    uuid.UUID `gorm:"type:uuid;primary_key"`
	OrderID               uuid.UUID
	IntegrationID         *uuid.UUID
	Platform              models.DeliveryPlatform
	PlatformOrderID       string
	PlatformOrderNumber   string
	PlatformStatus        string
	DeliveryType          string
	EstimatedPickupTime   *time.Time
	EstimatedDeliveryTime *time.Time
	ActualPickupTime      *time.Time
	ActualDeliveryTime    *time.Time
	RiderName             string
	RiderPhone            string
	RiderTrackingURL      string
	PlatformFee           float64
	DeliveryFee           float64
	PlatformDiscount      float64
	RawData               models.JSONB `gorm:"type:text"`
	CancelledAt           *time.Time
	CancelReason          string
	CancelledBy           string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	ConfirmedAt           *time.Time
}

func (SQLiteDeliveryOrderDetail) TableName() string {
	return "delivery_order_details"
}

type SQLiteDeliveryOrderStatusHistory struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key"`
	OrderID        uuid.UUID
	Status         string
	PlatformStatus string
	Notes          string
	RawEvent       models.JSONB `gorm:"type:text"`
	CreatedAt      time.Time
	CreatedBy      *uuid.UUID
}

func (SQLiteDeliveryOrderStatusHistory) TableName() string {
	return "delivery_order_status_history"
}

type SQLiteOrderItem struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key"`
	TenantID       uuid.UUID
	OrderID        uuid.UUID
	Quantity       float64
	UnitPrice      float64
	TotalPrice     float64
	Notes          string
	CreatedAt      time.Time
	TrashedAt      *time.Time
	ExtraFields    models.JSONB `gorm:"type:text"`
	PlatformItemID *string
	ItemName       *string
	ItemOptions    models.JSONB `gorm:"type:text"`
	ProductID      *uuid.UUID
}

func (SQLiteOrderItem) TableName() string {
	return "order_items"
}

type SQLiteProductWarehouseStock struct {
	ID            uuid.UUID `gorm:"type:uuid;primary_key"`
	TenantID      uuid.UUID
	ProductID     uuid.UUID
	WarehouseID   uuid.UUID
	Quantity      int
	LastUpdatedAt time.Time
}

func (SQLiteProductWarehouseStock) TableName() string {
	return "product_warehouse_stocks"
}

type SQLiteDeliveryProductMapping struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key"`
	TenantID       uuid.UUID
	Platform       models.DeliveryPlatform
	PlatformItemID string
	ProductID      *uuid.UUID
	SyncInventory  bool
}

func (SQLiteDeliveryProductMapping) TableName() string {
	return "delivery_product_mappings"
}

type SQLiteWarehouse struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key"`
	TenantID  uuid.UUID
	IsDefault bool
}

func (SQLiteWarehouse) TableName() string {
	return "warehouses"
}

func setupTestDB() {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		panic("failed to connect database")
	}
	database.DB = db

	db.AutoMigrate(
		&SQLiteOrder{},
		&SQLiteDeliveryIntegration{},
		&SQLiteOrderItem{},
		&SQLiteDeliveryOrderDetail{},
		&SQLiteDeliveryOrderStatusHistory{},
		&SQLiteDeliveryProductMapping{},
		&SQLiteWarehouse{},
		&SQLiteProductWarehouseStock{},
	)
}

func TestDeliveryWebhookV2_Foodpanda(t *testing.T) {
	setupTestDB()

	tenantID := uuid.New()
	webhookSecret := "secret123"
	merchantID := "fp_12345"

	// Create Integration
	integration := models.DeliveryIntegration{
		ID:            uuid.New(),
		TenantID:      tenantID,
		Platform:      models.DeliveryPlatform("foodpanda"),
		MerchantID:    merchantID,
		WebhookSecret: webhookSecret,
		IsEnabled:     true,
		IsConnected:   true,
		Settings:      models.JSONB{"base_url": "http://mock-url-will-be-replaced"},
	}
	database.DB.Create(&integration)

	// Create Fiber app
	app := fiber.New()
	app.Post("/api/v1/delivery/webhook/:platform", DeliveryWebhookV2)

	// Prepare payload
	payload := map[string]interface{}{
		"event_type":    "order_created",
		"event_id":      "evt_001",
		"order_id":      "fp_order_001",
		"status":        "new",
		"restaurant_id": merchantID, // Use restaurant_id for foodpanda matching
		"data": map[string]interface{}{
			"id":     "fp_order_001",
			"code":   "FP-001",
			"status": "new",
			"customer": map[string]interface{}{
				"name":  "John Doe",
				"phone": "+85212345678",
			},
			"items": []map[string]interface{}{
				{
					"id":          "item_1",
					"name":        "Burger",
					"quantity":    2,
					"unit_price":  50.0,
					"total_price": 100.0,
				},
			},
			"total": 120.0,
		},
	}
	jsonBody, _ := json.Marshal(payload)

	// Generate Signature
	signature := delivery.GenerateHMACSignature(webhookSecret, jsonBody)

	// Create Request
	req := httptest.NewRequest("POST", "/api/v1/delivery/webhook/foodpanda", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", signature)

	// Execute
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Read response body to avoid leak
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Verify Order Created
	var order models.Order
	err = database.DB.Where("platform_order_id = ?", "fp_order_001").First(&order).Error
	assert.NoError(t, err)
	assert.Equal(t, "pending", order.Status) // mapped from "new"
	assert.Equal(t, "delivery", order.SourceType)
}

func TestAcceptDeliveryOrderV2(t *testing.T) {
	setupTestDB()

	tenantID := uuid.New()
	platformOrderID := "fp_order_abc"

	// Mock Server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify URL
		if r.URL.Path == "/v1/orders/"+platformOrderID+"/accept" && r.Method == "POST" {
			w.WriteHeader(200)
			w.Write([]byte(`{"success":true}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer mockServer.Close()

	// Create Integration
	integration := models.DeliveryIntegration{
		ID:         uuid.New(),
		TenantID:   tenantID,
		Platform:   models.DeliveryPlatform("foodpanda"),
		MerchantID: "fp_vendor_1",
		IsEnabled:  true,
		Settings:   models.JSONB{"base_url": mockServer.URL},
	}
	database.DB.Create(&integration)

	// Create Order
	order := models.Order{
		ID:               uuid.New(),
		TenantID:         tenantID,
		Status:           "pending",
		SourceType:       "delivery",
		DeliveryPlatform: &[]string{"foodpanda"}[0],
		PlatformOrderID:  &platformOrderID,
		UpdatedAt:        time.Now(),
		CreatedAt:        time.Now(),
	}
	database.DB.Create(&order)

	// Create DeliveryOrderDetail
	detail := models.DeliveryOrderDetail{
		OrderID: order.ID,
	}
	database.DB.Create(&detail)

	// Create Fiber app
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		// Mock Middleware
		c.Locals("tenant_id", tenantID)
		userID := uuid.New()
		c.Locals("user_id", userID)
		return c.Next()
	})
	app.Post("/api/v1/delivery/orders/:id/accept", AcceptDeliveryOrderV2)

	// Execute
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/delivery/orders/%s/accept", order.ID), nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Read response body to avoid leak
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Verify DB Update
	var updatedOrder models.Order
	database.DB.First(&updatedOrder, order.ID)
	assert.Equal(t, "accepted", updatedOrder.Status)
}

func TestRejectDeliveryOrderV2(t *testing.T) {
	setupTestDB()

	tenantID := uuid.New()
	platformOrderID := "fp_order_xyz"

	// Mock Server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/orders/"+platformOrderID+"/reject" && r.Method == "POST" {
			w.WriteHeader(200)
			w.Write([]byte(`{"success":true}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer mockServer.Close()

	// Create Integration
	integration := models.DeliveryIntegration{
		ID:         uuid.New(),
		TenantID:   tenantID,
		Platform:   models.DeliveryPlatform("foodpanda"),
		MerchantID: "fp_vendor_1",
		IsEnabled:  true,
		Settings:   models.JSONB{"base_url": mockServer.URL},
	}
	database.DB.Create(&integration)

	// Create Order
	order := models.Order{
		ID:               uuid.New(),
		TenantID:         tenantID,
		Status:           "pending",
		SourceType:       "delivery",
		DeliveryPlatform: &[]string{"foodpanda"}[0],
		PlatformOrderID:  &platformOrderID,
		UpdatedAt:        time.Now(),
		CreatedAt:        time.Now(),
	}
	database.DB.Create(&order)

	// Create Order Detail (for cancel reason check)
	detail := models.DeliveryOrderDetail{
		OrderID: order.ID,
	}
	database.DB.Create(&detail)

	// Create Fiber app
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("tenant_id", tenantID)
		userID := uuid.New()
		c.Locals("user_id", userID)
		return c.Next()
	})
	app.Post("/api/v1/delivery/orders/:id/reject", RejectDeliveryOrderV2)

	// Payload
	payload := map[string]string{"reason": "Out of stock"}
	jsonBody, _ := json.Marshal(payload)

	// Execute
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/delivery/orders/%s/reject", order.ID), bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Read response body to avoid leak
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Verify DB Update
	var updatedOrder models.Order
	database.DB.First(&updatedOrder, order.ID)
	assert.Equal(t, "rejected", updatedOrder.Status)

	var updatedDetail models.DeliveryOrderDetail
	database.DB.Where("order_id = ?", order.ID).First(&updatedDetail)
	assert.Equal(t, "Out of stock", updatedDetail.CancelReason)
	assert.Equal(t, "merchant", updatedDetail.CancelledBy)
}
