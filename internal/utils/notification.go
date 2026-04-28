package utils

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/models"
	"time"

	"github.com/google/uuid"
)

// NotifyAllTenantUsers 向租户的所有用户发送通知消息
func NotifyAllTenantUsers(tenantID uuid.UUID, fromUserID *uuid.UUID, subject string, content string, messageType string) error {
	// 获取租户的所有活跃用户
	var users []models.User
	if err := database.DB.Where("tenant_id = ? AND status = ?", tenantID, "active").Find(&users).Error; err != nil {
		return err
	}

	// 为每个用户创建通知消息
	for _, user := range users {
		userID := user.ID // 创建副本以便获取指针
		message := models.Message{
			TenantID:    tenantID,
			FromUserID:  fromUserID,
			ToUserID:    &userID,
			Subject:     subject,
			Content:     content,
			MessageType: messageType,
			Status:      "active",
			IsRead:      false,
		}
		if err := database.DB.Create(&message).Error; err != nil {
			// 记录错误但继续处理其他用户
			continue
		}
	}

	return nil
}

// CreateNotificationAlertForAllUsers 为租户的所有用户创建通知提示（NotificationAlert）
// creatorUserID: 创建者用户ID，如果用户是创建者本人，则通知直接标记为已读
func CreateNotificationAlertForAllUsers(tenantID uuid.UUID, alertType string, title string, message string, link string, creatorUserID *uuid.UUID) error {
	// 获取租户的所有活跃用户
	var users []models.User
	if err := database.DB.Where("tenant_id = ? AND status = ?", tenantID, "active").Find(&users).Error; err != nil {
		return err
	}

	now := time.Now()
	// 为每个用户创建通知提示
	for _, user := range users {
		// 如果是创建者本人，直接标记为已读
		isRead := false
		readAt := (*time.Time)(nil)
		if creatorUserID != nil && user.ID == *creatorUserID {
			isRead = true
			readAt = &now
		}

		alert := models.NotificationAlert{
			ID:          uuid.New(),
			TenantID:    tenantID,
			UserID:      user.ID,
			Type:        alertType,
			Title:       title,
			Message:     message,
			Link:        link,
			IsRead:      isRead,
			ReadAt:      readAt,
			GeneratedAt: now,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := database.DB.Create(&alert).Error; err != nil {
			// 记录错误但继续处理其他用户
			continue
		}
	}

	return nil
}

// GenerateInvoiceNumber 自动生成发票号码
func GenerateInvoiceNumber(tenantID uuid.UUID) (string, error) {
	today := time.Now().Format("20060102")
	datePrefix := "INV-" + today + "-"
	var count int64

	// 查询今天已生成的发票数量（从 orders 的 extra_fields 中）
	var orders []models.Order
	database.DB.Where("tenant_id = ? AND extra_fields::text LIKE ?", tenantID, "%"+datePrefix+"%").Find(&orders)

	// 统计今天已使用的发票号码
	maxNum := 0
	for _, order := range orders {
		if order.ExtraFields != nil {
			fields := map[string]interface{}(order.ExtraFields)
			if records, exists := fields["payment_records"]; exists {
				if recordsList, ok := records.([]interface{}); ok {
					for _, r := range recordsList {
						if record, ok := r.(map[string]interface{}); ok {
							if invoiceNum, ok := record["invoice_number"].(string); ok {
								if len(invoiceNum) > len(datePrefix) {
									// 提取号码部分
									var num int
									if _, err := fmt.Sscanf(invoiceNum[len(datePrefix):], "%d", &num); err == nil {
										if num > maxNum {
											maxNum = num
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// 也查询 invoices 表
	database.DB.Model(&models.Invoice{}).Where("tenant_id = ? AND invoice_number LIKE ?", tenantID, datePrefix+"%").Count(&count)
	if int64(maxNum) > count {
		count = int64(maxNum)
	}

	invoiceNumber := datePrefix + fmt.Sprintf("%03d", count+1)
	return invoiceNumber, nil
}

// GenerateShippingNumber 自动生成发货单号码
func GenerateShippingNumber(tenantID uuid.UUID) (string, error) {
	today := time.Now().Format("20060102")
	datePrefix := "SHIP-" + today + "-"
	var count int64

	// 查询今天已生成的发货单数量（从 orders 的 extra_fields 中）
	var orders []models.Order
	database.DB.Where("tenant_id = ? AND extra_fields::text LIKE ?", tenantID, "%"+datePrefix+"%").Find(&orders)

	// 统计今天已使用的发货单号码
	maxNum := 0
	for _, order := range orders {
		if order.ExtraFields != nil {
			fields := map[string]interface{}(order.ExtraFields)
			// 检查 shipping_records
			if records, exists := fields["shipping_records"]; exists {
				if recordsList, ok := records.([]interface{}); ok {
					for _, r := range recordsList {
						if record, ok := r.(map[string]interface{}); ok {
							if shippingNum, ok := record["shipping_number"].(string); ok {
								if len(shippingNum) > len(datePrefix) {
									var num int
									if _, err := fmt.Sscanf(shippingNum[len(datePrefix):], "%d", &num); err == nil {
										if num > maxNum {
											maxNum = num
										}
									}
								}
							}
						}
					}
				}
			}
			// 检查 shipping_notes
			if notes, exists := fields["shipping_notes"]; exists {
				if notesList, ok := notes.([]interface{}); ok {
					for _, n := range notesList {
						if note, ok := n.(map[string]interface{}); ok {
							if shippingNum, ok := note["shipping_number"].(string); ok {
								if len(shippingNum) > len(datePrefix) {
									var num int
									if _, err := fmt.Sscanf(shippingNum[len(datePrefix):], "%d", &num); err == nil {
										if num > maxNum {
											maxNum = num
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if int64(maxNum) > count {
		count = int64(maxNum)
	}

	// 查询今天已预留的发货单号码
	var reservedCount int64
	database.DB.Model(&models.ReservedNumber{}).Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?",
		tenantID, "shipping_number", datePrefix+"%").Count(&reservedCount)

	sequence := count + reservedCount + 1
	shippingNumber := datePrefix + fmt.Sprintf("%03d", sequence)

	// 检查并确保编号唯一（如果已被使用，递增序号）
	for {
		// 检查是否已在订单中使用
		var found bool
		for _, order := range orders {
			if order.ExtraFields != nil {
				fields := map[string]interface{}(order.ExtraFields)
				if records, exists := fields["shipping_records"]; exists {
					if recordsList, ok := records.([]interface{}); ok {
						for _, r := range recordsList {
							if record, ok := r.(map[string]interface{}); ok {
								if num, ok := record["shipping_number"].(string); ok && num == shippingNumber {
									found = true
									break
								}
							}
						}
					}
				}
				if notes, exists := fields["shipping_notes"]; exists {
					if notesList, ok := notes.([]interface{}); ok {
						for _, n := range notesList {
							if note, ok := n.(map[string]interface{}); ok {
								if num, ok := note["shipping_number"].(string); ok && num == shippingNumber {
									found = true
									break
								}
							}
						}
					}
				}
			}
		}
		if !found {
			// 检查是否已预留
			var reserved models.ReservedNumber
			if err := database.DB.Where("tenant_id = ? AND field_name = ? AND field_value = ?",
				tenantID, "shipping_number", shippingNumber).First(&reserved).Error; err != nil {
				// 编号不存在，可以使用
				break
			}
		}
		// 如果已存在，递增序号
		sequence++
		shippingNumber = datePrefix + fmt.Sprintf("%03d", sequence)
	}

	return shippingNumber, nil
}

// GenerateOrderNumber 自动生成订单号码
func GenerateOrderNumber(tenantID uuid.UUID) (string, error) {
	today := time.Now().Format("20060102")
	datePrefix := "ORD-" + today + "-"
	var count int64

	// 查询今天已生成的订单数量
	database.DB.Model(&models.Order{}).Where("tenant_id = ? AND order_number LIKE ?", tenantID, datePrefix+"%").Count(&count)

	// 也查询已预留的订单号
	var reservedCount int64
	database.DB.Model(&models.ReservedNumber{}).Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?",
		tenantID, "order_number", datePrefix+"%").Count(&reservedCount)

	sequence := count + reservedCount + 1
	orderNumber := datePrefix + fmt.Sprintf("%03d", sequence)

	// 检查并确保编号唯一（如果已被使用，递增序号）
	for {
		var existingOrder models.Order
		if err := database.DB.Where("tenant_id = ? AND order_number = ?", tenantID, orderNumber).First(&existingOrder).Error; err != nil {
			// 订单号不存在，可以使用
			break
		}
		// 如果已存在，递增序号
		sequence++
		orderNumber = datePrefix + fmt.Sprintf("%03d", sequence)
	}

	return orderNumber, nil
}
