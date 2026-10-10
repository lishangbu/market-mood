// Package server owns HTTP and collector lifetimes, without process-global signals.
package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"market-mood/internal/app"
	"market-mood/internal/httpapi"
	"market-mood/internal/provider"
	"market-mood/internal/store"
	"net"
	"net/http"
	"time"
)

// Run binds before touching the database, and uses a shared ten-second deadline
// to drain HTTP and collectors. The command exits if workers cannot be joined;
// it must not close their database while they may still be using it.
func Run(ctx context.Context, options Options, version string, assets fs.FS) error {
	listener, err := net.Listen("tcp", options.Bind)
	if err != nil {
		return fmt.Errorf("无法监听 %s: %w", options.Bind, err)
	}
	defer listener.Close()
	config, err := provider.LoadConfig()
	if err != nil {
		return err
	}
	client, err := provider.New(config)
	if err != nil {
		return err
	}
	defer client.HTTP.CloseIdleConnections()
	db, err := store.Open(ctx, options.Database)
	if err != nil {
		return err
	}
	application, err := app.New(ctx, db, client)
	if err != nil {
		db.Close()
		return err
	}
	handler := httpapi.New(application, version, assets)
	slog.Info("HTTP服务已就绪", "address", listener.Addr().String(), "database", db.Path())
	joined, err := serve(ctx, listener, handler, application.Run, 10*time.Second)
	if !joined {
		return err
	}
	// Database cleanup has its own final one-second budget.
	closed := make(chan error, 1)
	go func() { closed <- db.Close() }()
	select {
	case closeErr := <-closed:
		return errors.Join(err, closeErr)
	case <-time.After(time.Second):
		return errors.Join(err, fmt.Errorf("数据库关闭超时"))
	}
}

func serve(ctx context.Context, listener net.Listener, handler http.Handler, workers func(context.Context) error, timeout time.Duration) (bool, error) {
	workCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	httpCtx, cancelHTTP := context.WithCancel(context.Background())
	defer cancelHTTP()
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20, BaseContext: func(net.Listener) context.Context { return httpCtx }}
	httpDone := make(chan error, 1)
	go func() { httpDone <- server.Serve(listener) }()
	workerDone := make(chan error, 1)
	go func() { workerDone <- workers(workCtx) }()
	var result error
	httpFinished, workersFinished := false, false
	select {
	case <-ctx.Done():
	case err := <-httpDone:
		httpFinished = true
		if !errors.Is(err, http.ErrServerClosed) {
			result = err
		}
	case err := <-workerDone:
		workersFinished = true
		result = errors.Join(fmt.Errorf("全部后台任务已退出"), err)
	}
	cancel()
	drainCtx, stop := context.WithTimeout(context.Background(), timeout)
	defer stop()
	drained := make(chan error, 1)
	go func() { drained <- server.Shutdown(drainCtx) }()
	shutdownFinished := false
	for !shutdownFinished || !workersFinished || !httpFinished {
		select {
		case err := <-drained:
			shutdownFinished = true
			drained = nil
			if err != nil {
				result = errors.Join(result, err)
			}
		case err := <-workerDone:
			workersFinished = true
			workerDone = nil
			result = errors.Join(result, err)
		case err := <-httpDone:
			httpFinished = true
			httpDone = nil
			if !errors.Is(err, http.ErrServerClosed) {
				result = errors.Join(result, err)
			}
		case <-drainCtx.Done():
			cancelHTTP()
			_ = server.Close()
			return false, errors.Join(result, fmt.Errorf("退出等待超时: %w", drainCtx.Err()))
		}
	}
	return true, result
}
