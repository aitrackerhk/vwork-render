package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"nwork/internal/database"
	"nwork/internal/models"
	"nwork/internal/utils"
)

// PublicGetServices returns active services for a tenant's public website.
// GET /api/v1/public/:subdomain/services
func PublicGetServices(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	var services []models.Service
	if err := database.DB.
		Where("tenant_id = ? AND status = ?", tenant.ID, "active").
		Preload("ServiceType").
		Order("created_at DESC").
		Limit(100).
		Find(&services).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to load services"})
	}

	// Return only public-safe fields
	result := make([]fiber.Map, 0, len(services))
	for _, s := range services {
		item := fiber.Map{
			"id":               s.ID,
			"name":             s.Name,
			"description":      s.Description,
			"price":            s.Price,
			"duration_minutes": s.DurationMinutes,
		}
		if s.ServiceType != nil {
			item["service_type"] = fiber.Map{
				"id":   s.ServiceType.ID,
				"name": s.ServiceType.Name,
			}
		}
		result = append(result, item)
	}

	return c.JSON(fiber.Map{"services": result})
}

// PublicGetServiceStaff returns active service staff for a tenant's public website.
// GET /api/v1/public/:subdomain/service-staff
// Optional query: ?service_type_id=<uuid> to filter by service type
func PublicGetServiceStaff(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	query := database.DB.
		Where("tenant_id = ? AND status = ?", tenant.ID, "active")

	// Optional filter by service type
	if stID := c.Query("service_type_id"); stID != "" {
		if _, err := uuid.Parse(stID); err == nil {
			query = query.Where("service_type_id = ?", stID)
		}
	}

	var staff []models.ServiceStaff
	if err := query.Order("name ASC").Limit(100).Find(&staff).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to load staff"})
	}

	// Return only public-safe fields
	result := make([]fiber.Map, 0, len(staff))
	for _, s := range staff {
		result = append(result, fiber.Map{
			"id":              s.ID,
			"name":            s.Name,
			"specialization":  s.Specialization,
			"service_type_id": s.ServiceTypeID,
		})
	}

	return c.JSON(fiber.Map{"staff": result})
}

