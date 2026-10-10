package market

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
)

// A nullable field must still be present. Rejecting absent or null non-nullable
// fields prevents damaged persisted observations from turning into zero values.
func requireFields(data []byte, required, nullable []string) error {
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return fmt.Errorf("预期JSON对象")
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return fmt.Errorf("JSON字段名异常")
		}
		if _, ok := fields[key]; ok {
			return fmt.Errorf("重复字段 %s", key)
		}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return err
		}
		fields[key] = value
	}
	if _, err := d.Token(); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("JSON对象后存在多余内容")
	}
	for _, field := range required {
		value, ok := fields[field]
		if !ok {
			return fmt.Errorf("缺少字段 %s", field)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) && !slices.Contains(nullable, field) {
			return fmt.Errorf("字段 %s 不可为null", field)
		}
	}
	return nil
}
