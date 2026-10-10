package sina

import (
	"context"
	"encoding/json"
	"fmt"
	"market-mood/internal/market"
	"market-mood/internal/provider/transport"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type Client struct {
	HTTP                    *http.Client
	HistoryBase, PricesBase string
}

func New(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = transport.NewClient()
	}
	return &Client{HTTP: httpClient, HistoryBase: "https://finance.sina.com.cn", PricesBase: "https://quotes.sina.cn"}
}
func symbol(code string) (string, error) {
	if len(code) != 6 {
		return "", fmt.Errorf("股票代码无效")
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return "", fmt.Errorf("股票代码无效")
		}
	}
	if strings.HasPrefix(code, "60") || strings.HasPrefix(code, "688") {
		return "sh" + code, nil
	}
	if strings.HasPrefix(code, "00") || strings.HasPrefix(code, "30") {
		return "sz" + code, nil
	}
	return "", fmt.Errorf("新浪日线仅支持沪深A股")
}
func (c *Client) Daily(ctx context.Context, code string) ([]market.Bar, error) {
	sym, err := symbol(code)
	if err != nil {
		return nil, err
	}
	body, err := transport.Get(ctx, c.HTTP, c.HistoryBase+"/realstock/company/"+sym+"/hisdata_klc2/klc_kl.js", "https://finance.sina.com.cn/", maxPayload+1024)
	if err != nil {
		return nil, err
	}
	rows, err := Decode(string(body), sym)
	if err != nil {
		return nil, err
	}
	// Rebuild requires recent rows only; full decoding validates the delta chain.
	if len(rows) > 15 {
		rows = rows[len(rows)-15:]
	}
	return rows, nil
}
func (c *Client) Prices(ctx context.Context, code string) ([]market.Bar, error) {
	sym, err := symbol(code)
	if err != nil {
		return nil, err
	}
	query := url.Values{"symbol": {sym}, "scale": {"240"}, "ma": {"no"}, "datalen": {"15"}}
	body, err := transport.Get(ctx, c.HTTP, c.PricesBase+"/cn/api/jsonp_v2.php/=/CN_MarketDataService.getKLineData?"+query.Encode(), "https://finance.sina.com.cn/", 1<<20)
	if err != nil {
		return nil, err
	}
	return ParsePrices(string(body))
}

// ParsePrices accepts only the known JSONP envelope; no script is executed.
func ParsePrices(body string) ([]market.Bar, error) {
	_, payload, ok := strings.Cut(body, "=(")
	if !ok {
		return nil, fmt.Errorf("新浪日线格式异常")
	}
	payload = strings.TrimSpace(payload)
	if !strings.HasSuffix(payload, ");") {
		return nil, fmt.Errorf("新浪日线结束符异常")
	}
	var raw []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSuffix(payload, ");")), &raw); err != nil {
		return nil, err
	}
	rows := make([]market.Bar, 0, len(raw))
	for i, row := range raw {
		var bar market.Bar
		if err := json.Unmarshal(row["day"], &bar.Day); err != nil {
			return nil, fmt.Errorf("日线 %d 日期: %w", i, err)
		}
		for _, field := range []struct {
			name   string
			target *float64
		}{{"open", &bar.Open}, {"close", &bar.Close}, {"high", &bar.High}, {"low", &bar.Low}, {"volume", &bar.Volume}} {
			var s string
			if err := json.Unmarshal(row[field.name], &s); err != nil {
				return nil, fmt.Errorf("日线 %d %s: %w", i, field.name, err)
			}
			value, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return nil, fmt.Errorf("日线 %d %s: %w", i, field.name, err)
			}
			*field.target = value
		}
		rows = append(rows, bar)
	}
	return rows, market.ValidateBars(rows)
}
