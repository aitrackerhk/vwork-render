package handlers

import (
	"errors"
	"fmt"
	"log"
	"nwork/internal/database"
	"nwork/internal/email"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ============================================
// 服務種類 (ServiceType) CRUD
// ============================================

func GetServiceTypes(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var serviceTypes []models.ServiceType
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}
	// Filter by code (e.g. ?code=course)
	if code := c.Query("code"); code != "" {
		query = query.Where("code = ?", code)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.ServiceType{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Find(&serviceTypes).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  serviceTypes,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetServiceType(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var serviceType models.ServiceType

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&serviceType).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Service type not found"})
	}

	return c.JSON(serviceType)
}

func CreateServiceType(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var serviceType models.ServiceType
	if err := c.BodyParser(&serviceType); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	serviceType.TenantID = tenantID

	if err := database.DB.Create(&serviceType).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(serviceType)
}

func UpdateServiceType(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var serviceType models.ServiceType

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&serviceType).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Service type not found"})
	}

	if err := c.BodyParser(&serviceType); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	serviceType.TenantID = tenantID

	if err := database.DB.Save(&serviceType).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(serviceType)
}

func DeleteServiceType(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.ServiceType{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Service type deleted"})
}

// ============================================
// 服務 (Service) CRUD
// ============================================

func GetServices(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	serviceTypeID := c.Query("service_type_id")

	var services []models.Service
	query := database.DB.Where("tenant_id = ?", tenantID)

	if serviceTypeID != "" {
		query = query.Where("service_type_id = ?", serviceTypeID)
	}

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Service{}).Count(&total)

	if err := query.Preload("ServiceType").Preload("ServiceTaxes").Offset(offset).Limit(limit).Find(&services).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  services,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetService(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var service models.Service

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Preload("ServiceType").Preload("ServiceTaxes").First(&service).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Service not found"})
	}

	// 附加 service_tax_ids 給 select2-multi
	result := make(map[string]interface{})
	result["id"] = service.ID
	result["tenant_id"] = service.TenantID
	result["service_type_id"] = service.ServiceTypeID
	result["service_type"] = service.ServiceType
	result["name"] = service.Name
	result["code"] = service.Code
	result["description"] = service.Description
	result["price"] = service.Price
	result["duration_minutes"] = service.DurationMinutes
	result["status"] = service.Status
	result["extra_fields"] = service.ExtraFields
	result["created_at"] = service.CreatedAt
	result["updated_at"] = service.UpdatedAt
	serviceTaxIDs := make([]string, 0)
	if service.ServiceTaxes != nil {
		for _, t := range service.ServiceTaxes {
			if t.ID != uuid.Nil {
				serviceTaxIDs = append(serviceTaxIDs, t.ID.String())
			}
		}
	}
	result["service_tax_ids"] = serviceTaxIDs
	return c.JSON(result)
}

func CreateService(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		ServiceTypeID   *uuid.UUID             `json:"service_type_id"`
		Name            string                 `json:"name"`
		Code            *string                `json:"code"`
		Description     string                 `json:"description"`
		Price           float64                `json:"price"`
		DurationMinutes *int                   `json:"duration_minutes"`
		Status          string                 `json:"status"`
		ServiceTaxIDs   []uuid.UUID            `json:"service_tax_ids"`
		ExtraFields     map[string]interface{} `json:"extra_fields"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}
	if req.Status == "" {
		req.Status = "active"
	}

	// Auto-enable show_on_vmarket if tenant has joined VMarket
	if req.ExtraFields == nil {
		req.ExtraFields = map[string]interface{}{}
	}
	if _, ok := req.ExtraFields["show_on_vmarket"]; !ok {
		if joined, err := getTenantVMarketJoined(tenantID); err == nil && joined {
			req.ExtraFields["show_on_vmarket"] = true
		}
	}

	service := models.Service{
		TenantID:        tenantID,
		ServiceTypeID:   req.ServiceTypeID,
		Name:            req.Name,
		Code:            req.Code,
		Description:     req.Description,
		Price:           req.Price,
		DurationMinutes: req.DurationMinutes,
		Status:          req.Status,
		ExtraFields:     models.JSONB(req.ExtraFields),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := database.DB.Create(&service).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 綁定服務稅
	if len(req.ServiceTaxIDs) > 0 {
		now := time.Now()
		for _, taxID := range req.ServiceTaxIDs {
			if taxID == uuid.Nil {
				continue
			}
			_ = database.DB.Create(&models.ServiceTaxRelation{
				ServiceID: service.ID,
				TaxID:     taxID,
				CreatedAt: now,
			}).Error
		}
	}

	return c.Status(201).JSON(service)
}

func UpdateService(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var service models.Service

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&service).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Service not found"})
	}

	var req struct {
		ServiceTypeID   *uuid.UUID              `json:"service_type_id"`
		Name            *string                 `json:"name"`
		Code            *string                 `json:"code"`
		Description     *string                 `json:"description"`
		Price           *float64                `json:"price"`
		DurationMinutes *int                    `json:"duration_minutes"`
		Status          *string                 `json:"status"`
		ServiceTaxIDs   []uuid.UUID             `json:"service_tax_ids"`
		ExtraFields     *map[string]interface{} `json:"extra_fields"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	service.TenantID = tenantID
	if req.ServiceTypeID != nil {
		service.ServiceTypeID = req.ServiceTypeID
	}
	if req.Name != nil {
		service.Name = *req.Name
	}
	if req.Code != nil {
		service.Code = req.Code
	}
	if req.Description != nil {
		service.Description = *req.Description
	}
	if req.Price != nil {
		service.Price = *req.Price
	}
	if req.DurationMinutes != nil {
		service.DurationMinutes = req.DurationMinutes
	}
	if req.Status != nil {
		service.Status = *req.Status
	}
	if req.ExtraFields != nil {
		extraFields := *req.ExtraFields
		// Preserve show_on_vmarket if not explicitly provided
		if _, ok := extraFields["show_on_vmarket"]; !ok {
			if service.ExtraFields != nil {
				if existing, ok := service.ExtraFields["show_on_vmarket"]; ok {
					extraFields["show_on_vmarket"] = existing
				}
			}
			if _, ok := extraFields["show_on_vmarket"]; !ok {
				if joined, err := getTenantVMarketJoined(tenantID); err == nil && joined {
					extraFields["show_on_vmarket"] = true
				}
			}
		}
		service.ExtraFields = models.JSONB(extraFields)
	}
	service.UpdatedAt = time.Now()

	if err := database.DB.Save(&service).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 更新服務稅關聯（先清空再重建）
	database.DB.Where("service_id = ?", service.ID).Delete(&models.ServiceTaxRelation{})
	if len(req.ServiceTaxIDs) > 0 {
		now := time.Now()
		for _, taxID := range req.ServiceTaxIDs {
			if taxID == uuid.Nil {
				continue
			}
			_ = database.DB.Create(&models.ServiceTaxRelation{
				ServiceID: service.ID,
				TaxID:     taxID,
				CreatedAt: now,
			}).Error
		}
	}

	return c.JSON(service)
}

func DeleteService(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.Service{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Service deleted"})
}

// ============================================
// 預約 (Appointment) CRUD
// ============================================

func GetAppointments(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	customerID := c.Query("customer_id")
	staffID := c.Query("staff_id")
	appointmentDate := c.Query("appointment_date")

	var appointments []models.Appointment
	query := database.DB.Where("tenant_id = ?", tenantID)

	if customerID != "" {
		query = query.Where("customer_id = ?", customerID)
	}
	if staffID != "" {
		query = query.Where("staff_id = ?", staffID)
	}
	if appointmentDate != "" {
		query = query.Where("appointment_date = ?", appointmentDate)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if labelIDs := parseUUIDListFromStrings(c.Query("label_id"), c.Query("label_ids")); len(labelIDs) > 0 {
		query = query.Joins("JOIN service_order_label_relations ON service_orders.id = service_order_label_relations.service_order_id").
			Where("service_order_label_relations.label_id IN ?", labelIDs).
			Group("service_orders.id")
	}
	// 按服務種類 ID 篩選（透過 appointments.service_id → services.service_type_id）
	if serviceTypeID := c.Query("service_type_id"); serviceTypeID != "" {
		query = query.Where("service_id IN (SELECT s.id FROM services s WHERE s.service_type_id = ? AND s.tenant_id = ?)", serviceTypeID, tenantID)
	}
	// 按服務種類代碼篩選
	if serviceTypeCode := c.Query("service_type_code"); serviceTypeCode != "" {
		query = query.Where("service_id IN (SELECT s.id FROM services s JOIN service_types st ON s.service_type_id = st.id WHERE st.code = ? AND st.tenant_id = ?)", serviceTypeCode, tenantID)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Appointment{}).Count(&total)

	if err := query.Preload("Customer").Preload("Service.ServiceType").Preload("Staff").
		Preload("Rooms").Preload("Equipments").Preload("Vehicles").
		Offset(offset).Limit(limit).Order("start_time ASC").
		Find(&appointments).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  appointments,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetAppointment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var appointment models.Appointment

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).
		Preload("Customer").Preload("Service").Preload("Staff").
		Preload("Rooms").Preload("Equipments").Preload("Vehicles").First(&appointment).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Appointment not found"})
	}

	return c.JSON(appointment)
}

func CreateAppointment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	// 解析請求體
	var req struct {
		models.Appointment
		RoomIDs      []uuid.UUID `json:"room_ids"`
		EquipmentIDs []uuid.UUID `json:"equipment_ids"`
		VehicleIDs   []uuid.UUID `json:"vehicle_ids"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	appointment := req.Appointment
	appointment.TenantID = tenantID

	// 確保時間在租戶時區中
	loc := utils.GetTenantLocation(tenantID)
	if !appointment.StartTime.IsZero() {
		appointment.StartTime = appointment.StartTime.In(loc)
	}
	if !appointment.EndTime.IsZero() {
		appointment.EndTime = appointment.EndTime.In(loc)
	}

	// 驗證開始和結束時間
	if appointment.StartTime.IsZero() || appointment.EndTime.IsZero() {
		return c.Status(400).JSON(fiber.Map{"error": "開始時間和結束時間不能為空"})
	}
	if appointment.EndTime.Before(appointment.StartTime) || appointment.EndTime.Equal(appointment.StartTime) {
		return c.Status(400).JSON(fiber.Map{"error": "結束時間必須晚於開始時間"})
	}

	// 向後兼容欄位：若前端未送 appointment_date/appointment_time，從 start_time 自動推導
	// 避免 DB not-null constraint（appointments.appointment_time）直接報錯
	if appointment.AppointmentDate.IsZero() {
		appointment.AppointmentDate = time.Date(
			appointment.StartTime.Year(),
			appointment.StartTime.Month(),
			appointment.StartTime.Day(),
			0, 0, 0, 0,
			loc,
		)
	}
	if strings.TrimSpace(string(appointment.AppointmentTime)) == "" {
		appointment.AppointmentTime = models.NewSQLTime(appointment.StartTime)
	}

	// 檢查房間、設備和車輛衝突
	conflicts := checkAppointmentConflicts(tenantID, appointment.StartTime, appointment.EndTime, req.RoomIDs, req.EquipmentIDs, req.VehicleIDs, uuid.Nil)
	if len(conflicts) > 0 {
		return c.Status(409).JSON(fiber.Map{
			"error":     "預約時間衝突",
			"conflicts": conflicts,
		})
	}

	// 創建預約
	if err := database.DB.Create(&appointment).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 關聯房間
	if len(req.RoomIDs) > 0 {
		for _, roomID := range req.RoomIDs {
			database.DB.Create(&models.AppointmentRoom{
				AppointmentID: appointment.ID,
				RoomID:        roomID,
			})
		}
	}

	// 關聯設備
	if len(req.EquipmentIDs) > 0 {
		for _, equipmentID := range req.EquipmentIDs {
			database.DB.Create(&models.AppointmentEquipment{
				AppointmentID: appointment.ID,
				EquipmentID:   equipmentID,
			})
		}
	}

	// 關聯車輛
	if len(req.VehicleIDs) > 0 {
		for _, vehicleID := range req.VehicleIDs {
			database.DB.Create(&models.AppointmentVehicle{
				AppointmentID: appointment.ID,
				VehicleID:     vehicleID,
			})
		}
	}

	// 重新載入關聯數據
	database.DB.Preload("Customer").Preload("Service").Preload("Staff").
		Preload("Rooms").Preload("Equipments").Preload("Vehicles").First(&appointment, appointment.ID)

	// 通知整個 domain 的所有用戶（檢查通知設置）
	var settings models.NotificationSettings
	if err := database.DB.Where("tenant_id = ?", tenantID).First(&settings).Error; err != nil {
		// 如果不存在設定，使用默認值（啟用）
		settings.AppointmentNotificationsEnabled = true
	}

	if settings.AppointmentNotificationsEnabled {
		customerName := "未知客戶"
		if appointment.Customer.ID != uuid.Nil {
			customerName = appointment.Customer.Name
		}
		serviceName := "未指定服務"
		if appointment.Service != nil {
			serviceName = appointment.Service.Name
		}
		title := fmt.Sprintf("新預約：%s", customerName)
		message := fmt.Sprintf("客戶：%s\n服務：%s\n開始時間：%s\n結束時間：%s\n狀態：%s",
			customerName, serviceName,
			appointment.StartTime.Format("2006-01-02 15:04"),
			appointment.EndTime.Format("2006-01-02 15:04"),
			appointment.Status)
		link := fmt.Sprintf("/appointments?appointment_id=%s", appointment.ID.String())
		// 获取创建预约的用户ID
		userID := middleware.GetUserID(c)
		var appointmentCreatorID *uuid.UUID
		if userID != uuid.Nil {
			appointmentCreatorID = &userID
		}
		go utils.CreateNotificationAlertForAllUsers(tenantID, "appointment_created", title, message, link, appointmentCreatorID)
	}

	return c.Status(201).JSON(appointment)
}

func UpdateAppointment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var appointment models.Appointment

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&appointment).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Appointment not found"})
	}

	// 解析請求體
	var req struct {
		models.Appointment
		RoomIDs      []uuid.UUID `json:"room_ids"`
		EquipmentIDs []uuid.UUID `json:"equipment_ids"`
		VehicleIDs   []uuid.UUID `json:"vehicle_ids"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 驗證開始和結束時間
	if !req.StartTime.IsZero() && !req.EndTime.IsZero() {
		if req.EndTime.Before(req.StartTime) || req.EndTime.Equal(req.StartTime) {
			return c.Status(400).JSON(fiber.Map{"error": "結束時間必須晚於開始時間"})
		}

		// 檢查衝突（排除當前預約）
		conflicts := checkAppointmentConflicts(tenantID, req.StartTime, req.EndTime, req.RoomIDs, req.EquipmentIDs, req.VehicleIDs, appointment.ID)
		if len(conflicts) > 0 {
			return c.Status(409).JSON(fiber.Map{
				"error":     "預約時間衝突",
				"conflicts": conflicts,
			})
		}
	}

	// 更新預約
	appointment.CustomerID = req.CustomerID
	appointment.ServiceID = req.ServiceID
	appointment.StaffID = req.StaffID
	loc := utils.GetTenantLocation(tenantID)
	if !req.StartTime.IsZero() {
		appointment.StartTime = req.StartTime.In(loc)
		// 同步 legacy 欄位，避免後續讀寫不一致
		appointment.AppointmentDate = time.Date(
			appointment.StartTime.Year(),
			appointment.StartTime.Month(),
			appointment.StartTime.Day(),
			0, 0, 0, 0,
			loc,
		)
		appointment.AppointmentTime = models.NewSQLTime(appointment.StartTime)
	}
	if !req.EndTime.IsZero() {
		appointment.EndTime = req.EndTime.In(loc)
	}
	appointment.Notes = req.Notes
	appointment.Status = req.Status

	if err := database.DB.Save(&appointment).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 更新房間關聯
	database.DB.Where("appointment_id = ?", appointment.ID).Delete(&models.AppointmentRoom{})
	if len(req.RoomIDs) > 0 {
		for _, roomID := range req.RoomIDs {
			database.DB.Create(&models.AppointmentRoom{
				AppointmentID: appointment.ID,
				RoomID:        roomID,
			})
		}
	}

	// 更新設備關聯
	database.DB.Where("appointment_id = ?", appointment.ID).Delete(&models.AppointmentEquipment{})
	if len(req.EquipmentIDs) > 0 {
		for _, equipmentID := range req.EquipmentIDs {
			database.DB.Create(&models.AppointmentEquipment{
				AppointmentID: appointment.ID,
				EquipmentID:   equipmentID,
			})
		}
	}

	// 更新車輛關聯
	database.DB.Where("appointment_id = ?", appointment.ID).Delete(&models.AppointmentVehicle{})
	if len(req.VehicleIDs) > 0 {
		for _, vehicleID := range req.VehicleIDs {
			database.DB.Create(&models.AppointmentVehicle{
				AppointmentID: appointment.ID,
				VehicleID:     vehicleID,
			})
		}
	}

	// 重新載入關聯數據
	database.DB.Preload("Customer").Preload("Service").Preload("Staff").
		Preload("Rooms").Preload("Equipments").Preload("Vehicles").First(&appointment, appointment.ID)

	return c.JSON(appointment)
}

