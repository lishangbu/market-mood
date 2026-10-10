package market

import (
	"cmp"
	"market-mood/internal/trading"
	"slices"
	"time"
)

func Ratio(a, b int) *float64 {
	if b <= 0 {
		return nil
	}
	return Ptr(float64(a) / float64(b) * 100)
}
func Linear(value *float64, low, high, weight float64) *float64 {
	if !finite(value) || high <= low {
		return nil
	}
	return Ptr(max(0, min(1, (*value-low)/(high-low))) * weight)
}
func makePool(id, title, description string, stocks []Stock, ready bool) Pool {
	if stocks == nil {
		stocks = []Stock{}
	}
	p := Pool{ID: id, Title: title, Description: description, Stocks: stocks, Ready: ready, Count: len(stocks)}
	sum, yang := 0., 0
	for _, s := range stocks {
		if ready && s.Valid() {
			p.ValidCount++
			sum += *s.Change
			if *s.Yang() {
				yang++
			}
		}
	}
	if p.ValidCount > 0 {
		p.Average = Ptr(sum / float64(p.ValidCount))
	}
	p.YangRate = Ratio(yang, p.ValidCount)
	return p
}
func sealing(sealed, broken []Stock) *Sealing {
	s := &Sealing{Sealed: len(sealed), Broken: len(broken), Rate: Ratio(len(sealed), len(sealed)+len(broken))}
	for _, stock := range sealed {
		if stock.Boards != nil && (s.Height == nil || *stock.Boards > *s.Height) {
			s.Height = Ptr(*stock.Boards)
		}
	}
	return s
}
func selectStocks(stocks []Stock, keep func(Stock) bool) []Stock {
	out := []Stock{}
	for _, s := range stocks {
		if keep(s) {
			out = append(out, s)
		}
	}
	return out
}
func first(s Stock) bool    { return s.Boards != nil && *s.Boards == 1 }
func capacity(s Stock) bool { return finite(s.Amount) && *s.Amount >= 1e9 }

