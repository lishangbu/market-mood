// Package tonghuashun reads public unadjusted daily history and dated limit pools.
package tonghuashun

import (
	"context"
	"encoding/json"
	"fmt"
	"market-mood/internal/market"
	"market-mood/internal/provider"
	"market-mood/internal/provider/transport"
	"market-mood/internal/trading"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const DailySource = "同花顺不复权日线"
const PoolSource = "同花顺历史封炸池"
const fields = "199112,10,330333,330334,1968584,3475914,9004,330329,133971,133970,9001,48,19,330323,330324,330325,330326,330327,330335,330336,330337,330338"

type Client struct {
	HTTP                *http.Client
	DailyBase, PoolBase string
	Now                 func() time.Time
}

func New(client *http.Client) *Client {
	if client == nil {
		client = transport.NewClient()
	}
	return &Client{HTTP: client, DailyBase: "https://d.10jqka.com.cn", PoolBase: "https://data.10jqka.com.cn", Now: trading.Now}
}
func ParseDaily(body, code, period string) ([]market.Bar, error) {
	prefix := "quotebridge_v6_line_hs_" + code + "_00_" + period + "("
	body = strings.TrimSpace(body)
	if !strings.HasPrefix(body, prefix) {
		return nil, fmt.Errorf("同花顺日线身份或复权方式不符")
	}
	payload := strings.TrimRight(strings.TrimPrefix(body, prefix), ";")
	if !strings.HasSuffix(payload, ")") {
		return nil, fmt.Errorf("同花顺日线格式错误")
	}
	var value struct {
		Data *string `json:"data"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSuffix(payload, ")")), &value); err != nil {
		return nil, err
	}
	if value.Data == nil {
		return nil, fmt.Errorf("同花顺日线数据缺失")
	}
	rows := []market.Bar{}
	for _, row := range strings.Split(*value.Data, ";") {
		if row == "" {
			continue
		}
		f := strings.Split(row, ",")
		if len(f) < 7 {
			return nil, fmt.Errorf("同花顺日线字段不足")
		}
		day, err := time.Parse("20060102", f[0])
		if err != nil {
			return nil, err
		}
		n := make([]float64, 6)
		for i := range n {
			n[i], err = strconv.ParseFloat(f[i+1], 64)
			if err != nil {
				return nil, err
			}
		}
		rows = append(rows, market.Bar{Day: day.Format(time.DateOnly), Open: n[0], High: n[1], Low: n[2], Close: n[3], Volume: n[4], Amount: market.Ptr(n[5])})
	}
	return rows, market.ValidateBars(rows)
}
func (c *Client) Daily(ctx context.Context, code, day string) ([]market.Bar, error) {
	if !provider.ValidCode(code) {
		return nil, fmt.Errorf("股票代码无效")
	}
	target, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return nil, err
	}
	fetch := func(period string) ([]market.Bar, error) {
		body, err := transport.Get(ctx, c.HTTP, c.DailyBase+"/v6/line/hs_"+code+"/00/"+period+".js", "https://q.10jqka.com.cn/", 8<<20)
		if err != nil {
			return nil, err
		}
		return ParseDaily(string(body), code, period)
	}
	rows, err := fetch("last")
	if err != nil {
		return nil, err
	}
	if rows[0].Day <= day {
		return rows, nil
	}
	return fetch(strconv.Itoa(target.Year()))
}
