package rebuild

import (
	"context"
	"encoding/json"
	"fmt"
	"market-mood/internal/catalog"
	"market-mood/internal/provider"
	"market-mood/internal/provider/transport"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Evidence struct {
	HTTP                  *http.Client
	SinaBase, SuspendBase string
}

func NewEvidence(client *http.Client) *Evidence {
	if client == nil {
		client = transport.NewClient()
	}
	return &Evidence{HTTP: client, SinaBase: "https://finance.sina.com.cn", SuspendBase: "https://datacenter-web.eastmoney.com"}
}
func (e *Evidence) CorporateAction(ctx context.Context, code, from, through string) (bool, error) {
	if !provider.ValidCode(code) {
		return false, fmt.Errorf("股票代码无效")
	}
	if _, err := time.Parse(time.DateOnly, from); err != nil {
		return false, err
	}
	if _, err := time.Parse(time.DateOnly, through); err != nil {
		return false, err
	}
	if from > through {
		return false, fmt.Errorf("除权窗口倒序")
	}
	symbol := "sz" + code
	if strings.HasPrefix(code, "6") {
		symbol = "sh" + code
	}
	body, err := transport.Get(ctx, e.HTTP, e.SinaBase+"/realstock/company/"+symbol+"/qfq.js", "", 4<<20)
	if err != nil {
		return false, err
	}
	return ParseCorporateAction(string(body), symbol, from, through)
}
func ParseCorporateAction(body, symbol, from, through string) (bool, error) {
	lhs, payload, ok := strings.Cut(body, "=")
	if !ok || strings.TrimSpace(lhs) != "var "+symbol+"qfq" {
		return false, fmt.Errorf("除权日期股票身份不符")
	}
	var value struct {
		Data *[]struct {
			Day string `json:"d"`
		} `json:"data"`
	}
	if err := json.NewDecoder(strings.NewReader(payload)).Decode(&value); err != nil {
		return false, err
	}
	if value.Data == nil {
		return false, fmt.Errorf("除权日期列表缺失")
	}
	found := false
	for _, row := range *value.Data {
		if _, err := time.Parse(time.DateOnly, row.Day); err != nil {
			return false, err
		}
		if row.Day >= from && row.Day <= through {
			found = true
		}
	}
	return found, nil
}

type Suspension struct {
	Code     string  `json:"SECURITY_CODE"`
	Identity string  `json:"SECUCODE"`
	Start    *string `json:"SUSPEND_START_TIME"`
	End      *string `json:"SUSPEND_END_TIME"`
	Resume   *string `json:"PREDICT_RESUME_DATE"`
	Expire   string  `json:"SUSPEND_EXPIRE"`
}

func FullDaySuspensions(rows []Suspension, day string) (map[string]bool, error) {
	date, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return nil, err
	}
	open, close := date.Add(9*time.Hour+30*time.Minute), date.Add(15*time.Hour)
	codes := map[string]bool{}
	parse := func(raw *string) (*time.Time, error) {
		if raw == nil {
			return nil, nil
		}
		at, err := time.Parse("2006-01-02 15:04:05", *raw)
		return &at, err
	}
	for _, row := range rows {
		code, exchange, ok := strings.Cut(row.Identity, ".")
		if !ok || code != row.Code || len(code) != 6 || (exchange != "SH" && exchange != "SZ" && exchange != "BJ") {
			return nil, fmt.Errorf("停牌证券身份不符")
		}
		for _, digit := range code {
			if digit < '0' || digit > '9' {
				return nil, fmt.Errorf("停牌证券代码异常")
			}
		}
		// The all-market report includes B shares. Ignore only positively
		// identified B-share code/exchange pairs; malformed A-share identities
		// must still invalidate the evidence rather than imply suspension.
		if exchange == "SZ" && strings.HasPrefix(code, "200") || exchange == "SH" && strings.HasPrefix(code, "900") {
			continue
		}
		if _, err := catalog.NewSecurity(code, code, strings.ToLower(exchange), "eastmoney"); err != nil {
			return nil, fmt.Errorf("停牌证券身份不符: %w", err)
		}
		start, err := parse(row.Start)
		if err != nil || start == nil {
			return nil, fmt.Errorf("停牌开始时间缺失或异常")
		}
		end, err := parse(row.End)
		if err != nil {
			return nil, err
		}
		resume, err := parse(row.Resume)
		if err != nil {
			return nil, err
		}
		if !start.After(open) && (end == nil || !end.Before(close)) && (resume == nil || resume.After(close)) && (end != nil || row.Expire == "连续停牌") {
			codes[code] = true
		}
	}
	return codes, nil
}
func (e *Evidence) Suspended(ctx context.Context, day string) (map[string]bool, error) {
	if _, err := time.Parse(time.DateOnly, day); err != nil {
		return nil, err
	}
	codes := map[string]bool{}
	expected := 0
	for page := 1; page <= 20; page++ {
		query := url.Values{"reportName": {"RPT_CUSTOM_SUSPEND_DATA_INTERFACE"}, "columns": {"ALL"}, "pageSize": {"500"}, "pageNumber": {strconv.Itoa(page)}, "filter": {"(MARKET=\"全部\")(DATETIME='" + day + "')"}}
		body, err := transport.Get(ctx, e.HTTP, e.SuspendBase+"/api/data/v1/get?"+query.Encode(), "", 4<<20)
		if err != nil {
			return nil, err
		}
		var value struct {
			Success bool `json:"success"`
			Result  struct {
				Pages int           `json:"pages"`
				Data  *[]Suspension `json:"data"`
			} `json:"result"`
		}
		if err := json.Unmarshal(body, &value); err != nil {
			return nil, err
		}
		pages := value.Result.Pages
		if !value.Success || pages < 1 || pages > 20 || value.Result.Data == nil || (expected != 0 && expected != pages) {
			return nil, fmt.Errorf("停复牌接口或分页异常")
		}
		expected = pages
		found, err := FullDaySuspensions(*value.Result.Data, day)
		if err != nil {
			return nil, err
		}
		for code := range found {
			codes[code] = true
		}
		if page == pages {
			return codes, nil
		}
	}
	return nil, fmt.Errorf("停复牌分页不完整")
}