func DeleteAppointment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.Appointment{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Appointment deleted"})
}

// ============================================
// 服務單 (ServiceOrder) CRUD
// ============================================

func GetServiceOrders(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	customerID := c.Query("customer_id")

	var serviceOrders []models.ServiceOrder
	query := database.DB.Where("tenant_id = ?", tenantID)

	if customerID != "" {
		query = query.Where("customer_id = ?", customerID)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// 搜索過濾（服務單號 + 客戶名 + 服務名）
	if search := c.Query("search"); search != "" {
		like := "%" + search + "%"
		query = query.Where(
			"(order_number ILIKE ? OR EXISTS (SELECT 1 FROM customers WHERE customers.id = service_orders.customer_id AND customers.name ILIKE ?) OR EXISTS (SELECT 1 FROM service_order_items soi JOIN services s ON soi.service_id = s.id WHERE soi.service_order_id = service_orders.id AND soi.trashed_at IS NULL AND s.name ILIKE ?))",
			like, like, like,
		)
	}

	// 標籤過濾（支援多選）
	if labelIDs := parseUUIDListFromStrings(c.Query("label_id"), c.Query("label_ids")); len(labelIDs) > 0 {
		query = query.Joins("JOIN service_order_label_relations ON service_orders.id = service_order_label_relations.service_order_id").
			Where("service_order_label_relations.label_id IN ?", labelIDs).
			Group("service_orders.id")
	}

	// 按服務種類篩選（透過 service_order_items → services → service_type_id）
	if serviceTypeID := c.Query("service_type_id"); serviceTypeID != "" {
		query = query.Where("id IN (SELECT soi.service_order_id FROM service_order_items soi JOIN services s ON soi.service_id = s.id WHERE s.service_type_id = ? AND soi.trashed_at IS NULL)", serviceTypeID)
	}
	// 按服務種類代碼篩選（用於 URL 連結，如 ?service_type_code=course）
	if serviceTypeCode := c.Query("service_type_code"); serviceTypeCode != "" {
		query = query.Where("id IN (SELECT soi.service_order_id FROM service_order_items soi JOIN services s ON soi.service_id = s.id JOIN service_types st ON s.service_type_id = st.id WHERE st.code = ? AND st.tenant_id = ? AND soi.trashed_at IS NULL)", serviceTypeCode, tenantID)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.ServiceOrder{}).Count(&total)

	if err := query.Preload("Customer").Preload("ServiceOrderItems.Service.ServiceType").Preload("ServiceOrderItems.Staff").
		Preload("Salesperson").Preload("Store").Preload("Labels").Preload("Appointments").
		Offset(offset).Limit(limit).Order("service_date DESC").
		Find(&serviceOrders).Error; err != nil {
		log.Printf("Error loading service orders: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to load service orders: " + err.Error()})
	}

	// 為每張服務單計算不重複的服務種類名稱列表及服務名稱列表
	type serviceOrderWithTypes struct {
		models.ServiceOrder
		ServiceTypes []string `json:"service_types"`
		Services     []string `json:"services"`
	}
	results := make([]serviceOrderWithTypes, len(serviceOrders))
	for i, so := range serviceOrders {
		results[i].ServiceOrder = so
		seenTypes := map[string]bool{}
		seenServices := map[string]bool{}
		for _, item := range so.ServiceOrderItems {
			if item.Service != nil {
				// 服務名稱
				svcName := item.Service.Name
				if svcName != "" && !seenServices[svcName] {
					seenServices[svcName] = true
					results[i].Services = append(results[i].Services, svcName)
				}
				// 服務種類
				if item.Service.ServiceType != nil {
					typeName := item.Service.ServiceType.Name
					if !seenTypes[typeName] {
						seenTypes[typeName] = true
						results[i].ServiceTypes = append(results[i].ServiceTypes, typeName)
					}
				}
			}
		}
		if results[i].ServiceTypes == nil {
			results[i].ServiceTypes = []string{}
		}
		if results[i].Services == nil {
			results[i].Services = []string{}
		}
	}

	return c.JSON(fiber.Map{
		"data":  results,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetServiceOrder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var serviceOrder models.ServiceOrder

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).
		Preload("Customer").Preload("ServiceOrderItems.Service").Preload("ServiceOrderItems.Staff").
		Preload("Salesperson").Preload("Store").Preload("Labels").Preload("Appointments").
		Preload("Order").
		First(&serviceOrder).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(404).JSON(fiber.Map{"error": "Service order not found"})
		}
		log.Printf("Error loading service order %s: %v", id, err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to load service order: " + err.Error()})
	}

	// 從 income 表獲取付款記錄（與訂單保持一致）
	var incomes []models.Income
	database.DB.Where("tenant_id = ? AND reference_type = ? AND reference_id = ?", tenantID, "service_order", serviceOrder.ID).Find(&incomes)

	// 構建結果，包含付款記錄
	result := make(map[string]interface{})
	result["id"] = serviceOrder.ID
	result["tenant_id"] = serviceOrder.TenantID
	result["order_number"] = serviceOrder.OrderNumber
	result["customer_id"] = serviceOrder.CustomerID
	result["customer"] = serviceOrder.Customer
	result["contact_name"] = serviceOrder.ContactName
	result["contact_email"] = serviceOrder.ContactEmail
	result["contact_phone"] = serviceOrder.ContactPhone
	result["contact_address"] = serviceOrder.ContactAddress
	result["salesperson_id"] = serviceOrder.SalespersonID
	result["salesperson"] = serviceOrder.Salesperson
	result["order_id"] = serviceOrder.OrderID
	result["order"] = serviceOrder.Order
	result["service_date"] = serviceOrder.ServiceDate
	result["status"] = serviceOrder.Status
	result["notes"] = serviceOrder.Notes
	result["service_order_items"] = serviceOrder.ServiceOrderItems
	result["labels"] = serviceOrder.Labels
	result["appointments"] = serviceOrder.Appointments
	result["created_at"] = serviceOrder.CreatedAt
	result["updated_at"] = serviceOrder.UpdatedAt
	if serviceOrder.ExtraFields != nil {
		fields := map[string]interface{}(serviceOrder.ExtraFields)
		if refundNotes, exists := fields["refund_notes"]; exists {
			result["refund_notes"] = refundNotes
		}
	}
	result["created_by"] = serviceOrder.CreatedBy
	result["updated_by"] = serviceOrder.UpdatedBy
	result["extra_fields"] = serviceOrder.ExtraFields

	// 從 income 表獲取付款記錄
	if len(incomes) > 0 {
		paymentRecords := make([]map[string]interface{}, len(incomes))
		for i, inc := range incomes {
			ef := map[string]interface{}{}
			if inc.ExtraFields != nil {
				ef = map[string]interface{}(inc.ExtraFields)
			}
			paymentRecords[i] = map[string]interface{}{
				"income_id":         inc.ID.String(),
				"payment_date":      inc.IncomeDate.Format("2006-01-02"),
				"payment_method":    inc.PaymentMethod,
				"payment_method_id": ef["payment_method_id"],
				"bank_account_id":   inc.BankAccountID,
				"amount":            inc.Amount,
				"invoice_number":    ef["invoice_number"],
				"reference_number":  ef["reference_number"],
				"notes":             inc.Notes,
			}
		}
		result["payment_records"] = paymentRecords
	} else {
		result["payment_records"] = []interface{}{}
	}

	return c.JSON(result)
}

