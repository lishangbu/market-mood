package httpapi

import (
	"context"
	"encoding/json"
	"market-mood/internal/app"
	"market-mood/internal/market"
	"market-mood/internal/provider"
	"market-mood/internal/store"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

func fixture(t *testing.T) (*app.App, http.Handler) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	client, err := provider.New(provider.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	at := "2026-09-07T10:00:00+08:00"
	if err := db.SaveMarket(ctx, market.MarketFrame{Day: "2026-09-07", CapturedAt: at, SourceTime: &at, Complete: true, Stocks: []market.Stock{{Code: "600000", Name: "浦发银行"}}}); err != nil {
		t.Fatal(err)
	}
	a, err := app.New(ctx, db, client)
	if err != nil {
		t.Fatal(err)
	}
	assets := fstest.MapFS{"index.html": {Data: []byte("<html>app</html>")}, "assets/main-abc.js": {Data: []byte("export {}")}}
	return a, New(a, "test", assets)
}
func request(h http.Handler, method, path, body, media string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if media != "" {
		r.Header.Set("Content-Type", media)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestRoutesAndErrors(t *testing.T) {
	_, h := fixture(t)
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/api/health", 503}, {"GET", "/api/tasks", 200}, {"GET", "/api/database", 200}, {"GET", "/api/dashboard", 200},
		{"GET", "/api/watchlist", 200}, {"GET", "/api/stocks", 200}, {"GET", "/api/stocks?page=0", 400},
		{"GET", "/api/stocks?page=1&page=2", 400}, {"GET", "/api/stocks?page_size=no", 400},
		{"GET", "/api/stocks?exchange=us", 400}, {"GET", "/api/stocks?q=%zz", 400},
		{"GET", "/api/stocks/invalid", 400}, {"GET", "/api/stocks/600000", 200},
		{"GET", "/api/history", 200}, {"GET", "/api/history/2026-02-30", 400},
		{"GET", "/api/history/2026-09-07", 200}, {"GET", "/api/backfill", 200}, {"GET", "/api/sources", 200},
		{"PUT", "/api/watchlist/600000", 200}, {"DELETE", "/api/watchlist/600000", 200},
		{"PUT", "/api/watchlist/bad", 400}, {"DELETE", "/api/watchlist/bad", 400},
		{"GET", "/api/not-found", 404}, {"POST", "/api/dashboard", 405}, {"GET", "/api/watchlist/600000", 405},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			w := request(h, tc.method, tc.path, "", "")
			if w.Code != tc.status {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
			if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") || !json.Valid(w.Body.Bytes()) {
				t.Fatal("not JSON", w.Body.String())
			}
			if tc.status == 405 && w.Header().Get("Allow") == "" {
				t.Fatal("missing Allow")
			}
		})
	}
}
func TestSourcesPersistenceAndBodyLimits(t *testing.T) {
	a, h := fixture(t)
	valid := `{"quotes":["eastmoney"],"stock_quotes":["tencent"]}`
	for _, tc := range []struct {
		body, media string
		status      int
	}{
		{valid, "application/json", 200}, {valid, "text/plain", 415},
		{`{`, "application/json", 400}, {valid + `{}`, "application/json", 400},
		{`{"quotes":[],"stock_quotes":["tdx"]}`, "application/json", 400},
		{strings.Repeat(" ", 17000), "application/json", 413},
	} {
		w := request(h, "PUT", "/api/sources", tc.body, tc.media)
		if w.Code != tc.status {
			t.Fatalf("want %d got %d %s", tc.status, w.Code, w.Body.String())
		}
	}
	config, err := a.Store.Config(context.Background())
	if err != nil || config.Quotes[0] != provider.Eastmoney || a.Provider.Config().Quotes[0] != provider.Eastmoney {
		t.Fatal(config, err)
	}
}
func TestHistoryAndStaticAssets(t *testing.T) {
	_, h := fixture(t)
	w := request(h, "GET", "/api/history/2026-09-07", "", "")
	var history map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if string(history["snapshots"]) != "[]" || string(history["close"]) != "null" || string(history["historical"]) != "null" {
		t.Fatal(w.Body.String())
	}
	for _, tc := range []struct {
		path, cache string
		status      int
	}{
		{"/", "no-cache", 200}, {"/index.html", "no-cache", 200},
		{"/assets/main-abc.js", "public,max-age=31536000,immutable", 200}, {"/missing", "", 404},
	} {
		w := request(h, "GET", tc.path, "", "")
		if w.Code != tc.status || w.Header().Get("Cache-Control") != tc.cache {
			t.Fatal(tc, w.Code, w.Header())
		}
	}
	w = request(h, "HEAD", "/", "", "")
	if w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestHealthDistinguishesRetryErrorsFromStoppedWorkers(t *testing.T) {
	a, h := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{}, 10)
	var wg sync.WaitGroup
	for _, s := range a.Tasks.Snapshot() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = a.Tasks.Run(ctx, s.ID, time.Hour, func(context.Context) error { ready <- struct{}{}; return context.DeadlineExceeded })
		}()
	}
	for range 10 {
		select {
		case <-ready:
		case <-time.After(3 * time.Second):
			t.Fatal("workers not started")
		}
	}
	if w := request(h, "GET", "/api/health", "", ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	cancel()
	wg.Wait()
	if w := request(h, "GET", "/api/health", "", ""); w.Code != 503 {
		t.Fatal(w.Code, w.Body.String())
	}
}
