package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type EmailAttachment struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Data        []byte `json:"data"`
}

type EmailAttachmentsJSONB []EmailAttachment

func (a EmailAttachmentsJSONB) Value() (driver.Value, error) {
	if a == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(a)
}

func (a *EmailAttachmentsJSONB) Scan(value interface{}) error {
	if value == nil {
		*a = EmailAttachmentsJSONB{}
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("unsupported Scan type for EmailAttachmentsJSONB: %T", value)
	}
	if len(bytes) == 0 {
		*a = EmailAttachmentsJSONB{}
		return nil
	}
	return json.Unmarshal(bytes, a)
}
