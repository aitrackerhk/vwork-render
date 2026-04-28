package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ============================================
// 打卡記錄 (Attendance) CRUD
// ============================================

func GetAttendances(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	
	var attendances []models.Attendance
	query := database.DB.Where("tenant_id = ?", tenantID)
	
	// 過濾用戶
	if userID := c.Query("user_id"); userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	
	// 過濾日期範圍
	if startDate := c.Query("start_date"); startDate != "" {
		query = query.Where("date >= ?", startDate)
	}
	if endDate := c.Query("end_date"); endDate != "" {
		query = query.Where("date <= ?", endDate)
	}
	
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit
	
	var total int64
	query.Model(&models.Attendance{}).Count(&total)
	
	if err := query.Preload("User").Order("date DESC, clock_in DESC").
		Offset(offset).Limit(limit).Find(&attendances).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	
	return c.JSON(fiber.Map{
		"data":  attendances,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// ============================================
// 打卡報告（統計）
// ============================================

type AttendanceReportRow struct {
	ID               uuid.UUID `json:"id"`
	UserID           uuid.UUID `json:"user_id"`
	UserName         string    `json:"user_name"`
	TotalDays        int64     `json:"total_days"`
	NormalDays       int64     `json:"normal_days"`
	LateDays         int64     `json:"late_days"`
	EarlyLeaveDays   int64     `json:"early_leave_days"`
	AbsentDays       int64     `json:"absent_days"`
	TotalWorkMinutes int64     `json:"total_work_minutes"`
	TotalOTMinutes   int64     `json:"total_ot_minutes"`
	StartDate        string    `json:"start_date"`
	EndDate          string    `json:"end_date"`
}

// GetAttendanceReports 回傳指定日期範圍內、按員工彙總的打卡統計
//
// Query:
// - start_date: YYYY-MM-DD（預設：本月第一天）
// - end_date: YYYY-MM-DD（預設：本月最後一天）
// - user_id: UUID（可選）
// - search: 員工名稱關鍵字（可選）
// - page/limit
func GetAttendanceReports(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	offset := (page - 1) * limit

	// 日期範圍：預設本月
	loc := utils.GetTenantLocation(tenantID)
	now := utils.NowInTenantTimezone(tenantID).In(loc)
	defaultStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc).Format("2006-01-02")
	defaultEnd := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, loc).Format("2006-01-02")

	startDateStr := c.Query("start_date", defaultStart)
	endDateStr := c.Query("end_date", defaultEnd)

	startDate, err := utils.ParseDateInTenantTimezone(tenantID, startDateStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid start_date"})
	}
	endDate, err := utils.ParseDateInTenantTimezone(tenantID, endDateStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid end_date"})
	}

	userID := c.Query("user_id")
	search := c.Query("search")

	// 統計 query
	base := database.DB.Table("attendances a").
		Select(`
			a.user_id as user_id,
			u.name as user_name,
			COUNT(*) as total_days,
			COALESCE(SUM(CASE WHEN a.status = 'normal' THEN 1 ELSE 0 END), 0) as normal_days,
			COALESCE(SUM(CASE WHEN a.status = 'late' THEN 1 ELSE 0 END), 0) as late_days,
			COALESCE(SUM(CASE WHEN a.status = 'early_leave' THEN 1 ELSE 0 END), 0) as early_leave_days,
			COALESCE(SUM(CASE WHEN a.status = 'absent' THEN 1 ELSE 0 END), 0) as absent_days,
			COALESCE(SUM(a.work_duration), 0) as total_work_minutes,
			COALESCE(SUM(a.ot_duration), 0) as total_ot_minutes
		`).
		Joins("JOIN users u ON u.id = a.user_id").
		Where("a.tenant_id = ? AND a.date >= ? AND a.date <= ?", tenantID, startDate, endDate)

	if userID != "" {
		base = base.Where("a.user_id = ?", userID)
	}
	if search != "" {
		base = base.Where("u.name ILIKE ?", "%"+search+"%")
	}

	// total：符合條件的 distinct user 數
	var total int64
	countQ := database.DB.Table("attendances a").
		Select("COUNT(DISTINCT a.user_id)").
		Joins("JOIN users u ON u.id = a.user_id").
		Where("a.tenant_id = ? AND a.date >= ? AND a.date <= ?", tenantID, startDate, endDate)
	if userID != "" {
		countQ = countQ.Where("a.user_id = ?", userID)
	}
	if search != "" {
		countQ = countQ.Where("u.name ILIKE ?", "%"+search+"%")
	}
	if err := countQ.Count(&total).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	type rowScan struct {
		UserID           uuid.UUID `gorm:"column:user_id"`
		UserName         string    `gorm:"column:user_name"`
		TotalDays        int64     `gorm:"column:total_days"`
		NormalDays       int64     `gorm:"column:normal_days"`
		LateDays         int64     `gorm:"column:late_days"`
		EarlyLeaveDays   int64     `gorm:"column:early_leave_days"`
		AbsentDays       int64     `gorm:"column:absent_days"`
		TotalWorkMinutes int64     `gorm:"column:total_work_minutes"`
		TotalOTMinutes   int64     `gorm:"column:total_ot_minutes"`
	}

	var rows []rowScan
	if err := base.Group("a.user_id, u.name").
		Order("u.name ASC").
		Offset(offset).Limit(limit).
		Scan(&rows).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	out := make([]AttendanceReportRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, AttendanceReportRow{
			ID:               r.UserID, // 給 dynamic-list 當 row id
			UserID:           r.UserID,
			UserName:         r.UserName,
			TotalDays:        r.TotalDays,
			NormalDays:       r.NormalDays,
			LateDays:         r.LateDays,
			EarlyLeaveDays:   r.EarlyLeaveDays,
			AbsentDays:       r.AbsentDays,
			TotalWorkMinutes: r.TotalWorkMinutes,
			TotalOTMinutes:   r.TotalOTMinutes,
			StartDate:        startDateStr,
			EndDate:          endDateStr,
		})
	}

	return c.JSON(fiber.Map{
		"data":  out,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetAttendance(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var attendance models.Attendance
	
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).
		Preload("User").First(&attendance).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Attendance not found"})
	}
	
	return c.JSON(attendance)
}

func ClockIn(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	
	// 使用 UTC 時間，與消息時間保持一致
	now := time.Now()
	today := utils.TodayInTenantTimezone(tenantID)
	
	// 檢查今天是否已打卡
	var attendance models.Attendance
	if err := database.DB.Where("tenant_id = ? AND user_id = ? AND date = ?", 
		tenantID, userID, today).First(&attendance).Error; err == nil {
		if attendance.ClockIn != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Already clocked in today"})
		}
		// 更新打卡時間
		attendance.ClockIn = &now
		attendance.Status = "normal"
		if err := database.DB.Save(&attendance).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to clock in"})
		}
		return c.JSON(attendance)
	}
	
	// 創建新記錄
	todayDate, _ := utils.ParseDateInTenantTimezone(tenantID, today)
	attendance = models.Attendance{
		TenantID:  tenantID,
		UserID:    userID,
		Date:      todayDate,
		ClockIn:   &now,
		Status:    "normal",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	
	if err := database.DB.Create(&attendance).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to clock in"})
	}
	
	return c.Status(201).JSON(attendance)
}

