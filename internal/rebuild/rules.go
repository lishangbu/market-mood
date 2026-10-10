// Package rebuild reconstructs historical membership from verified daily evidence.
package rebuild

import (
	"cmp"
	"fmt"
	"market-mood/internal/catalog"
	"market-mood/internal/market"
	"math"
	"slices"
	"strings"
	"time"
)

// LimitEligible excludes unknown listing dates and the first 30 calendar days.
// This deliberately differs from turnover ranking's first-five-trading-days rule.
func LimitEligible(s catalog.Security, day string) bool {
	upper := strings.ToUpper(s.Name)
	if (s.Exchange != "sh" && s.Exchange != "sz") || strings.Contains(upper, "ST") || strings.Contains(upper, "退") || strings.HasPrefix(upper, "N") || strings.HasPrefix(upper, "C") || s.ListingDate == nil {
		return false
	}
	listing, err := time.Parse(time.DateOnly, *s.ListingDate)
	if err != nil {
		return false
	}
	target, err := time.Parse(time.DateOnly, day)
	return err == nil && target.Sub(listing) > 30*24*time.Hour
}
func Top50Eligible(s catalog.Security, day string, calendar []string) (bool, error) {
	name := strings.ToUpper(s.Name)
	if (s.Exchange != "sh" && s.Exchange != "sz") || strings.Contains(name, "ST") || strings.Contains(name, "退") {
		return false, nil
	}
	if s.ListingDate == nil {
		return false, fmt.Errorf("上市日期待核验")
	}
	listing, err := time.Parse(time.DateOnly, *s.ListingDate)
	if err != nil {
		return false, err
	}
	target, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return false, err
	}
	if listing.After(target) {
		return false, nil
	}
	if target.Sub(listing) <= 30*24*time.Hour {
		if !slices.Contains(calendar, *s.ListingDate) {
			return false, fmt.Errorf("上市交易日不在已核验日历中")
		}
		days := map[string]bool{}
		for _, d := range calendar {
			if d >= *s.ListingDate && d <= day {
				days[d] = true
			}
		}
		return len(days) > 5, nil
	}
	return true, nil
}
func cents(value float64) int64 { return int64(math.Round(value * 100)) }
func limitPrice(bar, prior market.Bar, rate int64) (int64, error) {
	reference := prior.Close
	if bar.Prevclose != nil {
		reference = *bar.Prevclose
	}
	if reference <= 0 || math.IsNaN(reference) || math.IsInf(reference, 0) || reference > float64(math.MaxInt64)/20000 {
		return 0, fmt.Errorf("涨停参考价缺失或越界")
	}
	return (cents(reference)*(100+rate) + 50) / 100, nil
}

// Classify requires actual target-day bars. Missing-day or suspension exclusion
// belongs to the coordinator, where independent sources can corroborate it.
func Classify(s catalog.Security, rows []market.Bar, day, priorDay, source string) (*market.Stock, error) {
	if err := market.ValidateBars(rows); err != nil {
		return nil, err
	}
	index := slices.IndexFunc(rows, func(b market.Bar) bool { return b.Day == day })
	if index < 0 {
		return nil, fmt.Errorf("历史数据缺少目标交易日")
	}
	b := rows[index]
	if b.Volume == 0 {
		return nil, nil
	}
	if index < 2 {
		return nil, fmt.Errorf("首板判断缺少前序日线")
	}
	prior := rows[index-1]
	rate := int64(10)
	if strings.HasPrefix(s.Code, "30") || strings.HasPrefix(s.Code, "688") {
		rate = 20
	}
	up, err := limitPrice(b, prior, rate)
	if err != nil {
		return nil, err
	}
	if cents(b.High) != up {
		return nil, nil
	}
	priorUp, err := limitPrice(prior, rows[index-2], rate)
	if err != nil {
		return nil, err
	}
	if b.Amount == nil {
		return nil, fmt.Errorf("触板股票成交额缺失")
	}
	sealed := cents(b.Close) == up
	reference := prior.Close
	if b.Prevclose != nil {
		reference = *b.Prevclose
	}
	industry := ""
	if s.Industry != nil {
		industry = *s.Industry
	}
	stock := market.Stock{Code: s.Code, Name: s.Name, Industry: industry, Eligible: true, Price: market.Ptr(b.Close), Open: market.Ptr(b.Open), PreviousClose: market.Ptr(reference), Change: market.Ptr((b.Close/reference - 1) * 100), Amount: b.Amount, LimitUp: market.Ptr(float64(up) / 100), Sealed: &sealed, Provider: &source, SourceTime: market.Ptr(day + "T15:00:00+08:00")}
	if sealed && prior.Day == priorDay && cents(prior.Close) != priorUp {
		stock.Boards = market.Ptr(uint32(1))
	}
	return &stock, nil
}
func AmountStock(s catalog.Security, rows []market.Bar, day, source string) (market.Stock, error) {
	if err := market.ValidateBars(rows); err != nil {
		return market.Stock{}, err
	}
	index := slices.IndexFunc(rows, func(b market.Bar) bool { return b.Day == day })
	if index < 0 {
		return market.Stock{}, fmt.Errorf("缺少目标交易日日线")
	}
	b := rows[index]
	if b.Amount == nil {
		return market.Stock{}, fmt.Errorf("昨日成交额缺失")
	}
	if (b.Volume == 0) != (*b.Amount == 0) {
		return market.Stock{}, fmt.Errorf("成交量与成交额零值不一致")
	}
	industry := ""
	if s.Industry != nil {
		industry = *s.Industry
	}
	return market.Stock{Code: s.Code, Name: s.Name, Industry: industry, Eligible: true, Amount: b.Amount, Price: market.Ptr(b.Close), Open: market.Ptr(b.Open), SourceTime: market.Ptr(day + "T15:00:00+08:00"), Provider: &source}, nil
}

// FinishTop50 publishes nothing until every candidate is checked, no failures
// remain, and at least fifty distinct positive-turnover observations are proven.
func FinishTop50(state market.Top50State, stocks []market.Stock, at time.Time) market.Top50State {
	state.Running = false
	state.UpdatedAt = at.Format(time.RFC3339)
	state.Complete = false
	state.Stocks = []market.Stock{}
	seen := map[string]bool{}
	valid := []market.Stock{}
	for _, s := range stocks {
		if !s.Eligible || s.Amount == nil || math.IsNaN(*s.Amount) || math.IsInf(*s.Amount, 0) || *s.Amount <= 0 || seen[s.Code] {
			state.Failed++
			continue
		}
		seen[s.Code] = true
		valid = append(valid, s)
	}
	if state.Failed == 0 && state.Checked == state.Total && len(valid) >= 50 {
		slices.SortFunc(valid, func(a, b market.Stock) int {
			if n := cmp.Compare(*b.Amount, *a.Amount); n != 0 {
				return n
			}
			return cmp.Compare(a.Code, b.Code)
		})
		state.Stocks = valid[:50]
		state.Complete = true
		state.Error = nil
	} else {
		detail := ""
		if state.Error != nil {
			detail = "；" + *state.Error
		}
		state.Error = market.Ptr(fmt.Sprintf("昨日成交额待核验：%d只失败，%d只有效样本%s", state.Failed, len(valid), detail))
	}
	return state
}
