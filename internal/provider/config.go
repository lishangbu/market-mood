// Package provider selects and validates market data before publishing it.
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

type Source string

const (
	TDX         Source = "tdx"
	Tencent     Source = "tencent"
	Eastmoney   Source = "eastmoney"
	Tonghuashun Source = "tonghuashun"
)

func (s Source) Name() string {
	switch s {
	case TDX:
		return "通达信"
	case Tencent:
		return "腾讯行情（新浪股票目录）"
	case Eastmoney:
		return "东方财富"
	case Tonghuashun:
		return "同花顺"
	default:
		return string(s)
	}
}

type Config struct {
	Quotes      []Source `json:"quotes"`
	StockQuotes []Source `json:"stock_quotes"`
}

func DefaultConfig() Config {
	return Config{Quotes: []Source{Tencent, Eastmoney}, StockQuotes: []Source{TDX, Tencent, Eastmoney}}
}
func (c Config) Clone() Config {
	return Config{Quotes: slices.Clone(c.Quotes), StockQuotes: slices.Clone(c.StockQuotes)}
}
func (c Config) Validate() error {
	for i, list := range [][]Source{c.Quotes, c.StockQuotes} {
		if len(list) == 0 {
			return fmt.Errorf("来源列表不能为空")
		}
		seen := map[Source]bool{}
		for _, s := range list {
			if seen[s] {
				return fmt.Errorf("来源不能重复: %s", s)
			}
			seen[s] = true
			switch s {
			case TDX, Tencent, Eastmoney:
			case Tonghuashun:
				if i == 0 {
					return fmt.Errorf("同花顺仅支持个股报价")
				}
			default:
				return fmt.Errorf("未知来源 %q", s)
			}
		}
	}
	return nil
}
func DecodeConfig(data []byte) (Config, error) {
	var c Config
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return c, fmt.Errorf("来源配置存在多余内容")
	}
	return c, c.Validate()
}
func LoadConfig() (Config, error) {
	path, ok := os.LookupEnv("MARKET_MOOD_SOURCES_FILE")
	if !ok {
		return DefaultConfig(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("读取来源配置 %s: %w", path, err)
	}
	c, err := DecodeConfig(data)
	if err != nil {
		return Config{}, fmt.Errorf("来源配置 %s: %w", path, err)
	}
	return c, nil
}

// Select calls each adapter in configured order. The adapter's validation is part
// of success, so fast stale responses do not defeat a slower valid fallback.
func Select[T any](ctx context.Context, sources []Source, fetch func(context.Context, Source) (T, error)) (T, Source, []string, error) {
	reasons := []string{}
	var zero T
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return zero, "", reasons, err
		}
		value, err := fetch(ctx, source)
		if err == nil {
			return value, source, reasons, nil
		}
		reasons = append(reasons, source.Name()+": "+err.Error())
	}
	return zero, "", reasons, fmt.Errorf("所有配置行情源失败：%s", strings.Join(reasons, "；"))
}