func ClockOut(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	
	// 使用 UTC 時間，與消息時間保持一致
	now := time.Now()
	today := utils.TodayInTenantTimezone(tenantID)
	
	var attendance models.Attendance
	if err := database.DB.Where("tenant_id = ? AND user_id = ? AND date = ?", 
		tenantID, userID, today).First(&attendance).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Please clock in first"})
	}
	
	if attendance.ClockOut != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Already clocked out today"})
	}
	
	attendance.ClockOut = &now
	
	// 計算工作時長（兩個時間都是 UTC，直接計算）
	if attendance.ClockIn != nil {
		duration := now.Sub(*attendance.ClockIn)
		attendance.WorkDuration = int(duration.Minutes()) - attendance.BreakDuration
		
		// 計算加班（超過8小時的部分）
		if attendance.WorkDuration > 480 { // 8小時 = 480分鐘
			attendance.OTDuration = attendance.WorkDuration - 480
		}
	}
	
	attendance.UpdatedAt = time.Now()
	
	if err := database.DB.Save(&attendance).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to clock out"})
	}
	
	return c.JSON(attendance)
}

func UpdateAttendance(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var attendance models.Attendance
	
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&attendance).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Attendance not found"})
	}
	
	var req struct {
		ClockIn       *string `json:"clock_in"`
		ClockOut      *string `json:"clock_out"`
		BreakDuration int     `json:"break_duration"`
		Status        string  `json:"status"`
		Notes         string  `json:"notes"`
	}
	
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	
	if req.ClockIn != nil {
		clockIn, err := utils.ParseDateTimeInTenantTimezone(tenantID, *req.ClockIn)
		if err == nil {
		attendance.ClockIn = &clockIn
		}
	}
	if req.ClockOut != nil {
		clockOut, err := utils.ParseDateTimeInTenantTimezone(tenantID, *req.ClockOut)
		if err == nil {
		attendance.ClockOut = &clockOut
		}
		
		// 重新計算工作時長
		if attendance.ClockIn != nil {
			duration := clockOut.Sub(*attendance.ClockIn)
			attendance.WorkDuration = int(duration.Minutes()) - attendance.BreakDuration
			if attendance.WorkDuration > 480 {
				attendance.OTDuration = attendance.WorkDuration - 480
			}
		}
	}
	
	attendance.BreakDuration = req.BreakDuration
	if req.Status != "" {
		attendance.Status = req.Status
	}
	attendance.Notes = req.Notes
	attendance.UpdatedAt = time.Now()
	
	if err := database.DB.Save(&attendance).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update attendance"})
	}
	
	return c.JSON(attendance)
}

