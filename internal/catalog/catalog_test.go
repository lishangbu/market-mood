package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIdentityAndOfficialFields(t *testing.T) {
	row, err := NewSecurity("688001", "ST测试", "sh", "test")
	if err != nil || row.Board != "科创板" {
		t.Fatal(row, err)
	}
	for _, input := range [][3]string{{"600000", "测试", "sz"}, {"900001", "B股", "sh"}, {"000001", "", "sz"}} {
		if _, err := NewSecurity(input[0], input[1], input[2], "test"); err == nil {
			t.Fatal("bad identity", input)
		}
	}
	sh, err := (shRow{Code: "600000", Name: "浦发银行", ListingDate: "19991110"}).security()
	if err != nil || sh.ListingDate == nil || *sh.ListingDate != "1999-11-10" || sh.Industry != nil {
		t.Fatal(sh, err)
	}
	sz, err := (szRow{Code: "000001", Name: "<a><u>平安银行</u></a>", ListingDate: "1991-04-03", Industry: "金融业"}).security()
	if err != nil || sz.Name != "平安银行" || *sz.Industry != "金融业" {
		t.Fatal(sz, err)
	}
	if normalizedDate("-") != nil {
		t.Fatal("guessed date")
	}
}
func TestCatalogFallbackAndIncompleteOfficialChecks(t *testing.T) {
	type item struct {
		Symbol string `json:"symbol"`
		Code   string `json:"code"`
		Name   string `json:"name"`
	}
	items := []item{}
	for _, exchange := range []string{"sh", "sz", "bj"} {
		n, prefix := 1000, "600"
		if exchange == "sz" {
			prefix = "000"
		}
		if exchange == "bj" {
			prefix = "920"
			n = 100
		}
		for i := 0; i < n; i++ {
			code := fmt.Sprintf("%s%03d", prefix, i)
			items = append(items, item{exchange + code, code, "测试"})
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "StockCount"):
			fmt.Fprintf(w, "\"%d\"", len(items))
		case strings.Contains(r.URL.Path, "getHQNodeData"):
			var page int
			fmt.Sscan(r.URL.Query().Get("page"), &page)
			start := (page - 1) * 100
			json.NewEncoder(w).Encode(items[start:min(start+100, len(items))])
		case strings.HasSuffix(r.URL.Path, "nqxxCnzq.do"):
			w.Write([]byte("reachable, not verified"))
		default:
			http.Error(w, "provider unavailable", 503)
		}
	}))
	defer server.Close()
	c := New(server.Client())
	c.EastmoneyBase, c.SinaBase, c.SSEBase, c.SZSEBase, c.BSEBase = server.URL, server.URL, server.URL, server.URL, server.URL
	result, err := c.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Securities) != 2100 || len(result.FallbackReasons) != 1 || !strings.Contains(result.FallbackReasons[0], "东方财富") || !strings.Contains(result.ExchangeChecks["bj"], "解析尚未验证") || !strings.Contains(result.ExchangeChecks["sh"], "失败") {
		t.Fatal("wrong fallback/check state", result.ExchangeChecks)
	}
}
func TestOfficialMergeKeepsUnconfirmedAndRejectsPartial(t *testing.T) {
	original, _ := NewSecurity("601999", "未确认", "sh", "新浪")
	rows := []Security{original}
	official := []Security{}
	for i := 0; i < 1000; i++ {
		s, _ := NewSecurity(fmt.Sprintf("600%03d", i), "官方", "sh", "上交所")
		official = append(official, s)
	}
	out, check := mergeOfficial(rows, official[:2], "sh")
	if len(out) != 1 || !strings.Contains(check, "未采用") {
		t.Fatal("partial official list adopted")
	}
	out, check = mergeOfficial(rows, official, "sh")
	if len(out) != 1001 || out[0].Code != original.Code || !strings.Contains(check, "另有1只") {
		t.Fatal("unconfirmed stock removed")
	}
	official[1] = official[0]
	out, _ = mergeOfficial(rows, official, "sh")
	if len(out) != 1 {
		t.Fatal("duplicate official list adopted")
	}
}
