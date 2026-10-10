// Package transport supplies bounded HTTP reads shared by public data adapters.
package transport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"syscall"
	"time"
)

func NewClient() *http.Client { return &http.Client{Timeout: 15 * time.Second} }

// Get bounds decoded response bytes, including automatically decompressed bodies.
// Interrupted GETs get one retry; both attempts share the original time budget.
func Get(ctx context.Context, client *http.Client, url, referer string, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("响应长度上限无效")
	}
	// http.Client.Timeout normally starts anew on each Do. A shared deadline
	// includes body reads and prevents retries from doubling that budget.
	if client.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, client.Timeout)
		defer cancel()
	}
	for attempt := 0; ; attempt++ {
		data, err := getOnce(ctx, client, url, referer, limit)
		if err == nil || attempt == 1 || ctx.Err() != nil || !interrupted(err) {
			return data, err
		}
	}
}

func getOnce(ctx context.Context, client *http.Client, url, referer string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 MarketMood/1.0")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("行情 HTTP 状态 %d", response.StatusCode)
	}
	if response.ContentLength > limit {
		return nil, fmt.Errorf("行情响应超过 %d 字节", limit)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("行情响应超过 %d 字节", limit)
	}
	return data, nil
}

// Retry connection interruptions only. HTTP status, size, decoding, TLS, and
// timeout failures retain the existing provider fallback/backoff behavior.
func interrupted(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	// net/http does not expose the HTTP/2 GOAWAY error type. Match only its
	// orderly shutdown form, not protocol errors or arbitrary error messages.
	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		message := cause.Error()
		if strings.HasPrefix(message, "http2: server sent GOAWAY and closed the connection; LastStreamID=") &&
			strings.Contains(message, ", ErrCode=NO_ERROR, debug=") {
			return true
		}
	}
	return false
}
