// Package backfill imports verified index/limit history without fabricating sentiment.
package backfill

import (
	"context"
	"encoding/json"
	"fmt"
	"market-mood/internal/market"
	"market-mood/internal/provider"
	"market-mood/internal/provider/tonghuashun"
	"market-mood/internal/provider/transport"
	"market-mood/internal/trading"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

type DailyBar struct {
	Day    string  `json:"day"`
	Open   float64 `json:"open"`
	Close  float64 `json:"close"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Volume float64 `json:"volume"`
}
type HistoricalDay struct {
	FieldsVersion uint32             `json:"fields_version"`
	Day           string             `json:"day"`
	ImportedAt    string             `json:"imported_at"`
	Source        string             `json:"source"`
	Shanghai      DailyBar           `json:"shanghai"`
	Shenzhen      DailyBar           `json:"shenzhen"`
	Limits        *market.LimitFrame `json:"limits"`
	Limitation    *string            `json:"limitation"`
}
type Status struct {
	TargetDays []string `json:"target_days"`
	Completed  int      `json:"completed"`
	Running    bool     `json:"running"`
	Error      *string  `json:"error"`
	UpdatedAt  string   `json:"updated_at"`
}

func ValidateBars(bars []DailyBar) error {
	rows := make([]market.Bar, len(bars))
	for i, b := range bars {
		rows[i] = market.Bar{Day: b.Day, Open: b.Open, Close: b.Close, High: b.High, Low: b.Low, Volume: b.Volume}
	}
	return market.ValidateBars(rows)
}
func ValidateDay(day HistoricalDay) error {
	if day.FieldsVersion != 1 {
		return fmt.Errorf("historical fields_version=%d 不支持", day.FieldsVersion)
	}
	if _, err := time.Parse(time.DateOnly, day.Day); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339, day.ImportedAt); err != nil {
		return err
	}
	if day.Shanghai.Day != day.Day || day.Shenzhen.Day != day.Day || (day.Limits != nil && day.Limits.Day != day.Day) {
		return fmt.Errorf("历史记录日期不一致")
	}
	if err := ValidateBars([]DailyBar{day.Shanghai}); err != nil {
		return err
	}
	return ValidateBars([]DailyBar{day.Shenzhen})
}
func TargetDays(sh, sz []DailyBar, at time.Time) []string {
	at = at.In(trading.Shanghai)
	today := at.Format(time.DateOnly)
	common := map[string]bool{}
	for _, b := range sz {
		common[b.Day] = true
	}
	days := []string{}
	for _, b := range sh {
		if common[b.Day] && (b.Day < today || (b.Day == today && at.Hour() >= 16)) {
			days = append(days, b.Day)
		}
	}
	slices.Sort(days)
	days = slices.Compact(days)
	slices.Reverse(days)
	return days[:min(len(days), 5)]
}
func ParseIndex(raw json.RawMessage) ([]DailyBar, error) {
	var data [][]json.RawMessage
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	bars := make([]DailyBar, 0, len(data))
	for _, row := range data {
		if len(row) < 6 {
			return nil, fmt.Errorf("指数日线字段不足")
		}
		var d DailyBar
		if err := json.Unmarshal(row[0], &d.Day); err != nil {
			return nil, err
		}
		for i, target := range []*float64{&d.Open, &d.Close, &d.High, &d.Low, &d.Volume} {
			var value string
			if err := json.Unmarshal(row[i+1], &value); err != nil {
				return nil, err
			}
			n, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return nil, err
			}
			*target = n
		}
		bars = append(bars, d)
	}
	slices.SortFunc(bars, func(a, b DailyBar) int { return strings.Compare(a.Day, b.Day) })
	return bars, ValidateBars(bars)
}

type Client struct {
	Quotes *provider.Client
	THS    *tonghuashun.Client
}

func NewClient(quotes *provider.Client) *Client {
	return &Client{Quotes: quotes, THS: tonghuashun.New(quotes.HTTP)}
}
func (c *Client) Index(ctx context.Context, symbol string) ([]DailyBar, error) {
	if symbol != "sh000001" && symbol != "sz399001" {
		return nil, fmt.Errorf("不支持的指数")
	}
	query := url.Values{"param": {symbol + ",day,,,60,qfq"}}
	body, err := transport.Get(ctx, c.Quotes.HTTP, c.Quotes.TencentHistoryBase+"/appstock/app/fqkline/get?"+query.Encode(), "", 4<<20)
	if err != nil {
		return nil, err
	}
	var value struct {
		Code *int `json:"code"`
		Data map[string]struct {
			Day json.RawMessage `json:"day"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &value); err != nil {
		return nil, err
	}
	if value.Code == nil || *value.Code != 0 {
		return nil, fmt.Errorf("历史指数接口错误")
	}
	return ParseIndex(value.Data[symbol].Day)
}
func (c *Client) Limits(ctx context.Context, day string) (market.LimitFrame, string, error) {
	frame, err := c.Quotes.Limits(ctx, day)
	if err == nil {
		return frame, "东方财富历史封炸池", nil
	}
	frame, secondary := c.THS.Limits(ctx, day)
	if secondary != nil {
		return market.LimitFrame{}, "", fmt.Errorf("东方财富：%v；同花顺：%w", err, secondary)
	}
	return frame, tonghuashun.PoolSource, nil
}

