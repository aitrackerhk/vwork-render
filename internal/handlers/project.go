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
	"gorm.io/gorm"
)

// GetProjects 獲取項目列表
func GetProjects(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	// List item：附帶 tasks_total / tasks_done（讓前端做進度條更友善）
	type ProjectListItem struct {
		models.Project
		TasksTotal       int64   `json:"tasks_total"`
		TasksDone        int64   `json:"tasks_done"`
		ProjectTypeName  *string `json:"project_type_name,omitempty"`
		ProjectTypeColor *string `json:"project_type_color,omitempty"`
	}

	var projects []ProjectListItem

	query := database.DB.
		Table("projects").
		Select(`
			projects.*,
			COALESCE(COUNT(project_tasks.id), 0) AS tasks_total,
			COALESCE(SUM(CASE WHEN project_tasks.status = 'done' THEN 1 ELSE 0 END), 0) AS tasks_done,
			MAX(project_types.name) AS project_type_name,
			MAX(project_types.color) AS project_type_color
		`).
		Joins(`LEFT JOIN project_tasks ON project_tasks.project_id = projects.id AND project_tasks.tenant_id = projects.tenant_id`).
		Joins(`LEFT JOIN project_types ON project_types.id = projects.project_type_id AND project_types.tenant_id = projects.tenant_id`).
		Where("projects.tenant_id = ?", tenantID)

	// 搜索（code/name）
	if search := c.Query("search"); search != "" {
		query = query.Where("projects.code ILIKE ? OR projects.name ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	// 狀態過濾
	if status := c.Query("status"); status != "" {
		query = query.Where("projects.status = ?", status)
	}

	// 分頁
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var total int64
	// total 用 projects 本身計數
	database.DB.Model(&models.Project{}).Where("tenant_id = ?", tenantID).Count(&total)

	if err := query.
		Group("projects.id").
		Offset(offset).Limit(limit).
		Order("projects.created_at DESC").
		Find(&projects).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch projects: %v", err)})
	}

	// 補 Owner（避免複雜 join，數量不大用二次查詢）
	ownerIDs := make([]uuid.UUID, 0, len(projects))
	for _, p := range projects {
		if p.OwnerUserID != nil {
			ownerIDs = append(ownerIDs, *p.OwnerUserID)
		}
	}
	ownerMap := map[uuid.UUID]models.User{}
	if len(ownerIDs) > 0 {
		var owners []models.User
		database.DB.Where("tenant_id = ? AND id IN ?", tenantID, ownerIDs).Find(&owners)
		for _, u := range owners {
			u.PasswordHash = ""
			ownerMap[u.ID] = u
		}
	}
	for i := range projects {
		if projects[i].OwnerUserID != nil {
			if u, ok := ownerMap[*projects[i].OwnerUserID]; ok {
				projects[i].Owner = &u
			}
		}
	}

	return c.JSON(fiber.Map{
		"data":  projects,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetProject 獲取單個項目
func GetProject(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid project ID"})
	}

	var project models.Project
	if err := database.DB.
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Preload("ProjectType").
		Preload("Owner").
		Preload("Tasks", func(db *gorm.DB) *gorm.DB {
			return db.Order("sort_order ASC, created_at ASC")
		}).
		Preload("Tasks.Assignee").
		First(&project).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Project not found"})
	}

	return c.JSON(project)
}

// CreateProject 創建項目
func CreateProject(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		Code          string     `json:"code"`
		Name          string     `json:"name"`
		Description   string     `json:"description"`
		CoverURL      string     `json:"cover_url"`
		Status        string     `json:"status"`
		StartDate     *string    `json:"start_date"`
		EndDate       *string    `json:"end_date"`
		Budget        *float64   `json:"budget"`
		ProjectTypeID *uuid.UUID `json:"project_type_id"`
		OwnerUserID   *uuid.UUID `json:"owner_user_id"`
		Tasks         []struct {
			Title          string     `json:"title"`
			Description    string     `json:"description"`
			Status         string     `json:"status"`
			Priority       string     `json:"priority"`
			DueDate        *string    `json:"due_date"`
			AssigneeUserID *uuid.UUID `json:"assignee_user_id"`
			SortOrder      *int       `json:"sort_order"`
		} `json:"tasks"`
		ExtraFields map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request: " + err.Error()})
	}

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}

	// code 如有填，做唯一性檢查
	if req.Code != "" {
		var existing models.Project
		if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, req.Code).First(&existing).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Project code already exists"})
		}
	}

	// 自動生成項目編號（如果未提供）
	autoCode, err := generateAutoCode(tenantID, req.Code, autoCodeConfig{
		Prefix:     "PROJ-",
		FieldName:  "code",
		PageName:   "projects",
		Format:     codeFormatDate,
		TableModel: &models.Project{},
		Column:     "code",
	})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate project code: " + err.Error()})
	}

	now := utils.NowInTenantTimezone(tenantID)

	var startDatePtr *time.Time
	if req.StartDate != nil {
		if *req.StartDate == "" {
			startDatePtr = nil
		} else if t, err := utils.ParseDateInTenantTimezone(tenantID, *req.StartDate); err == nil {
			startDatePtr = &t
		}
	}

	var endDatePtr *time.Time
	if req.EndDate != nil {
		if *req.EndDate == "" {
			endDatePtr = nil
		} else if t, err := utils.ParseDateInTenantTimezone(tenantID, *req.EndDate); err == nil {
			endDatePtr = &t
		}
	}

	project := models.Project{
		TenantID:      tenantID,
		Code:          autoCode,
		Name:          req.Name,
		Description:   req.Description,
		CoverURL:      req.CoverURL,
		Status:        req.Status,
		StartDate:     startDatePtr,
		EndDate:       endDatePtr,
		ProjectTypeID: req.ProjectTypeID,
		OwnerUserID:   req.OwnerUserID,
		CreatedBy:     &userID,
		UpdatedBy:     &userID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if project.Status == "" {
		project.Status = "active"
	}

	if req.Budget != nil {
		project.Budget = *req.Budget
	}

	if req.ExtraFields != nil {
		project.ExtraFields = models.JSONB(req.ExtraFields)
	}

	newTaskCount := 0
	for _, t := range req.Tasks {
		if strings.TrimSpace(t.Title) != "" {
			newTaskCount++
		}
	}
	if err := middleware.EnforceTrialTaskLimit(c, tenantID, userID, newTaskCount, nil); err != nil {
		return err
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&project).Error; err != nil {
			return err
		}

		// 寫入 tasks（multi_row）
		for idx, t := range req.Tasks {
			if t.Title == "" {
				continue
			}
			var duePtr *time.Time
			if t.DueDate != nil {
				if *t.DueDate == "" {
					duePtr = nil
				} else if tt, err := utils.ParseDateInTenantTimezone(tenantID, *t.DueDate); err == nil {
					duePtr = &tt
				}
			}
			sortOrder := idx
			if t.SortOrder != nil {
				sortOrder = *t.SortOrder
			}
			task := models.ProjectTask{
				TenantID:       tenantID,
				ProjectID:      project.ID,
				Title:          t.Title,
				Description:    t.Description,
				Status:         t.Status,
				Priority:       t.Priority,
				DueDate:        duePtr,
				AssigneeUserID: t.AssigneeUserID,
				SortOrder:      sortOrder,
				CreatedAt:      now,
				UpdatedAt:      now,
				ExtraFields:    make(models.JSONB),
			}
			if task.Status == "" {
				task.Status = "todo"
			}
			if task.Priority == "" {
				task.Priority = "medium"
			}
			if err := tx.Create(&task).Error; err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create project: %v", err)})
	}

	// 成功建立後釋放預留編號
	releaseReservedCode(tenantID, "code", project.Code)

	// 回傳時帶 owner + tasks
	database.DB.Where("id = ? AND tenant_id = ?", project.ID, tenantID).
		Preload("ProjectType").
		Preload("Owner").
		Preload("Tasks", func(db *gorm.DB) *gorm.DB { return db.Order("sort_order ASC, created_at ASC") }).
		Preload("Tasks.Assignee").
		First(&project)

	return c.Status(201).JSON(project)
}

