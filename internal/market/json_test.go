package market

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWireArraysAndExplicitNulls(t *testing.T) {
	raw, err := json.Marshal(Stock{Code: "600000"})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bids", "asks", "fallback_reasons"} {
		if string(fields[name]) != "[]" {
			t.Fatal("array is null", name)
		}
	}
	if string(fields["amount"]) != "null" {
		t.Fatal("unknown amount replaced")
	}
	var s Stock
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	delete(fields, "amount")
	incomplete, _ := json.Marshal(fields)
	if err := json.Unmarshal(incomplete, &s); err == nil || !strings.Contains(err.Error(), "amount") {
		t.Fatal("missing amount accepted", err)
	}
	malformed := bytes.Replace(raw, []byte(`"code":"600000"`), []byte(`"code":null`), 1)
	if err := json.Unmarshal(malformed, &s); err == nil {
		t.Fatal("null identity accepted")
	}
	duplicate := append([]byte(`{"code":"600001",`), raw[1:]...)
	if err := json.Unmarshal(duplicate, &s); err == nil {
		t.Fatal("duplicate identity accepted")
	}
	d := Build(RuntimeData{}, testAt("2026-09-07T10:00:00+08:00"))
	raw, err = json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var restored Dashboard
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal("dashboard roundtrip", err)
	}
}