func CreateServiceOrder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		OrderNumber         string      `json:"order_number"`
		OrderID             *uuid.UUID  `json:"order_id"` // 關聯的訂單ID（服務套票轉化）
		CustomerID          *uuid.UUID  `json:"customer_id"`
		StoreID             *uuid.UUID  `json:"store_id"`
		ContactName         string      `json:"contact_name"`
		ContactEmail        string      `json:"contact_email"`
		ContactPhone        string      `json:"contact_phone"`
		ContactAddress      string      `json:"contact_address"`
		SalespersonID       *uuid.UUID  `json:"salesperson_id"`
		CommissionAmount    float64     `json:"commission_amount"`
		ServiceDate         string      `json:"service_date"`
		Status              string      `json:"status"`
		Notes               string      `json:"notes"`
		ReferralCode        string      `json:"referral_code"`
		OrderDiscount       float64     `json:"order_discount"`        // 全單折扣百分比
		OrderDiscountAmount float64     `json:"order_discount_amount"` // 全單折扣金額
		LabelIDs            []uuid.UUID `json:"label_ids"`
		ServiceOrderItems   []struct {
			ServiceID *uuid.UUID `json:"service_id"`
			StaffID   *uuid.UUID `json:"staff_id"`
			Quantity  float64    `json:"quantity"`
			UnitPrice float64    `json:"unit_price"`
			Notes     string     `json:"notes"`
		} `json:"service_order_items"`
		Appointments []struct {
			ID         *uuid.UUID `json:"id"` // 如果是更新，有ID；如果是新建，为nil
			CustomerID *uuid.UUID `json:"customer_id"`
			ServiceID  *uuid.UUID `json:"service_id"`
			StaffID    *uuid.UUID `json:"staff_id"`
			StartTime  string     `json:"start_time"` // ISO 8601 format
			EndTime    string     `json:"end_time"`   // ISO 8601 format
			Status     string     `json:"status"`
			Notes      string     `json:"notes"`
		} `json:"appointments"`
		PaymentRecords []map[string]interface{} `json:"payment_records"`
		ExtraFields    map[string]interface{}   `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if err := middleware.EnforceTrialServiceOrderLimit(c, tenantID, userID); err != nil {
		return err
	}

	// 如果沒有提供服務單號，自動生成
	if req.OrderNumber == "" {
		req.OrderNumber = "SO-" + time.Now().Format("20060102") + "-" + uuid.New().String()[:8]
	}

	// 檢查服務單號是否已存在
	var existingServiceOrder models.ServiceOrder
	if err := database.DB.Where("tenant_id = ? AND order_number = ?", tenantID, req.OrderNumber).First(&existingServiceOrder).Error; err == nil {
		// 如果已存在，重新生成
		req.OrderNumber = "SO-" + time.Now().Format("20060102") + "-" + uuid.New().String()[:8]
	}

	// 解析日期（使用租戶時區）
	serviceDate, err := utils.ParseDateInTenantTimezone(tenantID, req.ServiceDate)
	if err != nil {
		serviceDate = utils.NowInTenantTimezone(tenantID)
	}

	if req.Status == "" {
		req.Status = "confirmed"
	}

	// 如果沒有提供referral_code，嘗試從客戶信息中獲取
	if req.ReferralCode == "" && req.CustomerID != nil {
		var customer models.Customer
		if err := database.DB.Where("id = ? AND tenant_id = ?", req.CustomerID, tenantID).First(&customer).Error; err == nil {
			req.ReferralCode = customer.ReferralCode
		}
	}

	now := time.Now()

	// 處理 ExtraFields（不再包含 payment_records）
	extraFields := make(map[string]interface{})
	if req.ExtraFields != nil {
		for k, v := range req.ExtraFields {
			// 跳過 payment_records，不再保存到 ExtraFields
			if k != "payment_records" {
				extraFields[k] = v
			}
		}
	}

	serviceOrder := models.ServiceOrder{
		TenantID:         tenantID,
		OrderNumber:      req.OrderNumber,
		OrderID:          req.OrderID, // 關聯的訂單ID
		CustomerID:       req.CustomerID,
		StoreID:          req.StoreID,
		ContactName:      req.ContactName,
		ContactEmail:     req.ContactEmail,
		ContactPhone:     req.ContactPhone,
		ContactAddress:   req.ContactAddress,
		SalespersonID:    req.SalespersonID,
		CommissionAmount: req.CommissionAmount,
		ServiceDate:      serviceDate,
		Status:           req.Status,
		Notes:            req.Notes,
		ReferralCode:     req.ReferralCode,
		CreatedBy:        &userID,
		UpdatedBy:        &userID,
		CreatedAt:        now,
		UpdatedAt:        now,
		ExtraFields:      models.JSONB(extraFields),
	}

	// 計算總金額
	var subtotal float64
	for _, item := range req.ServiceOrderItems {
		itemTotal := item.Quantity * item.UnitPrice
		subtotal += itemTotal
	}

	// 應用折扣
	orderDiscountAmount := req.OrderDiscountAmount
	if req.OrderDiscount > 0 && orderDiscountAmount == 0 {
		orderDiscountAmount = subtotal * req.OrderDiscount / 100
	}

	serviceOrder.TotalAmount = subtotal - orderDiscountAmount
	if serviceOrder.TotalAmount < 0 {
		serviceOrder.TotalAmount = 0
	}

	// 計算積分（如果有客戶且積分設置存在）
	var pointsEarned int = 0
	if req.CustomerID != nil {
		var pointSetting models.PointSetting
		if err := database.DB.Where("tenant_id = ?", tenantID).First(&pointSetting).Error; err == nil {
			// 檢查是否開啟服務單消費獲得積分
			if pointSetting.EnableServiceOrderEarnPoints && pointSetting.PointsPerDollar > 0 {
				// 計算實際支付金額（總金額 - 折扣）
				// 服務單沒有 CouponDiscount 和 PointsDiscount 字段，使用 TotalAmount
				actualAmount := serviceOrder.TotalAmount
				if actualAmount > 0 {
					pointsEarned = int(actualAmount * pointSetting.PointsPerDollar)
				}
			}
		}
	}
	serviceOrder.PointsEarned = pointsEarned

	// 開始事務
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Create(&serviceOrder).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create service order: " + err.Error()})
	}

	// 創建服務單明細
	for _, itemReq := range req.ServiceOrderItems {
		itemTotal := itemReq.Quantity * itemReq.UnitPrice
		serviceOrderItem := models.ServiceOrderItem{
			TenantID:       tenantID,
			ServiceOrderID: serviceOrder.ID,
			ServiceID:      itemReq.ServiceID,
			StaffID:        itemReq.StaffID,
			Quantity:       itemReq.Quantity,
			UnitPrice:      itemReq.UnitPrice,
			TotalPrice:     itemTotal,
			Notes:          itemReq.Notes,
			CreatedAt:      now,
		}

		if err := tx.Create(&serviceOrderItem).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create service order item: " + err.Error()})
		}
	}

	// 處理標籤關聯
	if len(req.LabelIDs) > 0 {
		for _, labelID := range req.LabelIDs {
			// 驗證標籤屬於當前租戶
			var label models.ServiceOrderLabel
			if err := tx.Where("id = ? AND tenant_id = ?", labelID, tenantID).First(&label).Error; err == nil {
				// 創建服務單標籤關聯（使用多對多表）
				relation := map[string]interface{}{
					"service_order_id": serviceOrder.ID,
					"label_id":         labelID,
				}
				if err := tx.Table("service_order_label_relations").Create(relation).Error; err != nil {
					// 如果已存在，忽略錯誤
					continue
				}
			}
		}
	}

	// 處理預約記錄
	if len(req.Appointments) > 0 {
		for _, aptReq := range req.Appointments {
			// 解析時間（使用租戶時區）
			startTime, err := utils.ParseDateTimeInTenantTimezone(tenantID, aptReq.StartTime)
			if err != nil {
				tx.Rollback()
				return c.Status(400).JSON(fiber.Map{"error": "Invalid start_time format: " + err.Error()})
			}

			endTime, err := utils.ParseDateTimeInTenantTimezone(tenantID, aptReq.EndTime)
			if err != nil {
				tx.Rollback()
				return c.Status(400).JSON(fiber.Map{"error": "Invalid end_time format: " + err.Error()})
			}

			// 驗證時間
			if endTime.Before(startTime) || endTime.Equal(startTime) {
				tx.Rollback()
				return c.Status(400).JSON(fiber.Map{"error": "結束時間必須晚於開始時間"})
			}

			// 使用服務單的客戶ID（如果預約沒有指定客戶ID）
			customerID := aptReq.CustomerID
			if customerID == nil {
				customerID = req.CustomerID
			}
			if customerID == nil {
				tx.Rollback()
				return c.Status(400).JSON(fiber.Map{"error": "預約必須有客戶ID"})
			}

			// 驗證客戶是否存在且屬於當前租戶
			var customer models.Customer
			if err := tx.Where("id = ? AND tenant_id = ?", *customerID, tenantID).First(&customer).Error; err != nil {
				tx.Rollback()
				return c.Status(400).JSON(fiber.Map{"error": "Invalid customer ID: customer not found or does not belong to this tenant"})
			}

			// 驗證服務員是否存在且屬於當前租戶（如果提供了 staff_id）
			// appointments.staff_id 外鍵指向 service_staff 表
			if aptReq.StaffID != nil {
				var staff models.ServiceStaff
				if err := tx.Where("id = ? AND tenant_id = ?", *aptReq.StaffID, tenantID).First(&staff).Error; err != nil {
					tx.Rollback()
					log.Printf("[CreateServiceOrder] Invalid staff ID: %v, error: %v", *aptReq.StaffID, err)
					return c.Status(400).JSON(fiber.Map{"error": "Invalid staff ID: service staff not found or does not belong to this tenant"})
				}
				log.Printf("[CreateServiceOrder] Validated ServiceStaff ID: %v", *aptReq.StaffID)
			}

			// 再次驗證 staff_id（如果提供）在插入前
			if aptReq.StaffID != nil {
				var userCheck models.User
				if err := tx.Where("id = ? AND tenant_id = ?", *aptReq.StaffID, tenantID).First(&userCheck).Error; err != nil {
					tx.Rollback()
					log.Printf("[CreateServiceOrder] Final validation failed: staff_id %v does not exist in users table, error: %v", *aptReq.StaffID, err)
					return c.Status(400).JSON(fiber.Map{"error": "Invalid staff ID: user not found or does not belong to this tenant"})
				}
				log.Printf("[CreateServiceOrder] Final validation passed: staff_id %v exists in users table", *aptReq.StaffID)
			}

			// AppointmentDate 需要是日期部分，AppointmentTime 需要是時間部分
			appointmentDate := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, startTime.Location())
			appointmentTime := time.Date(2000, 1, 1, startTime.Hour(), startTime.Minute(), startTime.Second(), 0, startTime.Location())

			appointment := models.Appointment{
				TenantID:        tenantID,
				CustomerID:      *customerID,
				ServiceID:       aptReq.ServiceID,
				StaffID:         aptReq.StaffID,
				StartTime:       startTime,
				EndTime:         endTime,
				AppointmentDate: appointmentDate,
				AppointmentTime: models.NewSQLTime(appointmentTime),
				ServiceOrderID:  &serviceOrder.ID,
				Status:          aptReq.Status,
				Notes:           aptReq.Notes,
			}

			if aptReq.Status == "" {
				appointment.Status = "pending"
			}

			if err := tx.Create(&appointment).Error; err != nil {
				tx.Rollback()
				log.Printf("[CreateServiceOrder] Failed to create appointment: %v, appointment data: %+v", err, appointment)
				return c.Status(500).JSON(fiber.Map{"error": "Failed to create appointment: " + err.Error()})
			}
		}
	}

	// 為每個付款記錄創建 income 記錄（參考訂單的實現）
	if len(req.PaymentRecords) > 0 {
		for _, record := range req.PaymentRecords {
			// 提取付款記錄信息
			amount, _ := record["amount"].(float64)
			if amount > 0 {
				paymentDateStr, _ := record["payment_date"].(string)
				paymentMethod, _ := record["payment_method"].(string)
				paymentMethodID, _ := record["payment_method_id"].(string)
				bankAccountIDStr, _ := record["bank_account_id"].(string)
				notes, _ := record["notes"].(string)
				invoiceNumber, _ := record["invoice_number"].(string)
				referenceNumber, _ := record["reference_number"].(string)

				// 發票號碼（INV-YYYYMMDD-###）：透過 reserved_numbers 生成/預留
				if invoiceNumber == "" {
					if n, err := reserveNextNumber(tenantID, "invoice_number", "service-orders"); err == nil {
						invoiceNumber = n
					}
				} else {
					_, _ = reserveSpecificNumber(tenantID, "invoice_number", invoiceNumber, "service-orders")
				}

				// 解析日期
				var incomeDate time.Time
				if paymentDateStr != "" {
					if parsedDate, err := utils.ParseDateInTenantTimezone(tenantID, paymentDateStr); err == nil {
						incomeDate = parsedDate
					} else {
						incomeDate = serviceOrder.ServiceDate
					}
				} else {
					incomeDate = serviceOrder.ServiceDate
				}

				// 解析銀行賬戶ID
				var bankAccountID *uuid.UUID
				if bankAccountIDStr != "" {
					if parsedID, err := uuid.Parse(bankAccountIDStr); err == nil {
						bankAccountID = &parsedID
					}
				}

				// 構建標題（只使用 invoice_number）
				description := fmt.Sprintf("服務單 %s 的付款", serviceOrder.OrderNumber)
				if invoiceNumber != "" {
					description = invoiceNumber
				}

				// 創建 income 記錄
				relatedUserID := &userID
				if serviceOrder.SalespersonID != nil {
					relatedUserID = serviceOrder.SalespersonID
				}
				income := models.Income{
					TenantID:      tenantID,
					RelatedUserID: relatedUserID,
					IncomeType:    "service_order",
					ReferenceID:   &serviceOrder.ID,
					ReferenceType: "service_order",
					Category:      "service_order",
					Description:   description,
					Amount:        amount,
					IncomeDate:    incomeDate,
					PaymentMethod: paymentMethod,
					BankAccountID: bankAccountID,
					Status:        "confirmed",
					Notes:         notes,
					CreatedBy:     &userID,
					UpdatedBy:     &userID,
					CreatedAt:     now,
					UpdatedAt:     now,
					ExtraFields: models.JSONB(map[string]interface{}{
						"payment_method_id": paymentMethodID,
						"invoice_number":    invoiceNumber,
						"reference_number":  referenceNumber,
						"service_order_id":  serviceOrder.ID.String(),
					}),
				}

				if err := tx.Create(&income).Error; err != nil {
					// 創建失敗，不中斷服務單創建
				}
			}
		}
	} else {
		// 檢查是否自動生成付款記錄
		auto := getDocumentAutoSettingsForTenant(tenantID)
		shouldAutoGeneratePayment := auto.AutoGenerateServiceOrderPayment && serviceOrder.TotalAmount > 0

		if shouldAutoGeneratePayment {
			// 如果沒有手動付款記錄且設定開啟，自動生成一個付款記錄
			defPayMethod := defaultReceivingPaymentMethodCode(tenantID)
			defBankAccID := defaultReceivingBankAccountID(tenantID)

			// 自動生成發票號碼（並預留）
			invoiceNumber := ""
			if n, err := reserveNextNumber(tenantID, "invoice_number", "service-orders"); err == nil {
				invoiceNumber = n
			}

			// 構建標題（只使用 invoice_number）
			description := fmt.Sprintf("服務單 %s 的付款", serviceOrder.OrderNumber)
			if invoiceNumber != "" {
				description = invoiceNumber
			}

			// 創建 income 記錄
			relatedUserID := &userID
			if serviceOrder.SalespersonID != nil {
				relatedUserID = serviceOrder.SalespersonID
			}
			income := models.Income{
				TenantID:      tenantID,
				RelatedUserID: relatedUserID,
				IncomeType:    "service_order",
				ReferenceID:   &serviceOrder.ID,
				ReferenceType: "service_order",
				Category:      "service_order",
				Description:   description,
				Amount:        serviceOrder.TotalAmount,
				IncomeDate:    serviceOrder.ServiceDate,
				PaymentMethod: defPayMethod,
				BankAccountID: defBankAccID,
				Status:        "confirmed",
				Notes:         "",
				CreatedBy:     &userID,
				UpdatedBy:     &userID,
				CreatedAt:     now,
				UpdatedAt:     now,
				ExtraFields: models.JSONB(map[string]interface{}{
					"invoice_number":   invoiceNumber,
					"service_order_id": serviceOrder.ID.String(),
				}),
			}

			if err := tx.Create(&income).Error; err != nil {
				log.Printf("Failed to auto-generate payment record: %v", err)
			}
		}
	}

	// 如果客戶存在且獲得了積分，創建積分記錄並更新客戶總積分
	if req.CustomerID != nil && pointsEarned > 0 {
		// 避免重複加點：同一服務單只允許一筆 earned 記錄
		var existingPoint models.Point
		err := tx.Where("tenant_id = ? AND customer_id = ? AND source_type = ? AND source_id = ? AND points_type = ?",
			tenantID,
			*req.CustomerID,
			"service_order",
			serviceOrder.ID,
			"earned",
		).First(&existingPoint).Error
		if err == nil {
			// 已存在積分記錄，跳過避免重複累加
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Failed to check existing point record: %v", err)
		} else {
			// 創建積分記錄
			point := models.Point{
				TenantID:    tenantID,
				CustomerID:  *req.CustomerID,
				Points:      pointsEarned,
				PointsType:  "earned",
				SourceType:  "service_order",
				SourceID:    &serviceOrder.ID,
				Description: fmt.Sprintf("服務單 %s 消費獲得積分", serviceOrder.OrderNumber),
				Status:      "active",
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := tx.Create(&point).Error; err != nil {
				// 記錄錯誤但不影響服務單創建
				log.Printf("Failed to create point record: %v", err)
			} else {
				// 更新客戶總積分
				var customer models.Customer
				if err := tx.Where("id = ?", *req.CustomerID).First(&customer).Error; err == nil {
					customer.TotalPoints += pointsEarned
					tx.Save(&customer)
				}
			}
		}
	}

	tx.Commit()

	// 同步 Invoice（在 tx.Commit 後，Income 資料已持久化）
	syncOrderInvoice(database.DB, tenantID, "service_order", serviceOrder.ID, &userID)

	// 處理介紹人積分獎勵（僅在服務單完成時）
	if serviceOrder.CustomerID != nil && serviceOrder.Status == "completed" {
		refCode := serviceOrder.ReferralCode
		if refCode == "" {
			var customer models.Customer
			if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, *serviceOrder.CustomerID).First(&customer).Error; err == nil {
				refCode = customer.ReferralCode
			}
		}
		if refCode != "" {
			// 檢查是否開啟服務單介紹人獎勵積分
			var pointSetting models.PointSetting
			if err := database.DB.Where("tenant_id = ?", tenantID).First(&pointSetting).Error; err == nil {
				if pointSetting.EnableServiceOrderReferralBonus {
					processReferralBonus(tenantID, refCode, *serviceOrder.CustomerID, "服務單", serviceOrder.ID, serviceOrder.OrderNumber, pointsEarned)
				}
			} else {
				// 如果沒有設定，預設開啟（向後兼容）
				processReferralBonus(tenantID, refCode, *serviceOrder.CustomerID, "服務單", serviceOrder.ID, serviceOrder.OrderNumber, pointsEarned)
			}
		}
	}

	// 處理會員等級自動升級（檢查所有啟用自動升級的等級，按順序檢查直到不滿足條件）
	if serviceOrder.CustomerID != nil {
		var tenant models.Tenant
		if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
			var hasAutoUpgrade bool
			database.DB.Model(&models.MemberLevel{}).
				Where("tenant_id = ? AND auto_upgrade = ? AND status = ?", tenantID, true, "active").
				Select("COUNT(*) > 0").
				Scan(&hasAutoUpgrade)

			if hasAutoUpgrade {
				checkAndUpgradeMemberLevel(tenantID, *serviceOrder.CustomerID, serviceOrder.TotalAmount)
			}
		}
	}

	// 發送服務單確認 email（服務單已確認且有可用 email）
	if serviceOrder.Status == "confirmed" {
		emailTo := strings.TrimSpace(serviceOrder.ContactEmail)
		customerName := strings.TrimSpace(serviceOrder.ContactName)
		customerID := uuid.Nil
		if serviceOrder.CustomerID != nil {
			customerID = *serviceOrder.CustomerID
			var customer models.Customer
			if err := database.DB.Where("id = ?", *serviceOrder.CustomerID).First(&customer).Error; err == nil {
				if emailTo == "" {
					emailTo = strings.TrimSpace(customer.Email)
				}
				if customerName == "" {
					customerName = strings.TrimSpace(customer.Name)
				}
			}
		}
		if emailTo != "" {
			if customerName == "" {
				customerName = "客戶"
			}
			var tenant models.Tenant
			if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
				serviceDate := serviceOrder.ServiceDate.Format("2006-01-02")
				if err := email.EnqueueOrderConfirmationEmail(
					tenantID,
					tenant.Subdomain,
					customerID,
					emailTo,
					customerName,
					serviceOrder.OrderNumber,
					serviceDate,
					serviceOrder.TotalAmount,
					"service_order",
				); err != nil {
					log.Printf("Failed to enqueue service order confirmation email: %v", err)
				}
			}
		}
	}

	// 重新載入關聯數據
	database.DB.Where("id = ?", serviceOrder.ID).
		Preload("Customer").Preload("ServiceOrderItems.Service").Preload("ServiceOrderItems.Staff").
		Preload("Salesperson").Preload("Store").Preload("Labels").Preload("Appointments").
		First(&serviceOrder)

	// 若開啟自動生成：保存後直接生成/同步「相關支出」（佣金 / 稅）
	autoExpenses := getDocumentAutoSettingsForTenant(tenantID)
	if autoExpenses.AutoGenerateServiceOrderCommission || autoExpenses.AutoGenerateServiceTaxes {
		ensureAndSyncServiceOrderRelatedExpenses(tenantID, serviceOrder.ID, userID, autoExpenses)
	}

	// 通知整個 domain 的所有用戶（檢查通知設置）
	var settings models.NotificationSettings
	if err := database.DB.Where("tenant_id = ?", tenantID).First(&settings).Error; err != nil {
		// 如果不存在設定，使用默認值（啟用）
		settings.ServiceOrderNotificationsEnabled = true
	}

	if settings.ServiceOrderNotificationsEnabled {
		customerName := "散客"
		if serviceOrder.CustomerID != nil {
			if serviceOrder.Customer.ID != uuid.Nil {
				customerName = serviceOrder.Customer.Name
			}
		}
		title := fmt.Sprintf("新服務單：%s", serviceOrder.OrderNumber)
		message := fmt.Sprintf("服務單編號：%s\n客戶：%s\n總金額：$%.2f\n狀態：%s",
			serviceOrder.OrderNumber, customerName, serviceOrder.TotalAmount, serviceOrder.Status)
		link := fmt.Sprintf("/service-orders?service_order_id=%s", serviceOrder.ID.String())
		go utils.CreateNotificationAlertForAllUsers(tenantID, "service_order_created", title, message, link, &userID)
	}

	// Auto-refresh business goals related to service orders
	go RefreshActiveBusinessGoals(tenantID, []string{"service_order_count"})

	return c.Status(201).JSON(serviceOrder)
}

func UpdateServiceOrder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")
	var serviceOrder models.ServiceOrder

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&serviceOrder).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Service order not found"})
	}
	oldStatus := serviceOrder.Status

	// 保存原始積分值（用於後續計算差值）
	oldPointsEarned := serviceOrder.PointsEarned
	oldCustomerID := serviceOrder.CustomerID

	var req struct {
		OrderNumber       string      `json:"order_number"`
		CustomerID        *uuid.UUID  `json:"customer_id"`
		StoreID           *uuid.UUID  `json:"store_id"`
		ContactName       string      `json:"contact_name"`
		ContactEmail      string      `json:"contact_email"`
		ContactPhone      string      `json:"contact_phone"`
		ContactAddress    string      `json:"contact_address"`
		SalespersonID     *uuid.UUID  `json:"salesperson_id"`
		ServiceDate       string      `json:"service_date"`
		Status            string      `json:"status"`
		Notes             string      `json:"notes"`
		LabelIDs          []uuid.UUID `json:"label_ids"`
		ServiceOrderItems []struct {
			ServiceID *uuid.UUID `json:"service_id"`
			StaffID   *uuid.UUID `json:"staff_id"`
			Quantity  float64    `json:"quantity"`
			UnitPrice float64    `json:"unit_price"`
			Notes     string     `json:"notes"`
		} `json:"service_order_items"`
		Appointments []struct {
			ID         *uuid.UUID `json:"id"` // 如果是更新，有ID；如果是新建，为nil
			CustomerID *uuid.UUID `json:"customer_id"`
			ServiceID  *uuid.UUID `json:"service_id"`
			StaffID    *uuid.UUID `json:"staff_id"`
			StartTime  string     `json:"start_time"` // ISO 8601 format
			EndTime    string     `json:"end_time"`   // ISO 8601 format
			Status     string     `json:"status"`
			Notes      string     `json:"notes"`
		} `json:"appointments"`
		PaymentRecords []map[string]interface{} `json:"payment_records"`
		ExtraFields    map[string]interface{}   `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 更新服務單基本信息
	if req.OrderNumber != "" {
		serviceOrder.OrderNumber = req.OrderNumber
	}
	if req.CustomerID != nil {
		serviceOrder.CustomerID = req.CustomerID
	}
	serviceOrder.StoreID = req.StoreID
	serviceOrder.ContactName = req.ContactName
	serviceOrder.ContactEmail = req.ContactEmail
	serviceOrder.ContactPhone = req.ContactPhone
	serviceOrder.ContactAddress = req.ContactAddress
	serviceOrder.SalespersonID = req.SalespersonID
	if req.ServiceDate != "" {
		if serviceDate, err := utils.ParseDateInTenantTimezone(tenantID, req.ServiceDate); err == nil {
			serviceOrder.ServiceDate = serviceDate
		}
	}
	if req.Status != "" {
		serviceOrder.Status = req.Status
	}
	serviceOrder.Notes = req.Notes
	serviceOrder.UpdatedBy = &userID
	serviceOrder.UpdatedAt = time.Now()

	if req.ExtraFields != nil {
		serviceOrder.ExtraFields = models.JSONB(req.ExtraFields)
	}

	// 開始事務
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Save(&serviceOrder).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update service order: " + err.Error()})
	}

	// 刪除現有的服務單明細
	tx.Where("service_order_id = ?", serviceOrder.ID).Delete(&models.ServiceOrderItem{})

	// 重新創建服務單明細
	now := time.Now()
	for i, itemReq := range req.ServiceOrderItems {
		// 驗證數據
		if itemReq.Quantity <= 0 {
			tx.Rollback()
			return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("服務項 %d 的數量必須大於 0", i+1)})
		}
		if itemReq.UnitPrice < 0 {
			tx.Rollback()
			return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("服務項 %d 的單價不能為負數", i+1)})
		}

		itemTotal := itemReq.Quantity * itemReq.UnitPrice
		serviceOrderItem := models.ServiceOrderItem{
			TenantID:       tenantID,
			ServiceOrderID: serviceOrder.ID,
			ServiceID:      itemReq.ServiceID,
			StaffID:        itemReq.StaffID,
			Quantity:       itemReq.Quantity,
			UnitPrice:      itemReq.UnitPrice,
			TotalPrice:     itemTotal,
			Notes:          itemReq.Notes,
			CreatedAt:      now,
		}

		if err := tx.Create(&serviceOrderItem).Error; err != nil {
			tx.Rollback()
			log.Printf("Failed to create service order item: %v, item: %+v", err, serviceOrderItem)
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update service order item: " + err.Error()})
		}
	}

	// 處理標籤關聯
	if req.LabelIDs != nil {
		// 刪除現有標籤關聯
		tx.Table("service_order_label_relations").Where("service_order_id = ?", serviceOrder.ID).Delete(nil)

		// 添加新的標籤關聯
		for _, labelID := range req.LabelIDs {
			// 驗證標籤屬於當前租戶
			var label models.ServiceOrderLabel
			if err := tx.Where("id = ? AND tenant_id = ?", labelID, tenantID).First(&label).Error; err == nil {
				// 創建服務單標籤關聯（使用多對多表）
				relation := map[string]interface{}{
					"service_order_id": serviceOrder.ID,
					"label_id":         labelID,
				}
				if err := tx.Table("service_order_label_relations").Create(relation).Error; err != nil {
					// 如果已存在，忽略錯誤
					continue
				}
			}
		}
	}

	// 處理預約記錄
	// 先刪除所有現有預約（如果前端沒有傳送，則保留）
	if len(req.Appointments) > 0 {
		// 收集要保留的預約ID
		keepIDs := make(map[uuid.UUID]bool)
		for _, aptReq := range req.Appointments {
			if aptReq.ID != nil {
				keepIDs[*aptReq.ID] = true
			}
		}

		// 刪除不在列表中的預約
		var existingAppointments []models.Appointment
		tx.Where("service_order_id = ?", serviceOrder.ID).Find(&existingAppointments)
		for _, apt := range existingAppointments {
			if !keepIDs[apt.ID] {
				tx.Delete(&apt)
			}
		}

		// 創建或更新預約
		for _, aptReq := range req.Appointments {
			log.Printf("Processing appointment: %+v", aptReq)

			// 解析時間（使用租戶時區）
			startTime, err := utils.ParseDateTimeInTenantTimezone(tenantID, aptReq.StartTime)
			if err != nil {
				tx.Rollback()
				log.Printf("Failed to parse start_time: %s, error: %v", aptReq.StartTime, err)
				return c.Status(400).JSON(fiber.Map{"error": "Invalid start_time format: " + err.Error()})
			}
			log.Printf("Parsed start_time: %s -> %v", aptReq.StartTime, startTime)

			endTime, err := utils.ParseDateTimeInTenantTimezone(tenantID, aptReq.EndTime)
			if err != nil {
				tx.Rollback()
				log.Printf("Failed to parse end_time: %s, error: %v", aptReq.EndTime, err)
				return c.Status(400).JSON(fiber.Map{"error": "Invalid end_time format: " + err.Error()})
			}
			log.Printf("Parsed end_time: %s -> %v", aptReq.EndTime, endTime)

			// 驗證時間
			if endTime.Before(startTime) || endTime.Equal(startTime) {
				tx.Rollback()
				return c.Status(400).JSON(fiber.Map{"error": "結束時間必須晚於開始時間"})
			}

			// 使用服務單的客戶ID（如果預約沒有指定客戶ID）
			customerID := aptReq.CustomerID
			if customerID == nil {
				customerID = req.CustomerID
			}
			if customerID == nil {
				tx.Rollback()
				return c.Status(400).JSON(fiber.Map{"error": "預約必須有客戶ID"})
			}

			// 驗證客戶是否存在且屬於當前租戶
			var customer models.Customer
			if err := tx.Where("id = ? AND tenant_id = ?", *customerID, tenantID).First(&customer).Error; err != nil {
				tx.Rollback()
				log.Printf("Invalid customer ID: %v, error: %v", *customerID, err)
				return c.Status(400).JSON(fiber.Map{"error": "Invalid customer ID: customer not found or does not belong to this tenant"})
			}

			// 驗證服務是否存在且屬於當前租戶（如果提供了 service_id）
			if aptReq.ServiceID != nil {
				var service models.Service
				if err := tx.Where("id = ? AND tenant_id = ?", *aptReq.ServiceID, tenantID).First(&service).Error; err != nil {
					tx.Rollback()
					log.Printf("Invalid service ID: %v, error: %v", *aptReq.ServiceID, err)
					return c.Status(400).JSON(fiber.Map{"error": "Invalid service ID: service not found or does not belong to this tenant"})
				}
			}

			// 驗證服務員是否存在且屬於當前租戶（如果提供了 staff_id）
			// appointments.staff_id 外鍵指向 service_staff 表
			if aptReq.StaffID != nil {
				var staff models.ServiceStaff
				if err := tx.Where("id = ? AND tenant_id = ?", *aptReq.StaffID, tenantID).First(&staff).Error; err != nil {
					tx.Rollback()
					log.Printf("[UpdateServiceOrder] Invalid staff ID: %v, error: %v", *aptReq.StaffID, err)
					return c.Status(400).JSON(fiber.Map{"error": "Invalid staff ID: service staff not found or does not belong to this tenant"})
				}
				log.Printf("[UpdateServiceOrder] Validated ServiceStaff ID: %v", *aptReq.StaffID)
			}

			if aptReq.ID != nil {
				// 更新現有預約
				var appointment models.Appointment
				if err := tx.Where("id = ? AND tenant_id = ?", *aptReq.ID, tenantID).First(&appointment).Error; err == nil {
					// AppointmentDate 需要是日期部分，AppointmentTime 需要是時間部分
					appointmentDate := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, startTime.Location())
					appointmentTime := time.Date(2000, 1, 1, startTime.Hour(), startTime.Minute(), startTime.Second(), 0, startTime.Location())

					appointment.CustomerID = *customerID
					appointment.ServiceID = aptReq.ServiceID
					appointment.StaffID = aptReq.StaffID
					appointment.StartTime = startTime
					appointment.EndTime = endTime
					appointment.AppointmentDate = appointmentDate
					appointment.AppointmentTime = models.NewSQLTime(appointmentTime)
					appointment.Status = aptReq.Status
					appointment.Notes = aptReq.Notes
					if aptReq.Status == "" {
						appointment.Status = "pending"
					}
					if err := tx.Save(&appointment).Error; err != nil {
						tx.Rollback()
						log.Printf("Failed to update appointment: %v", err)
						return c.Status(500).JSON(fiber.Map{"error": "Failed to update appointment: " + err.Error()})
					}
				}
			} else {
				// 創建新預約
				// AppointmentDate 需要是日期部分，AppointmentTime 需要是時間部分
				appointmentDate := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, startTime.Location())
				appointmentTime := time.Date(2000, 1, 1, startTime.Hour(), startTime.Minute(), startTime.Second(), 0, startTime.Location())

				appointment := models.Appointment{
					TenantID:        tenantID,
					CustomerID:      *customerID,
					ServiceID:       aptReq.ServiceID,
					StaffID:         aptReq.StaffID, // staff_id 直接指向 service_staff 表
					StartTime:       startTime,
					EndTime:         endTime,
					AppointmentDate: appointmentDate,
					AppointmentTime: models.NewSQLTime(appointmentTime),
					ServiceOrderID:  &serviceOrder.ID,
					Status:          aptReq.Status,
					Notes:           aptReq.Notes,
				}

				if aptReq.Status == "" {
					appointment.Status = "confirmed"
				}

				// 驗證必填字段
				if appointment.StartTime.IsZero() {
					tx.Rollback()
					log.Printf("StartTime is zero for appointment")
					return c.Status(400).JSON(fiber.Map{"error": "StartTime cannot be zero"})
				}
				if appointment.EndTime.IsZero() {
					tx.Rollback()
					log.Printf("EndTime is zero for appointment")
					return c.Status(400).JSON(fiber.Map{"error": "EndTime cannot be zero"})
				}
				if appointment.CustomerID == uuid.Nil {
					tx.Rollback()
					log.Printf("CustomerID is nil for appointment")
					return c.Status(400).JSON(fiber.Map{"error": "CustomerID is required"})
				}

				log.Printf("Creating appointment with data: TenantID=%v, CustomerID=%v, ServiceID=%v, StaffID=%v, StartTime=%v, EndTime=%v, Status=%s",
					appointment.TenantID, appointment.CustomerID, appointment.ServiceID, appointment.StaffID, appointment.StartTime, appointment.EndTime, appointment.Status)

				if err := tx.Create(&appointment).Error; err != nil {
					tx.Rollback()
					log.Printf("Failed to create appointment: %v", err)
					log.Printf("Appointment data: TenantID=%v, CustomerID=%v, ServiceID=%v, StaffID=%v, StartTime=%v, EndTime=%v, AppointmentDate=%v, AppointmentTime=%v, Status=%s, Notes=%s",
						appointment.TenantID, appointment.CustomerID, appointment.ServiceID, appointment.StaffID,
						appointment.StartTime, appointment.EndTime, appointment.AppointmentDate, appointment.AppointmentTime,
						appointment.Status, appointment.Notes)
					return c.Status(500).JSON(fiber.Map{"error": "Failed to create appointment: " + err.Error()})
				}

				log.Printf("Appointment created successfully: %v", appointment.ID)
			}
		}
	}

	// 處理付款記錄（只保存到 income 表，不再保存到 ExtraFields）
	// 首先獲取所有現有的 income 記錄
	var existingIncomes []models.Income
	database.DB.Where("tenant_id = ? AND reference_type = ? AND reference_id = ?", tenantID, "service_order", serviceOrder.ID).Find(&existingIncomes)
	existingIncomeIDs := make(map[uuid.UUID]bool)
	for _, inc := range existingIncomes {
		existingIncomeIDs[inc.ID] = true
	}

	// 處理新的付款記錄（即使為空也要處理，以便刪除舊記錄）
	if len(req.PaymentRecords) > 0 {
		// 記錄哪些 income_id 在新的付款記錄中
		keptIncomeIDs := make(map[uuid.UUID]bool)

		// 為新的付款記錄創建或更新 income 記錄
		for _, record := range req.PaymentRecords {
			if record == nil {
				continue
			}

			// 處理金額（支持多種類型）
			var amount float64
			amountRaw, exists := record["amount"]
			if !exists {
				continue
			}

			switch v := amountRaw.(type) {
			case float64:
				amount = v
			case float32:
				amount = float64(v)
			case int:
				amount = float64(v)
			case int32:
				amount = float64(v)
			case int64:
				amount = float64(v)
			case string:
				if parsed, err := strconv.ParseFloat(v, 64); err == nil {
					amount = parsed
				}
			default:
				if strVal := fmt.Sprintf("%v", v); strVal != "" {
					if parsed, err := strconv.ParseFloat(strVal, 64); err == nil {
						amount = parsed
					}
				}
			}

			if amount > 0 {
				// 處理 income_id
				var incomeIDStr string
				if idVal, exists := record["income_id"]; exists {
					if idVal != nil {
						if idStr, ok := idVal.(string); ok {
							incomeIDStr = idStr
						}
					}
				}

				// 如果已經有 income_id，更新現有的 income 記錄
				if incomeIDStr != "" {
					if incomeID, err := uuid.Parse(incomeIDStr); err == nil {
						keptIncomeIDs[incomeID] = true
						var existingIncome models.Income
						if err := database.DB.Where("id = ? AND tenant_id = ?", incomeID, tenantID).First(&existingIncome).Error; err == nil {
							paymentDateStr, _ := record["payment_date"].(string)
							paymentMethod, _ := record["payment_method"].(string)
							paymentMethodID, _ := record["payment_method_id"].(string)
							bankAccountIDStr, _ := record["bank_account_id"].(string)
							notes, _ := record["notes"].(string)
							invoiceNumber, _ := record["invoice_number"].(string)
							referenceNumber, _ := record["reference_number"].(string)

							var incomeDate time.Time
							if paymentDateStr != "" {
								if parsedDate, err := utils.ParseDateInTenantTimezone(tenantID, paymentDateStr); err == nil {
									incomeDate = parsedDate
								} else {
									incomeDate = serviceOrder.ServiceDate
								}
							} else {
								incomeDate = serviceOrder.ServiceDate
							}

							var bankAccountID *uuid.UUID
							if bankAccountIDStr != "" {
								if parsedID, err := uuid.Parse(bankAccountIDStr); err == nil {
									bankAccountID = &parsedID
								}
							}

							incomeExtraFields := make(map[string]interface{})
							if existingIncome.ExtraFields != nil {
								incomeExtraFields = map[string]interface{}(existingIncome.ExtraFields)
							}
							if paymentMethodID != "" {
								incomeExtraFields["payment_method_id"] = paymentMethodID
							}
							if invoiceNumber != "" {
								incomeExtraFields["invoice_number"] = invoiceNumber
							}
							if referenceNumber != "" {
								incomeExtraFields["reference_number"] = referenceNumber
							}
							incomeExtraFields["service_order_id"] = serviceOrder.ID.String()

							existingIncome.Amount = amount
							existingIncome.IncomeDate = incomeDate
							existingIncome.PaymentMethod = paymentMethod
							existingIncome.BankAccountID = bankAccountID
							existingIncome.Notes = notes
							existingIncome.ExtraFields = models.JSONB(incomeExtraFields)
							existingIncome.UpdatedBy = &userID
							existingIncome.UpdatedAt = time.Now()

							database.DB.Save(&existingIncome)
						}
					}
				} else {
					// 如果沒有 income_id，創建新的 income 記錄
					paymentDateStr, _ := record["payment_date"].(string)
					paymentMethod, _ := record["payment_method"].(string)
					paymentMethodID, _ := record["payment_method_id"].(string)
					bankAccountIDStr, _ := record["bank_account_id"].(string)
					notes, _ := record["notes"].(string)
					invoiceNumber, _ := record["invoice_number"].(string)
					referenceNumber, _ := record["reference_number"].(string)

					// 自動生成發票號碼（並預留）
					if invoiceNumber == "" {
						if n, err := reserveNextNumber(tenantID, "invoice_number", "service-orders"); err == nil {
							invoiceNumber = n
						}
					} else {
						_, _ = reserveSpecificNumber(tenantID, "invoice_number", invoiceNumber, "service-orders")
					}

					var incomeDate time.Time
					if paymentDateStr != "" {
						if parsedDate, err := utils.ParseDateInTenantTimezone(tenantID, paymentDateStr); err == nil {
							incomeDate = parsedDate
						} else {
							incomeDate = serviceOrder.ServiceDate
						}
					} else {
						incomeDate = serviceOrder.ServiceDate
					}

					var bankAccountID *uuid.UUID
					if bankAccountIDStr != "" {
						if parsedID, err := uuid.Parse(bankAccountIDStr); err == nil {
							bankAccountID = &parsedID
						}
					}

					description := fmt.Sprintf("服務單 %s 的付款", serviceOrder.OrderNumber)
					if invoiceNumber != "" {
						description = invoiceNumber
					}

					relatedUserID := &userID
					if serviceOrder.SalespersonID != nil {
						relatedUserID = serviceOrder.SalespersonID
					}
					income := models.Income{
						TenantID:      tenantID,
						RelatedUserID: relatedUserID,
						IncomeType:    "service_order",
						ReferenceID:   &serviceOrder.ID,
						ReferenceType: "service_order",
						Category:      "service_order",
						Description:   description,
						Amount:        amount,
						IncomeDate:    incomeDate,
						PaymentMethod: paymentMethod,
						BankAccountID: bankAccountID,
						Status:        "confirmed",
						Notes:         notes,
						CreatedBy:     &userID,
						UpdatedBy:     &userID,
						CreatedAt:     time.Now(),
						UpdatedAt:     time.Now(),
						ExtraFields: models.JSONB(map[string]interface{}{
							"payment_method_id": paymentMethodID,
							"invoice_number":    invoiceNumber,
							"reference_number":  referenceNumber,
							"service_order_id":  serviceOrder.ID.String(),
						}),
					}

					if err := database.DB.Create(&income).Error; err == nil {
						keptIncomeIDs[income.ID] = true
					}
				}
			}
		}

		// 刪除不在新付款記錄中的 income 記錄
		for incomeID := range existingIncomeIDs {
			if !keptIncomeIDs[incomeID] {
				database.DB.Where("id = ? AND tenant_id = ?", incomeID, tenantID).Delete(&models.Income{})
			}
		}
	} else {
		// 檢查是否自動生成付款記錄
		auto := getDocumentAutoSettingsForTenant(tenantID)
		shouldAutoGeneratePayment := auto.AutoGenerateServiceOrderPayment && serviceOrder.TotalAmount > 0

		if shouldAutoGeneratePayment {
			// 如果沒有手動付款記錄且設定開啟，自動生成一個付款記錄
			defPayMethod := defaultReceivingPaymentMethodCode(tenantID)
			defBankAccID := defaultReceivingBankAccountID(tenantID)

			// 自動生成發票號碼（並預留）
			invoiceNumber := ""
			if n, err := reserveNextNumber(tenantID, "invoice_number", "service-orders"); err == nil {
				invoiceNumber = n
			}

			// 構建標題（只使用 invoice_number）
			description := fmt.Sprintf("服務單 %s 的付款", serviceOrder.OrderNumber)
			if invoiceNumber != "" {
				description = invoiceNumber
			}

			// 創建 income 記錄
			relatedUserID := &userID
			if serviceOrder.SalespersonID != nil {
				relatedUserID = serviceOrder.SalespersonID
			}
			income := models.Income{
				TenantID:      tenantID,
				RelatedUserID: relatedUserID,
				IncomeType:    "service_order",
				ReferenceID:   &serviceOrder.ID,
				ReferenceType: "service_order",
				Category:      "service_order",
				Description:   description,
				Amount:        serviceOrder.TotalAmount,
				IncomeDate:    serviceOrder.ServiceDate,
				PaymentMethod: defPayMethod,
				BankAccountID: defBankAccID,
				Status:        "confirmed",
				Notes:         "",
				CreatedBy:     &userID,
				UpdatedBy:     &userID,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				ExtraFields: models.JSONB(map[string]interface{}{
					"invoice_number":   invoiceNumber,
					"service_order_id": serviceOrder.ID.String(),
				}),
			}

			if err := database.DB.Create(&income).Error; err != nil {
				log.Printf("Failed to auto-generate payment record: %v", err)
			}
		} else {
			// 如果沒有付款記錄且不自動生成，刪除所有相關的 income 記錄
			database.DB.Where("tenant_id = ? AND reference_type = ? AND reference_id = ?", tenantID, "service_order", serviceOrder.ID).Delete(&models.Income{})
		}
	}

	tx.Commit()

	// 同步 Invoice（付款記錄已更新完畢）
	syncOrderInvoice(database.DB, tenantID, "service_order", serviceOrder.ID, &userID)

	// ============================================
	// 編輯服務單時積分調整：如果積分增加，補發差值到客戶積分
	// ============================================
	// 重新計算積分（如果有客戶且積分設置存在）
	var newPointsEarned int = 0
	var pointSetting models.PointSetting
	hasPointSetting := false
	if serviceOrder.CustomerID != nil {
		if err := database.DB.Where("tenant_id = ?", tenantID).First(&pointSetting).Error; err == nil {
			hasPointSetting = true
			// 檢查是否開啟服務單消費獲得積分
			if pointSetting.EnableServiceOrderEarnPoints && pointSetting.PointsPerDollar > 0 {
				// 計算實際支付金額（總金額 - 折扣）
				actualAmount := serviceOrder.TotalAmount - serviceOrder.CouponDiscount - serviceOrder.PointsDiscount
				if actualAmount > 0 {
					newPointsEarned = int(actualAmount * pointSetting.PointsPerDollar)
				}
			}
		}
	}

	// 更新服務單的 PointsEarned
	if newPointsEarned != serviceOrder.PointsEarned {
		serviceOrder.PointsEarned = newPointsEarned
		database.DB.Model(&serviceOrder).Update("points_earned", newPointsEarned)
	}

	// 如果開啟了積分調整功能，且積分增加，補發差值
	if hasPointSetting && pointSetting.EnablePointsAdjustmentOnEdit && serviceOrder.CustomerID != nil {
		pointsDiff := newPointsEarned - oldPointsEarned
		// 只處理積分增加的情況（差值為正）
		if pointsDiff > 0 {
			// 檢查客戶是否變更
			sameCustomer := oldCustomerID != nil && *oldCustomerID == *serviceOrder.CustomerID
			if sameCustomer {
				// 創建積分記錄
				pointNow := time.Now()
				point := models.Point{
					TenantID:    tenantID,
					CustomerID:  *serviceOrder.CustomerID,
					Points:      pointsDiff,
					PointsType:  "adjustment",
					SourceType:  "service_order_edit",
					SourceID:    &serviceOrder.ID,
					Description: fmt.Sprintf("編輯服務單 %s 補發積分差值 (+%d)", serviceOrder.OrderNumber, pointsDiff),
					Status:      "active",
					CreatedAt:   pointNow,
					UpdatedAt:   pointNow,
				}
				if err := database.DB.Create(&point).Error; err != nil {
					log.Printf("Failed to create point adjustment record for service order edit: %v", err)
				} else {
					// 更新客戶總積分
					var customer models.Customer
					if err := database.DB.Where("id = ?", *serviceOrder.CustomerID).First(&customer).Error; err == nil {
						customer.TotalPoints += pointsDiff
						database.DB.Save(&customer)
						log.Printf("Service order edit points adjustment: order=%s, customer=%s, diff=+%d", serviceOrder.OrderNumber, customer.Name, pointsDiff)
					}
				}
			}
		}
	}

	// ============================================
	// 退款單（refund_notes）
	// - 退款金額 -> expenses（category=refund）
	// - 取回傭金 -> incomes（reference_type=service_order_refund_commission）
	// 以 refund_notes 內的 *_id 做冪等，避免重複生成
	// ============================================
	if req.ExtraFields != nil {
		if refundNotes, exists := req.ExtraFields["refund_notes"]; exists {
			if notesList, ok := refundNotes.([]interface{}); ok && len(notesList) > 0 {
				// 服務單服務數量 map（用於限制：退的總量不能超過）
				orderQtyByService := map[string]float64{}
				for _, it := range req.ServiceOrderItems {
					if it.ServiceID == nil || *it.ServiceID == uuid.Nil {
						continue
					}
					orderQtyByService[it.ServiceID.String()] += it.Quantity
				}

				// 聚合退款數量
				refundQtyByService := map[string]float64{}
				for _, n := range notesList {
					note, ok := n.(map[string]interface{})
					if !ok {
						continue
					}
					itemsRaw, _ := note["items"].([]interface{})
					for _, ir := range itemsRaw {
						im, ok := ir.(map[string]interface{})
						if !ok {
							continue
						}
						sid, _ := im["service_id"].(string)
						if sid == "" {
							continue
						}
						var qty float64
						switch v := im["quantity"].(type) {
						case float64:
							qty = v
						case float32:
							qty = float64(v)
						case int:
							qty = float64(v)
						case int64:
							qty = float64(v)
						case string:
							if parsed, err := strconv.ParseFloat(v, 64); err == nil {
								qty = parsed
							}
						default:
							if s := fmt.Sprintf("%v", v); s != "" {
								if parsed, err := strconv.ParseFloat(s, 64); err == nil {
									qty = parsed
								}
							}
						}
						if qty < 0 {
							qty = 0
						}
						refundQtyByService[sid] += qty
					}
				}

				for sid, rq := range refundQtyByService {
					if oq, ok := orderQtyByService[sid]; ok {
						if rq > oq+1e-9 {
							return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("退款數量超過服務單數量：service_id=%s (退款 %.2f > 服務單 %.2f)", sid, rq, oq)})
						}
					} else if rq > 0 {
						return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("退款服務不在服務單中：service_id=%s", sid)})
					}
				}

				// 取客戶名（作為 vendor）
				vendor := ""
				if serviceOrder.CustomerID != nil {
					var customer models.Customer
					if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, *serviceOrder.CustomerID).First(&customer).Error; err == nil {
						vendor = customer.Name
					}
				}
				if vendor == "" {
					vendor = "客戶"
				}

				// 查佣金支出（用於計算取回傭金）
				commissionExpenseAmount := 0.0
				var commissionExpense models.Expense
				if err := database.DB.Where("tenant_id = ? AND reference_type = ? AND reference_id = ? AND category = ?", tenantID, "service_order", serviceOrder.ID, "commission").
					Order("created_at DESC").First(&commissionExpense).Error; err == nil {
					commissionExpenseAmount = commissionExpense.Amount
				} else if serviceOrder.CommissionAmount > 0 {
					commissionExpenseAmount = serviceOrder.CommissionAmount
				}

				// 逐張退款單處理：生成 Expense / Income（冪等）
				// 注意：無論是否生成 expense/income，都要保存 refund_notes 到 extra_fields
				for noteIndex, n := range notesList {
					note, ok := n.(map[string]interface{})
					if !ok {
						continue
					}

					refundNumber, _ := note["refund_number"].(string)
					refundDateStr, _ := note["refund_date"].(string)
					if refundDateStr == "" {
						refundDateStr = serviceOrder.ServiceDate.Format("2006-01-02")
					}
					refundDate, err := utils.ParseDateInTenantTimezone(tenantID, refundDateStr)
					if err != nil {
						refundDate = utils.NowInTenantTimezone(tenantID)
					}

					returnCommission := false
					if b, ok := note["return_commission"].(bool); ok {
						returnCommission = b
					} else if s, ok := note["return_commission"].(string); ok && (s == "true" || s == "1") {
						returnCommission = true
					}

					// items total
					itemsTotal := 0.0
					itemsRaw, _ := note["items"].([]interface{})
					for _, ir := range itemsRaw {
						im, ok := ir.(map[string]interface{})
						if !ok {
							continue
						}
						var qty float64
						switch v := im["quantity"].(type) {
						case float64:
							qty = v
						case float32:
							qty = float64(v)
						case int:
							qty = float64(v)
						case int64:
							qty = float64(v)
						case string:
							if parsed, err := strconv.ParseFloat(v, 64); err == nil {
								qty = parsed
							}
						default:
							if s := fmt.Sprintf("%v", v); s != "" {
								if parsed, err := strconv.ParseFloat(s, 64); err == nil {
									qty = parsed
								}
							}
						}
						if qty < 0 {
							qty = 0
						}

						var unit float64
						switch v := im["unit_price"].(type) {
						case float64:
							unit = v
						case float32:
							unit = float64(v)
						case int:
							unit = float64(v)
						case int64:
							unit = float64(v)
						case string:
							if parsed, err := strconv.ParseFloat(v, 64); err == nil {
								unit = parsed
							}
						default:
							if s := fmt.Sprintf("%v", v); s != "" {
								if parsed, err := strconv.ParseFloat(s, 64); err == nil {
									unit = parsed
								}
							}
						}
						if unit < 0 {
							unit = 0
						}
						itemsTotal += qty * unit
					}

					extraAmount := 0.0
					switch v := note["extra_amount"].(type) {
					case float64:
						extraAmount = v
					case float32:
						extraAmount = float64(v)
					case int:
						extraAmount = float64(v)
					case int64:
						extraAmount = float64(v)
					case string:
						if parsed, err := strconv.ParseFloat(v, 64); err == nil {
							extraAmount = parsed
						}
					default:
						if s := fmt.Sprintf("%v", v); s != "" {
							if parsed, err := strconv.ParseFloat(s, 64); err == nil {
								extraAmount = parsed
							}
						}
					}
					if extraAmount < 0 {
						extraAmount = 0
					}

					totalRefund := itemsTotal + extraAmount
					if totalRefund < 0 {
						totalRefund = 0
					}

					// Expense（退款金額 -> 支出）
					expenseIDStr, _ := note["expense_id"].(string)
					if totalRefund > 0 && expenseIDStr == "" {
						// 退款支出：使用「系統預設支出付款方法 / 付款賬戶」
						paymentMethod := "cash"
						var pm models.PaymentMethod
						if err := database.DB.Where("tenant_id = ? AND status = ? AND is_default_expense = ?", tenantID, "active", true).First(&pm).Error; err == nil && pm.Code != "" {
							paymentMethod = pm.Code
						} else if err := database.DB.Where("tenant_id = ? AND status = ? AND is_default = ?", tenantID, "active", true).First(&pm).Error; err == nil && pm.Code != "" {
							paymentMethod = pm.Code
						}
						var bankAccountID *uuid.UUID = nil
						{
							var ba models.BankAccount
							if err := database.DB.Where("tenant_id = ? AND status = ? AND is_default_payment = ?", tenantID, "active", true).First(&ba).Error; err == nil && ba.ID != uuid.Nil {
								id := ba.ID
								bankAccountID = &id
							}
						}

						description := fmt.Sprintf("退款單 %s（服務單 %s）", refundNumber, serviceOrder.OrderNumber)
						exp := models.Expense{
							TenantID:      tenantID,
							RelatedUserID: serviceOrder.SalespersonID,
							ExpenseType:   "other",
							ReferenceID:   &serviceOrder.ID,
							ReferenceType: "service_order_refund",
							Category:      "refund",
							Description:   description,
							Amount:        totalRefund,
							ExpenseDate:   refundDate,
							PaymentMethod: paymentMethod,
							BankAccountID: bankAccountID,
							Vendor:        vendor,
							Status:        "confirmed",
							Notes:         fmt.Sprintf("refund_note_index=%d", noteIndex),
							CreatedBy:     &userID,
							UpdatedBy:     &userID,
							CreatedAt:     time.Now(),
							UpdatedAt:     time.Now(),
							ExtraFields: models.JSONB(map[string]interface{}{
								"refund_number":    refundNumber,
								"service_order_id": serviceOrder.ID.String(),
								"note_index":       noteIndex,
							}),
						}
						if exp.RelatedUserID == nil {
							exp.RelatedUserID = &userID
						}
						if err := database.DB.Create(&exp).Error; err == nil {
							note["expense_id"] = exp.ID.String()
						}
					}

					// Income（取回傭金 -> 收入記錄）
					if returnCommission {
						incomeIDStr, _ := note["income_id"].(string)
						if incomeIDStr == "" && commissionExpenseAmount > 0 {
							ratio := 0.0
							if serviceOrder.TotalAmount > 0 {
								ratio = itemsTotal / serviceOrder.TotalAmount
							}
							if ratio < 0 {
								ratio = 0
							}
							if ratio > 1 {
								ratio = 1
							}
							amt := commissionExpenseAmount * ratio
							if amt > 0 {
								desc := fmt.Sprintf("取回傭金（退款單 %s / 服務單 %s）", refundNumber, serviceOrder.OrderNumber)
								relatedUserID := &userID
								if serviceOrder.SalespersonID != nil {
									relatedUserID = serviceOrder.SalespersonID
								}
								inc := models.Income{
									TenantID:      tenantID,
									RelatedUserID: relatedUserID,
									IncomeType:    "other",
									ReferenceID:   &serviceOrder.ID,
									ReferenceType: "service_order_refund_commission",
									Category:      "refund_commission",
									Description:   desc,
									Amount:        amt,
									IncomeDate:    refundDate,
									PaymentMethod: "cash",
									Status:        "confirmed",
									Notes:         fmt.Sprintf("refund_note_index=%d", noteIndex),
									CreatedBy:     &userID,
									UpdatedBy:     &userID,
									CreatedAt:     time.Now(),
									UpdatedAt:     time.Now(),
									ExtraFields: models.JSONB(map[string]interface{}{
										"refund_number":    refundNumber,
										"service_order_id": serviceOrder.ID.String(),
										"note_index":       noteIndex,
										"ratio":            ratio,
									}),
								}
								if err := database.DB.Create(&inc).Error; err == nil {
									note["income_id"] = inc.ID.String()
								}
							}
						}
					}
				}

				// 無論是否有 changed，都要保存 refund_notes 到 extra_fields
				// 因為即使沒有生成 expense/income，退款單數據本身也需要保存
				req.ExtraFields["refund_notes"] = notesList
				serviceOrder.ExtraFields = models.JSONB(req.ExtraFields)
				database.DB.Model(&serviceOrder).Update("extra_fields", serviceOrder.ExtraFields)
			}
		}
	}

	// 同步已生成的「相關支出」（佣金 / 稅）金額（best-effort）
	// - 若開啟自動生成：保存後直接生成/同步「相關支出」（佣金 / 稅）
	// - 否則：只同步已存在支出金額（不自動生成）
	autoExpenses := getDocumentAutoSettingsForTenant(tenantID)
	if autoExpenses.AutoGenerateServiceOrderCommission || autoExpenses.AutoGenerateServiceTaxes {
		// IMPORTANT: 同步執行，確保保存完成時「相關支出」已生成/同步，前端才會在保存後跳回列表
		ensureAndSyncServiceOrderRelatedExpenses(tenantID, serviceOrder.ID, userID, autoExpenses)
	} else {
		// 同步執行（保持一致，避免保存後仍在背景更新）
		syncServiceOrderRelatedExpenses(tenantID, serviceOrder.ID)
	}

	// 狀態轉 completed 時，發放介紹人積分
	if oldStatus != "completed" && serviceOrder.Status == "completed" && serviceOrder.CustomerID != nil {
		refCode := serviceOrder.ReferralCode
		if refCode == "" {
			var customer models.Customer
			if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, *serviceOrder.CustomerID).First(&customer).Error; err == nil {
				refCode = customer.ReferralCode
			}
		}
		if refCode != "" {
			// 檢查是否開啟服務單介紹人獎勵積分
			var pointSetting models.PointSetting
			if err := database.DB.Where("tenant_id = ?", tenantID).First(&pointSetting).Error; err == nil {
				if pointSetting.EnableServiceOrderReferralBonus {
					processReferralBonus(tenantID, refCode, *serviceOrder.CustomerID, "服務單", serviceOrder.ID, serviceOrder.OrderNumber, serviceOrder.PointsEarned)
				}
			} else {
				// 如果沒有設定，預設開啟（向後兼容）
				processReferralBonus(tenantID, refCode, *serviceOrder.CustomerID, "服務單", serviceOrder.ID, serviceOrder.OrderNumber, serviceOrder.PointsEarned)
			}
		}
	}

	// 狀態轉 confirmed 時寄送服務單確認 email（有 email 即寄）
	if oldStatus != "confirmed" && serviceOrder.Status == "confirmed" {
		emailTo := strings.TrimSpace(serviceOrder.ContactEmail)
		customerName := strings.TrimSpace(serviceOrder.ContactName)
		customerID := uuid.Nil
		if serviceOrder.CustomerID != nil {
			customerID = *serviceOrder.CustomerID
			var customer models.Customer
			if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, *serviceOrder.CustomerID).First(&customer).Error; err == nil {
				if emailTo == "" {
					emailTo = strings.TrimSpace(customer.Email)
				}
				if customerName == "" {
					customerName = strings.TrimSpace(customer.Name)
				}
			}
		}
		if emailTo != "" {
			if customerName == "" {
				customerName = "客戶"
			}
			var tenant models.Tenant
			if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
				serviceDate := serviceOrder.ServiceDate.Format("2006-01-02")
				if err := email.EnqueueOrderConfirmationEmail(
					tenantID,
					tenant.Subdomain,
					customerID,
					emailTo,
					customerName,
					serviceOrder.OrderNumber,
					serviceDate,
					serviceOrder.TotalAmount,
					"service_order",
				); err != nil {
					log.Printf("Failed to enqueue service order confirmation email: %v", err)
				}
			}
		}
	}

	// 重新載入關聯數據
	database.DB.Where("id = ?", serviceOrder.ID).
		Preload("Customer").Preload("ServiceOrderItems.Service").Preload("ServiceOrderItems.Staff").
		Preload("Salesperson").Preload("Store").Preload("Labels").Preload("Appointments").
		First(&serviceOrder)

	// Auto-refresh business goals related to service orders
	go RefreshActiveBusinessGoals(tenantID, []string{"service_order_count"})

	return c.JSON(serviceOrder)
}

