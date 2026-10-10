package sina

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestDailyHTTP(t *testing.T) {
	fixture, err := os.ReadFile("../../../testdata/sina-sh600000-klc2.js")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/realstock/company/sh600000/hisdata_klc2/klc_kl.js" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Write(fixture)
	}))
	defer server.Close()
	c := New(server.Client())
	c.HistoryBase = server.URL
	rows, err := c.Daily(context.Background(), "600000")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 15 || rows[len(rows)-1].Day != "2026-10-09" {
		t.Fatal("daily window")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Daily(ctx, "600000"); err == nil {
		t.Fatal("canceled request succeeded")
	}
}
func TestPricesUnknownAmount(t *testing.T) {
	body := `=([{"day":"2026-09-07","open":"9.43","high":"9.46","low":"9.19","close":"9.23","volume":"86711332"}]);`
	rows, err := ParsePrices(body)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Amount != nil || rows[0].Prevclose != nil {
		t.Fatal("price-only history invented missing fields")
	}
	for _, raw := range []string{`=([]);`, `=([{"day":"2026-09-07"}]);`, body + "evil()"} {
		if _, err := ParsePrices(raw); err == nil {
			t.Fatal("malformed prices accepted")
		}
	}
}
