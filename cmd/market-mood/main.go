package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"market-mood/frontend"
	"market-mood/internal/server"
	"os"
	"os/signal"
	"syscall"
)

var version = "0.1.0"

func main() {
	options, err := server.Parse(os.Args[1:], os.Getenv, os.Stdout)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err == nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		err = server.Run(ctx, options, version, frontend.Assets())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "市场情绪查看器无法运行：", err)
		os.Exit(1)
	}
}