func DeleteServiceOrder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.ServiceOrder{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Auto-refresh business goals related to service orders
	go RefreshActiveBusinessGoals(tenantID, []string{"service_order_count"})

	return c.JSON(fiber.Map{"message": "Service order deleted"})
}

// ============================================
// 服務員 (ServiceStaff) CRUD
// ============================================

func GetServiceStaffs(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var staffs []models.ServiceStaff
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR phone ILIKE ? OR employee_number ILIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	// 按服務種類 ID 篩選
	if serviceTypeID := c.Query("service_type_id"); serviceTypeID != "" {
		query = query.Where("service_type_id = ?", serviceTypeID)
	}
	// 按服務種類代碼篩選（用於 URL 連結，如 ?service_type_code=course）
	if serviceTypeCode := c.Query("service_type_code"); serviceTypeCode != "" {
		query = query.Where("service_type_id IN (SELECT id FROM service_types WHERE code = ? AND tenant_id = ?)", serviceTypeCode, tenantID)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.ServiceStaff{}).Count(&total)

	query = query.Preload("User").Preload("ServiceType")
	if err := query.Offset(offset).Limit(limit).Find(&staffs).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  staffs,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetServiceStaff(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var staff models.ServiceStaff

	query := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id)
	if err := query.Preload("User").First(&staff).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Service staff not found"})
	}

	return c.JSON(staff)
}