// UpdateProject 更新項目
func UpdateProject(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid project ID"})
	}

	var project models.Project
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&project).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Project not found"})
	}

	var req struct {
		Code          *string    `json:"code"`
		Name          *string    `json:"name"`
		Description   *string    `json:"description"`
		CoverURL      *string    `json:"cover_url"`
		Status        *string    `json:"status"`
		StartDate     *string    `json:"start_date"`
		EndDate       *string    `json:"end_date"`
		Budget        *float64   `json:"budget"`
		ProjectTypeID *uuid.UUID `json:"project_type_id"`
		OwnerUserID   *uuid.UUID `json:"owner_user_id"`
		Tasks         *[]struct {
			Title          string     `json:"title"`
			Description    string     `json:"description"`
			Status         string     `json:"status"`
			Priority       string     `json:"priority"`
			DueDate        *string    `json:"due_date"`
			AssigneeUserID *uuid.UUID `json:"assignee_user_id"`
			SortOrder      *int       `json:"sort_order"`
		} `json:"tasks"`
		ExtraFields map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request: " + err.Error()})
	}

	if req.Code != nil {
		if *req.Code != "" {
			var existing models.Project
			if err := database.DB.Where("tenant_id = ? AND code = ? AND id != ?", tenantID, *req.Code, id).First(&existing).Error; err == nil {
				return c.Status(400).JSON(fiber.Map{"error": "Project code already exists"})
			}
		}
		project.Code = *req.Code // 允許空字串清空
	}
	if req.Name != nil {
		if *req.Name == "" {
			return c.Status(400).JSON(fiber.Map{"error": "Name cannot be empty"})
		}
		project.Name = *req.Name
	}
	if req.Description != nil {
		project.Description = *req.Description
	}
	if req.CoverURL != nil {
		project.CoverURL = *req.CoverURL // 允許空字串清空
	}
	if req.Status != nil {
		project.Status = *req.Status
	}
	if req.ProjectTypeID != nil {
		project.ProjectTypeID = req.ProjectTypeID
	}
	if req.OwnerUserID != nil {
		project.OwnerUserID = req.OwnerUserID
	}
	if req.Budget != nil {
		project.Budget = *req.Budget
	}

	if req.StartDate != nil {
		if *req.StartDate == "" {
			project.StartDate = nil
		} else if t, err := utils.ParseDateInTenantTimezone(tenantID, *req.StartDate); err == nil {
			project.StartDate = &t
		}
	}
	if req.EndDate != nil {
		if *req.EndDate == "" {
			project.EndDate = nil
		} else if t, err := utils.ParseDateInTenantTimezone(tenantID, *req.EndDate); err == nil {
			project.EndDate = &t
		}
	}

	if req.ExtraFields != nil {
		project.ExtraFields = models.JSONB(req.ExtraFields)
	}

	project.UpdatedBy = &userID
	project.UpdatedAt = utils.NowInTenantTimezone(tenantID)

	if req.Tasks != nil {
		newTaskCount := 0
		for _, t := range *req.Tasks {
			if strings.TrimSpace(t.Title) != "" {
				newTaskCount++
			}
		}
		if err := middleware.EnforceTrialTaskLimit(c, tenantID, userID, newTaskCount, &project.ID); err != nil {
			return err
		}
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&project).Error; err != nil {
			return err
		}

		// 如果有帶 tasks，就以它為準（replace）
		if req.Tasks != nil {
			if err := tx.Where("tenant_id = ? AND project_id = ?", tenantID, project.ID).Delete(&models.ProjectTask{}).Error; err != nil {
				return err
			}

			now := utils.NowInTenantTimezone(tenantID)
			for idx, t := range *req.Tasks {
				if t.Title == "" {
					continue
				}
				var duePtr *time.Time
				if t.DueDate != nil {
					if *t.DueDate == "" {
						duePtr = nil
					} else if tt, err := utils.ParseDateInTenantTimezone(tenantID, *t.DueDate); err == nil {
						duePtr = &tt
					}
				}
				sortOrder := idx
				if t.SortOrder != nil {
					sortOrder = *t.SortOrder
				}
				task := models.ProjectTask{
					TenantID:       tenantID,
					ProjectID:      project.ID,
					Title:          t.Title,
					Description:    t.Description,
					Status:         t.Status,
					Priority:       t.Priority,
					DueDate:        duePtr,
					AssigneeUserID: t.AssigneeUserID,
					SortOrder:      sortOrder,
					CreatedAt:      now,
					UpdatedAt:      now,
					ExtraFields:    make(models.JSONB),
				}
				if task.Status == "" {
					task.Status = "todo"
				}
				if task.Priority == "" {
					task.Priority = "medium"
				}
				if err := tx.Create(&task).Error; err != nil {
					return err
				}
			}
		}

		return nil
	}); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to update project: %v", err)})
	}

	database.DB.Where("id = ? AND tenant_id = ?", project.ID, tenantID).
		Preload("ProjectType").
		Preload("Owner").
		Preload("Tasks", func(db *gorm.DB) *gorm.DB { return db.Order("sort_order ASC, created_at ASC") }).
		Preload("Tasks.Assignee").
		First(&project)
	return c.JSON(project)
}

// DeleteProject 刪除項目
func DeleteProject(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid project ID"})
	}

	var project models.Project
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&project).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Project not found"})
	}

	if err := database.DB.Delete(&project).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to delete project: %v", err)})
	}

	return c.JSON(fiber.Map{"message": "Project deleted"})
}
