package handlers

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type resourceUsageCalendarEvent struct {
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

// GetResourceUsageCalendarEvents returns FullCalendar-compatible events that represent:
// - Appointments that use any resources (rooms/equipments/vehicles)
// - Project resource reservations
//
// Query params:
// - start_date=YYYY-MM-DD
// - end_date=YYYY-MM-DD
func GetResourceUsageCalendarEvents(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	startDate := strings.TrimSpace(c.Query("start_date"))
	endDate := strings.TrimSpace(c.Query("end_date"))
	if startDate == "" || endDate == "" {
		return c.Status(400).JSON(fiber.Map{"error": "start_date and end_date are required"})
	}

	// Parse dates in tenant timezone as day boundaries.
	loc := utils.GetTenantLocation(tenantID)
	startDay, err1 := time.ParseInLocation("2006-01-02", startDate, loc)
	endDay, err2 := time.ParseInLocation("2006-01-02", endDate, loc)
	if err1 != nil || err2 != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid start_date/end_date format"})
	}
	// FullCalendar end is exclusive-ish; we treat range as [start, end) by adding one day to include end date coverage.
	rangeStart := startDay
	rangeEnd := endDay.Add(24 * time.Hour)

	events := make([]resourceUsageCalendarEvent, 0, 512)

	// 1) Appointments with any resource usage
	// We join the pivot tables to ensure only resource-used appointments are included.
	apptQ := database.DB.
		Table("appointments").
		Select("appointments.*").
		Where("appointments.tenant_id = ?", tenantID).
		Where("appointments.status != ?", "cancelled").
		Where("(appointments.start_time < ? AND appointments.end_time > ?)", rangeEnd, rangeStart).
		Joins("LEFT JOIN appointment_rooms ar ON ar.appointment_id = appointments.id").
		Joins("LEFT JOIN appointment_equipments ae ON ae.appointment_id = appointments.id").
		Joins("LEFT JOIN appointment_vehicles av ON av.appointment_id = appointments.id").
		Where("(ar.appointment_id IS NOT NULL OR ae.appointment_id IS NOT NULL OR av.appointment_id IS NOT NULL)").
		Group("appointments.id")

	// Load base appointment rows first
	var baseAppts []models.Appointment
	if err := apptQ.Find(&baseAppts).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch appointments: %v", err)})
	}

	if len(baseAppts) > 0 {
		ids := make([]uuid.UUID, 0, len(baseAppts))
		for _, a := range baseAppts {
			ids = append(ids, a.ID)
		}
		var full []models.Appointment
		if err := database.DB.
			Where("tenant_id = ? AND id IN ?", tenantID, ids).
			Preload("Customer").
			Preload("Rooms").
			Preload("Equipments").
			Preload("Vehicles").
			Find(&full).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to preload appointments: %v", err)})
		}

		for _, a := range full {
			// Build a short resource summary
			parts := make([]string, 0, 6)
			for _, r := range a.Rooms {
				if r.Name != "" {
					parts = append(parts, "房間:"+r.Name)
				}
			}
			for _, e := range a.Equipments {
				if e.Name != "" {
					parts = append(parts, "設備:"+e.Name)
				}
			}
			for _, v := range a.Vehicles {
				if v.Name != "" {
					parts = append(parts, "車輛:"+v.Name)
				}
			}
			resourceText := strings.Join(parts, " / ")

			title := "預約"
			if a.Customer.Name != "" {
				title = a.Customer.Name + " 預約"
			}
			if resourceText != "" {
				title = title + "（" + resourceText + "）"
			}

			events = append(events, resourceUsageCalendarEvent{
				ID:    "appt_" + a.ID.String(),
				Title: title,
				Start: a.StartTime.Format(time.RFC3339),
				End:   a.EndTime.Format(time.RFC3339),
				AllDay: false,
				BackgroundColor: "#0ea5e9",
				BorderColor:     "#0ea5e9",
				TextColor:       "#ffffff",
				ExtendedProps: map[string]interface{}{
					"category":       "appointment",
					"appointment_id": a.ID.String(),
					"url":            "/appointments/" + a.ID.String() + "/edit",
				},
			})
		}
	}

	// 2) Project resource reservations
	type resRow struct {
		models.ProjectResourceReservation
		ProjectName string `json:"project_name"`
		ResourceName string `json:"resource_name"`
	}

	var res []resRow
	// We join projects to get project name.
	// For resource names, resolve per-type in Go with small caches (like project_reservation.go).
	if err := database.DB.
		Table("project_resource_reservations prr").
		Select("prr.*, p.name AS project_name").
		Joins("LEFT JOIN projects p ON p.id = prr.project_id AND p.tenant_id = prr.tenant_id").
		Where("prr.tenant_id = ? AND prr.status != ?", tenantID, "cancelled").
		Where("(prr.start_time < ? AND prr.end_time > ?)", rangeEnd, rangeStart).
		Order("prr.start_time ASC").
		Find(&res).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch reservations: %v", err)})
	}

	roomCache := map[uuid.UUID]string{}
	eqCache := map[uuid.UUID]string{}
	vehCache := map[uuid.UUID]string{}
	for i := range res {
		name := ""
		switch res[i].ResourceType {
		case "room":
			if v, ok := roomCache[res[i].ResourceID]; ok {
				name = v
			} else {
				var m models.Room
				if err := database.DB.Select("name").Where("tenant_id = ? AND id = ?", tenantID, res[i].ResourceID).First(&m).Error; err == nil {
					name = m.Name
				}
				roomCache[res[i].ResourceID] = name
			}
		case "equipment":
			if v, ok := eqCache[res[i].ResourceID]; ok {
				name = v
			} else {
				var m models.Equipment
				if err := database.DB.Select("name").Where("tenant_id = ? AND id = ?", tenantID, res[i].ResourceID).First(&m).Error; err == nil {
					name = m.Name
				}
				eqCache[res[i].ResourceID] = name
			}
		case "vehicle":
			if v, ok := vehCache[res[i].ResourceID]; ok {
				name = v
			} else {
				var m models.Vehicle
				if err := database.DB.Select("name").Where("tenant_id = ? AND id = ?", tenantID, res[i].ResourceID).First(&m).Error; err == nil {
					name = m.Name
				}
				vehCache[res[i].ResourceID] = name
			}
		}
		res[i].ResourceName = name
	}

	for _, r := range res {
		prefix := "預留"
		if r.ProjectName != "" {
			prefix = r.ProjectName + " 預留"
		}
		resText := r.ResourceName
		if resText == "" {
			resText = r.ResourceType
		}
		title := prefix + "（" + resText + "）"

		events = append(events, resourceUsageCalendarEvent{
			ID:    "res_" + r.ID.String(),
			Title: title,
			Start: r.StartTime.Format(time.RFC3339),
			End:   r.EndTime.Format(time.RFC3339),
			AllDay: false,
			BackgroundColor: "#a855f7",
			BorderColor:     "#a855f7",
			TextColor:       "#ffffff",
			ExtendedProps: map[string]interface{}{
				"category":        "reservation",
				"reservation_id":  r.ID.String(),
				"project_id":      r.ProjectID.String(),
				"resource_type":   r.ResourceType,
				"resource_id":     r.ResourceID.String(),
				"url":             "/projects/" + r.ProjectID.String() + "/edit#tab-resources",
			},
		})
	}

	return c.JSON(fiber.Map{"data": events})
}