func CreateServiceStaff(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		Name           string                 `json:"name"`
		Phone          string                 `json:"phone"`
		EmployeeNumber *string                `json:"employee_number"`
		Specialization string                 `json:"specialization"`
		HourlyRate     float64                `json:"hourly_rate"`
		Status         string                 `json:"status"`
		ExtraFields    map[string]interface{} `json:"extra_fields"`
		UserID         *uuid.UUID             `json:"user_id"`         // 可選
		ServiceTypeID  *uuid.UUID             `json:"service_type_id"` // 所屬服務類別
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}

	staff := models.ServiceStaff{
		TenantID:       tenantID,
		Name:           req.Name,
		Phone:          req.Phone,
		UserID:         req.UserID,
		ServiceTypeID:  req.ServiceTypeID,
		EmployeeNumber: req.EmployeeNumber,
		Specialization: req.Specialization,
		HourlyRate:     req.HourlyRate,
		Status:         req.Status,
		ExtraFields:    models.JSONB(req.ExtraFields),
	}

	if err := database.DB.Create(&staff).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 預載入關聯信息以便返回
	database.DB.Preload("User").Preload("ServiceType").First(&staff, staff.ID)

	return c.Status(201).JSON(staff)
}

func UpdateServiceStaff(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var staff models.ServiceStaff

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&staff).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Service staff not found"})
	}

	var req struct {
		Name           string                 `json:"name"`
		Phone          string                 `json:"phone"`
		EmployeeNumber *string                `json:"employee_number"`
		Specialization string                 `json:"specialization"`
		HourlyRate     float64                `json:"hourly_rate"`
		Status         string                 `json:"status"`
		ExtraFields    map[string]interface{} `json:"extra_fields"`
		UserID         *uuid.UUID             `json:"user_id"`
		ServiceTypeID  *uuid.UUID             `json:"service_type_id"` // 所屬服務類別
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}

	// 更新字段
	staff.Name = req.Name
	staff.Phone = req.Phone
	staff.EmployeeNumber = req.EmployeeNumber
	staff.ServiceTypeID = req.ServiceTypeID
	staff.Specialization = req.Specialization
	staff.HourlyRate = req.HourlyRate
	staff.Status = req.Status
	staff.ExtraFields = models.JSONB(req.ExtraFields)
	if req.UserID != nil {
		staff.UserID = req.UserID
	}
	staff.TenantID = tenantID

	if err := database.DB.Save(&staff).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(staff)
}

