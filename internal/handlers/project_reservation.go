package handlers

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type reservationResp struct {
	models.ProjectResourceReservation
	ResourceName string `json:"resource_name"`
	UserName     string `json:"user_name"`
}

func ListProjectReservations(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	projectID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid project ID"})
	}

	var rs []models.ProjectResourceReservation
	if err := database.DB.Where("tenant_id = ? AND project_id = ? AND status != ?", tenantID, projectID, "cancelled").
		Order("start_time ASC").
		Find(&rs).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch reservations: %v", err)})
	}

	// resolve names with small caches
	roomCache := map[uuid.UUID]string{}
	eqCache := map[uuid.UUID]string{}
	vehCache := map[uuid.UUID]string{}
	userCache := map[uuid.UUID]string{}
	out := make([]reservationResp, 0, len(rs))
	for _, r := range rs {
		name := ""
		switch r.ResourceType {
		case "room":
			if v, ok := roomCache[r.ResourceID]; ok {
				name = v
			} else {
				var m models.Room
				if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, r.ResourceID).First(&m).Error; err == nil {
					name = m.Name
				}
				roomCache[r.ResourceID] = name
			}
		case "equipment":
			if v, ok := eqCache[r.ResourceID]; ok {
				name = v
			} else {
				var m models.Equipment
				if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, r.ResourceID).First(&m).Error; err == nil {
					name = m.Name
				}
				eqCache[r.ResourceID] = name
			}
		case "vehicle":
			if v, ok := vehCache[r.ResourceID]; ok {
				name = v
			} else {
				var m models.Vehicle
				if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, r.ResourceID).First(&m).Error; err == nil {
					name = m.Name
				}
				vehCache[r.ResourceID] = name
			}
		}

		uname := ""
		if r.UserID != nil && *r.UserID != uuid.Nil {
			if v, ok := userCache[*r.UserID]; ok {
				uname = v
			} else {
				var u models.User
				if err := database.DB.Select("name").Where("tenant_id = ? AND id = ?", tenantID, *r.UserID).First(&u).Error; err == nil {
					uname = u.Name
				}
				userCache[*r.UserID] = uname
			}
		}

		out = append(out, reservationResp{ProjectResourceReservation: r, ResourceName: name, UserName: uname})
	}

	return c.JSON(fiber.Map{"data": out})
}

func ReplaceProjectReservations(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	projectID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid project ID"})
	}

	var req struct {
		Reservations []struct {
			ResourceType string    `json:"resource_type"`
			ResourceID   uuid.UUID `json:"resource_id"`
			StartTime    string    `json:"start_time"` // datetime-local: 2006-01-02T15:04
			EndTime      string    `json:"end_time"`
			Notes        string    `json:"notes"`
			UserID       *uuid.UUID `json:"user_id"`
		} `json:"reservations"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	loc := utils.GetTenantLocation(tenantID)
	type key struct {
		t string
		i uuid.UUID
	}
	seen := map[key][][2]time.Time{}

	parsed := make([]models.ProjectResourceReservation, 0, len(req.Reservations))
	for _, r := range req.Reservations {
		if r.ResourceType == "" || r.ResourceID == uuid.Nil || r.StartTime == "" || r.EndTime == "" {
			continue
		}
		start, err1 := time.ParseInLocation("2006-01-02T15:04", r.StartTime, loc)
		end, err2 := time.ParseInLocation("2006-01-02T15:04", r.EndTime, loc)
		if err1 != nil || err2 != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid start_time/end_time format"})
		}
		if !start.Before(end) {
			return c.Status(400).JSON(fiber.Map{"error": "Start time must be before end time"})
		}

		// in-request overlap check per resource
		k := key{t: r.ResourceType, i: r.ResourceID}
		for _, it := range seen[k] {
			if it[0].Before(end) && it[1].After(start) {
				return c.Status(400).JSON(fiber.Map{"error": "同一資源的預留時間重疊"})
			}
		}
		seen[k] = append(seen[k], [2]time.Time{start, end})

		parsed = append(parsed, models.ProjectResourceReservation{
			TenantID:      tenantID,
			ProjectID:     projectID,
			ResourceType:  r.ResourceType,
			ResourceID:    r.ResourceID,
			StartTime:     start,
			EndTime:       end,
			Status:        "active",
			Notes:         r.Notes,
			UserID:        r.UserID,
			CreatedBy:     &userID,
			UpdatedBy:     &userID,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		})
	}

	// validate conflicts (appointments + other reservations) unless allow_overlap
	for _, r := range parsed {
		allowOverlap := false
		switch r.ResourceType {
		case "room":
			var m models.Room
			if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, r.ResourceID).First(&m).Error; err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "Room not found"})
			}
			allowOverlap = m.AllowOverlap
		case "equipment":
			var m models.Equipment
			if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, r.ResourceID).First(&m).Error; err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "Equipment not found"})
			}
			allowOverlap = m.AllowOverlap
		case "vehicle":
			var m models.Vehicle
			if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, r.ResourceID).First(&m).Error; err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "Vehicle not found"})
			}
			allowOverlap = m.AllowOverlap
		default:
			return c.Status(400).JSON(fiber.Map{"error": "Invalid resource_type"})
		}

		if allowOverlap {
			continue
		}

		// appointment conflicts
		var roomIDs, eqIDs, vehIDs []uuid.UUID
		if r.ResourceType == "room" {
			roomIDs = []uuid.UUID{r.ResourceID}
		}
		if r.ResourceType == "equipment" {
			eqIDs = []uuid.UUID{r.ResourceID}
		}
		if r.ResourceType == "vehicle" {
			vehIDs = []uuid.UUID{r.ResourceID}
		}
		conflicts := checkAppointmentConflicts(tenantID, r.StartTime, r.EndTime, roomIDs, eqIDs, vehIDs, uuid.Nil)
		if len(conflicts) > 0 {
			return c.Status(400).JSON(fiber.Map{"error": conflicts[0]})
		}

		// other reservations conflicts
		var count int64
		if err := database.DB.Model(&models.ProjectResourceReservation{}).
			Where("tenant_id = ? AND status != ? AND resource_type = ? AND resource_id = ?", tenantID, "cancelled", r.ResourceType, r.ResourceID).
			Where("(start_time < ? AND end_time > ?)", r.EndTime, r.StartTime).
			Count(&count).Error; err == nil && count > 0 {
			return c.Status(400).JSON(fiber.Map{"error": "該資源在此時間段已被預留"})
		}
	}

	// replace in tx
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND project_id = ?", tenantID, projectID).
			Delete(&models.ProjectResourceReservation{}).Error; err != nil {
			return err
		}
		if len(parsed) == 0 {
			return nil
		}
		return tx.Create(&parsed).Error
	}); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to save reservations: %v", err)})
	}

	return c.JSON(fiber.Map{"success": true})
}