// Build derives the dashboard without I/O or mutation of the input. Historical
// membership is selected before joining current quotes, never ranked again today.
func Build(data RuntimeData, at time.Time) Dashboard {
	at = at.In(trading.Shanghai)
	d := Dashboard{GeneratedAt: at.Format(time.RFC3339), Session: trading.Session(data.Calendar, at), Pools: []Pool{}, Sources: []SourceStatus{}, Notes: []string{"评分 v1 为透明的初始观察指标，尚未经回测。"}}
	day := ""
	if data.Market != nil {
		day = data.Market.Day
	}
	if data.Limits != nil {
		day = max(day, data.Limits.Day)
	}
	if day != "" {
		d.Day = Ptr(day)
	}
	market, limits := data.Market, data.Limits
	if market != nil && (market.Day != day || !market.Complete) {
		market = nil
	}
	if limits != nil && limits.Day != day {
		limits = nil
	}
	previousDay := trading.Previous(data.Calendar, day)
	previous := data.Previous
	if previous != nil && (previousDay == "" || previous.Market.Day != previousDay || !previous.Market.Complete || previous.Limits.Day != previousDay) {
		previous = nil
	}
	old := data.PreviousLimits
	if old != nil && (previousDay == "" || old.Day != previousDay) {
		old = nil
	}
	if previous != nil {
		old = &previous.Limits
	}
	quotes := map[string]Stock{}
	if market != nil {
		for _, s := range market.Stocks {
			quotes[s.Code] = s
		}
		d.ObservedAt = market.SourceTime
	}
	track := func(stocks []Stock) []Stock {
		result := make([]Stock, 0, len(stocks))
		for _, s := range stocks {
			if q, ok := quotes[s.Code]; ok {
				result = append(result, WithLimitFields(q, limits))
			} else {
				result = append(result, Stock{Code: s.Code, Name: s.Name, Industry: s.Industry, Eligible: s.Eligible, Bids: []BookLevel{}, Asks: []BookLevel{}, FallbackReasons: []string{}})
			}
		}
		return result
	}
	var oldSealed, oldBroken []Stock
	if old != nil {
		oldSealed, oldBroken = old.Sealed, old.Broken
		d.PreviousDay = Ptr(old.Day)
	}
	ready := old != nil && market != nil
	d.Pools = append(d.Pools,
		makePool("yesterday-cap-first", "昨日十亿首板", "昨日主板首板、收盘成交额≥10亿元；观察今日等权表现。", track(selectStocks(oldSealed, func(s Stock) bool { return s.Eligible && s.MainBoard() && capacity(s) && first(s) })), ready),
		makePool("yesterday-cap-broken", "昨日十亿炸板", "昨日主板收盘炸板、成交额≥10亿元；包含触板后未封住的股票。", track(selectStocks(oldBroken, func(s Stock) bool { return s.Eligible && s.MainBoard() && capacity(s) })), ready),
		makePool("yesterday-20", "昨日20cm首板", "昨日创业板/科创板首板；观察今日表现。", track(selectStocks(oldSealed, func(s Stock) bool { return s.Eligible && s.Twenty() && first(s) })), ready),
		makePool("yesterday-first", "昨日主板首板", "昨日主板收盘首板，不限成交额。", track(selectStocks(oldSealed, func(s Stock) bool { return s.Eligible && s.MainBoard() && first(s) })), ready))
	rebuilt := data.PreviousTop50
	if rebuilt != nil && (!rebuilt.Complete || rebuilt.Version != 1 || len(rebuilt.Stocks) != 50 || previousDay == "" || rebuilt.Day != previousDay) {
		rebuilt = nil
	}
	var top []Stock
	if previous != nil {
		top = selectStocks(previous.Market.Stocks, func(s Stock) bool { return s.Eligible && finite(s.Amount) })
	} else if rebuilt != nil {
		top = slices.Clone(rebuilt.Stocks)
		seen := map[string]bool{}
		for _, s := range top {
			if !s.Eligible || !finite(s.Amount) || seen[s.Code] {
				rebuilt = nil
				top = nil
				break
			}
			seen[s.Code] = true
		}
	}
	slices.SortStableFunc(top, func(a, b Stock) int { return cmp.Compare(*b.Amount, *a.Amount) })
	if len(top) > 50 {
		top = top[:50]
	}
	d.Pools = append(d.Pools, makePool("yesterday-top50", "昨日成交前50", "昨日沪深A股收盘成交额前50，名单固定，今日等权表现。", track(top), (previous != nil || rebuilt != nil) && market != nil))
	var sealed, broken []Stock
	seen := map[string]bool{}
	if limits != nil {
		for i, list := range [][]Stock{limits.Sealed, limits.Broken} {
			for _, s := range list {
				if !s.Eligible || !s.MainBoard() || seen[s.Code] {
					continue
				}
				seen[s.Code] = true
				if q, ok := quotes[s.Code]; ok {
					s.Open, s.PreviousClose, s.SourceTime = q.Open, q.PreviousClose, q.SourceTime
				}
				if i == 0 {
					sealed = append(sealed, s)
				} else {
					broken = append(broken, s)
				}
			}
		}
		d.Sealing = sealing(sealed, broken)
	}
	oldCodes := map[string]bool{}
	for _, s := range oldSealed {
		oldCodes[s.Code] = true
	}
	capSealed := selectStocks(sealed, func(s Stock) bool { return capacity(s) && first(s) })
	capBroken := selectStocks(broken, func(s Stock) bool { return capacity(s) && old != nil && !oldCodes[s.Code] })
	if limits != nil && old != nil {
		d.Capacity = sealing(capSealed, capBroken)
		d.Capacity.Height = sealing(selectStocks(sealed, capacity), nil).Height
	}
	d.Pools = append(d.Pools,
		makePool("today-cap", "今日十亿首板", "主板当日累计成交额≥10亿元，前一交易日未收盘涨停；回封归封板。高度单独按全部容量封板股计算。", append(capSealed, capBroken...), d.Capacity != nil),
		makePool("today-sealing", "主板封板与炸板", "沪深主板非ST，含连板；封板率=当前封板家数÷触板家数，按股票去重。", append(sealed, broken...), limits != nil))
	if market != nil {
		b := &Breadth{}
		for _, s := range market.Stocks {
			if !s.Valid() {
				b.Excluded++
				continue
			}
			b.Valid++
			switch {
			case *s.Change > 0:
				b.Up++
			case *s.Change < 0:
				b.Down++
			default:
				b.Flat++
			}
			if *s.Yang() {
				b.Yang++
			} else {
				b.Yin++
			}
		}
		d.Breadth = b
	}
	ms := SourceStatus{Name: "全市场行情", Error: data.MarketError, Stale: true, FallbackReasons: []string{}}
	if m := data.Market; m != nil {
		ms.CapturedAt = Ptr(m.CapturedAt)
		ms.SourceTime = m.SourceTime
		ms.Provider = m.Provider
		ms.FallbackReasons = append(ms.FallbackReasons, m.FallbackReasons...)
		ms.Stale = trading.Stale(m.SourceTime, m.Day, data.Calendar, at)
	}
	ls := SourceStatus{Name: "涨停 / 炸板池", Provider: Ptr("东方财富"), Error: data.LimitError, Stale: true, FallbackReasons: []string{}}
	if l := data.Limits; l != nil {
		ls.CapturedAt = Ptr(l.CapturedAt)
		ls.SourceTime = Ptr(l.Day)
		ls.Stale = trading.PoolStale(l.CapturedAt, l.Day, data.Calendar, at)
	}
	d.Sources = []SourceStatus{ms, ls}
	addFactor := func(name string, value *float64, weight, low, high float64, unit string) {
		d.Score.Factors = append(d.Score.Factors, Factor{Name: name, Value: value, Weight: weight, Range: [2]float64{low, high}, Unit: unit, Score: Linear(value, low, high, weight)})
	}
	addFactor("昨日十亿首板表现", d.Pools[0].Average, 20, -3, 3, "%")
	addFactor("昨日十亿首板阳率", d.Pools[0].YangRate, 15, 20, 80, "%")
	addFactor("昨日十亿炸板表现", d.Pools[1].Average, 10, -3, 3, "%")
	addFactor("昨日十亿炸板阳率", d.Pools[1].YangRate, 10, 20, 80, "%")
	addFactor("昨日成交50表现", d.Pools[4].Average, 10, -3, 3, "%")
	addFactor("昨日成交50阳率", d.Pools[4].YangRate, 10, 20, 80, "%")
	var count, rate *float64
	if d.Capacity != nil {
		count = Ptr(float64(d.Capacity.Sealed))
		rate = d.Capacity.Rate
	}
	addFactor("十亿首板封板家数", count, 10, 0, 20, "家")
	addFactor("十亿首板封板率", rate, 15, 30, 90, "%")
	d.Score.Version = "v1"
	sum, complete := 0., true
	for _, f := range d.Score.Factors {
		if f.Score == nil {
			complete = false
		} else {
			sum += *f.Score
		}
	}
	if complete {
		if !ms.Stale && !ls.Stale && ms.Error == nil && ls.Error == nil && data.CalendarError == nil {
			d.Score.Total = Ptr(sum)
		} else {
			d.Notes = append(d.Notes, "部分行情过期或更新失败，暂停生成当前总评分。分项保留最后观测值，请查看数据时间。")
		}
	}
	if previous == nil && rebuilt == nil {
		d.Notes = append(d.Notes, "昨日成交前50历史成交额待补齐；封炸板池由独立任务重建。")
	}
	if data.CalendarError != nil {
		d.Notes = append(d.Notes, "交易日历："+*data.CalendarError)
	}
	return d
}