// ============================================
// 請假申請 (Leave Request) CRUD
// ============================================

func GetLeaveRequests(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	
	var requests []models.LeaveRequest
	query := database.DB.Where("tenant_id = ?", tenantID)
	
	// 過濾用戶
	if userID := c.Query("user_id"); userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	
	// 過濾狀態
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit
	
	var total int64
	query.Model(&models.LeaveRequest{}).Count(&total)
	
	if err := query.Preload("User").Preload("Approver").
		Order("created_at DESC").Offset(offset).Limit(limit).Find(&requests).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	
	return c.JSON(fiber.Map{
		"data":  requests,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func CreateLeaveRequest(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	
	var req struct {
		LeaveType string `json:"leave_type"`
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
		Reason    string `json:"reason"`
	}
	
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	
	startDate, err := utils.ParseDateInTenantTimezone(tenantID, req.StartDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid start date"})
	}
	
	endDate, err := utils.ParseDateInTenantTimezone(tenantID, req.EndDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid end date"})
	}
	
	if endDate.Before(startDate) {
		return c.Status(400).JSON(fiber.Map{"error": "End date must be after start date"})
	}
	
	days := endDate.Sub(startDate).Hours()/24 + 1
	
	leaveRequest := models.LeaveRequest{
		TenantID:  tenantID,
		UserID:    userID,
		LeaveType: req.LeaveType,
		StartDate: startDate,
		EndDate:   endDate,
		Days:      days,
		Reason:    req.Reason,
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	
	if err := database.DB.Create(&leaveRequest).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create leave request"})
	}
	
	return c.Status(201).JSON(leaveRequest)
}

func ApproveLeaveRequest(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	approverID := middleware.GetUserID(c)
	
	var request models.LeaveRequest
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&request).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Leave request not found"})
	}
	
	now := time.Now()
	request.Status = "approved"
	request.ApprovedBy = &approverID
	request.ApprovedAt = &now
	request.UpdatedAt = now
	
	if err := database.DB.Save(&request).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to approve leave request"})
	}
	
	return c.JSON(request)
}

