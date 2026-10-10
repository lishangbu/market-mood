package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBoundedHTTPAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bad":
			http.Error(w, "unavailable", 503)
		case "/large":
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			w.Write([]byte(strings.Repeat("x", 100)))
		case "/wait":
			<-r.Context().Done()
		default:
			if r.Header.Get("Referer") != "https://example.test/" {
				t.Error("referer missing")
			}
			w.Write([]byte("ok"))
		}
	}))
	defer server.Close()
	client := server.Client()
	body, err := Get(context.Background(), client, server.URL, "https://example.test/", 10)
	if err != nil || string(body) != "ok" {
		t.Fatal(string(body), err)
	}
	for _, path := range []string{"/bad", "/large"} {
		if _, err := Get(context.Background(), client, server.URL+path, "", 10); err == nil {
			t.Fatal("bad response accepted", path)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := Get(ctx, client, server.URL+"/wait", "", 10); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