// PublicCreateAppointment creates an appointment from the public booking form.
// POST /api/v1/public/:subdomain/appointments
//
// Supports two modes:
//  1. Logged-in customer (customer_id cookie) — uses existing customer record
//  2. Guest booking — requires name + email/phone, creates or finds customer
func PublicCreateAppointment(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	var req struct {
		// Service & staff selection
		ServiceID string `json:"service_id"`
		StaffID   string `json:"staff_id"`

		// Date/time — expected in tenant timezone, format: "2006-01-02T15:04" or RFC3339
		StartTime string `json:"start_time"`

		// Guest customer info (used if not logged in)
		CustomerName  string `json:"customer_name"`
		CustomerEmail string `json:"customer_email"`
		CustomerPhone string `json:"customer_phone"`

		// Notes
		Notes string `json:"notes"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// --- Resolve customer ---
	var customerID uuid.UUID

	// Try cookie first (logged-in customer)
	if cookieID, err := getCustomerIDFromCookie(c); err == nil && cookieID != nil {
		var customer models.Customer
		if err := database.DB.Where("tenant_id = ? AND id = ?", tenant.ID, *cookieID).First(&customer).Error; err == nil {
			customerID = customer.ID
		}
	}

	// If not logged in, use guest info
	if customerID == uuid.Nil {
		name := strings.TrimSpace(req.CustomerName)
		email := strings.TrimSpace(req.CustomerEmail)
		phone := strings.TrimSpace(req.CustomerPhone)

		if name == "" {
			return c.Status(400).JSON(fiber.Map{"error": "Customer name is required"})
		}
		if email == "" && phone == "" {
			return c.Status(400).JSON(fiber.Map{"error": "Email or phone is required"})
		}

		// Find existing customer by email or phone
		var customer models.Customer
		found := false
		if email != "" {
			if err := database.DB.Where("tenant_id = ? AND email = ?", tenant.ID, email).First(&customer).Error; err == nil {
				found = true
			}
		}
		if !found && phone != "" {
			if err := database.DB.Where("tenant_id = ? AND phone = ?", tenant.ID, phone).First(&customer).Error; err == nil {
				found = true
			}
		}

		if !found {
			customer = models.Customer{
				TenantID: tenant.ID,
				Name:     name,
				Email:    email,
				Phone:    phone,
				Status:   "active",
			}
			if err := database.DB.Create(&customer).Error; err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Failed to create customer"})
			}
		}
		customerID = customer.ID
	}

	// --- Parse start time ---
	if req.StartTime == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Start time is required"})
	}

	loc := utils.GetTenantLocation(tenant.ID)
	startTime, err := utils.ParseDateTimeInTenantTimezone(tenant.ID, req.StartTime)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid start time format"})
	}

	// Validate: must be in the future
	now := time.Now().In(loc)
	if startTime.Before(now) {
		return c.Status(400).JSON(fiber.Map{"error": "Appointment time must be in the future"})
	}

	// --- Determine duration and end time ---
	durationMinutes := 60 // default 1 hour
	var serviceID *uuid.UUID
	if req.ServiceID != "" {
		if sid, err := uuid.Parse(req.ServiceID); err == nil {
			var service models.Service
			if err := database.DB.Where("id = ? AND tenant_id = ? AND status = ?", sid, tenant.ID, "active").First(&service).Error; err == nil {
				serviceID = &sid
				if service.DurationMinutes != nil && *service.DurationMinutes > 0 {
					durationMinutes = *service.DurationMinutes
				}
			}
		}
	}

	endTime := startTime.Add(time.Duration(durationMinutes) * time.Minute)

	// --- Staff ---
	var staffID *uuid.UUID
	if req.StaffID != "" {
		if sid, err := uuid.Parse(req.StaffID); err == nil {
			var staff models.ServiceStaff
			if err := database.DB.Where("id = ? AND tenant_id = ? AND status = ?", sid, tenant.ID, "active").First(&staff).Error; err == nil {
				staffID = &sid
			}
		}
	}

	// --- Derive legacy fields ---
	appointmentDate := time.Date(
		startTime.Year(), startTime.Month(), startTime.Day(),
		0, 0, 0, 0, loc,
	)

	// --- Create appointment ---
	appointment := models.Appointment{
		TenantID:        tenant.ID,
		CustomerID:      customerID,
		ServiceID:       serviceID,
		StaffID:         staffID,
		StartTime:       startTime,
		EndTime:         endTime,
		AppointmentDate: appointmentDate,
		AppointmentTime: models.NewSQLTime(startTime),
		DurationMinutes: &durationMinutes,
		Notes:           strings.TrimSpace(req.Notes),
		Status:          "pending",
	}

	if err := database.DB.Create(&appointment).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create appointment"})
	}

	// Reload with relations for notification
	database.DB.Preload("Customer").Preload("Service").Preload("Staff").
		First(&appointment, appointment.ID)

	// --- Notify CMS users ---
	var settings models.NotificationSettings
	if err := database.DB.Where("tenant_id = ?", tenant.ID).First(&settings).Error; err != nil {
		settings.AppointmentNotificationsEnabled = true
	}

	if settings.AppointmentNotificationsEnabled {
		customerName := appointment.Customer.Name
		if customerName == "" {
			customerName = "Guest"
		}
		serviceName := "未指定服務"
		if appointment.Service != nil {
			serviceName = appointment.Service.Name
		}
		title := fmt.Sprintf("新網站預約：%s", customerName)
		message := fmt.Sprintf("客戶：%s\n服務：%s\n時間：%s\n狀態：pending",
			customerName, serviceName,
			startTime.Format("2006-01-02 15:04"))
		link := fmt.Sprintf("/appointments?appointment_id=%s", appointment.ID.String())
		go utils.CreateNotificationAlertForAllUsers(tenant.ID, "appointment_created", title, message, link, nil)
	}

	return c.Status(201).JSON(fiber.Map{
		"message":        "Appointment created successfully",
		"appointment_id": appointment.ID,
	})
}
