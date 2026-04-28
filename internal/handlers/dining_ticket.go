package handlers

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"nwork/internal/database"
	"nwork/internal/models"
)

func buildDiningQueueTicketPrefix(areaCode string, now time.Time) string {
	code := strings.TrimSpace(areaCode)
	if code == "" {
		code = "X"
	}
	return fmt.Sprintf("QUE-%s-%s-", now.Format("20060102"), code)
}

func parseDiningQueueTicketSeq(ticket string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(ticket), "-")
	if len(parts) < 2 {
		return 0, false
	}
	seq, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil || seq <= 0 {
		return 0, false
	}
	return seq, true
}

func getDiningAreaCode(tenantID uuid.UUID, areaID *uuid.UUID) string {
	if areaID == nil {
		return "X"
	}
	var area models.DiningArea
	if err := database.DB.Select("code").Where("id = ? AND tenant_id = ?", *areaID, tenantID).First(&area).Error; err != nil {
		return "X"
	}
	code := strings.TrimSpace(area.Code)
	if code == "" {
		return "X"
	}
	return code
}

func isDiningQueueTicketInUse(tenantID uuid.UUID, ticket string) bool {
	ticket = strings.TrimSpace(ticket)
	if ticket == "" {
		return false
	}
	var count int64
	database.DB.Model(&models.DiningQueue{}).
		Where("tenant_id = ? AND ticket_number = ?", tenantID, ticket).
		Count(&count)
	if count > 0 {
		return true
	}
	database.DB.Model(&models.ReservedNumber{}).
		Where("tenant_id = ? AND field_name = ? AND field_value = ?", tenantID, "ticket_number", ticket).
		Count(&count)
	return count > 0
}

func loadDiningQueueUsedSeqs(tenantID uuid.UUID, storeID *uuid.UUID, prefix string, dayStart, dayEnd time.Time, includeReserved bool) (map[int]bool, error) {
	used := map[int]bool{}

	queueQuery := database.DB.Model(&models.DiningQueue{}).
		Select("ticket_number").
		Where("tenant_id = ? AND created_at >= ? AND created_at < ? AND ticket_number LIKE ?", tenantID, dayStart, dayEnd, prefix+"%")
	if storeID != nil {
		queueQuery = queueQuery.Where("store_id = ?", *storeID)
	}

	var tickets []string
	if err := queueQuery.Pluck("ticket_number", &tickets).Error; err != nil {
		return used, err
	}
	for _, ticket := range tickets {
		if seq, ok := parseDiningQueueTicketSeq(ticket); ok {
			used[seq] = true
		}
	}

	if includeReserved {
		var reserved []string
		if err := database.DB.Model(&models.ReservedNumber{}).
			Select("field_value").
			Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?", tenantID, "ticket_number", prefix+"%").
			Pluck("field_value", &reserved).Error; err != nil {
			return used, err
		}
		for _, ticket := range reserved {
			if seq, ok := parseDiningQueueTicketSeq(ticket); ok {
				used[seq] = true
			}
		}
	}

	return used, nil
}

func findNextAvailableSeq(used map[int]bool) int {
	if len(used) == 0 {
		return 1
	}
	maxSeq := 0
	for seq := range used {
		if seq > maxSeq {
			maxSeq = seq
		}
	}
	for i := 1; i <= maxSeq; i++ {
		if !used[i] {
			return i
		}
	}
	return maxSeq + 1
}

func nextDiningQueueTicket(tenantID uuid.UUID, storeID *uuid.UUID, areaID *uuid.UUID, now time.Time, includeReserved bool) (string, int, error) {
	areaCode := getDiningAreaCode(tenantID, areaID)
	prefix := buildDiningQueueTicketPrefix(areaCode, now)

	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dayEnd := dayStart.Add(24 * time.Hour)

	used, err := loadDiningQueueUsedSeqs(tenantID, storeID, prefix, dayStart, dayEnd, includeReserved)
	if err != nil {
		return "", 0, err
	}
	seq := findNextAvailableSeq(used)
	ticket := fmt.Sprintf("%s%03d", prefix, seq)
	return ticket, seq, nil
}
