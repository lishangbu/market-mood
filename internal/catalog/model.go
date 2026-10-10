// Package catalog defines verified security identities, separate from live prices.
package catalog

import (
	"fmt"
	"strings"
	"time"
)

type Security struct {
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	Exchange      string  `json:"exchange"`
	Board         string  `json:"board"`
	Industry      *string `json:"industry"`
	ListingDate   *string `json:"listing_date"`
	ListingStatus *string `json:"listing_status"`
	Provider      string  `json:"provider"`
}
type Status struct {
	UpdatedAt       *string           `json:"updated_at"`
	AttemptedAt     *string           `json:"attempted_at"`
	Provider        *string           `json:"provider"`
	Error           *string           `json:"error"`
	FallbackReasons []string          `json:"fallback_reasons"`
	Counts          map[string]int    `json:"counts"`
	ExchangeChecks  map[string]string `json:"exchange_checks"`
}
type Query struct {
	Q        string `json:"q"`
	Exchange string `json:"exchange"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}
type Page struct {
	Items    []Security `json:"items"`
	Total    int        `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
	Status   Status     `json:"status"`
}

func NewSecurity(code, name, exchange, provider string) (Security, error) {
	s := Security{Code: code, Name: strings.TrimSpace(name), Exchange: exchange, Provider: provider}
	if len(code) != 6 || s.Name == "" {
		return s, fmt.Errorf("证券代码或名称无效")
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return s, fmt.Errorf("证券代码无效")
		}
	}
	switch {
	case exchange == "sh" && strings.HasPrefix(code, "688"):
		s.Board = "科创板"
	case exchange == "sh" && strings.HasPrefix(code, "60"):
		s.Board = "主板"
	case exchange == "sz" && strings.HasPrefix(code, "30"):
		s.Board = "创业板"
	case exchange == "sz" && strings.HasPrefix(code, "00"):
		s.Board = "主板"
	case exchange == "bj" && (strings.HasPrefix(code, "4") || strings.HasPrefix(code, "8") || strings.HasPrefix(code, "92")):
		s.Board = "北交所"
	default:
		return s, fmt.Errorf("非沪深北A股代码或市场不匹配")
	}
	return s, nil
}
func (s Security) Validate() error {
	basic, err := NewSecurity(s.Code, s.Name, s.Exchange, s.Provider)
	if err != nil {
		return err
	}
	if basic.Board != s.Board {
		return fmt.Errorf("证券板块不符")
	}
	if s.ListingDate != nil {
		if _, err := time.Parse(time.DateOnly, *s.ListingDate); err != nil {
			return fmt.Errorf("上市日期: %w", err)
		}
	}
	return nil
}
func (q Query) Validate() error {
	if q.Page < 1 || q.Page > 100000 {
		return fmt.Errorf("page应为1至100000")
	}
	if q.PageSize < 1 || q.PageSize > 100 {
		return fmt.Errorf("page_size应为1至100")
	}
	if len(q.Q) > 120 {
		return fmt.Errorf("搜索词过长")
	}
	if q.Exchange != "" && q.Exchange != "sh" && q.Exchange != "sz" && q.Exchange != "bj" {
		return fmt.Errorf("exchange应为sh/sz/bj")
	}
	return nil
}
func ValidateDirectory(rows []Security) (map[string]int, error) {
	counts := map[string]int{}
	seen := map[string]bool{}
	for _, s := range rows {
		if err := s.Validate(); err != nil {
			return nil, fmt.Errorf("证券 %s: %w", s.Code, err)
		}
		if seen[s.Code] {
			return nil, fmt.Errorf("股票目录重复代码")
		}
		seen[s.Code] = true
		counts[s.Exchange]++
	}
	for exchange, minimum := range map[string]int{"sh": 1000, "sz": 1000, "bj": 100} {
		if counts[exchange] < minimum {
			return nil, fmt.Errorf("股票目录市场覆盖不足: %s", exchange)
		}
	}
	return counts, nil
}
