package handlers

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"nwork/internal/models"
)

const (
	diningAssignModeKey            = "dining_table_assign_mode"
	diningReleaseModeKey           = "dining_table_release_mode"
	diningReleaseDelayKey          = "dining_table_release_delay_minutes"
	diningQueueRequirePhoneKey     = "dining_queue_require_phone"
	diningMenuCategoryKey          = "dining_menu_category"
	diningMenuOnlyMessageKey       = "dining_menu_only_message"
	diningAssignModeDefault        = "auto"   // auto | manual
	diningReleaseModeDefault       = "manual" // auto_pay | delay | manual
	diningReleaseDelayDefault      = 10
	diningQueueRequirePhoneDefault = "false"
	diningMenuCategoryDefault      = ""
	diningMenuOnlyMessageDefault   = "此頁面僅供查看菜單，如需點餐請掃描餐桌上的 QR Code"
)

func GetDiningSettings(c *fiber.Ctx) error {
	assignMode := strings.TrimSpace(models.GetSystemSetting(diningAssignModeKey, diningAssignModeDefault))
	releaseMode := strings.TrimSpace(models.GetSystemSetting(diningReleaseModeKey, diningReleaseModeDefault))
	delayStr := strings.TrimSpace(models.GetSystemSetting(diningReleaseDelayKey, strconv.Itoa(diningReleaseDelayDefault)))
	requirePhoneStr := strings.TrimSpace(models.GetSystemSetting(diningQueueRequirePhoneKey, diningQueueRequirePhoneDefault))
	menuCategory := strings.TrimSpace(models.GetSystemSetting(diningMenuCategoryKey, diningMenuCategoryDefault))
	menuOnlyMessage := strings.TrimSpace(models.GetSystemSetting(diningMenuOnlyMessageKey, diningMenuOnlyMessageDefault))
	delay, err := strconv.Atoi(delayStr)
	if err != nil || delay < 0 {
		delay = diningReleaseDelayDefault
	}
	requirePhone := strings.EqualFold(requirePhoneStr, "true") || requirePhoneStr == "1"

	return c.JSON(fiber.Map{
		"table_assign_mode":           assignMode,
		"table_release_mode":          releaseMode,
		"table_release_delay_minutes": delay,
		"queue_require_phone":         requirePhone,
		"menu_category":               menuCategory,
		"menu_only_message":           menuOnlyMessage,
	})
}

type diningSettingsReq struct {
	TableAssignMode       string `json:"table_assign_mode"`
	TableReleaseMode      string `json:"table_release_mode"`
	TableReleaseDelayMins int    `json:"table_release_delay_minutes"`
	QueueRequirePhone     bool   `json:"queue_require_phone"`
	MenuCategory          string `json:"menu_category"`
	MenuOnlyMessage       string `json:"menu_only_message"`
}

func UpdateDiningSettings(c *fiber.Ctx) error {
	var req diningSettingsReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	assignMode := strings.TrimSpace(req.TableAssignMode)
	releaseMode := strings.TrimSpace(req.TableReleaseMode)

	if assignMode != "auto" && assignMode != "manual" {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid table_assign_mode"})
	}
	if releaseMode != "auto_pay" && releaseMode != "delay" && releaseMode != "manual" {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid table_release_mode"})
	}

	delay := req.TableReleaseDelayMins
	if delay < 0 {
		delay = 0
	}

	if err := models.SetSystemSetting(diningAssignModeKey, assignMode, nil); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save dining settings"})
	}
	if err := models.SetSystemSetting(diningReleaseModeKey, releaseMode, nil); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save dining settings"})
	}
	if err := models.SetSystemSetting(diningReleaseDelayKey, strconv.Itoa(delay), nil); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save dining settings"})
	}
	requirePhoneValue := "false"
	if req.QueueRequirePhone {
		requirePhoneValue = "true"
	}
	if err := models.SetSystemSetting(diningQueueRequirePhoneKey, requirePhoneValue, nil); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save dining settings"})
	}
	if err := models.SetSystemSetting(diningMenuCategoryKey, strings.TrimSpace(req.MenuCategory), nil); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save dining settings"})
	}
	menuOnlyMsg := strings.TrimSpace(req.MenuOnlyMessage)
	if menuOnlyMsg == "" {
		menuOnlyMsg = diningMenuOnlyMessageDefault
	}
	if err := models.SetSystemSetting(diningMenuOnlyMessageKey, menuOnlyMsg, nil); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save dining settings"})
	}

	return c.JSON(fiber.Map{"message": "ok"})
}