type Store interface {
	BackfillStatus(context.Context) (Status, error)
	SaveBackfillStatus(context.Context, Status) error
	HistoricalDay(context.Context, string) (*HistoricalDay, error)
	HistoricalCount(context.Context, []string) (int, error)
	SaveBackfillDay(context.Context, HistoricalDay, Status) (Status, error)
}
type Source interface {
	Index(context.Context, string) ([]DailyBar, error)
	Limits(context.Context, string) (market.LimitFrame, string, error)
}
type Engine struct {
	Store  Store
	Source Source
	Now    func() time.Time
}

func (e *Engine) Run(ctx context.Context) (result error) {
	status, err := e.Store.BackfillStatus(ctx)
	if err != nil {
		return err
	}
	status.Running = true
	status.Error = nil
	status.UpdatedAt = e.Now().Format(time.RFC3339)
	defer func() {
		if result != nil {
			status.Running = false
			status.Error = market.Ptr(result.Error())
			status.UpdatedAt = e.Now().Format(time.RFC3339)
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			defer cancel()
			if err := e.Store.SaveBackfillStatus(cleanup, status); err != nil {
				result = fmt.Errorf("%v；保存补录错误: %w", result, err)
			}
		}
	}()
	if err := e.Store.SaveBackfillStatus(ctx, status); err != nil {
		return err
	}
	type response struct {
		symbol string
		bars   []DailyBar
		err    error
	}
	results := make(chan response, 2)
	for _, symbol := range []string{"sh000001", "sz399001"} {
		go func(symbol string) { bars, err := e.Source.Index(ctx, symbol); results <- response{symbol, bars, err} }(symbol)
	}
	groups := map[string][]DailyBar{}
	var failure error
	for range 2 {
		r := <-results
		if r.err != nil {
			failure = r.err
		} else if err := ValidateBars(r.bars); err != nil {
			failure = fmt.Errorf("%s: %w", r.symbol, err)
		} else {
			groups[r.symbol] = r.bars
		}
	}
	if failure != nil {
		return failure
	}
	sh, sz := groups["sh000001"], groups["sz399001"]
	targets := TargetDays(sh, sz, e.Now())
	if len(targets) != 5 {
		return fmt.Errorf("历史接口不足5个完整交易日")
	}
	completed, err := e.Store.HistoricalCount(ctx, targets)
	if err != nil {
		return err
	}
	status.TargetDays, status.Completed = targets, completed
	if err := e.Store.SaveBackfillStatus(ctx, status); err != nil {
		return err
	}
	for _, day := range status.TargetDays {
		cached, err := e.Store.HistoricalDay(ctx, day)
		if err != nil {
			return err
		}
		if cached != nil && cached.Limits != nil {
			continue
		}
		a := sh[slices.IndexFunc(sh, func(b DailyBar) bool { return b.Day == day })]
		b := sz[slices.IndexFunc(sz, func(b DailyBar) bool { return b.Day == day })]
		frame, source, poolErr := e.Source.Limits(ctx, day)
		var limits *market.LimitFrame
		var limitation string
		if poolErr != nil {
			limitation = "封炸池不可补录：" + poolErr.Error() + "。仅有指数日线；情绪评分、股票池表现及盘中曲线缺失。"
			source = "腾讯指数日线"
		} else {
			limits = &frame
			source = "腾讯指数日线；" + source + "（严格校验日期）"
			limitation = "指数及封炸池已补录；专题源未提供的字段保留暂无。缺少当时完整股票行情，未重建情绪评分、市场宽度或昨日股票池表现。"
		}
		historical := HistoricalDay{FieldsVersion: 1, Day: day, ImportedAt: e.Now().Format(time.RFC3339), Source: source, Shanghai: a, Shenzhen: b, Limits: limits, Limitation: &limitation}
		status, err = e.Store.SaveBackfillDay(ctx, historical, status)
		if err != nil {
			return err
		}
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	status.Running = false
	status.UpdatedAt = e.Now().Format(time.RFC3339)
	return e.Store.SaveBackfillStatus(ctx, status)
}
