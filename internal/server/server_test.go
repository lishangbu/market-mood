package server

import (
	"context"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestOptionsPrecedence(t *testing.T) {
	env := map[string]string{"MARKET_MOOD_DB": "saved.db", "MARKET_MOOD_DATA_DIR": "old", "MARKET_MOOD_BIND": "127.0.0.1:1234"}
	get := func(key string) string { return env[key] }
	o, err := Parse([]string{"--data-dir", "new", "--bind", "[::1]:0"}, get, io.Discard)
	if err != nil || o.Database != "saved.db" || o.Bind != "[::1]:0" {
		t.Fatal(o, err)
	}
	o, err = Parse([]string{"--db", "a.db", "--db", "b.db"}, get, io.Discard)
	if err != nil || o.Database != "b.db" {
		t.Fatal(o, err)
	}
	for _, args := range [][]string{{"--bind", "localhost:8080"}, {"--db"}, {"--unknown"}, {"extra"}} {
		if _, err := Parse(args, get, io.Discard); err == nil {
			t.Fatal(args)
		}
	}
	if _, err := Parse([]string{"--help"}, get, io.Discard); !errors.Is(err, flag.ErrHelp) {
		t.Fatal(err)
	}
}
func TestShutdownJoinsWorkersAndHTTP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	release := make(chan struct{})
	workerStopped := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		joined, err := serve(ctx, listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-release; w.Write([]byte("ok")) }), func(ctx context.Context) error { <-ctx.Done(); close(workerStopped); return nil }, time.Second)
		if !joined && err == nil {
			err = errors.New("not joined")
		}
		done <- err
	}()
	response := make(chan error, 1)
	go func() {
		res, err := http.Get("http://" + listener.Addr().String())
		if err == nil {
			_, err = io.Copy(io.Discard, res.Body)
			res.Body.Close()
		}
		response <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("HTTP not serving")
	}
	cancel()
	select {
	case <-workerStopped:
	case <-time.After(time.Second):
		t.Fatal("workers not cancelled")
	}
	select {
	case err := <-done:
		t.Fatal("did not drain request", err)
	default:
	}
	close(release)
	if err := <-response; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
func TestShutdownDeadlineDoesNotWaitForever(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	release := make(chan struct{})
	defer close(release)
	start := time.Now()
	joined, err := serve(ctx, listener, http.NotFoundHandler(), func(context.Context) error { <-release; return nil }, 20*time.Millisecond)
	if joined || err == nil || time.Since(start) > time.Second {
		t.Fatal(joined, err)
	}
}
