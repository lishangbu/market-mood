package tonghuashun

import (
	"context"
	"encoding/json"
	"fmt"
	"market-mood/internal/market"
	"market-mood/internal/provider"
	"market-mood/internal/provider/transport"
	"market-mood/internal/trading"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func numeric(raw json.RawMessage, negative bool) (*float64, error) {
	s := strings.TrimSpace(string(raw))
	if len(s) > 0 && s[0] == '"' {
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
	}
	if s == "" || s == "null" || s == "--" {
		return nil, nil
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || (!negative && n < 0) {
		return nil, fmt.Errorf("同花顺数值无效 %q", s)
	}
	return &n, nil
}
func eventTime(raw json.RawMessage, day string) (*string, error) {
	n, err := numeric(raw, false)
	if err != nil || n == nil {
		return nil, err
	}
	if *n == 0 {
		return nil, nil
	}
	if math.Trunc(*n) != *n || *n > 253402271999 {
		return nil, fmt.Errorf("同花顺封板时间无效")
	}
	at := time.Unix(int64(*n), 0).In(trading.Shanghai)
	if at.Format(time.DateOnly) != day {
		return nil, fmt.Errorf("同花顺封板事件跨日")
	}
	clock := at.Format("15:04:05")
	if clock < "09:15:00" || clock > "15:00:59" {
		return nil, fmt.Errorf("同花顺封板事件不在交易时段")
	}
	return &clock, nil
}
func statistics(value string) *market.LimitStats {
	if value == "首板" {
		return &market.LimitStats{Days: 1, Count: 1}
	}
	days, count, ok := strings.Cut(value, "天")
	if !ok || !strings.HasSuffix(count, "板") {
		return nil
	}
	d, e1 := strconv.ParseUint(days, 10, 32)
	n, e2 := strconv.ParseUint(strings.TrimSuffix(count, "板"), 10, 32)
	if e1 != nil || e2 != nil || n == 0 || d < n {
		return nil
	}
	return &market.LimitStats{Days: uint32(d), Count: uint32(n)}
}
func parseStock(v map[string]json.RawMessage, day string, sealed bool) (market.Stock, error) {
	var s market.Stock
	var name, code, stats string
	_ = json.Unmarshal(v["code"], &code)
	_ = json.Unmarshal(v["name"], &name)
	_ = json.Unmarshal(v["high_days"], &stats)
	if !provider.ValidCode(code) || strings.TrimSpace(name) == "" {
		return s, fmt.Errorf("同花顺股票身份缺失或无效")
	}
	s = market.Stock{Code: code, Name: name, Eligible: provider.Eligible(code, name), Sealed: &sealed, Provider: market.Ptr(PoolSource), SourceTime: &day, LimitStats: statistics(stats)}
	if sealed && s.LimitStats != nil && s.LimitStats.Days == s.LimitStats.Count {
		s.Boards = &s.LimitStats.Count
	}
	for _, field := range []struct {
		key      string
		target   **float64
		negative bool
	}{{"latest", &s.Price, false}, {"change_rate", &s.Change, true}, {"turnover", &s.Amount, false}, {"currency_value", &s.FloatCap, false}, {"sum_market_value", &s.MarketCap, false}, {"turnover_rate", &s.TurnoverRate, false}} {
		value, err := numeric(v[field.key], field.negative)
		if err != nil {
			return s, fmt.Errorf("字段 %s: %w", field.key, err)
		}
		*field.target = value
	}
	var err error
	s.FirstSealTime, err = eventTime(v["first_limit_up_time"], day)
	if err != nil {
		return s, err
	}
	s.LastSealTime, err = eventTime(v["last_limit_up_time"], day)
	if err != nil {
		return s, err
	}
	if s.FirstSealTime != nil && s.LastSealTime != nil && *s.FirstSealTime > *s.LastSealTime {
		return s, fmt.Errorf("同花顺封板时间倒序")
	}
	return s, nil
}

type poolPage struct {
	total, sealed, broken int
	stocks                []market.Stock
}

func parsePage(body []byte, day string, sealed bool, page int) (poolPage, error) {
	var v struct {
		Status *int `json:"status_code"`
		Data   struct {
			Date string `json:"date"`
			Page struct {
				Total *int `json:"total"`
				Page  *int `json:"page"`
			} `json:"page"`
			Counts struct {
				Today struct {
					Num  *int `json:"num"`
					Open *int `json:"open_num"`
				} `json:"today"`
			} `json:"limit_up_count"`
			Info *[]map[string]json.RawMessage `json:"info"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return poolPage{}, err
	}
	if v.Status == nil || *v.Status != 0 {
		return poolPage{}, fmt.Errorf("同花顺历史池请求失败")
	}
	d := v.Data
	if d.Date != strings.ReplaceAll(day, "-", "") {
		return poolPage{}, fmt.Errorf("同花顺历史池日期不符")
	}
	if d.Page.Total == nil || *d.Page.Total < 0 || *d.Page.Total > 10000 || d.Page.Page == nil || d.Counts.Today.Num == nil || d.Counts.Today.Open == nil || d.Info == nil {
		return poolPage{}, fmt.Errorf("同花顺历史池总数或列表缺失")
	}
	total := *d.Page.Total
	if *d.Page.Page != page && !(total == 0 && *d.Page.Page == 0) {
		return poolPage{}, fmt.Errorf("同花顺历史池页码不符")
	}
	expected := *d.Counts.Today.Open
	if sealed {
		expected = *d.Counts.Today.Num
	}
	if total != expected || *d.Counts.Today.Num < 0 || *d.Counts.Today.Open < 0 {
		return poolPage{}, fmt.Errorf("同花顺池与汇总数量不符")
	}
	if len(*d.Info) > total || (total > 0 && len(*d.Info) == 0) {
		return poolPage{}, fmt.Errorf("同花顺历史池分页不完整")
	}
	out := poolPage{total: total, sealed: *d.Counts.Today.Num, broken: *d.Counts.Today.Open, stocks: []market.Stock{}}
	for _, raw := range *d.Info {
		s, err := parseStock(raw, day, sealed)
		if err != nil {
			return out, err
		}
		out.stocks = append(out.stocks, s)
	}
	return out, nil
}
func (c *Client) pool(ctx context.Context, day string, sealed bool) (poolPage, error) {
	endpoint := "open_limit_pool"
	if sealed {
		endpoint = "limit_up_pool"
	}
	var all poolPage
	seen := map[string]bool{}
	for page := 1; page <= 50; page++ {
		query := url.Values{"page": {strconv.Itoa(page)}, "limit": {"200"}, "filter": {"HS,GEM2STAR"}, "order_field": {"199112"}, "order_type": {"0"}, "date": {strings.ReplaceAll(day, "-", "")}, "field": {fields}}
		body, err := transport.Get(ctx, c.HTTP, c.PoolBase+"/dataapi/limit_up/"+endpoint+"?"+query.Encode(), "https://data.10jqka.com.cn/", 4<<20)
		if err != nil {
			return all, err
		}
		next, err := parsePage(body, day, sealed, page)
		if err != nil {
			return all, err
		}
		if page == 1 {
			all = next
			all.stocks = []market.Stock{}
		} else if all.total != next.total || all.sealed != next.sealed || all.broken != next.broken {
			return all, fmt.Errorf("同花顺历史池分页汇总变化")
		}
		for _, s := range next.stocks {
			if seen[s.Code] {
				return all, fmt.Errorf("同花顺历史池重复股票")
			}
			seen[s.Code] = true
			all.stocks = append(all.stocks, s)
		}
		if len(all.stocks) > all.total {
			return all, fmt.Errorf("同花顺历史池数量超出总数")
		}
		if len(all.stocks) == all.total {
			return all, nil
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return all, ctx.Err()
		case <-timer.C:
		}
	}
	return all, fmt.Errorf("同花顺历史池分页超限")
}
func combine(day string, sealed, broken poolPage, at time.Time) (market.LimitFrame, error) {
	if sealed.sealed != broken.sealed || sealed.broken != broken.broken {
		return market.LimitFrame{}, fmt.Errorf("同花顺封炸池汇总不一致")
	}
	seen := map[string]bool{}
	for _, s := range sealed.stocks {
		seen[s.Code] = true
	}
	for _, s := range broken.stocks {
		if seen[s.Code] {
			return market.LimitFrame{}, fmt.Errorf("同花顺封炸池股票重叠")
		}
	}
	return market.LimitFrame{Day: day, CapturedAt: at.Format(time.RFC3339), Sealed: sealed.stocks, Broken: broken.stocks}, nil
}
func (c *Client) Limits(ctx context.Context, day string) (market.LimitFrame, error) {
	target, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return market.LimitFrame{}, err
	}
	at := c.Now().In(trading.Shanghai)
	today := at.Format(time.DateOnly)
	if target.Format(time.DateOnly) > today || (day == today && at.Hour() < 16) {
		return market.LimitFrame{}, fmt.Errorf("同花顺历史补录仅接受已结束日期")
	}
	sealed, err := c.pool(ctx, day, true)
	if err != nil {
		return market.LimitFrame{}, err
	}
	broken, err := c.pool(ctx, day, false)
	if err != nil {
		return market.LimitFrame{}, err
	}
	return combine(day, sealed, broken, at)
}
