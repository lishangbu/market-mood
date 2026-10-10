package provider

import (
	"encoding/json"
	"fmt"
	"market-mood/internal/market"
	"market-mood/internal/trading"
	"math"
	"strconv"
	"strings"
	"time"
)

type object map[string]json.RawMessage

func text(v json.RawMessage) string { var s string; _ = json.Unmarshal(v, &s); return s }
func number(v json.RawMessage) *float64 {
	raw := string(v)
	if len(raw) > 0 && raw[0] == '"' {
		raw = text(v)
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	return &f
}
func uintValue(v json.RawMessage) *uint32 {
	n := number(v)
	if n == nil || *n < 0 || *n > math.MaxUint32 || math.Trunc(*n) != *n {
		return nil
	}
	return market.Ptr(uint32(*n))
}
func scaled(v *float64, factor float64) *float64 {
	if v == nil {
		return nil
	}
	n := *v * factor
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return nil
	}
	return &n
}
func positive(v *float64) *float64 {
	if v == nil || *v <= 0 {
		return nil
	}
	return v
}
func nonnegative(v *float64) *float64 {
	if v == nil || *v < 0 {
		return nil
	}
	return v
}
func timestamp(raw json.RawMessage) *string {
	n := number(raw)
	if n == nil || math.Trunc(*n) != *n || *n < 0 || *n > 253402271999 {
		return nil
	}
	return market.Ptr(time.Unix(int64(*n), 0).In(trading.Shanghai).Format(time.RFC3339))
}
func ValidCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return false
		}
	}
	for _, prefix := range []string{"60", "00", "30", "688", "4", "8", "92"} {
		if strings.HasPrefix(code, prefix) {
			return true
		}
	}
	return false
}
func Eligible(code, name string) bool {
	if !ValidCode(code) || strings.TrimSpace(name) == "" {
		return false
	}
	upper := strings.ToUpper(name)
	return (strings.HasPrefix(code, "60") || strings.HasPrefix(code, "00") || strings.HasPrefix(code, "30") || strings.HasPrefix(code, "688")) && !strings.Contains(upper, "ST") && !strings.Contains(name, "退") && !strings.HasPrefix(upper, "N") && !strings.HasPrefix(upper, "C")
}
func symbol(code string) string {
	if strings.HasPrefix(code, "6") {
		return "sh" + code
	}
	if strings.HasPrefix(code, "4") || strings.HasPrefix(code, "8") || strings.HasPrefix(code, "92") {
		return "bj" + code
	}
	return "sz" + code
}
func array(raw json.RawMessage) ([]object, error) {
	var rows []object
	if len(raw) > 0 && raw[0] == '[' {
		if err := json.Unmarshal(raw, &rows); err != nil {
			return nil, err
		}
		return rows, nil
	}
	var items object
	if err := json.Unmarshal(raw, &items); err != nil || items == nil {
		return nil, fmt.Errorf("数据源列表格式异常")
	}
	if _, ok := items["c"]; ok {
		return []object{items}, nil
	}
	if _, ok := items["f12"]; ok {
		return []object{items}, nil
	}
	rows = []object{}
	for _, v := range items {
		var row object
		if err := json.Unmarshal(v, &row); err != nil || row == nil {
			return nil, fmt.Errorf("列表成员格式异常")
		}
		rows = append(rows, row)
	}
	return rows, nil
}
func parseTencent(line string) (market.Stock, error) {
	var s market.Stock
	_, rest, ok := strings.Cut(line, "\"")
	if !ok {
		return s, fmt.Errorf("腾讯行情格式异常")
	}
	payload, _, ok := strings.Cut(rest, "\"")
	if !ok {
		return s, fmt.Errorf("腾讯行情结束符缺失")
	}
	f := strings.Split(payload, "~")
	if len(f) <= 57 {
		return s, fmt.Errorf("腾讯行情字段不足")
	}
	if !ValidCode(f[2]) {
		return s, fmt.Errorf("腾讯代码异常")
	}
	n := func(i int) *float64 { return number(json.RawMessage(f[i])) }
	var observed *string
	if dt, err := time.ParseInLocation("20060102150405", f[30], trading.Shanghai); err == nil {
		observed = market.Ptr(dt.Format(time.RFC3339))
	}
	up, down := positive(n(47)), positive(n(48))
	return market.Stock{Code: f[2], Name: f[1], Price: n(3), PreviousClose: n(4), Open: n(5), Change: n(32), Amount: scaled(n(57), 1e4), FloatCap: scaled(n(44), 1e8), MarketCap: scaled(n(45), 1e8), TurnoverRate: n(38), SourceTime: observed, Eligible: Eligible(f[2], f[1]) && up != nil && down != nil, LimitUp: up, LimitDown: down}, nil
}
func parseTHS(body, code string) (market.Stock, error) {
	var s market.Stock
	prefix := "quotebridge_v6_realhead_hs_" + code + "_last("
	body = strings.TrimSpace(body)
	if !strings.HasPrefix(body, prefix) {
		return s, fmt.Errorf("同花顺回调不匹配")
	}
	raw := strings.TrimSuffix(strings.TrimRight(strings.TrimPrefix(body, prefix), ";"), ")")
	var value struct {
		Items object `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return s, err
	}
	v := value.Items
	if text(v["5"]) != code {
		return s, fmt.Errorf("同花顺代码不匹配")
	}
	date, err := time.ParseInLocation("2006-01-02 15:04", text(v["updateTime"]), trading.Shanghai)
	if err != nil {
		return s, fmt.Errorf("报价更新时间: %w", err)
	}
	name := text(v["name"])
	return market.Stock{Code: code, Name: name, Eligible: Eligible(code, name), Price: number(v["10"]), Open: number(v["7"]), PreviousClose: number(v["6"]), Amount: number(v["19"]), Change: number(v["199112"]), LimitUp: number(v["69"]), LimitDown: number(v["70"]), SourceTime: market.Ptr(date.Format(time.RFC3339)), Provider: market.Ptr(Tonghuashun.Name())}, nil
}
func eastQuote(v object) market.Stock {
	code, name := text(v["f12"]), text(v["f14"])
	return market.Stock{Code: code, Name: name, Eligible: Eligible(code, name), Industry: text(v["f100"]), Price: number(v["f2"]), Change: number(v["f3"]), Amount: number(v["f6"]), FloatCap: number(v["f21"]), MarketCap: number(v["f20"]), TurnoverRate: number(v["f8"]), Open: number(v["f17"]), PreviousClose: number(v["f18"]), SourceTime: timestamp(v["f124"])}
}
func sealTime(raw json.RawMessage) *string {
	n := uintValue(raw)
	if n == nil || *n == 0 {
		return nil
	}
	h, m, s := *n/10000, *n/100%100, *n%100
	if h > 23 || m > 59 || s > 59 {
		return nil
	}
	return market.Ptr(fmt.Sprintf("%02d:%02d:%02d", h, m, s))
}
func limitQuote(v object, sealed bool) market.Stock {
	code, name := text(v["c"]), text(v["n"])
	s := market.Stock{Code: code, Name: name, Eligible: Eligible(code, name), Industry: text(v["hybk"]), Price: scaled(number(v["p"]), .001), Change: number(v["zdp"]), Amount: number(v["amount"]), Boards: uintValue(v["lbc"]), BreakCount: uintValue(v["zbc"]), FloatCap: nonnegative(number(v["ltsz"])), MarketCap: nonnegative(number(v["tshare"])), TurnoverRate: nonnegative(number(v["hs"])), FirstSealTime: sealTime(v["fbt"]), Sealed: market.Ptr(sealed)}
	if sealed {
		s.SealAmount = nonnegative(number(v["fund"]))
		s.LastSealTime = sealTime(v["lbt"])
	}
	var stats object
	if json.Unmarshal(v["zttj"], &stats) == nil {
		days, count := uintValue(stats["days"]), uintValue(stats["ct"])
		if days != nil && count != nil && *days > 0 && *count <= *days {
			s.LimitStats = &market.LimitStats{Days: *days, Count: *count}
		}
	}
	return s
}

// FrameTime uses the oldest valid quote and rejects cross-day or broad batches.
func FrameTime(stocks []market.Stock, captured time.Time) (string, string, error) {
	var oldest, newest time.Time
	for _, s := range stocks {
		if !s.Valid() {
			continue
		}
		if s.SourceTime == nil {
			return "", "", fmt.Errorf("有效行情 %s 缺少时间戳", s.Code)
		}
		t, err := time.Parse(time.RFC3339, *s.SourceTime)
		if err != nil {
			return "", "", err
		}
		t = t.In(trading.Shanghai)
		if oldest.IsZero() || t.Before(oldest) {
			oldest = t
		}
		if newest.IsZero() || t.After(newest) {
			newest = t
		}
	}
	if oldest.IsZero() {
		return "", "", fmt.Errorf("没有有效行情")
	}
	if newest.After(captured) {
		return "", "", fmt.Errorf("行情时间超前")
	}
	day := oldest.Format(time.DateOnly)
	if newest.Format(time.DateOnly) != day {
		return "", "", fmt.Errorf("行情跨交易日")
	}
	if newest.Sub(oldest) > 180*time.Second {
		return "", "", fmt.Errorf("分批行情时间跨度超过180秒")
	}
	return day, oldest.Format(time.RFC3339), nil
}
