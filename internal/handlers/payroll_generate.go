package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GenerateCurrentMonthPayrolls 根據員工薪資自動生成當月薪資草稿
func GenerateCurrentMonthPayrolls(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	// 當月期間 YYYY-MM
	now := time.Now()
	payPeriod := now.Format("2006-01")

	// 計算當月第一天和最後一天
	loc := utils.GetTenantLocation(tenantID)
	nowInLoc := utils.NowInTenantTimezone(tenantID).In(loc)
	firstDay := time.Date(nowInLoc.Year(), nowInLoc.Month(), 1, 0, 0, 0, 0, loc)
	lastDay := time.Date(nowInLoc.Year(), nowInLoc.Month()+1, 0, 23, 59, 59, 0, loc)

	// 取所有 active 用戶且薪資>0
	var users []models.User
	if err := database.DB.Where("tenant_id = ? AND status = ? AND salary > 0", tenantID, "active").Find(&users).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch users"})
	}

	// 已存在的當月薪資，避免重複
	var existing []models.Payroll
	database.DB.Where("tenant_id = ? AND pay_period = ?", tenantID, payPeriod).Find(&existing)
	existingMap := make(map[uuid.UUID]bool)
	for _, p := range existing {
		existingMap[p.UserID] = true
	}

	created := 0
	monthlyCount := 0
	hourlyCount := 0
	withSalaryCount := 0

	for _, u := range users {
		if existingMap[u.ID] {
			continue
		}

		withSalaryCount++

		var base float64
		var otHours float64
		var otAmount float64

		// 根據薪資方式計算
		salaryMode := u.SalaryMode
		if salaryMode == "" {
			salaryMode = "monthly" // 默認月薪
		}

		if salaryMode == "hourly" {
			// 時薪：根據打卡記錄計算
			hourlyRate := u.Salary
			var attendances []models.Attendance
			database.DB.Where("tenant_id = ? AND user_id = ? AND date >= ? AND date <= ?",
				tenantID, u.ID, firstDay, lastDay).Find(&attendances)

			totalWorkMinutes := 0
			totalOTMinutes := 0

			for _, att := range attendances {
				if att.WorkDuration > 0 {
					totalWorkMinutes += att.WorkDuration
				}
				if att.OTDuration > 0 {
					totalOTMinutes += att.OTDuration
				}
			}

			// 轉換為小時
			workHours := float64(totalWorkMinutes) / 60.0
			otHours = float64(totalOTMinutes) / 60.0

			// 計算基本薪資（工作時數 * 時薪）
			base = workHours * hourlyRate
			otRate := 1.5
			otAmount = otHours * hourlyRate * otRate

			hourlyCount++
		} else {
			// 月薪：使用固定薪資
			base = u.Salary
			otHours = 0.0
			otRate := 1.5
			otAmount = otHours * (base / 160) * otRate

			monthlyCount++
		}

		allowances := 0.0
		deductions := 0.0
		empRate := 0.05
		emprRate := 0.05

		gross := base + otAmount + allowances
		mpfBase := gross
		if mpfBase > 30000 {
			mpfBase = 30000
		}
		mpfEmp := mpfBase * empRate
		mpfEmpr := mpfBase * emprRate
		mpfTotal := mpfEmp + mpfEmpr
		net := gross - mpfEmp - deductions

		payroll := models.Payroll{
			TenantID:               tenantID,
			UserID:                 u.ID,
			PayPeriod:              payPeriod,
			BaseSalary:             base,
			OTHours:                otHours,
			OTAmount:               otAmount,
			Allowances:             allowances,
			Deductions:             deductions,
			EmployeeMandatoryRate:  empRate,
			EmployerMandatoryRate:  emprRate,
			MPFEmployee:            mpfEmp,
			MPFEmployer:            mpfEmpr,
			MPFTotal:               mpfTotal,
			GrossSalary:            gross,
			NetSalary:              net,
			Status:                 "draft",
			CreatedAt:              now,
			UpdatedAt:              now,
		}
		_ = database.DB.Create(&payroll).Error
		created++
	}

	return c.JSON(fiber.Map{
		"message":         "generated",
		"created":         created,
		"pay_period":      payPeriod,
		"with_salary":     withSalaryCount,
		"monthly":         monthlyCount,
		"hourly":          hourlyCount,
	})
}


