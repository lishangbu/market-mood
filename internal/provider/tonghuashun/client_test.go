package tonghuashun

import (
	"encoding/json"
	"market-mood/internal/market"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestCapturedDaily(t *testing.T) {
	body := string(fixture(t, "ths-600000-daily.js"))
	rows, err := ParseDaily(body, "600000", "last")
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(rows, func(b market.Bar) bool { return b.Day == "2026-09-07" })
	if i < 0 {
		t.Fatal("missing day")
	}
	b := rows[i]
	if b.Open != 9.43 || b.High != 9.46 || b.Low != 9.19 || b.Close != 9.23 || b.Volume != 86711332 || *b.Amount != 806326230 {
		t.Fatal(b)
	}
	for _, bad := range []string{strings.Replace(body, "_00_", "_01_", 1), strings.ReplaceAll(body, "9.43,9.46,9.19,9.23", "9.43,9.00,9.19,9.23"), strings.ReplaceAll(body, "806326230.00", "NaN")} {
		if _, err := ParseDaily(bad, "600000", "last"); err == nil {
			t.Fatal("bad daily accepted")
		}
	}
	if _, err := ParseDaily(body, "600001", "last"); err == nil {
		t.Fatal("wrong stock accepted")
	}
}
func TestCapturedPoolsAndUnverifiedFields(t *testing.T) {
	sealed, err := parsePage(fixture(t, "ths-history-sealed-20260918.json"), "2026-09-18", true, 1)
	if err != nil {
		t.Fatal(err)
	}
	broken, err := parsePage(fixture(t, "ths-history-broken-20260918.json"), "2026-09-18", false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.total != 77 || broken.total != 25 {
		t.Fatal("pool totals")
	}
	frame, err := combine("2026-09-18", sealed, broken, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range frame.Sealed {
		if s.SealAmount != nil || s.BreakCount != nil || *s.Provider != PoolSource {
			t.Fatal("unverified fields invented")
		}
	}
	for _, s := range frame.Broken {
		if *s.Sealed || s.SealAmount != nil {
			t.Fatal("broken semantic")
		}
		if s.Code == "603679" && (*s.Amount != 416530480 || *s.Price != 25.6) {
			t.Fatal("units")
		}
	}
}
func TestPoolRejections(t *testing.T) {
	body := fixture(t, "ths-history-sealed-20260918.json")
	if _, err := parsePage(body, "2026-09-17", true, 1); err == nil {
		t.Fatal("wrong date")
	}
	if _, err := parsePage(body, "2026-09-18", true, 2); err == nil {
		t.Fatal("wrong page")
	}
	var v map[string]any
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatal(err)
	}
	data := v["data"].(map[string]any)
	info := data["info"].([]any)
	row := info[0].(map[string]any)
	row["first_limit_up_time"] = "1789610400"
	bad, _ := json.Marshal(v)
	if _, err := parsePage(bad, "2026-09-18", true, 1); err == nil {
		t.Fatal("cross-day event")
	}
	for _, value := range []string{"5天3板", "bad"} {
		stats := statistics(value)
		if value == "5天3板" && (stats == nil || stats.Days == stats.Count) {
			t.Fatal("nonconsecutive boards")
		}
	}
}