func DeleteServiceStaff(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.ServiceStaff{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Service staff deleted"})
}

// ============================================
// 房間設備 (RoomEquipment) CRUD
// ============================================

func GetRoomEquipments(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	roomName := c.Query("room_name")

	var equipments []models.RoomEquipment
	query := database.DB.Where("tenant_id = ?", tenantID)

	if roomName != "" {
		query = query.Where("room_name = ?", roomName)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.RoomEquipment{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Find(&equipments).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  equipments,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetRoomEquipment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var equipment models.RoomEquipment

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&equipment).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Room equipment not found"})
	}

	return c.JSON(equipment)
}

func CreateRoomEquipment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var equipment models.RoomEquipment
	if err := c.BodyParser(&equipment); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	equipment.TenantID = tenantID

	if err := database.DB.Create(&equipment).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(equipment)
}

func UpdateRoomEquipment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var equipment models.RoomEquipment

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&equipment).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Room equipment not found"})
	}

	if err := c.BodyParser(&equipment); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	equipment.TenantID = tenantID

	if err := database.DB.Save(&equipment).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(equipment)
}

func DeleteRoomEquipment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.RoomEquipment{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Room equipment deleted"})
}

// ============================================
// 房間 (Room) CRUD
// ============================================

func GetRooms(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var rooms []models.Room
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR code ILIKE ?", "%"+search+"%", "%"+search+"%")
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Room{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Order("name ASC").Find(&rooms).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 讓前端可以直接用 image_url（像 products 一樣），實際存於 extra_fields.image_url
	for i := range rooms {
		if rooms[i].ExtraFields != nil {
			if v, ok := rooms[i].ExtraFields["image_url"]; ok {
				if s, ok := v.(string); ok && s != "" {
					rooms[i].ImageURL = &s
				}
			}
		}
	}

	return c.JSON(fiber.Map{
		"data":  rooms,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetRoom(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var room models.Room

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&room).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Room not found"})
	}

	if room.ExtraFields != nil {
		if v, ok := room.ExtraFields["image_url"]; ok {
			if s, ok := v.(string); ok && s != "" {
				room.ImageURL = &s
			}
		}
	}

	return c.JSON(room)
}

func CreateRoom(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var room models.Room
	if err := c.BodyParser(&room); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	room.TenantID = tenantID

	// 圖片：前端傳 image_url，實際存到 extra_fields.image_url（不需要 DB 遷移）
	if room.ExtraFields == nil {
		room.ExtraFields = models.JSONB{}
	}
	if room.ImageURL != nil && *room.ImageURL != "" {
		room.ExtraFields["image_url"] = *room.ImageURL
	} else {
		delete(room.ExtraFields, "image_url")
	}

	// 如果沒有提供編號，自動生成
	if room.Code == nil || *room.Code == "" {
		var count int64
		prefix := "RM-"

		database.DB.Model(&models.Room{}).Where("tenant_id = ? AND code LIKE ?", tenantID, prefix+"%").Count(&count)

		sequence := count + 1
		code := prefix + fmt.Sprintf("%05d", sequence)
		room.Code = &code
	}

	if err := database.DB.Create(&room).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 回傳時也補 image_url
	if v, ok := room.ExtraFields["image_url"]; ok {
		if s, ok := v.(string); ok && s != "" {
			room.ImageURL = &s
		}
	}

	return c.Status(201).JSON(room)
}

func UpdateRoom(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var room models.Room

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&room).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Room not found"})
	}

	if err := c.BodyParser(&room); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	room.TenantID = tenantID

	if room.ExtraFields == nil {
		room.ExtraFields = models.JSONB{}
	}
	if room.ImageURL != nil && *room.ImageURL != "" {
		room.ExtraFields["image_url"] = *room.ImageURL
	} else {
		delete(room.ExtraFields, "image_url")
	}

	if err := database.DB.Save(&room).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	if v, ok := room.ExtraFields["image_url"]; ok {
		if s, ok := v.(string); ok && s != "" {
			room.ImageURL = &s
		}
	}

	return c.JSON(room)
}

