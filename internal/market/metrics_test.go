package market

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func testAt(s string) time.Time {
	at, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return at
}
func testStock(code string, change float64) Stock {
	return Stock{Code: code, Name: code, Eligible: true, Price: Ptr(10.), Open: Ptr(9.), Change: Ptr(change), Amount: Ptr(2e9), Boards: Ptr(uint32(1)), SourceTime: Ptr("2026-09-07T10:00:30+08:00")}
}
func sample() RuntimeData {
	a, b, c := testStock("600000", 2), testStock("600001", 1), testStock("600002", 3)
	return RuntimeData{Calendar: []string{"2026-09-04", "2026-09-07"}, Previous: &CloseFrame{Market: MarketFrame{Day: "2026-09-04", Complete: true, Stocks: []Stock{a, b}}, Limits: LimitFrame{Day: "2026-09-04", Sealed: []Stock{a}, Broken: []Stock{b}}}, Market: &MarketFrame{Day: "2026-09-07", Complete: true, SourceTime: a.SourceTime, Stocks: []Stock{a, b, c}}, Limits: &LimitFrame{Day: "2026-09-07", CapturedAt: "2026-09-07T10:00:40+08:00", Sealed: []Stock{c}}}
}
func TestScoreRequiresFreshSuccessfulSources(t *testing.T) {
	data := sample()
	at := testAt("2026-09-07T10:01:00+08:00")
	if Build(data, at).Score.Total == nil {
		t.Fatal("complete fresh inputs have no score")
	}
	data.LimitError = Ptr("更新失败")
	got := Build(data, at)
	if got.Score.Total != nil {
		t.Fatal("failed source published score")
	}
	for _, f := range got.Score.Factors {
		if f.Score == nil {
			t.Fatal("failure discarded last factor")
		}
	}
	data.LimitError = nil
	if Build(data, at.Add(10*time.Minute)).Score.Total != nil {
		t.Fatal("stale score")
	}
	data.CalendarError = Ptr("日历失败")
	if Build(data, at).Score.Total != nil {
		t.Fatal("calendar failure score")
	}
}
func TestYesterdayMembershipAndMissingQuotes(t *testing.T) {
	data := sample()
	data.Market.Stocks[0].Amount = Ptr(1.)
	data.Market.Stocks[0].Change = Ptr(-2.)
	got := Build(data, testAt("2026-09-07T10:01:00+08:00"))
	if got.Pools[0].Count != 1 || got.Pools[0].Average == nil || *got.Pools[0].Average != -2 {
		t.Fatal("membership reranked today")
	}
	data.Market.Stocks = data.Market.Stocks[1:]
	got = Build(data, testAt("2026-09-07T10:01:00+08:00"))
	p := got.Pools[0]
	if p.Count != 1 || p.ValidCount != 0 || p.Average != nil || p.Stocks[0].Price != nil || p.Stocks[0].Code != "600000" {
		t.Fatalf("missing quote lost identity or reused old price: %+v", p)
	}
}
func TestRebuildDoesNotClaimFullHistory(t *testing.T) {
	data := sample()
	data.PreviousLimits = &data.Previous.Limits
	data.Previous = nil
	got := Build(data, testAt("2026-09-07T10:01:00+08:00"))
	if !got.Pools[0].Ready || got.Pools[4].Ready {
		t.Fatal("reconstructed events must not unlock top50")
	}
	data.PreviousLimits.Day = "2026-09-03"
	if Build(data, testAt("2026-09-07T10:01:00+08:00")).Pools[0].Ready {
		t.Fatal("wrong yesterday accepted")
	}
}
func TestResealsAreUniqueAndInputsUnchanged(t *testing.T) {
	data := sample()
	s := data.Limits.Sealed[0]
	data.Limits.Sealed = append(data.Limits.Sealed, s)
	data.Limits.Broken = []Stock{s}
	before, _ := json.Marshal(data)
	got := Build(data, testAt("2026-09-07T10:01:00+08:00"))
	after, _ := json.Marshal(data)
	if got.Sealing.Sealed != 1 || got.Sealing.Broken != 0 || *got.Sealing.Rate != 100 {
		t.Fatal("duplicate reseal counted")
	}
	if string(before) != string(after) {
		t.Fatal("calculation mutated input")
	}
}
func TestUnknownAndInvalidSamples(t *testing.T) {
	if Ratio(0, 0) != nil || *Ratio(0, 1) != 0 {
		t.Fatal("unknown confused with zero")
	}
	for _, v := range []float64{-10, 0, 10} {
		score := Linear(&v, -3, 3, 20)
		want := map[float64]float64{-10: 0, 0: 10, 10: 20}[v]
		if *score != want {
			t.Fatal(score)
		}
	}
	if Linear(nil, -3, 3, 20) != nil {
		t.Fatal("missing factor defaulted")
	}
	a, b := testStock("600000", 2), testStock("600001", -20)
	b.Open = Ptr(0.)
	p := makePool("x", "x", "x", []Stock{a, b}, true)
	if p.Count != 2 || p.ValidCount != 1 || *p.Average != 2 {
		t.Fatal("suspension in denominator")
	}
	a.Change = Ptr(math.NaN())
	if a.Valid() {
		t.Fatal("nonfinite quote accepted")
	}
	empty := Build(RuntimeData{}, testAt("2026-09-07T10:00:00+08:00"))
	if empty.Score.Total != nil || empty.Breadth != nil || len(empty.Pools) != 7 {
		t.Fatal("empty state fabricated data")
	}
}
func TestEventJoinPreservesQuoteAndDate(t *testing.T) {
	s := testStock("600000", 2)
	event := s
	event.Amount = Ptr(1.)
	event.Sealed = Ptr(true)
	event.SealAmount = Ptr(2e8)
	event.BreakCount = Ptr(uint32(2))
	frame := &LimitFrame{Day: "2026-09-07", Sealed: []Stock{event}}
	got := WithLimitFields(s, frame)
	if *got.Amount != 2e9 || *got.SealAmount != 2e8 || *got.BreakCount != 2 {
		t.Fatal("event/quote fields mixed")
	}
	frame.Day = "2026-09-04"
	if WithLimitFields(s, frame).BreakCount != nil {
		t.Fatal("cross-day join")
	}
	frame.Day = "2026-09-07"
	event.Sealed = Ptr(false)
	frame.Sealed = nil
	frame.Broken = []Stock{event}
	if WithLimitFields(s, frame).SealAmount != nil {
		t.Fatal("broken stock retains seal orders")
	}
}

func TestWeekendRefreshOfVerifiedClosedPoolKeepsLastDayScore(t *testing.T) {
	data := sample()
	data.Calendar = []string{"2026-09-03", "2026-09-04", "2026-09-07"}
	data.Market.Day = "2026-09-04"
	data.Market.SourceTime = Ptr("2026-09-04T15:05:00+08:00")
	data.Limits.Day = "2026-09-04"
	data.Limits.CapturedAt = "2026-09-05T10:00:00+08:00"
	data.Previous.Market.Day = "2026-09-03"
	data.Previous.Limits.Day = "2026-09-03"
	at := testAt("2026-09-05T10:01:00+08:00")
	got := Build(data, at)
	if got.Score.Total == nil || got.Sources[1].Stale {
		t.Fatal("verified closed-day pool refreshed on weekend marked stale", got.Sources)
	}
	data.Limits.CapturedAt = "2026-09-04T10:00:00+08:00"
	if Build(data, at).Score.Total != nil {
		t.Fatal("pre-close cached pool accepted as close")
	}
	data.Limits.CapturedAt = "2026-09-05T10:02:00+08:00"
	if Build(data, at).Score.Total != nil {
		t.Fatal("future fetch accepted")
	}
}
