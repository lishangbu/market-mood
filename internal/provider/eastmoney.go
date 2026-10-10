package provider

import (
	"context"
	"fmt"
	"market-mood/internal/market"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (c *Client) eastStock(ctx context.Context, code string) (market.Stock, error) {
	exchange := "0"
	if strings.HasPrefix(code, "6") {
		exchange = "1"
	}
	d, err := c.eastData(ctx, c.QuoteBase+"/api/qt/stock/get", url.Values{"secid": {exchange + "." + code}, "fields": {"f43,f46,f48,f51,f52,f57,f58,f60,f86,f170,f59,f116,f117,f168"}})
	if err != nil {
		return market.Stock{}, err
	}
	if text(d["f57"]) != code {
		return market.Stock{}, fmt.Errorf("股票代码不匹配")
	}
	precision := uintValue(d["f59"])
	if precision == nil || *precision > 8 {
		return market.Stock{}, fmt.Errorf("报价小数位缺失或异常")
	}
	scale := math.Pow10(-int(*precision))
	name := text(d["f58"])
	return market.Stock{Code: code, Name: name, Eligible: Eligible(code, name), Price: scaled(number(d["f43"]), scale), Open: scaled(number(d["f46"]), scale), PreviousClose: scaled(number(d["f60"]), scale), Change: scaled(number(d["f170"]), .01), Amount: number(d["f48"]), FloatCap: number(d["f117"]), MarketCap: number(d["f116"]), TurnoverRate: scaled(number(d["f168"]), .01), SourceTime: timestamp(d["f86"]), LimitUp: scaled(number(d["f51"]), scale), LimitDown: scaled(number(d["f52"]), scale)}, nil
}

type quotePage struct {
	total  int
	stocks []market.Stock
}

func (c *Client) marketPage(ctx context.Context, page int) (quotePage, error) {
	d, err := c.eastData(ctx, c.QuoteBase+"/api/qt/clist/get", url.Values{"pn": {strconv.Itoa(page)}, "pz": {"100"}, "po": {"0"}, "np": {"1"}, "fltt": {"2"}, "invt": {"2"}, "fid": {"f12"}, "fs": {"m:0+t:6,m:0+t:80,m:1+t:2,m:1+t:23"}, "fields": {"f12,f14,f2,f3,f6,f8,f20,f21,f17,f18,f100,f124"}})
	if err != nil {
		return quotePage{}, err
	}
	total := uintValue(d["total"])
	if total == nil {
		return quotePage{}, fmt.Errorf("缺少市场总数")
	}
	rows, err := array(d["diff"])
	if err != nil {
		return quotePage{}, err
	}
	result := quotePage{total: int(*total), stocks: make([]market.Stock, 0, len(rows))}
	for _, row := range rows {
		result.stocks = append(result.stocks, eastQuote(row))
	}
	return result, nil
}
func (c *Client) eastMarket(ctx context.Context) ([]market.Stock, error) {
	first, err := c.marketPage(ctx, 1)
	if err != nil {
		return nil, err
	}
	if first.total == 0 || len(first.stocks) == 0 {
		return nil, fmt.Errorf("全市场快照为空")
	}
	pages := (first.total + len(first.stocks) - 1) / len(first.stocks)
	if pages > 150 {
		return nil, fmt.Errorf("市场分页数量异常")
	}
	rest, err := batches(ctx, pages-1, func(ctx context.Context, i int) (quotePage, error) { return c.marketPage(ctx, i+2) })
	if err != nil {
		return nil, err
	}
	stocks := first.stocks
	for _, page := range rest {
		if page.total != first.total {
			return nil, fmt.Errorf("分页期间市场范围发生变化")
		}
		stocks = append(stocks, page.stocks...)
	}
	codes := make([]string, len(stocks))
	for i, s := range stocks {
		codes[i] = s.Code
	}
	if len(stocks) != first.total {
		return nil, fmt.Errorf("全市场分页不完整")
	}
	if err := validateCodes(stocks, codes); err != nil {
		return nil, err
	}
	return stocks, nil
}
func (c *Client) limitPool(ctx context.Context, day string, sealed bool) ([]market.Stock, error) {
	endpoint := "getTopicZBPool"
	if sealed {
		endpoint = "getTopicZTPool"
	}
	date := strings.ReplaceAll(day, "-", "")
	all := []market.Stock{}
	expected := -1
	for page := 0; page < 30; page++ {
		data, err := c.eastData(ctx, c.LimitBase+"/"+endpoint, url.Values{"ut": {"7eea3edcaed734bea9cbfc24409ed989"}, "dpt": {"wz.ztzt"}, "Pageindex": {strconv.Itoa(page)}, "pagesize": {"100"}, "sort": {"fbt:asc"}, "date": {date}})
		if err != nil {
			return nil, err
		}
		qdate := text(data["qdate"])
		if qdate == "" {
			qdate = string(data["qdate"])
		}
		if qdate != date {
			return nil, fmt.Errorf("涨停池日期与请求日期不符")
		}
		total := uintValue(data["tc"])
		if total == nil {
			return nil, fmt.Errorf("缺少股票池总数")
		}
		if expected >= 0 && expected != int(*total) {
			return nil, fmt.Errorf("股票池分页总数变化")
		}
		expected = int(*total)
		if expected == 0 {
			return all, nil
		}
		rows, err := array(data["pool"])
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return nil, fmt.Errorf("股票池分页不完整")
		}
		for _, row := range rows {
			all = append(all, limitQuote(row, sealed))
		}
		if len(all) >= expected {
			if len(all) != expected {
				return nil, fmt.Errorf("股票池长度异常")
			}
			codes := make([]string, len(all))
			for i, s := range all {
				codes[i] = s.Code
			}
			return all, validateCodes(all, codes)
		}
	}
	return nil, fmt.Errorf("股票池分页超限")
}
func (c *Client) Limits(ctx context.Context, day string) (market.LimitFrame, error) {
	if _, err := time.Parse(time.DateOnly, day); err != nil {
		return market.LimitFrame{}, err
	}
	pools, err := batches(ctx, 2, func(ctx context.Context, i int) ([]market.Stock, error) { return c.limitPool(ctx, day, i == 0) })
	if err != nil {
		return market.LimitFrame{}, err
	}
	return market.LimitFrame{Day: day, CapturedAt: c.Now().Format(time.RFC3339), Sealed: pools[0], Broken: pools[1]}, nil
}
