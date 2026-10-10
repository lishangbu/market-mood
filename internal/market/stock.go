package market

import (
	"math"
	"strings"
)

func Ptr[T any](v T) *T { return &v }

func finite(v *float64) bool { return v != nil && !math.IsNaN(*v) && !math.IsInf(*v, 0) }

func (s Stock) MainBoard() bool {
	for _, prefix := range []string{"60", "000", "001", "002", "003"} {
		if strings.HasPrefix(s.Code, prefix) {
			return true
		}
	}
	return false
}

func (s Stock) Twenty() bool {
	return strings.HasPrefix(s.Code, "30") || strings.HasPrefix(s.Code, "688")
}

func (s Stock) Valid() bool {
	return s.Eligible && finite(s.Price) && *s.Price > 0 && finite(s.Open) && *s.Open > 0 && finite(s.Change)
}

func (s Stock) Yang() *bool {
	if !s.Valid() {
		return nil
	}
	return Ptr(*s.Price >= *s.Open)
}

// WithLimitFields joins event metadata only within the same trading date.
// Quote turnover is independent of buy orders at the limit price.
func WithLimitFields(stock Stock, frame *LimitFrame) Stock {
	if frame == nil || stock.SourceTime == nil || len(*stock.SourceTime) < 10 || (*stock.SourceTime)[:10] != frame.Day {
		return stock
	}
	for _, list := range [][]Stock{frame.Sealed, frame.Broken} {
		for _, event := range list {
			if event.Code != stock.Code {
				continue
			}
			stock.Sealed, stock.SealAmount = event.Sealed, nil
			if event.Sealed != nil && *event.Sealed {
				stock.SealAmount = event.SealAmount
			}
			stock.BreakCount, stock.FirstSealTime, stock.LastSealTime = event.BreakCount, event.FirstSealTime, event.LastSealTime
			stock.LimitStats, stock.Boards = event.LimitStats, event.Boards
			if stock.FloatCap == nil {
				stock.FloatCap = event.FloatCap
			}
			if stock.MarketCap == nil {
				stock.MarketCap = event.MarketCap
			}
			if stock.TurnoverRate == nil {
				stock.TurnoverRate = event.TurnoverRate
			}
			if stock.Industry == "" {
				stock.Industry = event.Industry
			}
			return stock
		}
	}
	return stock
}
