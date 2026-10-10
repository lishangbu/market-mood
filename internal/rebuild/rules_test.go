package rebuild

import (
	"fmt"
	"market-mood/internal/catalog"
	"market-mood/internal/market"
	"testing"
	"time"
)

func security() catalog.Security {
	s, _ := catalog.NewSecurity("600000", "测试", "sh", "test")
	s.ListingDate = market.Ptr("1999-11-10")
	return s
}
func bars() []market.Bar {
	return []market.Bar{{Day: "2026-09-03", Open: 10, Close: 10, High: 10, Low: 10, Volume: 100, Amount: market.Ptr(1000.)}, {Day: "2026-09-04", Open: 10, Close: 10, High: 10, Low: 10, Volume: 100, Amount: market.Ptr(1000.)}, {Day: "2026-09-07", Open: 10.1, Close: 11, High: 11, Low: 10, Volume: 100, Amount: market.Ptr(2e9)}}
}
func TestLimitClassificationAndUncertainHeight(t *testing.T) {
	rows := bars()
	s, err := Classify(security(), rows, "2026-09-07", "2026-09-04", "test")
	if err != nil || s == nil || !*s.Sealed || s.Boards == nil || *s.Boards != 1 || s.BreakCount != nil || s.SealAmount != nil {
		t.Fatal(s, err)
	}
	rows[2].Close = 10.5
	s, err = Classify(security(), rows, "2026-09-07", "2026-09-04", "test")
	if err != nil || s == nil || *s.Sealed || s.Boards != nil {
		t.Fatal("broken should not infer height", s, err)
	}
	rows[2].Amount = nil
	if _, err := Classify(security(), rows, "2026-09-07", "2026-09-04", "test"); err == nil {
		t.Fatal("missing amount accepted")
	}
	if _, err := Classify(security(), bars(), "2026-09-08", "2026-09-07", "test"); err == nil {
		t.Fatal("missing target proves suspension")
	}
	rows = bars()
	rows[2].High = 12
	s, err = Classify(security(), rows, "2026-09-07", "2026-09-04", "test")
	if err != nil || s != nil {
		t.Fatal("nonstandard range classified")
	}
	rows = bars()
	s, err = Classify(security(), rows, "2026-09-07", "2026-09-05", "test")
	if err != nil || s.Boards != nil {
		t.Fatal("unverified prior trading date")
	}
}
func TestListingRulesDifferAndUnknownFailsRanking(t *testing.T) {
	s := security()
	s.ListingDate = nil
	if LimitEligible(s, "2026-09-07") {
		t.Fatal("unknown listing eligible")
	}
	if _, err := Top50Eligible(s, "2026-09-07", nil); err == nil {
		t.Fatal("unknown listing silently excluded")
	}
	s.ListingDate = market.Ptr("2026-08-31")
	cal := []string{"2026-08-31", "2026-09-01", "2026-09-02", "2026-09-03", "2026-09-04", "2026-09-07"}
	if LimitEligible(s, "2026-09-07") {
		t.Fatal("30-day rule")
	}
	eligible, err := Top50Eligible(s, "2026-09-07", cal)
	if err != nil || !eligible {
		t.Fatal("sixth day omitted", eligible, err)
	}
	eligible, err = Top50Eligible(s, "2026-09-04", cal)
	if err != nil || eligible {
		t.Fatal("first five included")
	}
}
func TestRankingCompletenessAndStableTies(t *testing.T) {
	stocks := []market.Stock{}
	for i := 59; i >= 0; i-- {
		stocks = append(stocks, market.Stock{Code: fmt.Sprintf("600%03d", i), Eligible: true, Amount: market.Ptr(1e9)})
	}
	state := market.Top50State{Version: 1, Total: 60, Checked: 60}
	out := FinishTop50(state, stocks, time.Now())
	if !out.Complete || len(out.Stocks) != 50 || out.Stocks[0].Code != "600000" || out.Stocks[49].Code != "600049" {
		t.Fatal("ranking wrong")
	}
	state.Failed = 1
	out = FinishTop50(state, stocks, time.Now())
	if out.Complete || len(out.Stocks) != 0 || out.Error == nil {
		t.Fatal("partial ranking published")
	}
	state.Failed = 0
	state.Checked = 59
	if FinishTop50(state, stocks, time.Now()).Complete {
		t.Fatal("unchecked range published")
	}
	rows := bars()
	rows[2].Volume = 0
	if _, err := AmountStock(security(), rows, "2026-09-07", "test"); err == nil {
		t.Fatal("zero volume with positive amount accepted")
	}
}
