package backfill

import (
	"encoding/json"
	"market-mood/internal/trading"
	"slices"
	"testing"
	"time"
)

func TestEndedCommonDays(t *testing.T) {
	sh := []DailyBar{{Day: "2026-10-08"}, {Day: "2026-10-09"}, {Day: "2026-10-12"}}
	sz := []DailyBar{{Day: "2026-10-09"}, {Day: "2026-10-12"}}
	at := time.Date(2026, 10, 9, 15, 59, 0, 0, trading.Shanghai)
	if got := TargetDays(sh, sz, at); len(got) != 0 {
		t.Fatal(got)
	}
	if got := TargetDays(sh, sz, at.Add(time.Minute)); !slices.Equal(got, []string{"2026-10-09"}) {
		t.Fatal(got)
	}
}
func TestIndexValidation(t *testing.T) {
	valid := `["2026-10-09","10","11","12","9","100"]`
	for _, raw := range []string{`[]`, `[null]`, `[["2026-10-09","NaN","11","12","9","100"]]`, `[` + valid + `,` + valid + `]`, `[["2026-10-09","10","11","9","9","100"]]`} {
		if _, err := ParseIndex(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if bars, err := ParseIndex(json.RawMessage(`[` + valid + `]`)); err != nil || len(bars) != 1 {
		t.Fatal(bars, err)
	}
}
