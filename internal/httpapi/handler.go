// Package httpapi exposes the stable same-origin JSON API through net/http.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"market-mood/internal/app"
	"market-mood/internal/catalog"
	"market-mood/internal/provider"
	"market-mood/internal/tasks"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

type apiError struct {
	status  int
	message string
}

func (e apiError) Error() string { return e.message }
func bad(message string) error   { return apiError{http.StatusBadRequest, message} }

type endpoint func(*http.Request) (any, error)

func writeJSON(w http.ResponseWriter, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		status = http.StatusInternalServerError
		data = []byte(`{"error":"响应数据无效"}`)
		slog.Error("序列化HTTP响应", "error", err)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(append(data, '\n'))
}
func adapt(fn endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value, err := fn(r)
		if err == nil {
			writeJSON(w, http.StatusOK, value)
			return
		}
		status := http.StatusInternalServerError
		var e apiError
		switch {
		case errors.As(err, &e):
			status = e.status
		case errors.Is(err, app.ErrInvalidCode):
			status = http.StatusBadRequest
		case errors.Is(err, app.ErrUnavailable):
			status = http.StatusServiceUnavailable
		}
		if status >= 500 {
			slog.Error("HTTP请求失败", "method", r.Method, "path", r.URL.Path, "error", err)
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
	}
}