func DeleteRoom(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.Room{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Room deleted"})
}

// ============================================
// 設備 (Equipment) CRUD
// ============================================

func GetEquipments(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var equipments []models.Equipment
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR code ILIKE ?", "%"+search+"%", "%"+search+"%")
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if equipmentType := c.Query("equipment_type"); equipmentType != "" {
		query = query.Where("equipment_type = ?", equipmentType)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Equipment{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Order("name ASC").Find(&equipments).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	for i := range equipments {
		if equipments[i].ExtraFields != nil {
			if v, ok := equipments[i].ExtraFields["image_url"]; ok {
				if s, ok := v.(string); ok && s != "" {
					equipments[i].ImageURL = &s
				}
			}
		}
	}

	return c.JSON(fiber.Map{
		"data":  equipments,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetEquipment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var equipment models.Equipment

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&equipment).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Equipment not found"})
	}

	if equipment.ExtraFields != nil {
		if v, ok := equipment.ExtraFields["image_url"]; ok {
			if s, ok := v.(string); ok && s != "" {
				equipment.ImageURL = &s
			}
		}
	}

	return c.JSON(equipment)
}

func CreateEquipment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var equipment models.Equipment
	if err := c.BodyParser(&equipment); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	equipment.TenantID = tenantID

	if equipment.ExtraFields == nil {
		equipment.ExtraFields = models.JSONB{}
	}
	if equipment.ImageURL != nil && *equipment.ImageURL != "" {
		equipment.ExtraFields["image_url"] = *equipment.ImageURL
	} else {
		delete(equipment.ExtraFields, "image_url")
	}

	// 如果沒有提供編號，自動生成
	if equipment.Code == nil || *equipment.Code == "" {
		var count int64
		prefix := "EQ-"

		database.DB.Model(&models.Equipment{}).Where("tenant_id = ? AND code LIKE ?", tenantID, prefix+"%").Count(&count)

		sequence := count + 1
		code := prefix + fmt.Sprintf("%05d", sequence)
		equipment.Code = &code
	}

	if err := database.DB.Create(&equipment).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	if v, ok := equipment.ExtraFields["image_url"]; ok {
		if s, ok := v.(string); ok && s != "" {
			equipment.ImageURL = &s
		}
	}

	return c.Status(201).JSON(equipment)
}

func UpdateEquipment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var equipment models.Equipment

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&equipment).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Equipment not found"})
	}

	if err := c.BodyParser(&equipment); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	equipment.TenantID = tenantID

	if equipment.ExtraFields == nil {
		equipment.ExtraFields = models.JSONB{}
	}
	if equipment.ImageURL != nil && *equipment.ImageURL != "" {
		equipment.ExtraFields["image_url"] = *equipment.ImageURL
	} else {
		delete(equipment.ExtraFields, "image_url")
	}

	if err := database.DB.Save(&equipment).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	if v, ok := equipment.ExtraFields["image_url"]; ok {
		if s, ok := v.(string); ok && s != "" {
			equipment.ImageURL = &s
		}
	}

	return c.JSON(equipment)
}

