package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/text/encoding/simplifiedchinese"
	"market-mood/internal/market"
	"market-mood/internal/provider/tdx"
	"market-mood/internal/provider/transport"
	"market-mood/internal/trading"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type Identity struct{ Name, Industry string }
type Client struct {
	HTTP                                                                                  *http.Client
	TDX                                                                                   *tdx.Client
	QuoteBase, HistoryBase, LimitBase, TencentBase, TencentHistoryBase, SinaBase, THSBase string
	Now                                                                                   func() time.Time
	mu                                                                                    sync.RWMutex
	config                                                                                Config
	directory                                                                             map[string]Identity
	calendarMu                                                                            sync.Mutex
	calendar                                                                              []string
	calendarAt                                                                            time.Time
	universeMu                                                                            sync.Mutex
	universe                                                                              []string
	universeAt                                                                            time.Time
}

func New(config Config) (*Client, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	base := os.Getenv("MARKET_MOOD_QUOTE_BASE")
	if base == "" {
		base = "https://push2.eastmoney.com"
	}
	return &Client{HTTP: transport.NewClient(), TDX: tdx.New(), QuoteBase: strings.TrimRight(base, "/"), HistoryBase: "https://push2his.eastmoney.com", LimitBase: "https://push2ex.eastmoney.com", TencentBase: "https://qt.gtimg.cn", TencentHistoryBase: "https://web.ifzq.gtimg.cn", SinaBase: "https://vip.stock.finance.sina.com.cn", THSBase: "https://d.10jqka.com.cn", Now: trading.Now, config: config.Clone(), directory: map[string]Identity{}}, nil
}
func (c *Client) Config() Config { c.mu.RLock(); defer c.mu.RUnlock(); return c.config.Clone() }
func (c *Client) SetConfig(config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config = config.Clone()
	return nil
}
func (c *Client) SetDirectory(directory map[string]Identity) {
	copied := make(map[string]Identity, len(directory))
	for code, s := range directory {
		copied[code] = s
	}
	c.mu.Lock()
	c.directory = copied
	c.mu.Unlock()
}
func (c *Client) json(ctx context.Context, path string, query url.Values, target any) error {
	if len(query) > 0 {
		path += "?" + strings.ReplaceAll(query.Encode(), "%2C", ",")
	}
	body, err := transport.Get(ctx, c.HTTP, path, "https://quote.eastmoney.com/", 16<<20)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("行情响应无法解析: %w", err)
	}
	return nil
}
func (c *Client) eastData(ctx context.Context, path string, query url.Values) (object, error) {
	var envelope struct {
		RC   int    `json:"rc"`
		Data object `json:"data"`
	}
	if err := c.json(ctx, path, query, &envelope); err != nil {
		return nil, err
	}
	if envelope.RC != 0 || envelope.Data == nil {
		return nil, fmt.Errorf("数据源返回错误或暂无数据 rc=%d", envelope.RC)
	}
	return envelope.Data, nil
}
func (c *Client) Calendar(ctx context.Context) ([]string, error) {
	c.calendarMu.Lock()
	defer c.calendarMu.Unlock()
	if len(c.calendar) > 0 && time.Since(c.calendarAt) < time.Minute {
		return slices.Clone(c.calendar), nil
	}
	days, err := c.eastCalendar(ctx)
	if err != nil {
		days, err = c.tencentCalendar(ctx)
	}
	if err != nil {
		return nil, err
	}
	c.calendar, c.calendarAt = days, time.Now()
	return slices.Clone(days), nil
}
func validateCalendar(days []string) ([]string, error) {
	if len(days) == 0 {
		return nil, fmt.Errorf("交易日列表为空")
	}
	for _, day := range days {
		if _, err := time.Parse(time.DateOnly, day); err != nil {
			return nil, fmt.Errorf("交易日格式异常: %w", err)
		}
	}
	slices.Sort(days)
	return slices.Compact(days), nil
}
func (c *Client) eastCalendar(ctx context.Context) ([]string, error) {
	year := c.Now().In(trading.Shanghai).Year()
	data, err := c.eastData(ctx, c.HistoryBase+"/api/qt/stock/kline/get", url.Values{"secid": {"1.000001"}, "klt": {"101"}, "fqt": {"0"}, "beg": {fmt.Sprintf("%d0101", year-1)}, "end": {fmt.Sprintf("%d1231", year)}, "fields1": {"f1,f2,f3,f4,f5,f6"}, "fields2": {"f51,f52,f53,f54,f55,f56,f57"}})
	if err != nil {
		return nil, err
	}
	var rows []string
	if err := json.Unmarshal(data["klines"], &rows); err != nil {
		return nil, err
	}
	days := make([]string, 0, len(rows))
	for _, row := range rows {
		day, _, _ := strings.Cut(row, ",")
		days = append(days, day)
	}
	return validateCalendar(days)
}
func (c *Client) tencentCalendar(ctx context.Context) ([]string, error) {
	var value struct {
		Code int `json:"code"`
		Data map[string]struct {
			Day [][]json.RawMessage `json:"day"`
		} `json:"data"`
	}
	if err := c.json(ctx, c.TencentHistoryBase+"/appstock/app/fqkline/get", url.Values{"param": {"sh000001,day,,,640,qfq"}}, &value); err != nil {
		return nil, err
	}
	if value.Code != 0 {
		return nil, fmt.Errorf("腾讯日历错误")
	}
	days := []string{}
	for _, row := range value.Data["sh000001"].Day {
		if len(row) == 0 {
			return nil, fmt.Errorf("腾讯日历空行")
		}
		days = append(days, text(row[0]))
	}
	return validateCalendar(days)
}

