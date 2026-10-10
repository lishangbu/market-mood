// sourcecheck probes real adapters without opening or modifying an application DB.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"market-mood/internal/catalog"
	"market-mood/internal/provider"
	"market-mood/internal/provider/sina"
	"market-mood/internal/provider/tdx"
	"market-mood/internal/turnover"
	"os"
	"time"
)

func main() {
	code := flag.String("code", "600000", "沪深证券代码")
	timeout := flag.Duration("timeout", 45*time.Second, "每项检查超时")
	catalogOnly := flag.Bool("catalog", false, "只检查全市场目录及交易所资料")
	flag.Parse()
	if !provider.ValidCode(*code) {
		fmt.Fprintln(os.Stderr, "invalid code")
		os.Exit(2)
	}
	c, err := provider.New(provider.DefaultConfig())
	if err != nil {
		panic(err)
	}
	type result struct {
		Name      string `json:"name"`
		ElapsedMS int64  `json:"elapsed_ms"`
		Value     any    `json:"value,omitempty"`
		Error     string `json:"error,omitempty"`
	}
	encoder := json.NewEncoder(os.Stdout)
	failed := false
	run := func(name string, f func(context.Context) (any, error)) {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		start := time.Now()
		v, err := f(ctx)
		r := result{Name: name, ElapsedMS: time.Since(start).Milliseconds(), Value: v}
		if err != nil {
			r.Error = err.Error()
			failed = true
		}
		encoder.Encode(r)
	}
	if *catalogOnly {
		run("catalog", func(ctx context.Context) (any, error) {
			result, err := catalog.New(c.HTTP).Fetch(ctx)
			if err != nil {
				return nil, err
			}
			counts, err := catalog.ValidateDirectory(result.Securities)
			if err != nil {
				return nil, err
			}
			known := 0
			for _, s := range result.Securities {
				if s.ListingDate != nil {
					known++
				}
			}
			return map[string]any{"count": len(result.Securities), "counts": counts, "known_listing_dates": known, "fallback_reasons": result.FallbackReasons, "exchange_checks": result.ExchangeChecks}, nil
		})
		if failed {
			os.Exit(1)
		}
		return
	}
	var calendar []string
	run("calendar", func(ctx context.Context) (any, error) {
		var err error
		calendar, err = c.Calendar(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": len(calendar), "latest": calendar[len(calendar)-1]}, nil
	})
	run("stock", func(ctx context.Context) (any, error) { return c.Stock(ctx, *code) })
	run("sina_daily", func(ctx context.Context) (any, error) {
		rows, err := sina.New(c.HTTP).Daily(ctx, *code)
		if err != nil {
			return nil, err
		}
		return rows, nil
	})
	run("tdx_daily", func(ctx context.Context) (any, error) { return tdx.New().Daily(ctx, *code) })
	run("turnover", func(ctx context.Context) (any, error) {
		s, err := turnover.New(c.HTTP).Fetch(ctx, calendar)
		if err != nil {
			return nil, err
		}
		return s.View(calendar, c.Now()), nil
	})
	if failed {
		os.Exit(1)
	}
}
