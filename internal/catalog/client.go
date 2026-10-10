package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"market-mood/internal/provider/transport"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	HTTP                                                *http.Client
	EastmoneyBase, SinaBase, SSEBase, SZSEBase, BSEBase string
}
type Result struct {
	Securities      []Security
	FallbackReasons []string
	ExchangeChecks  map[string]string
}

func New(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = transport.NewClient()
		httpClient.Timeout = 20 * time.Second
	}
	return &Client{HTTP: httpClient, EastmoneyBase: "https://push2.eastmoney.com", SinaBase: "https://vip.stock.finance.sina.com.cn", SSEBase: "https://query.sse.com.cn", SZSEBase: "https://www.szse.cn", BSEBase: "https://www.bse.cn"}
}
func (c *Client) get(ctx context.Context, path, referer string, query url.Values, target any) error {
	if len(query) > 0 {
		path += "?" + strings.ReplaceAll(query.Encode(), "%2C", ",")
	}
	body, err := transport.Get(ctx, c.HTTP, path, referer, 16<<20)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}
func (c *Client) eastmoney(ctx context.Context) ([]Security, error) {
	rows := []Security{}
	expected := 0
	for page := 1; page <= 100; page++ {
		var data struct {
			RC   int `json:"rc"`
			Data struct {
				Total int `json:"total"`
				Diff  []struct {
					Code     string `json:"f12"`
					Name     string `json:"f14"`
					Market   *int   `json:"f13"`
					Industry string `json:"f100"`
				} `json:"diff"`
			} `json:"data"`
		}
		err := c.get(ctx, c.EastmoneyBase+"/api/qt/clist/get", "https://quote.eastmoney.com/", url.Values{"pn": {strconv.Itoa(page)}, "pz": {"100"}, "np": {"1"}, "fid": {"f12"}, "po": {"0"}, "fs": {"m:0+t:6,m:0+t:80,m:1+t:2,m:1+t:23,m:0+t:81+s:2048"}, "fields": {"f12,f14,f13,f100"}}, &data)
		if err != nil {
			return nil, err
		}
		count := data.Data.Total
		if data.RC != 0 || count < 2100 || count > 10000 || (expected != 0 && expected != count) {
			return nil, fmt.Errorf("目录错误码或分页总数异常")
		}
		expected = count
		if len(data.Data.Diff) == 0 {
			return nil, fmt.Errorf("目录分页为空")
		}
		for _, item := range data.Data.Diff {
			exchange := ""
			if strings.HasPrefix(item.Code, "4") || strings.HasPrefix(item.Code, "8") || strings.HasPrefix(item.Code, "92") {
				exchange = "bj"
			} else if item.Market != nil && *item.Market == 1 {
				exchange = "sh"
			} else if item.Market != nil && *item.Market == 0 {
				exchange = "sz"
			}
			row, err := NewSecurity(item.Code, item.Name, exchange, "东方财富")
			if err != nil {
				return nil, err
			}
			row.Industry = optional(item.Industry)
			rows = append(rows, row)
		}
		if len(rows) >= count {
			if len(rows) != count {
				return nil, fmt.Errorf("目录分页数量不一致")
			}
			return rows, nil
		}
	}
	return nil, fmt.Errorf("目录分页超过上限")
}
func (c *Client) sina(ctx context.Context) ([]Security, error) {
	base := c.SinaBase + "/quotes_service/api/json_v2.php/Market_Center"
	body, err := transport.Get(ctx, c.HTTP, base+".getHQNodeStockCount?node=hs_a", "", 1024)
	if err != nil {
		return nil, err
	}
	count, err := strconv.Atoi(strings.Trim(strings.TrimSpace(string(body)), "\""))
	if err != nil || count < 2100 || count > 10000 {
		return nil, fmt.Errorf("新浪目录总数异常")
	}
	rows := make([]Security, 0, count)
	for page := 1; page <= (count+99)/100; page++ {
		var items []struct {
			Symbol string `json:"symbol"`
			Code   string `json:"code"`
			Name   string `json:"name"`
		}
		if err := c.get(ctx, base+".getHQNodeData", "", url.Values{"page": {strconv.Itoa(page)}, "num": {"100"}, "sort": {"symbol"}, "asc": {"1"}, "node": {"hs_a"}, "symbol": {""}, "_s_r_a": {"page"}}, &items); err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, fmt.Errorf("新浪目录分页为空")
		}
		for _, item := range items {
			if len(item.Symbol) != 8 || item.Symbol[2:] != item.Code {
				return nil, fmt.Errorf("新浪目录代码不一致")
			}
			row, err := NewSecurity(item.Code, item.Name, item.Symbol[:2], "新浪")
			if err != nil {
				return nil, err
			}
			rows = append(rows, row)
		}
	}
	if len(rows) != count {
		return nil, fmt.Errorf("新浪目录分页不完整")
	}
	return rows, nil
}
func (c *Client) Fetch(ctx context.Context) (Result, error) {
	reasons := []string{}
	for _, source := range []string{"东方财富", "新浪"} {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		var rows []Security
		var err error
		if source == "东方财富" {
			rows, err = c.eastmoney(ctx)
		} else {
			rows, err = c.sina(ctx)
		}
		if err == nil {
			_, err = ValidateDirectory(rows)
		}
		if err != nil {
			reasons = append(reasons, source+"："+err.Error())
			continue
		}
		rows, checks := c.supplement(ctx, rows)
		for exchange, check := range c.SupplementListingDates(ctx, rows) {
			checks[exchange] += "；" + check
		}
		if _, err := ValidateDirectory(rows); err != nil {
			return Result{}, err
		}
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		return Result{Securities: rows, FallbackReasons: reasons, ExchangeChecks: checks}, nil
	}
	return Result{}, fmt.Errorf("股票目录同步失败：%s", strings.Join(reasons, "；"))
}
