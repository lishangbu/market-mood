package catalog

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func optional(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" {
		return nil
	}
	return &value
}
func normalizedDate(value string) *string {
	for _, layout := range []string{"20060102", time.DateOnly} {
		if d, err := time.Parse(layout, value); err == nil {
			v := d.Format(time.DateOnly)
			return &v
		}
	}
	return nil
}
func plainName(value string) string {
	var out strings.Builder
	inTag := false
	for _, c := range value {
		switch c {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				out.WriteRune(c)
			}
		}
	}
	return out.String()
}

type shRow struct {
	Code        string `json:"A_STOCK_CODE"`
	Name        string `json:"SEC_NAME_CN"`
	ListingDate string `json:"LIST_DATE"`
	Industry    string `json:"CSRC_CODE_DESC"`
}

func (row shRow) security() (Security, error) {
	s, err := NewSecurity(row.Code, row.Name, "sh", "上交所")
	s.ListingDate = normalizedDate(row.ListingDate)
	s.Industry = optional(row.Industry)
	return s, err
}

type szRow struct {
	Code        string `json:"agdm"`
	Name        string `json:"agjc"`
	ListingDate string `json:"agssrq"`
	Industry    string `json:"sshymc"`
}

func (row szRow) security() (Security, error) {
	s, err := NewSecurity(row.Code, plainName(row.Name), "sz", "深交所")
	s.ListingDate = normalizedDate(row.ListingDate)
	s.Industry = optional(row.Industry)
	return s, err
}
func (c *Client) shDirectory(ctx context.Context) ([]Security, error) {
	rows := []Security{}
	for _, board := range []string{"1", "8"} {
		var value struct {
			Result   []shRow `json:"result"`
			PageHelp struct {
				Total *int `json:"total"`
			} `json:"pageHelp"`
		}
		err := c.get(ctx, c.SSEBase+"/sseQuery/commonQuery.do", "https://www.sse.com.cn/assortment/stock/list/share/", url.Values{"STOCK_TYPE": {board}, "sqlId": {"COMMON_SSE_CP_GPJCTPZ_GPLB_GP_L"}, "COMPANY_STATUS": {"2,4,5,7,8"}, "type": {"inParams"}, "isPagination": {"true"}, "pageHelp.pageSize": {"10000"}, "pageHelp.pageNo": {"1"}}, &value)
		if err != nil {
			return nil, err
		}
		if value.PageHelp.Total == nil || *value.PageHelp.Total != len(value.Result) {
			return nil, fmt.Errorf("上交所名单分页不完整")
		}
		for _, item := range value.Result {
			if strings.HasPrefix(item.Code, "689") {
				continue
			}
			s, err := item.security()
			if err != nil {
				return nil, err
			}
			rows = append(rows, s)
		}
	}
	return rows, nil
}
func (c *Client) szDirectory(ctx context.Context) ([]Security, error) {
	rows := []Security{}
	expected := 0
	for page := 1; page <= 500; page++ {
		var tabs []struct {
			Metadata struct {
				Tabkey string `json:"tabkey"`
				Count  int    `json:"recordcount"`
			} `json:"metadata"`
			Data []szRow `json:"data"`
		}
		err := c.get(ctx, c.SZSEBase+"/api/report/ShowReport/data", "", url.Values{"SHOWTYPE": {"JSON"}, "CATALOGID": {"1110"}, "TABKEY": {"tab1"}, "PAGENO": {fmt.Sprint(page)}, "PAGESIZE": {"10000"}}, &tabs)
		if err != nil {
			return nil, err
		}
		found := false
		for _, tab := range tabs {
			if tab.Metadata.Tabkey != "tab1" {
				continue
			}
			if found {
				return nil, fmt.Errorf("深交所A股表重复")
			}
			found = true
			count := tab.Metadata.Count
			if count < 1000 || count > 10000 || (expected != 0 && expected != count) {
				return nil, fmt.Errorf("深交所总数异常或变化")
			}
			expected = count
			if len(tab.Data) == 0 {
				return nil, fmt.Errorf("深交所分页为空")
			}
			for _, item := range tab.Data {
				s, err := item.security()
				if err != nil {
					return nil, err
				}
				rows = append(rows, s)
			}
		}
		if !found {
			return nil, fmt.Errorf("深交所A股表缺失")
		}
		if len(rows) >= expected {
			if len(rows) != expected {
				return nil, fmt.Errorf("深交所名单数量不一致")
			}
			return rows, nil
		}
	}
	return nil, fmt.Errorf("深交所分页超过上限")
}
func mergeOfficial(rows []Security, official []Security, exchange string) ([]Security, string) {
	codes := map[string]bool{}
	for _, s := range official {
		if err := s.Validate(); err != nil || s.Exchange != exchange || codes[s.Code] {
			return rows, "官网名单身份或重复代码异常，未采用"
		}
		codes[s.Code] = true
	}
	if len(official) < 1000 {
		return rows, "官网名单数量异常，未采用"
	}
	positions := map[string]int{}
	missing := 0
	for i, s := range rows {
		positions[s.Code] = i
		if s.Exchange == exchange && !codes[s.Code] {
			missing++
		}
	}
	added := 0
	for _, s := range official {
		if i, ok := positions[s.Code]; ok {
			rows[i] = s
		} else {
			positions[s.Code] = len(rows)
			rows = append(rows, s)
			added++
		}
	}
	return rows, fmt.Sprintf("官网核对完成：补充%d只，聚合目录另有%d只未在官网名单中确认（保留，不推断退市）", added, missing)
}
func (c *Client) supplement(ctx context.Context, rows []Security) ([]Security, map[string]string) {
	type result struct {
		exchange string
		rows     []Security
		err      error
	}
	results := make(chan result, 2)
	go func() { r, e := c.shDirectory(ctx); results <- result{"sh", r, e} }()
	go func() { r, e := c.szDirectory(ctx); results <- result{"sz", r, e} }()
	checks := map[string]string{}
	for range 2 {
		r := <-results
		if r.err != nil {
			checks[r.exchange] = "官网核对失败：" + r.err.Error()
		} else {
			rows, checks[r.exchange] = mergeOfficial(rows, r.rows, r.exchange)
		}
	}
	// BSE connectivity is not evidence of a validated official directory.
	form := url.Values{"page": {"0"}, "typejb": {"T"}, "xxfcbj[]": {"2"}, "xxzqdm": {""}, "sortfield": {"xxzqdm"}, "sorttype": {"asc"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BSEBase+"/nqxxController/nqxxCnzq.do", strings.NewReader(form.Encode()))
	if err == nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		var response *http.Response
		response, err = c.HTTP.Do(req)
		if err == nil {
			response.Body.Close()
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				err = fmt.Errorf("HTTP %d", response.StatusCode)
			}
		}
	}
	if err != nil {
		checks["bj"] = "官网核对未完成：" + err.Error() + "；使用聚合目录"
	} else {
		checks["bj"] = "官网可连接，但名单解析尚未验证；使用聚合目录"
	}
	for exchange, result := range checks {
		slog.Info("股票目录名单核对", "exchange", exchange, "result", result)
	}
	return rows, checks
}
