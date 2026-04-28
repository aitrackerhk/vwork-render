package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DocumentAutoSettings 單據相關的系統自動生成設定
type DocumentAutoSettings struct {
	ID       uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"tenant_id"`
	// Deprecated: AutoGenerateCommission（舊版）同時套用「訂單」與「服務單」的傭金自動生成。
	// 仍保留 DB 欄位以向後兼容，但前端已改用下面兩個新欄位。
	AutoGenerateCommission             bool      `gorm:"default:true" json:"auto_generate_commission"`
	AutoGenerateOrderCommission        bool      `gorm:"default:true" json:"auto_generate_order_commission"`
	AutoGenerateServiceOrderCommission bool      `gorm:"default:true" json:"auto_generate_service_order_commission"`
	AutoGenerateOrderTaxes             bool      `gorm:"default:true" json:"auto_generate_order_taxes"`
	AutoGenerateServiceTaxes           bool      `gorm:"default:true" json:"auto_generate_service_taxes"`
	AutoGenerateOrderPayment           bool      `gorm:"default:true" json:"auto_generate_order_payment"`
	AutoGenerateServiceOrderPayment    bool      `gorm:"default:true" json:"auto_generate_service_order_payment"`
	AutoGeneratePurchaseOrderExpense   bool      `gorm:"default:true" json:"auto_generate_purchase_order_expense"`
	AutoUseLastSupplierQuotationPrice  bool      `gorm:"default:true" json:"auto_use_last_supplier_quotation_price"`
	AutoCalculateOrderShipping         bool      `gorm:"default:false" json:"auto_calculate_order_shipping"`
	CreatedAt                          time.Time `json:"created_at"`
	UpdatedAt                          time.Time `json:"updated_at"`
}

func (DocumentAutoSettings) TableName() string { return "document_auto_settings" }

func (s *DocumentAutoSettings) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}
