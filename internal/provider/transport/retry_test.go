package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type brokenBody struct {
	closed  bool
	failure error
}

func (b *brokenBody) Read(p []byte) (int, error) { return copy(p, "partial"), b.failure }
func (b *brokenBody) Close() error               { b.closed = true; return nil }

func TestGetRecoversInterruptedBody(t *testing.T) {
	for _, failure := range []error{io.ErrUnexpectedEOF, errors.New(`http2: server sent GOAWAY and closed the connection; LastStreamID=1999, ErrCode=NO_ERROR, debug=""`)} {
		t.Run(failure.Error(), func(t *testing.T) {
			broken := &brokenBody{failure: failure}
			attempts := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				attempts++
				if r.Method != http.MethodGet || r.Header.Get("Referer") != "https://example.test/" {
					t.Fatal("request changed")
				}
				var body io.ReadCloser = broken
				if attempts > 1 {
					if !broken.closed {
						t.Fatal("failed body not closed before retry")
					}
					body = io.NopCloser(strings.NewReader("complete"))
				}
				return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
			})}
			data, err := Get(context.Background(), client, "https://example.test/", "https://example.test/", 100)
			if err != nil || string(data) != "complete" || attempts != 2 {
				t.Fatalf("data=%q attempts=%d err=%v", data, attempts, err)
			}
		})
	}
}

func TestGetRetryBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name     string
		failure  error
		status   int
		body     string
		attempts int
	}{
		{name: "reset", failure: syscall.ECONNRESET, attempts: 2},
		{name: "eof", failure: io.EOF, attempts: 2},
		{name: "cancel", failure: context.Canceled, attempts: 1},
		{name: "deadline", failure: context.DeadlineExceeded, attempts: 1},
		{name: "unknown", failure: errors.New("TLS certificate failure"), attempts: 1},
		{name: "goaway protocol error", failure: errors.New(`http2: server sent GOAWAY and closed the connection; LastStreamID=1, ErrCode=PROTOCOL_ERROR, debug=""`), attempts: 1},
		{name: "status", status: 503, attempts: 1},
		{name: "size", status: 200, body: strings.Repeat("x", 101), attempts: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attempts := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				attempts++
				if tc.failure != nil {
					return nil, tc.failure
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}
			data, err := Get(context.Background(), client, "https://example.test/", "", 100)
			if err == nil || data != nil || attempts != tc.attempts {
				t.Fatalf("attempts=%d err=%v data=%q", attempts, err, data)
			}
		})
	}
}

func TestGetRetriesShareDeadline(t *testing.T) {
	for _, callerBudget := range []bool{false, true} {
		t.Run(map[bool]string{false: "client", true: "caller"}[callerBudget], func(t *testing.T) {
			var deadlines []time.Time
			client := &http.Client{Timeout: time.Second, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				deadline, ok := r.Context().Deadline()
				if !ok {
					t.Fatal("missing deadline")
				}
				deadlines = append(deadlines, deadline)
				if len(deadlines) == 1 {
					return nil, io.EOF
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
			})}
			ctx := context.Background()
			if callerBudget {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 500*time.Millisecond)
				defer cancel()
			}
			data, err := Get(ctx, client, "https://example.test/", "", 100)
			if err != nil || string(data) != "ok" || len(deadlines) != 2 {
				t.Fatalf("%q %v deadlines=%v", data, err, deadlines)
			}
			if !deadlines[0].Equal(deadlines[1]) {
				t.Fatalf("retry reset deadline: %v", deadlines)
			}
		})
	}
}

func TestGetRecoversRealTruncatedResponse(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Content-Length", "20")
			w.Write([]byte("partial"))
			return
		}
		w.Write([]byte("complete"))
	}))
	defer server.Close()
	body, err := Get(context.Background(), server.Client(), server.URL, "", 100)
	if err != nil || string(body) != "complete" || attempts.Load() != 2 {
		t.Fatalf("data=%q attempts=%d err=%v", body, attempts.Load(), err)
	}
}

func TestGetStopsAtSharedDeadline(t *testing.T) {
	attempts := 0
	client := &http.Client{Timeout: 30 * time.Millisecond, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return nil, io.EOF
		}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	body, err := Get(context.Background(), client, "https://example.test/", "", 100)
	if body != nil || !errors.Is(err, context.DeadlineExceeded) || attempts != 2 {
		t.Fatalf("data=%q attempts=%d err=%v", body, attempts, err)
	}
}

func TestGetDoesNotRetryAfterCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		cancel()
		return nil, io.EOF
	})}
	body, err := Get(ctx, client, "https://example.test/", "", 100)
	if body != nil || err == nil || attempts != 1 {
		t.Fatalf("data=%q attempts=%d err=%v", body, attempts, err)
	}
}
