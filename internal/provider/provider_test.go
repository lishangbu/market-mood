package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/text/encoding/simplifiedchinese"
	"market-mood/internal/market"
	"market-mood/internal/trading"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func rawObject(s string) object {
	var v object
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		panic(err)
	}
	return v
}
func TestSourceConfigAndSelection(t *testing.T) {
	for _, raw := range []string{`{}`, `{"quotes":["tencent"],"stock_quotes":["bad"]}`, `{"quotes":["tencent","tencent"],"stock_quotes":["tdx"]}`, `{"quotes":["tonghuashun"],"stock_quotes":["tdx"]}`, `{"quotes":["tencent"],"stock_quotes":["tdx"],"extra":1}`, `{"quotes":["tencent"],"stock_quotes":["tdx"]} {}`} {
		if _, err := DecodeConfig([]byte(raw)); err == nil {
			t.Fatalf("invalid config %s", raw)
		}
	}
	calls := []Source{}
	value, source, reasons, err := Select(context.Background(), []Source{TDX, Tencent, Eastmoney}, func(ctx context.Context, s Source) (int, error) {
		calls = append(calls, s)
		if s == TDX {
			return 0, errors.New("行情过期")
		}
		return 42, nil
	})
	if err != nil || value != 42 || source != Tencent || len(reasons) != 1 || !slices.Equal(calls, []Source{TDX, Tencent}) {
		t.Fatal(value, source, reasons, err, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, _, err = Select(ctx, []Source{TDX}, func(context.Context, Source) (int, error) { t.Fatal("called after cancel"); return 0, nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestLimitFields(t *testing.T) {
	v := rawObject(`{"c":"600000","n":"测试","amount":1200000000,"fund":35000000,"zbc":0,"p":9230,"ltsz":1230000000,"tshare":2340000000,"hs":2.5,"fbt":92500,"lbt":145901,"zttj":{"days":3,"ct":2},"lbc":2}`)
	a, b := limitQuote(v, true), limitQuote(v, false)
	if *a.Amount != 1200000000 || *a.SealAmount != 35000000 || b.SealAmount != nil || *a.Price != 9.23 || *a.BreakCount != 0 || *a.FirstSealTime != "09:25:00" || *a.LastSealTime != "14:59:01" || b.LastSealTime != nil || a.LimitStats.Count != 2 {
		t.Fatalf("bad limit fields %+v", a)
	}
	for _, n := range []string{"null", "-1", "1.5", "4294967296"} {
		if uintValue(json.RawMessage(n)) != nil {
			t.Fatalf("invalid count %s", n)
		}
	}
	if sealTime(json.RawMessage("246060")) != nil {
		t.Fatal("invalid seal time")
	}
}
func TestTHSSourceDate(t *testing.T) {
	body, err := os.ReadFile("../../testdata/ths-600000.js")
	if err != nil {
		t.Fatal(err)
	}
	s, err := parseTHS(string(body), "600000")
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "浦发银行" || *s.Price != 9.54 || *s.Amount != 915309830.00 || *s.SourceTime != "2026-10-09T15:30:00+08:00" {
		t.Fatalf("quote %+v", s)
	}
	if _, err := parseTHS(string(body), "600001"); err == nil {
		t.Fatal("wrong stock accepted")
	}
	if _, err := parseTHS(strings.ReplaceAll(string(body), "updateTime", "missing"), "600000"); err == nil {
		t.Fatal("server time used as quote time")
	}
}
func quoteLine(code, stamp string) string {
	fields := make([]string, 58)
	fields[1] = "浦发银行"
	fields[2] = code
	fields[3] = "9.23"
	fields[4] = "9.5"
	fields[5] = "9.43"
	fields[30] = stamp
	fields[32] = "-2.84"
	fields[38] = "1.2"
	fields[44] = "100"
	fields[45] = "200"
	fields[47] = "10.45"
	fields[48] = "8.55"
	fields[57] = "80632.6225"
	return "v_sh" + code + "=\"" + strings.Join(fields, "~") + "\";\n"
}
func TestStockFallbackRejectsStaleAndKeepsReason(t *testing.T) {
	at := time.Date(2026, 9, 7, 10, 2, 0, 0, trading.Shanghai)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/qt/stock/kline/get":
			fmt.Fprint(w, `{"rc":0,"data":{"klines":["2026-09-04,1","2026-09-07,1"]}}`)
		case "/q=sh600000":
			body, _ := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(quoteLine("600000", "20260904150000")))
			w.Write(body)
		case "/api/qt/stock/get":
			fmt.Fprintf(w, `{"rc":0,"data":{"f57":"600000","f58":"浦发银行","f59":2,"f43":923,"f46":943,"f60":950,"f170":-284,"f48":806326225,"f86":%d}}`, at.Unix())
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c, _ := New(Config{Quotes: []Source{Tencent}, StockQuotes: []Source{Tencent, Eastmoney}})
	c.HTTP = server.Client()
	c.HistoryBase, c.TencentBase, c.QuoteBase = server.URL, server.URL, server.URL
	c.Now = func() time.Time { return at }
	s, err := c.Stock(context.Background(), "600000")
	if err != nil {
		t.Fatal(err)
	}
	if *s.Provider != "东方财富" || len(s.FallbackReasons) != 1 || !strings.Contains(s.FallbackReasons[0], "过期") || *s.Price != 9.23 {
		t.Fatalf("fallback %+v", s)
	}
}
func TestTencentUnitsAndCompleteness(t *testing.T) {
	s, err := parseTencent(quoteLine("600000", "20260907150000"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "浦发银行" || *s.Amount != 806326225 || *s.FloatCap != 1e10 || *s.MarketCap != 2e10 {
		t.Fatalf("units %+v", s)
	}
	if err := validateCodes([]market.Stock{s, s}, []string{"600000", "600001"}); err == nil {
		t.Fatal("duplicate accepted")
	}
	if err := validateCodes([]market.Stock{s}, []string{"600001"}); err == nil {
		t.Fatal("wrong identity accepted")
	}
}
func TestFrameTime(t *testing.T) {
	captured := time.Date(2026, 9, 7, 10, 2, 0, 0, trading.Shanghai)
	quote := func(stamp string) market.Stock {
		return market.Stock{Eligible: true, Price: market.Ptr(10.), Open: market.Ptr(9.), Change: market.Ptr(1.), SourceTime: &stamp}
	}
	a, b := quote("2026-09-07T10:00:00+08:00"), quote("2026-09-07T10:01:00+08:00")
	_, observed, err := FrameTime([]market.Stock{a, b}, captured)
	if err != nil || observed != *a.SourceTime {
		t.Fatal(observed, err)
	}
	for _, stamp := range []string{"2026-09-07T09:00:00+08:00", "2026-09-04T10:00:00+08:00", "2026-09-07T10:03:00+08:00"} {
		if _, _, err := FrameTime([]market.Stock{a, quote(stamp)}, captured); err == nil {
			t.Fatal("bad frame accepted", stamp)
		}
	}
	a.SourceTime = nil
	if _, _, err := FrameTime([]market.Stock{a}, captured); err == nil {
		t.Fatal("missing time accepted")
	}
}
func TestPoolRejectsWrongDayAndChangingTotal(t *testing.T) {
	for _, mode := range []string{"day", "total", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				day, total := 20260907, 2
				if mode == "day" {
					day = 20260904
				}
				if mode == "total" && r.URL.Query().Get("Pageindex") == "1" {
					total = 3
				}
				fmt.Fprintf(w, `{"rc":0,"data":{"qdate":%d,"tc":%d,"pool":[{"c":"600000","n":"测试"}]}}`, day, total)
			}))
			defer server.Close()
			c, _ := New(DefaultConfig())
			c.LimitBase = server.URL
			c.HTTP = server.Client()
			if _, err := c.Limits(context.Background(), "2026-09-07"); err == nil {
				t.Fatal("invalid pool accepted")
			}
		})
	}
}
