package market

import (
	"fmt"
	"math"
	"time"
)

// ValidateBars checks chronological OHLC data before it can enter the cache.
func ValidateBars(rows []Bar) error {
	if len(rows) == 0 {
		return fmt.Errorf("历史日线为空")
	}
	previous := ""
	for i, b := range rows {
		if _, err := time.Parse(time.DateOnly, b.Day); err != nil {
			return fmt.Errorf("日线 %d 日期: %w", i, err)
		}
		if b.Day <= previous {
			return fmt.Errorf("日线 %d 日期倒序或重复", i)
		}
		previous = b.Day
		for _, n := range []float64{b.Open, b.Close, b.High, b.Low, b.Volume} {
			if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
				return fmt.Errorf("日线 %s 价格或量无效", b.Day)
			}
		}
		if b.Low > b.Open || b.Low > b.Close || b.High < b.Open || b.High < b.Close {
			return fmt.Errorf("日线 %s OHLC 范围异常", b.Day)
		}
		if b.Amount != nil && (!finite(b.Amount) || *b.Amount < 0) {
			return fmt.Errorf("日线 %s 成交额无效", b.Day)
		}
		if b.Prevclose != nil && (!finite(b.Prevclose) || *b.Prevclose < 0) {
			return fmt.Errorf("日线 %s 前收无效", b.Day)
		}
	}
	return nil
}