func RejectLeaveRequest(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	
	var req struct {
		RejectReason string `json:"reject_reason"`
	}
	
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	
	var request models.LeaveRequest
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&request).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Leave request not found"})
	}
	
	request.Status = "rejected"
	request.RejectReason = req.RejectReason
	request.UpdatedAt = time.Now()
	
	if err := database.DB.Save(&request).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to reject leave request"})
	}
	
	return c.JSON(request)
}

func GetLeaveRequest(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var request models.LeaveRequest
	
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).
		Preload("User").Preload("Approver").First(&request).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Leave request not found"})
	}
	
	return c.JSON(request)
}

func UpdateLeaveRequest(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var request models.LeaveRequest
	
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&request).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Leave request not found"})
	}
	
	var req struct {
		LeaveType string `json:"leave_type"`
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
		Reason    string `json:"reason"`
		Status    string `json:"status"`
	}
	
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	
	if req.LeaveType != "" {
		request.LeaveType = req.LeaveType
	}
	
	if req.StartDate != "" {
		startDate, err := utils.ParseDateInTenantTimezone(tenantID, req.StartDate)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid start date"})
		}
		request.StartDate = startDate
	}
	
	if req.EndDate != "" {
		endDate, err := utils.ParseDateInTenantTimezone(tenantID, req.EndDate)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid end date"})
		}
		request.EndDate = endDate
		
		// 重新計算天數
		if !request.StartDate.IsZero() {
			if endDate.Before(request.StartDate) {
				return c.Status(400).JSON(fiber.Map{"error": "End date must be after start date"})
			}
			request.Days = endDate.Sub(request.StartDate).Hours()/24 + 1
		}
	}
	
	if req.Reason != "" {
		request.Reason = req.Reason
	}
	
	if req.Status != "" {
		request.Status = req.Status
	}
	
	request.UpdatedAt = time.Now()
	
	if err := database.DB.Save(&request).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update leave request"})
	}
	
	return c.JSON(request)
}

// ============================================
// 薪資記錄 (Payroll) CRUD
// ============================================

func GetPayrolls(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	
	var payrolls []models.Payroll
	query := database.DB.Where("tenant_id = ?", tenantID)
	
	// 過濾用戶
	if userID := c.Query("user_id"); userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	
	// 過濾期間
	if payPeriod := c.Query("pay_period"); payPeriod != "" {
		query = query.Where("pay_period = ?", payPeriod)
	}
	
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit
	
	var total int64
	query.Model(&models.Payroll{}).Count(&total)
	
	if err := query.Preload("User").
		Preload("Contributions", "tenant_id = ?", tenantID).
		Order("pay_period DESC").
		Offset(offset).Limit(limit).Find(&payrolls).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	
	return c.JSON(fiber.Map{
		"data":  payrolls,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetPayroll(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var payroll models.Payroll

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).
		Preload("User").
		Preload("Contributions", "tenant_id = ?", tenantID).
		Preload("Adjustments", "tenant_id = ?", tenantID).
		First(&payroll).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Payroll not found"})
	}

	return c.JSON(payroll)
}

func DeletePayroll(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	// 先確認存在且屬於租戶
	var payroll models.Payroll
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&payroll).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Payroll not found"})
	}

	// 刪除供款明細（若有）
	database.DB.Where("tenant_id = ? AND payroll_id = ?", tenantID, payroll.ID).Delete(&models.PayrollContribution{})

	// 刪除 payroll
	if err := database.DB.Delete(&payroll).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete payroll"})
	}

	return c.JSON(fiber.Map{"message": "Payroll deleted successfully"})
}

