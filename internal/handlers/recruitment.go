package handlers

import (
	"fmt"
	"strings"
	"time"

	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ===== Vacancies =====

func GetJobVacancies(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	offset := (page - 1) * limit

	search := strings.TrimSpace(c.Query("search"))
	status := strings.TrimSpace(c.Query("status"))

	query := database.DB.Model(&models.JobVacancy{}).Where("tenant_id = ?", tenantID)
	if search != "" {
		query = query.Where("title ILIKE ?", "%"+search+"%")
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	query.Count(&total)

	var items []models.JobVacancy
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch job vacancies: %v", err)})
	}

	// 補 department（手動）
	depIDs := make([]uuid.UUID, 0, len(items))
	for _, v := range items {
		if v.DepartmentID != nil {
			depIDs = append(depIDs, *v.DepartmentID)
		}
	}
	depMap := map[uuid.UUID]models.Department{}
	if len(depIDs) > 0 {
		var deps []models.Department
		database.DB.Where("id IN ?", depIDs).Find(&deps)
		for _, d := range deps {
			depMap[d.ID] = d
		}
	}
	for i := range items {
		if items[i].DepartmentID != nil {
			if d, ok := depMap[*items[i].DepartmentID]; ok {
				items[i].Department = &d
			}
		}
	}

	return c.JSON(fiber.Map{
		"data":  items,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetJobVacancy(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	var item models.JobVacancy
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&item).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Not found"})
	}

	if item.DepartmentID != nil {
		var dep models.Department
		if err := database.DB.Where("id = ?", *item.DepartmentID).First(&dep).Error; err == nil {
			item.Department = &dep
		}
	}

	return c.JSON(item)
}

func CreateJobVacancy(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var req struct {
		Title        string     `json:"title"`
		DepartmentID *uuid.UUID `json:"department_id"`
		Headcount    int        `json:"headcount"`
		Status       string     `json:"status"`
		Description  string     `json:"description"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Title is required"})
	}
	if req.Headcount <= 0 {
		req.Headcount = 1
	}
	if req.Status != "open" && req.Status != "closed" {
		req.Status = "open"
	}

	item := models.JobVacancy{
		TenantID:     tenantID,
		Title:        req.Title,
		DepartmentID: req.DepartmentID,
		Headcount:    req.Headcount,
		Status:       req.Status,
		Description:  req.Description,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := database.DB.Create(&item).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create"})
	}
	return c.Status(201).JSON(item)
}

func UpdateJobVacancy(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	var item models.JobVacancy
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&item).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Not found"})
	}

	var req struct {
		Title        *string    `json:"title"`
		DepartmentID *uuid.UUID `json:"department_id"`
		Headcount    *int       `json:"headcount"`
		Status       *string    `json:"status"`
		Description  *string    `json:"description"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Title != nil {
		t := strings.TrimSpace(*req.Title)
		if t != "" {
			item.Title = t
		}
	}
	if req.DepartmentID != nil {
		item.DepartmentID = req.DepartmentID
	}
	if req.Headcount != nil && *req.Headcount > 0 {
		item.Headcount = *req.Headcount
	}
	if req.Status != nil {
		if *req.Status == "open" || *req.Status == "closed" {
			item.Status = *req.Status
		}
	}
	if req.Description != nil {
		item.Description = *req.Description
	}
	item.UpdatedAt = time.Now()

	if err := database.DB.Save(&item).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update"})
	}
	return c.JSON(item)
}

func DeleteJobVacancy(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.JobVacancy{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete"})
	}
	return c.JSON(fiber.Map{"message": "Deleted"})
}

// ===== Applicants =====

func GetJobApplicants(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	offset := (page - 1) * limit

	search := strings.TrimSpace(c.Query("search"))
	status := strings.TrimSpace(c.Query("status"))

	query := database.DB.Model(&models.JobApplicant{}).Where("tenant_id = ?", tenantID).Preload("Vacancy")
	if search != "" {
		query = query.Where("candidate_name ILIKE ? OR candidate_last_name ILIKE ? OR email ILIKE ? OR phone ILIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	query.Count(&total)

	var items []models.JobApplicant
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch job applicants: %v", err)})
	}

	// 附加 candidate_display_name（供 CMS 列表與 select2 顯示用）
	type applicantDTO struct {
		models.JobApplicant
		CandidateDisplayName string `json:"candidate_display_name"`
	}
	out := make([]applicantDTO, 0, len(items))
	for _, a := range items {
		first := strings.TrimSpace(a.CandidateName)
		last := strings.TrimSpace(a.CandidateLastName)
		display := first
		if last != "" {
			hasCJK := strings.ContainsFunc(first, func(r rune) bool { return r >= 0x3400 && r <= 0x9FFF }) ||
				strings.ContainsFunc(last, func(r rune) bool { return r >= 0x3400 && r <= 0x9FFF })
			if hasCJK {
				display = last + first
			} else {
				display = first + " " + last
			}
		}
		out = append(out, applicantDTO{JobApplicant: a, CandidateDisplayName: display})
	}

	return c.JSON(fiber.Map{
		"data":  out,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetJobApplicant(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	var item models.JobApplicant
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Preload("Vacancy").First(&item).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Not found"})
	}
	return c.JSON(item)
}

func CreateJobApplicant(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var req struct {
		VacancyID         *uuid.UUID `json:"vacancy_id"`
		CandidateName     string     `json:"candidate_name"`
		CandidateLastName string     `json:"candidate_last_name"`
		Email             string     `json:"email"`
		Phone             string     `json:"phone"`
		ProfilePic        string     `json:"profile_pic"`
		Status            string     `json:"status"`
		Notes             string     `json:"notes"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	req.CandidateName = strings.TrimSpace(req.CandidateName)
	req.CandidateLastName = strings.TrimSpace(req.CandidateLastName)
	if req.CandidateName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Candidate name is required"})
	}
	if req.Status == "" {
		req.Status = "applied"
	}
	switch req.Status {
	case "applied", "interview", "offered", "hired", "rejected":
	default:
		req.Status = "applied"
	}

	item := models.JobApplicant{
		TenantID:          tenantID,
		VacancyID:         req.VacancyID,
		CandidateName:     req.CandidateName,
		CandidateLastName: req.CandidateLastName,
		Email:             strings.TrimSpace(req.Email),
		Phone:             strings.TrimSpace(req.Phone),
		ProfilePic:        strings.TrimSpace(req.ProfilePic),
		Status:            req.Status,
		Notes:             req.Notes,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := database.DB.Create(&item).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create"})
	}
	return c.Status(201).JSON(item)
}

func UpdateJobApplicant(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	var item models.JobApplicant
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&item).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Not found"})
	}

	var req struct {
		VacancyID         *uuid.UUID `json:"vacancy_id"`
		CandidateName     *string    `json:"candidate_name"`
		CandidateLastName *string    `json:"candidate_last_name"`
		Email             *string    `json:"email"`
		Phone             *string    `json:"phone"`
		ProfilePic        *string    `json:"profile_pic"`
		Status            *string    `json:"status"`
		Notes             *string    `json:"notes"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.VacancyID != nil {
		item.VacancyID = req.VacancyID
	}
	if req.CandidateName != nil {
		n := strings.TrimSpace(*req.CandidateName)
		if n != "" {
			item.CandidateName = n
		}
	}
	if req.CandidateLastName != nil {
		item.CandidateLastName = strings.TrimSpace(*req.CandidateLastName)
	}
	if req.Email != nil {
		item.Email = strings.TrimSpace(*req.Email)
	}
	if req.Phone != nil {
		item.Phone = strings.TrimSpace(*req.Phone)
	}
	if req.ProfilePic != nil {
		item.ProfilePic = strings.TrimSpace(*req.ProfilePic)
	}
	if req.Status != nil {
		switch *req.Status {
		case "applied", "interview", "offered", "hired", "rejected":
			item.Status = *req.Status
		}
	}
	if req.Notes != nil {
		item.Notes = *req.Notes
	}
	item.UpdatedAt = time.Now()

	if err := database.DB.Save(&item).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update"})
	}
	return c.JSON(item)
}

func DeleteJobApplicant(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.JobApplicant{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete"})
	}
	return c.JSON(fiber.Map{"message": "Deleted"})
}

// ===== Hires =====

func GetJobHires(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	offset := (page - 1) * limit

	search := strings.TrimSpace(c.Query("search"))
	status := strings.TrimSpace(c.Query("status"))

	query := database.DB.Model(&models.JobHire{}).Where("tenant_id = ?", tenantID).Preload("Vacancy").Preload("Applicant")
	if search != "" {
		query = query.Where("candidate_name ILIKE ? OR candidate_last_name ILIKE ? OR email ILIKE ? OR phone ILIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	query.Count(&total)

	var items []models.JobHire
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch job hires: %v", err)})
	}

	// 附加 candidate_display_name
	type hireDTO struct {
		models.JobHire
		CandidateDisplayName string `json:"candidate_display_name"`
	}
	out := make([]hireDTO, 0, len(items))
	for _, h := range items {
		first := strings.TrimSpace(h.CandidateName)
		last := strings.TrimSpace(h.CandidateLastName)
		display := first
		if last != "" {
			hasCJK := strings.ContainsFunc(first, func(r rune) bool { return r >= 0x3400 && r <= 0x9FFF }) ||
				strings.ContainsFunc(last, func(r rune) bool { return r >= 0x3400 && r <= 0x9FFF })
			if hasCJK {
				display = last + first
			} else {
				display = first + " " + last
			}
		}
		out = append(out, hireDTO{
			JobHire:               h,
			CandidateDisplayName: display,
		})
	}

	return c.JSON(fiber.Map{
		"data":  out,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetJobHire(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	var item models.JobHire
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Preload("Vacancy").Preload("Applicant").First(&item).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Not found"})
	}
	return c.JSON(item)
}

func CreateJobHire(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var req struct {
		VacancyID         *uuid.UUID `json:"vacancy_id"`
		ApplicantID       *uuid.UUID `json:"applicant_id"`
		CandidateName     string     `json:"candidate_name"`
		CandidateLastName string     `json:"candidate_last_name"`
		Email             string     `json:"email"`
		Phone             string     `json:"phone"`
		Status            string     `json:"status"`
		StartDate         string     `json:"start_date"`
		Notes             string     `json:"notes"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	req.CandidateName = strings.TrimSpace(req.CandidateName)
	req.CandidateLastName = strings.TrimSpace(req.CandidateLastName)

	// 若有 applicant_id 且未填候選人資料，從 applicant 帶入（連接聘請 DB）
	if req.ApplicantID != nil {
		var app models.JobApplicant
		if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, *req.ApplicantID).First(&app).Error; err == nil {
			if req.CandidateName == "" {
				req.CandidateName = strings.TrimSpace(app.CandidateName)
			}
			if req.CandidateLastName == "" {
				req.CandidateLastName = strings.TrimSpace(app.CandidateLastName)
			}
			if strings.TrimSpace(req.Email) == "" {
				req.Email = strings.TrimSpace(app.Email)
			}
			if strings.TrimSpace(req.Phone) == "" {
				req.Phone = strings.TrimSpace(app.Phone)
			}
			if req.VacancyID == nil && app.VacancyID != nil {
				req.VacancyID = app.VacancyID
			}
		}
	}

	if req.CandidateName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Candidate name is required"})
	}
	if req.Status == "" {
		req.Status = "applied"
	}
	switch req.Status {
	case "applied", "interview", "offered", "hired", "rejected":
	default:
		req.Status = "applied"
	}

	var startDatePtr *time.Time
	if strings.TrimSpace(req.StartDate) != "" {
		if t, err := time.Parse("2006-01-02", req.StartDate); err == nil {
			startDatePtr = &t
		}
	}

	item := models.JobHire{
		TenantID:          tenantID,
		VacancyID:         req.VacancyID,
		ApplicantID:       req.ApplicantID,
		CandidateName:     req.CandidateName,
		CandidateLastName: req.CandidateLastName,
		Email:             req.Email,
		Phone:             req.Phone,
		Status:            req.Status,
		StartDate:         startDatePtr,
		Notes:             req.Notes,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := database.DB.Create(&item).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create"})
	}
	return c.Status(201).JSON(item)
}

func UpdateJobHire(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	var item models.JobHire
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&item).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Not found"})
	}

	var req struct {
		VacancyID         *uuid.UUID `json:"vacancy_id"`
		ApplicantID       *uuid.UUID `json:"applicant_id"`
		CandidateName     *string    `json:"candidate_name"`
		CandidateLastName *string    `json:"candidate_last_name"`
		Email             *string    `json:"email"`
		Phone             *string    `json:"phone"`
		Status            *string    `json:"status"`
		StartDate         *string    `json:"start_date"`
		Notes             *string    `json:"notes"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.VacancyID != nil {
		item.VacancyID = req.VacancyID
	}
	if req.ApplicantID != nil {
		item.ApplicantID = req.ApplicantID
	}
	if req.CandidateName != nil {
		n := strings.TrimSpace(*req.CandidateName)
		if n != "" {
			item.CandidateName = n
		}
	}
	if req.CandidateLastName != nil {
		item.CandidateLastName = strings.TrimSpace(*req.CandidateLastName)
	}
	if req.Email != nil {
		item.Email = *req.Email
	}
	if req.Phone != nil {
		item.Phone = *req.Phone
	}
	if req.Status != nil {
		switch *req.Status {
		case "applied", "interview", "offered", "hired", "rejected":
			item.Status = *req.Status
		}
	}
	if req.StartDate != nil {
		s := strings.TrimSpace(*req.StartDate)
		if s == "" {
			item.StartDate = nil
		} else if t, err := time.Parse("2006-01-02", s); err == nil {
			item.StartDate = &t
		}
	}
	if req.Notes != nil {
		item.Notes = *req.Notes
	}
	item.UpdatedAt = time.Now()

	if err := database.DB.Save(&item).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update"})
	}
	return c.JSON(item)
}

func DeleteJobHire(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.JobHire{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete"})
	}
	return c.JSON(fiber.Map{"message": "Deleted"})
}