// batches runs at most four calls at a time, joining every launched worker before
// returning. Result order follows request order and cancellation reaches all I/O.
func batches[T any](ctx context.Context, count int, fetch func(context.Context, int) (T, error)) ([]T, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make([]T, count)
	for start := 0; start < count; start += 4 {
		end := min(start+4, count)
		var wg sync.WaitGroup
		errs := make([]error, end-start)
		for index := start; index < end; index++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				value, err := fetch(ctx, i)
				if err != nil {
					errs[i-start] = err
					cancel()
					return
				}
				results[i] = value
			}(index)
		}
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				return nil, err
			}
		}
	}
	return results, nil
}
func (c *Client) universeCodes(ctx context.Context) ([]string, error) {
	c.universeMu.Lock()
	defer c.universeMu.Unlock()
	if len(c.universe) > 0 && time.Since(c.universeAt) < time.Hour {
		return slices.Clone(c.universe), nil
	}
	path := c.SinaBase + "/quotes_service/api/json_v2.php/Market_Center."
	body, err := transport.Get(ctx, c.HTTP, path+"getHQNodeStockCount?node=hs_a", "", 1024)
	if err != nil {
		return nil, err
	}
	total, err := strconv.Atoi(strings.Trim(strings.TrimSpace(string(body)), "\""))
	if err != nil || total < 1000 || total >= 10000 {
		return nil, fmt.Errorf("股票目录总数异常")
	}
	pages, err := batches(ctx, (total+99)/100, func(ctx context.Context, i int) ([]object, error) {
		var rows []object
		err := c.json(ctx, path+"getHQNodeData", url.Values{"page": {strconv.Itoa(i + 1)}, "num": {"100"}, "sort": {"symbol"}, "asc": {"1"}, "node": {"hs_a"}, "symbol": {""}, "_s_r_a": {"page"}}, &rows)
		return rows, err
	})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	codes := []string{}
	for _, rows := range pages {
		for _, row := range rows {
			sym := text(row["symbol"])
			if len(sym) != 8 || seen[sym] {
				return nil, fmt.Errorf("股票目录身份异常或重复")
			}
			seen[sym] = true
			if (strings.HasPrefix(sym, "sh") || strings.HasPrefix(sym, "sz")) && ValidCode(sym[2:]) {
				codes = append(codes, sym[2:])
			}
		}
	}
	if len(seen) != total {
		return nil, fmt.Errorf("股票目录分页不完整")
	}
	slices.Sort(codes)
	c.universe, c.universeAt = codes, time.Now()
	return slices.Clone(codes), nil
}
func (c *Client) tencentBatch(ctx context.Context, codes []string) ([]market.Stock, error) {
	symbols := make([]string, len(codes))
	for i, code := range codes {
		symbols[i] = symbol(code)
	}
	raw, err := transport.Get(ctx, c.HTTP, c.TencentBase+"/q="+strings.Join(symbols, ","), "", 4<<20)
	if err != nil {
		return nil, err
	}
	body, err := simplifiedchinese.GBK.NewDecoder().Bytes(raw)
	if err != nil || !utf8.Valid(body) || strings.ContainsRune(string(body), '\ufffd') {
		return nil, fmt.Errorf("行情中文解码失败")
	}
	out := make([]market.Stock, 0, len(codes))
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		s, err := parseTencent(line)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := validateCodes(out, codes); err != nil {
		return nil, err
	}
	return out, nil
}
func validateCodes(stocks []market.Stock, codes []string) error {
	expected := map[string]bool{}
	for _, code := range codes {
		if !ValidCode(code) || expected[code] {
			return fmt.Errorf("请求代码异常或重复")
		}
		expected[code] = true
	}
	if len(stocks) != len(codes) {
		return fmt.Errorf("报价批次不完整")
	}
	for _, s := range stocks {
		if !expected[s.Code] {
			return fmt.Errorf("报价身份错误或重复")
		}
		delete(expected, s.Code)
	}
	return nil
}
func (c *Client) tdxStocks(ctx context.Context, codes []string) ([]market.Stock, error) {
	directory := map[string]Identity{}
	c.mu.RLock()
	for _, code := range codes {
		if s, ok := c.directory[code]; ok {
			directory[code] = s
		}
	}
	c.mu.RUnlock()
	if len(directory) != len(codes) {
		return nil, fmt.Errorf("通达信行情需要完整本地股票目录")
	}
	stocks, err := c.TDX.Quotes(ctx, codes)
	if err != nil {
		return nil, err
	}
	for i := range stocks {
		s := &stocks[i]
		basic := directory[s.Code]
		s.Name, s.Industry = basic.Name, basic.Industry
		s.Eligible = Eligible(s.Code, s.Name)
		s.Provider = market.Ptr(TDX.Name())
	}
	return stocks, nil
}
func (c *Client) Stock(ctx context.Context, code string) (market.Stock, error) {
	if !ValidCode(code) {
		return market.Stock{}, fmt.Errorf("股票代码无效")
	}
	calendar, err := c.Calendar(ctx)
	if err != nil {
		return market.Stock{}, err
	}
	stock, source, reasons, err := Select(ctx, c.Config().StockQuotes, func(ctx context.Context, source Source) (market.Stock, error) {
		var s market.Stock
		var err error
		switch source {
		case TDX, Tencent:
			var rows []market.Stock
			if source == TDX {
				rows, err = c.tdxStocks(ctx, []string{code})
			} else {
				rows, err = c.tencentBatch(ctx, []string{code})
			}
			if err == nil {
				if len(rows) != 1 {
					return s, fmt.Errorf("个股行情为空")
				}
				s = rows[0]
			}
		case Eastmoney:
			s, err = c.eastStock(ctx, code)
		case Tonghuashun:
			var body []byte
			body, err = transport.Get(ctx, c.HTTP, c.THSBase+"/v6/realhead/hs_"+code+"/last.js", "https://q.10jqka.com.cn/", 1<<20)
			if err == nil {
				s, err = parseTHS(string(body), code)
			}
		default:
			err = fmt.Errorf("未知来源")
		}
		if err != nil {
			return s, err
		}
		day := ""
		if s.SourceTime != nil && len(*s.SourceTime) >= 10 {
			day = (*s.SourceTime)[:10]
		}
		if trading.Stale(s.SourceTime, day, calendar, c.Now()) {
			return s, fmt.Errorf("个股行情过期或时间缺失")
		}
		return s, nil
	})
	if err != nil {
		return market.Stock{}, err
	}
	stock.Provider = market.Ptr(source.Name())
	stock.FallbackReasons = reasons
	return stock, nil
}
func (c *Client) Market(ctx context.Context) (market.MarketFrame, error) {
	calendar, err := c.Calendar(ctx)
	if err != nil {
		return market.MarketFrame{}, err
	}
	frame, source, reasons, err := Select(ctx, c.Config().Quotes, func(ctx context.Context, source Source) (market.MarketFrame, error) {
		var stocks []market.Stock
		var err error
		switch source {
		case Eastmoney:
			stocks, err = c.eastMarket(ctx)
		case Tencent, TDX:
			var codes []string
			codes, err = c.universeCodes(ctx)
			if err == nil {
				if source == TDX {
					stocks, err = c.tdxStocks(ctx, codes)
				} else {
					var chunks [][]market.Stock
					chunks, err = batches(ctx, (len(codes)+59)/60, func(ctx context.Context, i int) ([]market.Stock, error) {
						return c.tencentBatch(ctx, codes[i*60:min(i*60+60, len(codes))])
					})
					for _, rows := range chunks {
						stocks = append(stocks, rows...)
					}
				}
			}
		default:
			err = fmt.Errorf("来源不支持全市场")
		}
		if err != nil {
			return market.MarketFrame{}, err
		}
		at := c.Now()
		day, observed, err := FrameTime(stocks, at)
		if err != nil {
			return market.MarketFrame{}, err
		}
		if trading.Stale(&observed, day, calendar, at) {
			return market.MarketFrame{}, fmt.Errorf("行情过期或时间缺失")
		}
		return market.MarketFrame{Day: day, CapturedAt: at.Format(time.RFC3339), SourceTime: &observed, Stocks: stocks, Complete: true}, nil
	})
	if err != nil {
		return market.MarketFrame{}, err
	}
	frame.Provider = market.Ptr(source.Name())
	frame.FallbackReasons = reasons
	for i := range frame.Stocks {
		frame.Stocks[i].Provider = frame.Provider
		frame.Stocks[i].FallbackReasons = slices.Clone(reasons)
	}
	return frame, nil
}