func DeleteEquipment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.Equipment{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Equipment deleted"})
}

// ============================================
// 車輛 (Vehicle) CRUD
// ============================================

func GetVehicles(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var vehicles []models.Vehicle
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR code ILIKE ? OR license_plate ILIKE ?", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if vehicleType := c.Query("vehicle_type"); vehicleType != "" {
		query = query.Where("vehicle_type = ?", vehicleType)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Vehicle{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Order("name ASC").Find(&vehicles).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	for i := range vehicles {
		if vehicles[i].ExtraFields != nil {
			if v, ok := vehicles[i].ExtraFields["image_url"]; ok {
				if s, ok := v.(string); ok && s != "" {
					vehicles[i].ImageURL = &s
				}
			}
		}
	}

	return c.JSON(fiber.Map{
		"data":  vehicles,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetVehicle(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var vehicle models.Vehicle

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&vehicle).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Vehicle not found"})
	}

	if vehicle.ExtraFields != nil {
		if v, ok := vehicle.ExtraFields["image_url"]; ok {
			if s, ok := v.(string); ok && s != "" {
				vehicle.ImageURL = &s
			}
		}
	}

	return c.JSON(vehicle)
}

func CreateVehicle(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var vehicle models.Vehicle
	if err := c.BodyParser(&vehicle); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	vehicle.TenantID = tenantID

	if vehicle.ExtraFields == nil {
		vehicle.ExtraFields = models.JSONB{}
	}
	if vehicle.ImageURL != nil && *vehicle.ImageURL != "" {
		vehicle.ExtraFields["image_url"] = *vehicle.ImageURL
	} else {
		delete(vehicle.ExtraFields, "image_url")
	}

	// 如果沒有提供編號，自動生成
	if vehicle.Code == nil || *vehicle.Code == "" {
		var count int64
		prefix := "VEH-"

		database.DB.Model(&models.Vehicle{}).Where("tenant_id = ? AND code LIKE ?", tenantID, prefix+"%").Count(&count)

		sequence := count + 1
		code := prefix + fmt.Sprintf("%05d", sequence)
		vehicle.Code = &code
	}

	if err := database.DB.Create(&vehicle).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	if v, ok := vehicle.ExtraFields["image_url"]; ok {
		if s, ok := v.(string); ok && s != "" {
			vehicle.ImageURL = &s
		}
	}

	return c.Status(201).JSON(vehicle)
}

func UpdateVehicle(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var vehicle models.Vehicle

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&vehicle).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Vehicle not found"})
	}

	if err := c.BodyParser(&vehicle); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	vehicle.TenantID = tenantID

	if vehicle.ExtraFields == nil {
		vehicle.ExtraFields = models.JSONB{}
	}
	if vehicle.ImageURL != nil && *vehicle.ImageURL != "" {
		vehicle.ExtraFields["image_url"] = *vehicle.ImageURL
	} else {
		delete(vehicle.ExtraFields, "image_url")
	}

	if err := database.DB.Save(&vehicle).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	if v, ok := vehicle.ExtraFields["image_url"]; ok {
		if s, ok := v.(string); ok && s != "" {
			vehicle.ImageURL = &s
		}
	}

	return c.JSON(vehicle)
}

func DeleteVehicle(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.Vehicle{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Vehicle deleted"})
}

// ============================================
// 預約衝突檢測
// ============================================

// checkAppointmentConflicts 檢查預約時間衝突
func checkAppointmentConflicts(tenantID uuid.UUID, startTime, endTime time.Time, roomIDs, equipmentIDs, vehicleIDs []uuid.UUID, excludeAppointmentID uuid.UUID) []string {
	var conflicts []string

	// 檢查房間衝突
	for _, roomID := range roomIDs {
		var room models.Room
		if err := database.DB.Where("id = ?", roomID).First(&room).Error; err != nil {
			continue
		}

		// 如果房間允許重複使用，跳過衝突檢查
		if room.AllowOverlap {
			continue
		}

		// 查找該時間段內使用該房間的其他預約
		var count int64
		query := database.DB.Table("appointments").
			Joins("INNER JOIN appointment_rooms ON appointments.id = appointment_rooms.appointment_id").
			Where("appointments.tenant_id = ?", tenantID).
			Where("appointment_rooms.room_id = ?", roomID).
			Where("appointments.status != ?", "cancelled").
			Where("(appointments.start_time < ? AND appointments.end_time > ?)", endTime, startTime)

		if excludeAppointmentID != uuid.Nil {
			query = query.Where("appointments.id != ?", excludeAppointmentID)
		}

		query.Count(&count)

		if count > 0 {
			conflicts = append(conflicts, fmt.Sprintf("房間 %s 在該時間段已被預約", room.Name))
		}
	}

	// 檢查設備衝突
	for _, equipmentID := range equipmentIDs {
		var equipment models.Equipment
		if err := database.DB.Where("id = ?", equipmentID).First(&equipment).Error; err != nil {
			continue
		}

		// 如果設備允許重複使用，跳過衝突檢查
		if equipment.AllowOverlap {
			continue
		}

		// 查找該時間段內使用該設備的其他預約
		var count int64
		query := database.DB.Table("appointments").
			Joins("INNER JOIN appointment_equipments ON appointments.id = appointment_equipments.appointment_id").
			Where("appointments.tenant_id = ?", tenantID).
			Where("appointment_equipments.equipment_id = ?", equipmentID).
			Where("appointments.status != ?", "cancelled").
			Where("(appointments.start_time < ? AND appointments.end_time > ?)", endTime, startTime)

		if excludeAppointmentID != uuid.Nil {
			query = query.Where("appointments.id != ?", excludeAppointmentID)
		}

		query.Count(&count)

		if count > 0 {
			conflicts = append(conflicts, fmt.Sprintf("設備 %s 在該時間段已被預約", equipment.Name))
		}
	}

	// 檢查車輛衝突
	for _, vehicleID := range vehicleIDs {
		var vehicle models.Vehicle
		if err := database.DB.Where("id = ?", vehicleID).First(&vehicle).Error; err != nil {
			continue
		}

		// 如果車輛允許重複使用，跳過衝突檢查
		if vehicle.AllowOverlap {
			continue
		}

		// 查找該時間段內使用該車輛的其他預約
		var count int64
		query := database.DB.Table("appointments").
			Joins("INNER JOIN appointment_vehicles ON appointments.id = appointment_vehicles.appointment_id").
			Where("appointments.tenant_id = ?", tenantID).
			Where("appointment_vehicles.vehicle_id = ?", vehicleID).
			Where("appointments.status != ?", "cancelled").
			Where("(appointments.start_time < ? AND appointments.end_time > ?)", endTime, startTime)

		if excludeAppointmentID != uuid.Nil {
			query = query.Where("appointments.id != ?", excludeAppointmentID)
		}

		query.Count(&count)

		if count > 0 {
			conflicts = append(conflicts, fmt.Sprintf("車輛 %s 在該時間段已被預約", vehicle.Name))
		}
	}

	return conflicts
}

// CheckAppointmentConflict 檢查預約衝突的 API 端點
func CheckAppointmentConflict(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		StartTime    time.Time   `json:"start_time"`
		EndTime      time.Time   `json:"end_time"`
		RoomIDs      []uuid.UUID `json:"room_ids"`
		EquipmentIDs []uuid.UUID `json:"equipment_ids"`
		VehicleIDs   []uuid.UUID `json:"vehicle_ids"`
		ExcludeID    *uuid.UUID  `json:"exclude_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 確保時間在租戶時區中
	loc := utils.GetTenantLocation(tenantID)
	if !req.StartTime.IsZero() {
		req.StartTime = req.StartTime.In(loc)
	}
	if !req.EndTime.IsZero() {
		req.EndTime = req.EndTime.In(loc)
	}

	excludeID := uuid.Nil
	if req.ExcludeID != nil {
		excludeID = *req.ExcludeID
	}

	conflicts := checkAppointmentConflicts(tenantID, req.StartTime, req.EndTime, req.RoomIDs, req.EquipmentIDs, req.VehicleIDs, excludeID)

	return c.JSON(fiber.Map{
		"has_conflict": len(conflicts) > 0,
		"conflicts":    conflicts,
	})
}

// GetAppointmentCalendarEvents 返回 FullCalendar 兼容的預約事件
// 支持 service_type_id, service_type_code, staff_id, customer_id, status 篩選
func GetAppointmentCalendarEvents(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	if startDate == "" || endDate == "" {
		return c.Status(400).JSON(fiber.Map{"error": "start_date and end_date are required"})
	}

	loc := utils.GetTenantLocation(tenantID)
	startDay, err1 := time.ParseInLocation("2006-01-02", startDate, loc)
	endDay, err2 := time.ParseInLocation("2006-01-02", endDate, loc)
	if err1 != nil || err2 != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid date format"})
	}
	rangeStart := startDay
	rangeEnd := endDay.Add(24 * time.Hour)

	query := database.DB.Where("tenant_id = ?", tenantID).
		Where("start_time < ? AND end_time > ?", rangeEnd, rangeStart)

	// 篩選：service_type_id（透過 service → service_type）
	if serviceTypeID := c.Query("service_type_id"); serviceTypeID != "" {
		query = query.Where("service_id IN (SELECT s.id FROM services s WHERE s.service_type_id = ? AND s.tenant_id = ?)", serviceTypeID, tenantID)
	}
	// 篩選：service_type_code
	if serviceTypeCode := c.Query("service_type_code"); serviceTypeCode != "" {
		query = query.Where("service_id IN (SELECT s.id FROM services s JOIN service_types st ON s.service_type_id = st.id WHERE st.code = ? AND st.tenant_id = ?)", serviceTypeCode, tenantID)
	}
	// 篩選：staff_id
	if staffID := c.Query("staff_id"); staffID != "" {
		query = query.Where("staff_id = ?", staffID)
	}
	// 篩選：customer_id
	if customerID := c.Query("customer_id"); customerID != "" {
		query = query.Where("customer_id = ?", customerID)
	}
	// 篩選：status
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	var appointments []models.Appointment
	if err := query.Preload("Customer").Preload("Service.ServiceType").Preload("Staff").
		Order("start_time ASC").Find(&appointments).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch appointments: " + err.Error()})
	}

	statusColors := map[string][3]string{
		"confirmed": {"#198754", "#157347", "#fff"},
		"completed": {"#0d6efd", "#0b5ed7", "#fff"},
		"cancelled": {"#6c757d", "#5c636a", "#fff"},
		"pending":   {"#ffc107", "#e0a800", "#000"},
	}

	type calEvent struct {
		ID              string                 `json:"id"`
		Title           string                 `json:"title"`
		Start           string                 `json:"start"`
		End             string                 `json:"end"`
		AllDay          bool                   `json:"allDay"`
		BackgroundColor string                 `json:"backgroundColor,omitempty"`
		BorderColor     string                 `json:"borderColor,omitempty"`
		TextColor       string                 `json:"textColor,omitempty"`
		ExtendedProps   map[string]interface{} `json:"extendedProps,omitempty"`
	}

	events := make([]calEvent, 0, len(appointments))
	for _, a := range appointments {
		// 構建標題
		parts := make([]string, 0, 3)
		if a.Service != nil {
			parts = append(parts, a.Service.Name)
		}
		if a.Customer.Name != "" {
			parts = append(parts, a.Customer.Name)
		}
		if a.Staff != nil {
			parts = append(parts, a.Staff.Name)
		}
		title := strings.Join(parts, " - ")
		if title == "" {
			title = "預約"
		}

		colors := statusColors[a.Status]
		if colors == [3]string{} {
			colors = statusColors["pending"]
		}

		props := map[string]interface{}{
			"status": a.Status,
		}
		if a.Customer.Name != "" {
			props["customer_name"] = a.Customer.Name
		}
		if a.Staff != nil {
			props["staff_name"] = a.Staff.Name
		}
		if a.Service != nil {
			props["service_name"] = a.Service.Name
			if a.Service.ServiceType != nil {
				props["service_type_name"] = a.Service.ServiceType.Name
			}
		}

		events = append(events, calEvent{
			ID:              a.ID.String(),
			Title:           title,
			Start:           a.StartTime.In(loc).Format(time.RFC3339),
			End:             a.EndTime.In(loc).Format(time.RFC3339),
			BackgroundColor: colors[0],
			BorderColor:     colors[1],
			TextColor:       colors[2],
			ExtendedProps:   props,
		})
	}

	return c.JSON(fiber.Map{"data": events})
}
