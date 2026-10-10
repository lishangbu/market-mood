package rebuild

import (
	"context"
	"encoding/json"
	"market-mood/internal/market"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSuspensionIntervals(t *testing.T) {
	row := func(code, start string) Suspension {
		return Suspension{Code: code, Identity: code + ".SZ", Start: &start, Expire: "连续停牌"}
	}
	rows := []Suspension{row("002998", "2026-09-04 09:30:00"), row("301390", "2026-09-10 09:30:00"), row("002870", "2026-09-14 09:30:00"), row("000001", "2026-09-10 10:00:00"), row("000002", "2026-09-09 09:30:00")}
	rows[0].End = market.Ptr("2026-09-10 15:00:00")
	rows[0].Resume = market.Ptr("2026-09-11 00:00:00")
	rows[4].End = market.Ptr("2026-09-10 10:00:00")
	codes, err := FullDaySuspensions(rows, "2026-09-10")
	if err != nil || len(codes) != 2 || !codes["002998"] || !codes["301390"] {
		t.Fatal(codes, err)
	}
	rows[0].Identity = "002870.SZ"
	if _, err := FullDaySuspensions(rows, "2026-09-10"); err == nil {
		t.Fatal("bad identity proves suspension")
	}
}
func TestCorporateActionWindowAndIdentity(t *testing.T) {
	body := `var sh600000qfq={"data":[{"d":"2026-09-04"},{"d":"2025-05-01"}]};`
	has, err := ParseCorporateAction(body, "sh600000", "2026-09-04", "2026-09-07")
	if err != nil || !has {
		t.Fatal(has, err)
	}
	has, err = ParseCorporateAction(body, "sh600000", "2026-09-07", "2026-09-08")
	if err != nil || has {
		t.Fatal(has, err)
	}
	if _, err := ParseCorporateAction(body, "sh600001", "2026-09-04", "2026-09-07"); err == nil {
		t.Fatal("wrong security")
	}
	if _, err := ParseCorporateAction(`var sh600000qfq={};`, "sh600000", "2026-09-04", "2026-09-07"); err == nil {
		t.Fatal("missing actions silently means no actions")
	}
}

func TestCapturedSuspensionsIncludeValidAShares(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/eastmoney-suspensions-20261008.json")
	if err != nil {
		t.Fatal(err)
	}
	var response struct{ Result struct{ Data []Suspension } }
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	codes, err := FullDaySuspensions(response.Result.Data, "2026-10-08")
	if err != nil || !codes["000582"] || !codes["000668"] || codes["200016"] {
		t.Fatal("valid A-share suspension evidence lost", codes, err)
	}
}
func TestMixedShareClassSuspensions(t *testing.T) {
	rows := []Suspension{
		{Code: "000582", Identity: "000582.SZ", Start: market.Ptr("2026-10-08 09:30:00"), Expire: "连续停牌"},
		{Code: "200016", Identity: "200016.SZ", Start: market.Ptr("2026-09-04 09:30:00"), Expire: "连续停牌"},
	}
	if codes, err := FullDaySuspensions(rows[:1], "2026-10-08"); err != nil || !codes["000582"] {
		t.Fatal("A-share alone fails", codes, err)
	}
	codes, err := FullDaySuspensions(rows, "2026-10-08")
	if err != nil || len(codes) != 1 || !codes["000582"] {
		t.Fatal(codes, err)
	}
}
func TestSuspensionRejectsMismatchedExchange(t *testing.T) {
	for _, identity := range []string{"000582.SH", "200016.SH", "900901.SZ", "123456.SZ", "200016X.SZ"} {
		code, _, _ := strings.Cut(identity, ".")
		rows := []Suspension{{Code: code, Identity: identity, Start: market.Ptr("2026-10-08 09:30:00"), Expire: "连续停牌"}}
		if _, err := FullDaySuspensions(rows, "2026-10-08"); err == nil {
			t.Fatal("invalid identity accepted", identity)
		}
	}
}
func TestLiveSuspensionEvidence(t *testing.T) {
	if os.Getenv("MARKET_MOOD_LIVE_TEST") != "1" {
		t.Skip("explicit live validation only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	codes, err := NewEvidence(nil).Suspended(ctx, "2026-10-08")
	if err != nil || !codes["000582"] || !codes["000668"] || codes["200016"] {
		t.Fatal(codes, err)
	}
	t.Logf("verified %d full-day A-share suspensions", len(codes))
}