func CreatePayroll(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	
	var req struct {
		UserID      uuid.UUID `json:"user_id"`
		PayPeriod   string    `json:"pay_period"`
		BaseSalary  float64   `json:"base_salary"`
		OTHours     float64   `json:"ot_hours"`
		OTRate      float64   `json:"ot_rate"` // 加班時薪倍數（如1.5）
		Allowances  float64   `json:"allowances"`
		Deductions  float64   `json:"deductions"`
		EmployeeMandatoryRate float64 `json:"employee_mandatory_rate"` // 員工強制供款 %
		EmployerMandatoryRate float64 `json:"employer_mandatory_rate"` // 雇主強制供款 %
		Adjustments []struct {
			Name      string  `json:"name"`
			Direction string  `json:"direction"` // add/subtract
			Mode      string  `json:"mode"`      // percent/fixed
			Rate      float64 `json:"rate"`
			Amount    float64 `json:"amount"`
		} `json:"adjustments"`
		Contributions []struct {
			Name   string  `json:"name"`   // 款項名稱
			Payer  string  `json:"payer"`  // employee, employer
			Mode   string  `json:"mode"`   // percent, fixed
			Rate   float64 `json:"rate"`
			Amount float64 `json:"amount"`
		} `json:"contributions"`
		Notes       string    `json:"notes"`
	}
	
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	
	if req.OTRate == 0 {
		req.OTRate = 1.5 // 默認1.5倍
	}
	// 默認供款 5%
	if req.EmployeeMandatoryRate == 0 && req.EmployerMandatoryRate == 0 {
		req.EmployeeMandatoryRate = 0.05
		req.EmployerMandatoryRate = 0.05
	}
	
	// 計算加班費
	otAmount := req.OTHours * (req.BaseSalary / 160) * req.OTRate // 假設月薪160小時
	
	// 計算附加項目（多 row）
	adjAdd := 0.0
	adjSub := 0.0
	for _, a := range req.Adjustments {
		mode := a.Mode
		if mode != "percent" && mode != "fixed" {
			mode = "fixed"
		}
		dir := a.Direction
		if dir != "add" && dir != "subtract" {
			dir = "add"
		}
		val := 0.0
		if mode == "percent" {
			val = req.BaseSalary * a.Rate
		} else {
			val = a.Amount
		}
		if dir == "subtract" {
			adjSub += val
		} else {
			adjAdd += val
		}
	}

	// 計算總薪金（津貼/扣除不再單獨輸入，全部由附加項目決定）
	allowancesTotal := adjAdd
	deductionsTotal := adjSub
	grossSalary := req.BaseSalary + otAmount + allowancesTotal
	
	// 計算 MPF（基於總薪金，但上限為$30,000）
	mpfBase := grossSalary
	if mpfBase > 30000 {
		mpfBase = 30000
	}
	mpfEmployee := mpfBase * req.EmployeeMandatoryRate
	mpfEmployer := mpfBase * req.EmployerMandatoryRate
	mpfTotal := mpfEmployee + mpfEmployer

	// 計算自定義供款
	employeeExtra := 0.0
	employerExtra := 0.0
	for _, citem := range req.Contributions {
		val := 0.0
		if citem.Mode == "percent" {
			val = grossSalary * citem.Rate
		} else {
			val = citem.Amount
		}
		if citem.Payer == "employer" {
			employerExtra += val
		} else {
			employeeExtra += val
		}
	}
	
	// 計算淨薪金
	netSalary := grossSalary - mpfEmployee - deductionsTotal - employeeExtra
	
	payroll := models.Payroll{
		TenantID:    tenantID,
		UserID:      req.UserID,
		PayPeriod:   req.PayPeriod,
		BaseSalary:  req.BaseSalary,
		OTHours:     req.OTHours,
		OTAmount:    otAmount,
		MPFEmployee: mpfEmployee,
		MPFEmployer: mpfEmployer,
		MPFTotal:    mpfTotal,
		EmployeeMandatoryRate: req.EmployeeMandatoryRate,
		EmployerMandatoryRate: req.EmployerMandatoryRate,
		Allowances:  allowancesTotal,
		Deductions:  deductionsTotal,
		GrossSalary: grossSalary,
		NetSalary:   netSalary,
		Status:      "draft",
		Notes:       req.Notes,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Contributions: []models.PayrollContribution{},
		Adjustments:   []models.PayrollAdjustment{},
	}
	
	if err := database.DB.Create(&payroll).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create payroll"})
	}

	// 保存供款明細
	for _, citem := range req.Contributions {
		contrib := models.PayrollContribution{
			TenantID:  tenantID,
			PayrollID: payroll.ID,
			Name:      citem.Name,
			Payer:     citem.Payer,
			Mode:      citem.Mode,
			Rate:      citem.Rate,
			Amount:    citem.Amount,
			CreatedAt: time.Now(),
		}
		database.DB.Create(&contrib)
	}

	// 保存附加項目
	for _, a := range req.Adjustments {
		mode := a.Mode
		if mode != "percent" && mode != "fixed" {
			mode = "fixed"
		}
		dir := a.Direction
		if dir != "add" && dir != "subtract" {
			dir = "add"
		}
		adj := models.PayrollAdjustment{
			TenantID:  tenantID,
			PayrollID: payroll.ID,
			Name:      a.Name,
			Direction: dir,
			Mode:      mode,
			Rate:      a.Rate,
			Amount:    a.Amount,
			CreatedAt: time.Now(),
		}
		database.DB.Create(&adj)
	}
	
	return c.Status(201).JSON(payroll)
}