// New accepts built assets as an fs.FS; the executable supplies its embedded FS.
func New(a *app.App, version string, assets fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/database", adapt(func(r *http.Request) (any, error) { return a.Store.DatabaseMetrics(), nil }))
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		rows := a.Tasks.Snapshot()
		state := "ok"
		status := http.StatusOK
		if !tasks.Healthy(rows) {
			state = "degraded"
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, map[string]any{"status": state, "name": "Market Mood", "version": version, "tasks": rows})
	})
	mux.HandleFunc("GET /api/tasks", adapt(func(r *http.Request) (any, error) {
		rows := a.Tasks.Snapshot()
		status := "ok"
		if tasks.Degraded(rows) {
			status = "degraded"
		}
		return map[string]any{"status": status, "tasks": rows}, nil
	}))
	mux.HandleFunc("GET /api/dashboard", adapt(func(r *http.Request) (any, error) { return a.Dashboard(r.Context()) }))
	mux.HandleFunc("GET /api/watchlist", adapt(func(r *http.Request) (any, error) { return a.Watchlist(r.Context()) }))
	mux.HandleFunc("PUT /api/watchlist/{code}", adapt(func(r *http.Request) (any, error) {
		return map[string]bool{"ok": true}, a.AddWatch(r.Context(), r.PathValue("code"))
	}))
	mux.HandleFunc("DELETE /api/watchlist/{code}", adapt(func(r *http.Request) (any, error) {
		return map[string]bool{"ok": true}, a.RemoveWatch(r.Context(), r.PathValue("code"))
	}))
	mux.HandleFunc("GET /api/stocks", adapt(func(r *http.Request) (any, error) {
		values, err := parseQuery(r)
		if err != nil {
			return nil, err
		}
		return a.Store.Securities(r.Context(), values)
	}))
	mux.HandleFunc("GET /api/stocks/{code}", adapt(func(r *http.Request) (any, error) { return a.Stock(r.Context(), r.PathValue("code")) }))
	mux.HandleFunc("GET /api/history", adapt(func(r *http.Request) (any, error) { return a.Store.Days(r.Context()) }))
	mux.HandleFunc("GET /api/backfill", adapt(func(r *http.Request) (any, error) { return a.Store.BackfillStatus(r.Context()) }))
	mux.HandleFunc("GET /api/history/{day}", adapt(func(r *http.Request) (any, error) {
		day := r.PathValue("day")
		if _, err := time.Parse(time.DateOnly, day); err != nil {
			return nil, bad("日期格式无效")
		}
		snapshots, err := a.Store.History(r.Context(), day)
		if err != nil {
			return nil, err
		}
		close, err := a.Store.CloseView(r.Context(), day)
		if err != nil {
			return nil, err
		}
		historical, err := a.Store.HistoricalDay(r.Context(), day)
		if err != nil {
			return nil, err
		}
		return map[string]any{"snapshots": snapshots, "close": close, "historical": historical}, nil
	}))
	mux.HandleFunc("GET /api/sources", adapt(func(r *http.Request) (any, error) { return sources(a.Provider.Config()), nil }))
	mux.HandleFunc("PUT /api/sources", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		adapt(func(r *http.Request) (any, error) {
			media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || media != "application/json" {
				return nil, apiError{http.StatusUnsupportedMediaType, "需要application/json"}
			}
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				var limit *http.MaxBytesError
				if errors.As(err, &limit) {
					return nil, apiError{http.StatusRequestEntityTooLarge, "配置内容过大"}
				}
				return nil, bad("无法读取配置")
			}
			config, err := provider.DecodeConfig(raw)
			if err != nil {
				return nil, bad(err.Error())
			}
			return map[string]bool{"ok": true}, a.SetConfig(r.Context(), config)
		})(w, r)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			allowed := []string{}
			for _, method := range []string{"GET", "HEAD", "PUT", "DELETE"} {
				probe := *r
				probe.Method = method
				_, pattern := mux.Handler(&probe)
				if pattern != "/" && pattern != "" {
					allowed = append(allowed, method)
				}
			}
			if len(allowed) > 0 {
				w.Header().Set("Allow", strings.Join(allowed, ", "))
				writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "请求方法不支持"})
				return
			}
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "接口不存在"})
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			w.Header().Set("Allow", "GET, HEAD")
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "请求方法不支持"})
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		if assets == nil || !fs.ValidPath(name) {
			http.NotFound(w, r)
			return
		}
		data, err := fs.ReadFile(assets, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		contentType := mime.TypeByExtension(path.Ext(name))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public,max-age=31536000,immutable")
		if name == "index.html" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		if r.Method != "HEAD" {
			_, _ = w.Write(data)
		}
	})
	return mux
}
func parseQuery(r *http.Request) (catalog.Query, error) {
	q := catalog.Query{Page: 1, PageSize: 20}
	values, err := parseValues(r)
	if err != nil {
		return q, err
	}
	q.Q, q.Exchange = values["q"], values["exchange"]
	for key, target := range map[string]*int{"page": &q.Page, "page_size": &q.PageSize} {
		if value, ok := values[key]; ok {
			n, err := strconv.Atoi(value)
			if err != nil {
				return q, bad(key + "必须为整数")
			}
			*target = n
		}
	}
	if err := q.Validate(); err != nil {
		return q, bad(err.Error())
	}
	return q, nil
}
func parseValues(r *http.Request) (map[string]string, error) {
	// Reject duplicate parameters instead of silently choosing one value.
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, bad("查询参数无效")
	}
	out := map[string]string{}
	for key, list := range values {
		if len(list) != 1 {
			return nil, bad(fmt.Sprintf("查询参数%s重复", key))
		}
		out[key] = list[0]
	}
	return out, nil
}
func sources(config provider.Config) any {
	return map[string]any{
		"quotes": config.Quotes, "stock_quotes": config.StockQuotes, "limits": "eastmoney", "historical_indices": "tencent",
		"historical_stocks": []string{"tdx", "sina", "tonghuashun"}, "historical_limits": []string{"eastmoney", "tonghuashun"}, "turnover": []string{"tencent", "eastmoney"}, "universe": "sina",
		"tonghuashun": map[string]any{"enabled": true, "capabilities": []string{"stock_quotes", "historical_stocks", "historical_limits"}, "reason": "支持个股报价、不复权日线和历史封炸池；实时封炸池、全市场和iFinD尚未接入"},
	}
}
