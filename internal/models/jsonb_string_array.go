package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// StringArrayJSONB 用於在 Postgres JSONB 欄位存放字串陣列（例如 ["order"]）
// 避免 gorm 把 []string 當成 postgres text[]，導致 jsonb 寫入時出現 SQLSTATE 22P02。
type StringArrayJSONB []string

func (a StringArrayJSONB) Value() (driver.Value, error) {
	if a == nil {
		return []byte("[]"), nil
	}
	b, err := json.Marshal([]string(a))
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (a *StringArrayJSONB) Scan(value interface{}) error {
	if value == nil {
		*a = StringArrayJSONB{}
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("unsupported Scan type for StringArrayJSONB: %T", value)
	}
	if len(bytes) == 0 {
		*a = StringArrayJSONB{}
		return nil
	}

	// Preferred format: JSON array of strings: ["dashboard","customers"]
	var out []string
	if err := json.Unmarshal(bytes, &out); err == nil {
		*a = StringArrayJSONB(out)
		return nil
	}

	// Backward compatible: some legacy rows may store JSON objects instead of arrays, e.g.
	//   {}                         -> []
	//   {"dashboard":true}          -> ["dashboard"]
	//   {"0":"dashboard"}           -> ["dashboard"]
	//   {"dashboard":"dashboard"}   -> ["dashboard"]
	var obj map[string]interface{}
	if err := json.Unmarshal(bytes, &obj); err != nil {
		return err
	}
	if len(obj) == 0 {
		*a = StringArrayJSONB{}
		return nil
	}

	seen := map[string]struct{}{}
	add := func(s string) {
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}

	for k, v := range obj {
		switch vv := v.(type) {
		case bool:
			// {"dashboard":true}
			if vv {
				add(k)
			}
		case float64:
			// {"dashboard":1}
			if vv != 0 {
				add(k)
			}
		case string:
			// {"0":"dashboard"} or {"dashboard":"dashboard"}
			if vv != "" {
				add(vv)
			} else {
				add(k)
			}
		default:
			// Unknown shape: best-effort fall back to the key name
			add(k)
		}
	}

	*a = StringArrayJSONB(out)
	return nil
}