func UpdatePayroll(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var payroll models.Payroll
	
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).
		Preload("Contributions", "tenant_id = ?", tenantID).
		Preload("Adjustments", "tenant_id = ?", tenantID).
		First(&payroll).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Payroll not found"})
	}
	
	var req struct {
		BaseSalary  float64 `json:"base_salary"`
		OTHours     float64 `json:"ot_hours"`
		OTRate      float64 `json:"ot_rate"`
		Allowances  float64 `json:"allowances"`
		Deductions  float64 `json:"deductions"`
		EmployeeMandatoryRate float64 `json:"employee_mandatory_rate"`
		EmployerMandatoryRate float64 `json:"employer_mandatory_rate"`
		Adjustments []struct {
			Name      string  `json:"name"`
			Direction string  `json:"direction"` // add/subtract
			Mode      string  `json:"mode"`      // percent/fixed
			Rate      float64 `json:"rate"`
			Amount    float64 `json:"amount"`
		} `json:"adjustments"`
		Contributions []struct {
			Name   string  `json:"name"`   // 款項名稱
			Payer  string  `json:"payer"`  // employee, employer
			Mode   string  `json:"mode"`   // percent, fixed
			Rate   float64 `json:"rate"`
			Amount float64 `json:"amount"`
		} `json:"contributions"`
		Status      string  `json:"status"`
		Notes       string  `json:"notes"`
	}
	
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	
	if req.OTRate == 0 {
		req.OTRate = 1.5
	}
	if req.EmployeeMandatoryRate == 0 && req.EmployerMandatoryRate == 0 {
		req.EmployeeMandatoryRate = 0.05
		req.EmployerMandatoryRate = 0.05
	}
	
	// 重新計算
	otAmount := req.OTHours * (req.BaseSalary / 160) * req.OTRate
	adjAdd := 0.0
	adjSub := 0.0
	for _, a := range req.Adjustments {
		mode := a.Mode
		if mode != "percent" && mode != "fixed" {
			mode = "fixed"
		}
		dir := a.Direction
		if dir != "add" && dir != "subtract" {
			dir = "add"
		}
		val := 0.0
		if mode == "percent" {
			val = req.BaseSalary * a.Rate
		} else {
			val = a.Amount
		}
		if dir == "subtract" {
			adjSub += val
		} else {
			adjAdd += val
		}
	}
	// 計算總薪金（津貼/扣除不再單獨輸入，全部由附加項目決定）
	allowancesTotal := adjAdd
	deductionsTotal := adjSub
	grossSalary := req.BaseSalary + otAmount + allowancesTotal
	
	mpfBase := grossSalary
	if mpfBase > 30000 {
		mpfBase = 30000
	}
	mpfEmployee := mpfBase * req.EmployeeMandatoryRate
	mpfEmployer := mpfBase * req.EmployerMandatoryRate
	mpfTotal := mpfEmployee + mpfEmployer

	employeeExtra := 0.0
	employerExtra := 0.0
	for _, citem := range req.Contributions {
		val := 0.0
		if citem.Mode == "percent" {
			val = grossSalary * citem.Rate
		} else {
			val = citem.Amount
		}
		if citem.Payer == "employer" {
			employerExtra += val
		} else {
			employeeExtra += val
		}
	}
	netSalary := grossSalary - mpfEmployee - deductionsTotal
	netSalary -= employeeExtra
	
	payroll.BaseSalary = req.BaseSalary
	payroll.OTHours = req.OTHours
	payroll.OTAmount = otAmount
	payroll.MPFEmployee = mpfEmployee
	payroll.MPFEmployer = mpfEmployer
	payroll.MPFTotal = mpfTotal
	payroll.EmployeeMandatoryRate = req.EmployeeMandatoryRate
	payroll.EmployerMandatoryRate = req.EmployerMandatoryRate
	payroll.Allowances = allowancesTotal
	payroll.Deductions = deductionsTotal
	payroll.GrossSalary = grossSalary
	payroll.NetSalary = netSalary
	if req.Status != "" {
		payroll.Status = req.Status
	}
	payroll.Notes = req.Notes
	payroll.UpdatedAt = time.Now()
	
	if err := database.DB.Save(&payroll).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update payroll"})
	}

	// 重建供款明細
	database.DB.Where("payroll_id = ?", payroll.ID).Delete(&models.PayrollContribution{})
	for _, citem := range req.Contributions {
		contrib := models.PayrollContribution{
			TenantID:  tenantID,
			PayrollID: payroll.ID,
			Name:      citem.Name,
			Payer:     citem.Payer,
			Mode:      citem.Mode,
			Rate:      citem.Rate,
			Amount:    citem.Amount,
			CreatedAt: time.Now(),
		}
		database.DB.Create(&contrib)
	}

	// 重建附加項目
	database.DB.Where("payroll_id = ?", payroll.ID).Delete(&models.PayrollAdjustment{})
	for _, a := range req.Adjustments {
		mode := a.Mode
		if mode != "percent" && mode != "fixed" {
			mode = "fixed"
		}
		dir := a.Direction
		if dir != "add" && dir != "subtract" {
			dir = "add"
		}
		adj := models.PayrollAdjustment{
			TenantID:  tenantID,
			PayrollID: payroll.ID,
			Name:      a.Name,
			Direction: dir,
			Mode:      mode,
			Rate:      a.Rate,
			Amount:    a.Amount,
			CreatedAt: time.Now(),
		}
		database.DB.Create(&adj)
	}
	
	return c.JSON(payroll)
}

