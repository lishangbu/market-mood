package catalog

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSinaListingDateIdentity(t *testing.T) {
	row, _ := NewSecurity("000001", "平安银行", "sz", "新浪")
	page := `<a href="https://finance.sina.com.cn/realstock/company/sz000001/nc.shtml">平安银行(000001)</a><table id="comInfo1"><tr><td>上市日期：</td><td><a href="/corp/view/vCI_CorpInfoLink.php?stockid=000001&InMarketDate=199104">1991-04-03</a></td></tr></table>`
	date, err := parseSinaListing([]byte(page), row)
	if err != nil || date != "1991-04-03" {
		t.Fatal(date, err)
	}
	for _, bad := range []string{
		`<a href="/realstock/company/sz000002/nc.shtml"></a><table id="comInfo1"><a href="?stockid=000001&InMarketDate=199104">1991-04-03</a></table>`,
		`<a href="/realstock/company/sz000001/nc.shtml"></a><table id="comInfo1"><a href="?stockid=000002&InMarketDate=199104">1991-04-03</a></table>`,
		`<a href="/realstock/company/sz000001/nc.shtml"></a><table id="comInfo1"><a href="?stockid=000001&InMarketDate=199104">1991-02-30</a></table>`,
		`<a href="/realstock/company/sz000001/nc.shtml"></a><table id="comInfo1"></table><a href="?stockid=000001&InMarketDate=199104">1991-04-03</a>`,
	} {
		if _, err := parseSinaListing([]byte(bad), row); err == nil {
			t.Fatal("accepted wrong identity/date", bad)
		}
	}
}

func TestSupplementListingDatesPreservesVerifiedAndRejectsWrongStock(t *testing.T) {
	verified, _ := NewSecurity("600000", "浦发银行", "sh", "上交所")
	date := "1999-11-10"
	verified.ListingDate = &date
	good, _ := NewSecurity("000001", "平安银行", "sz", "新浪")
	wrong, _ := NewSecurity("000002", "万科A", "sz", "新浪")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "600000") {
			t.Error("re-fetched verified listing date")
		}
		fmt.Fprint(w, `<a href="/realstock/company/sz000001/nc.shtml"></a><table id="comInfo1"><a href="?stockid=000001&InMarketDate=199104">1991-04-03</a></table>`)
	}))
	defer server.Close()
	c := New(server.Client())
	c.SinaBase = server.URL
	rows := []Security{verified, good, wrong}
	checks := c.SupplementListingDates(context.Background(), rows)
	if *rows[0].ListingDate != date || rows[0].Provider != "上交所" {
		t.Fatal("verified data changed", rows[0])
	}
	if rows[1].ListingDate == nil || *rows[1].ListingDate != "1991-04-03" || !strings.Contains(rows[1].Provider, "公司资料") {
		t.Fatal("missing verified fallback", rows[1])
	}
	if rows[2].ListingDate != nil || !strings.Contains(checks["sz"], "仍待核验1只") {
		t.Fatal("wrong stock adopted", rows[2], checks)
	}
}
