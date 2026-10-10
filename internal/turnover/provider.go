package turnover

import (
	"context"
	"encoding/json"
	"fmt"
	"market-mood/internal/market"
	"market-mood/internal/provider/transport"
	"market-mood/internal/trading"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func amount(raw string) (float64, error) {
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return 0, fmt.Errorf("成交额数值异常 %q", raw)
	}
	return n, nil
}
func ParseEastmoney(body []byte, code string) (Days, error) {
	var v struct {
		RC   int `json:"rc"`
		Data struct {
			Code   string   `json:"code"`
			Trends []string `json:"trends"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, err
	}
	if v.RC != 0 || v.Data.Code != code {
		return nil, fmt.Errorf("指数响应身份异常")
	}
	days, cumulative := Days{}, map[string]float64{}
	for _, row := range v.Data.Trends {
		f := strings.Split(row, ",")
		if len(f) < 7 {
			return nil, fmt.Errorf("分时字段不足")
		}
		day, minute, ok := strings.Cut(f[0], " ")
		if !ok {
			return nil, fmt.Errorf("分时时间异常")
		}
		n, err := amount(f[6])
		if err != nil {
			return nil, err
		}
		cumulative[day] += n
		if err := insert(days, day, minute, cumulative[day]); err != nil {
			return nil, err
		}
	}
	return days, Validate(days)
}
func ParseTencent(body []byte, symbol string) (Days, error) {
	var v struct {
		Code int `json:"code"`
		Data map[string]struct {
			Data []struct {
				Date string   `json:"date"`
				Data []string `json:"data"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, err
	}
	if v.Code != 0 {
		return nil, fmt.Errorf("腾讯分时返回错误")
	}
	days := Days{}
	for _, group := range v.Data[symbol].Data {
		date, err := time.Parse("20060102", group.Date)
		if err != nil {
			return nil, err
		}
		for _, row := range group.Data {
			f := strings.Fields(row)
			if len(f) < 4 || len(f[0]) != 4 {
				return nil, fmt.Errorf("腾讯分钟字段不足")
			}
			if f[0] == "1300" {
				continue
			}
			n, err := amount(f[3])
			if err != nil {
				return nil, err
			}
			if err := insert(days, date.Format(time.DateOnly), f[0][:2]+":"+f[0][2:], n); err != nil {
				return nil, err
			}
		}
	}
	return days, Validate(days)
}

type Client struct {
	HTTP                       *http.Client
	TencentBase, EastmoneyBase string
	Now                        func() time.Time
}

func New(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = transport.NewClient()
	}
	return &Client{HTTP: httpClient, TencentBase: "https://web.ifzq.gtimg.cn", EastmoneyBase: "https://push2his.eastmoney.com", Now: trading.Now}
}
func (c *Client) pair(ctx context.Context, east bool) (Days, error) {
	type result struct {
		index int
		days  Days
		err   error
	}
	results := make(chan result, 2)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for i, symbol := range []string{"sh000001", "sz399001"} {
		go func(i int, symbol string) {
			path := c.TencentBase + "/appstock/app/day/query?code=" + symbol
			code := symbol[2:]
			if east {
				exchange := "1"
				if i == 1 {
					exchange = "0"
				}
				path = c.EastmoneyBase + "/api/qt/stock/trends2/get?secid=" + exchange + "." + code + "&fields1=f1,f2,f3,f4,f5,f6,f7,f8,f9,f10,f11&fields2=f51,f52,f53,f54,f55,f56,f57,f58&ndays=5&iscr=0"
			}
			body, err := transport.Get(ctx, c.HTTP, path, "https://quote.eastmoney.com/", 4<<20)
			var days Days
			if err == nil {
				if east {
					days, err = ParseEastmoney(body, code)
				} else {
					days, err = ParseTencent(body, symbol)
				}
			}
			results <- result{i, days, err}
		}(i, symbol)
	}
	var groups [2]Days
	var failure error
	for range 2 {
		r := <-results
		if r.err != nil {
			if failure == nil {
				failure = r.err
			}
			cancel()
		} else {
			groups[r.index] = r.days
		}
	}
	if failure != nil {
		return nil, failure
	}
	return Combine(groups[0], groups[1])
}

// Fetch switches both exchanges and the entire five-day window together.
func (c *Client) Fetch(ctx context.Context, calendar []string) (State, error) {
	reasons := []string{}
	for _, east := range []bool{false, true} {
		if err := ctx.Err(); err != nil {
			return State{}, err
		}
		days, err := c.pair(ctx, east)
		name := "腾讯行情"
		if east {
			name = "东方财富"
		}
		at := c.Now()
		candidate := State{Days: days, Provider: name, CapturedAt: market.Ptr(at.Format(time.RFC3339))}
		if err == nil && len(calendar) > 0 && trading.Session(calendar, at) != "集合竞价" && candidate.View(calendar, at).Stale {
			err = fmt.Errorf("指数分时已过期")
		}
		if err == nil {
			return candidate, nil
		}
		reasons = append(reasons, name+": "+err.Error())
	}
	return State{}, fmt.Errorf("两市分时采集失败：%s", strings.Join(reasons, "；"))
}