// ============================================
// 假期 (Holiday) CRUD
// ============================================

func GetHolidays(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	
	var holidays []models.Holiday
	query := database.DB.Where("tenant_id = ?", tenantID)
	
	// 過濾狀態
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	
	// 過濾日期範圍
	if startDate := c.Query("start_date"); startDate != "" {
		query = query.Where("start_date >= ? OR end_date >= ?", startDate, startDate)
	}
	if endDate := c.Query("end_date"); endDate != "" {
		query = query.Where("start_date <= ? OR end_date <= ?", endDate, endDate)
	}
	
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset := (page - 1) * limit
	
	var total int64
	query.Model(&models.Holiday{}).Count(&total)
	
	if err := query.Order("start_date ASC").
		Offset(offset).Limit(limit).Find(&holidays).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	
	return c.JSON(fiber.Map{
		"data":  holidays,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetHoliday(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	
	var holiday models.Holiday
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&holiday).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Holiday not found"})
	}
	
	return c.JSON(holiday)
}

func CreateHoliday(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	
	var req struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		StartDate    string `json:"start_date"`
		EndDate      string `json:"end_date"`
		IsRecurring  interface{} `json:"is_recurring"` // 可能是 bool, string "true"/"false", 或 "yes"/"no"
		Status       string `json:"status"`
	}
	
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	
	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}
	
	startDate, err := utils.ParseDateInTenantTimezone(tenantID, req.StartDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid start date"})
	}
	
	endDate, err := utils.ParseDateInTenantTimezone(tenantID, req.EndDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid end date"})
	}
	
	if endDate.Before(startDate) {
		return c.Status(400).JSON(fiber.Map{"error": "End date must be after or equal to start date"})
	}
	
	if req.Status == "" {
		req.Status = "active"
	}
	
	// 處理 IsRecurring（支持多種格式）
	var isRecurring bool
	switch v := req.IsRecurring.(type) {
	case bool:
		isRecurring = v
	case string:
		isRecurring = v == "true" || v == "yes" || v == "1"
	case nil:
		isRecurring = false
	default:
		isRecurring = false
	}
	
	now := utils.NowInTenantTimezone(tenantID)
	holiday := models.Holiday{
		TenantID:     tenantID,
		Name:         req.Name,
		Description:  req.Description,
		StartDate:    startDate,
		EndDate:      endDate,
		IsRecurring:  isRecurring,
		RecurringRule: "", // 不再使用
		Status:       req.Status,
		CreatedBy:    &userID,
		UpdatedBy:    &userID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	
	if err := database.DB.Create(&holiday).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create holiday: " + err.Error()})
	}
	
	return c.Status(201).JSON(holiday)
}

