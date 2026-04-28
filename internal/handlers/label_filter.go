package handlers

import (
	"strings"

	"github.com/google/uuid"
)

// parseUUIDListFromStrings 從多個逗號分隔的字符串解析 UUID 列表（用於標籤過濾）
func parseUUIDListFromStrings(values ...string) []uuid.UUID {
	ids := make([]uuid.UUID, 0)
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			continue
		}
		parts := strings.Split(v, ",")
		for _, part := range parts {
			p := strings.TrimSpace(part)
			if p == "" {
				continue
			}
			if id, err := uuid.Parse(p); err == nil {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// parseUUIDListFromInterface 從 interface 解析 UUID 列表（支援 string/[]string/[]interface{})
func parseUUIDListFromInterface(value interface{}) []uuid.UUID {
	ids := make([]uuid.UUID, 0)
	if value == nil {
		return ids
	}

	switch v := value.(type) {
	case string:
		return parseUUIDListFromStrings(v)
	case []string:
		for _, s := range v {
			ids = append(ids, parseUUIDListFromStrings(s)...)
		}
	case []interface{}:
		for _, item := range v {
			switch iv := item.(type) {
			case string:
				ids = append(ids, parseUUIDListFromStrings(iv)...)
			case []string:
				for _, s := range iv {
					ids = append(ids, parseUUIDListFromStrings(s)...)
				}
			}
		}
	}

	return ids
}