func UpdateHoliday(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")
	
	var holiday models.Holiday
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&holiday).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Holiday not found"})
	}
	
	var req struct {
		Name         *string `json:"name"`
		Description  *string `json:"description"`
		StartDate    *string `json:"start_date"`
		EndDate      *string `json:"end_date"`
		IsRecurring  interface{} `json:"is_recurring"` // 可能是 bool, string "true"/"false", 或 "yes"/"no"
		Status       *string `json:"status"`
	}
	
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	
	if req.Name != nil {
		holiday.Name = *req.Name
	}
	if req.Description != nil {
		holiday.Description = *req.Description
	}
	if req.StartDate != nil {
		startDate, err := utils.ParseDateInTenantTimezone(tenantID, *req.StartDate)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid start date"})
		}
		holiday.StartDate = startDate
	}
	if req.EndDate != nil {
		endDate, err := utils.ParseDateInTenantTimezone(tenantID, *req.EndDate)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid end date"})
		}
		holiday.EndDate = endDate
	}
	if req.IsRecurring != nil {
		// 處理 IsRecurring（支持多種格式）
		switch v := req.IsRecurring.(type) {
		case bool:
			holiday.IsRecurring = v
		case string:
			holiday.IsRecurring = v == "true" || v == "yes" || v == "1"
		default:
			holiday.IsRecurring = false
		}
		holiday.RecurringRule = "" // 不再使用
	}
	if req.Status != nil {
		holiday.Status = *req.Status
	}
	
	// 驗證日期
	if holiday.EndDate.Before(holiday.StartDate) {
		return c.Status(400).JSON(fiber.Map{"error": "End date must be after or equal to start date"})
	}
	
	holiday.UpdatedBy = &userID
	holiday.UpdatedAt = time.Now()
	
	if err := database.DB.Save(&holiday).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update holiday: " + err.Error()})
	}
	
	return c.JSON(holiday)
}

func DeleteHoliday(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.Holiday{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	
	return c.JSON(fiber.Map{"message": "Holiday deleted"})
}

