// Package turnover handles index-wide traded amounts independently of stock pools.
package turnover

import (
	"fmt"
	"market-mood/internal/market"
	"market-mood/internal/trading"
	"math"
	"slices"
	"time"
)

type Days map[string]map[string]float64
type State struct {
	Days       Days    `json:"days"`
	Provider   string  `json:"provider"`
	CapturedAt *string `json:"captured_at"`
	Error      *string `json:"error"`
}

func Minutes() []string {
	out := make([]string, 0, 241)
	for m := 570; m <= 900; m++ {
		if m > 690 && m < 781 {
			continue
		}
		out = append(out, fmt.Sprintf("%02d:%02d", m/60, m%60))
	}
	return out
}
func insert(days Days, day, minute string, amount float64) error {
	if _, err := time.Parse(time.DateOnly, day); err != nil {
		return err
	}
	if !slices.Contains(Minutes(), minute) {
		return fmt.Errorf("非交易分钟 %s", minute)
	}
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 {
		return fmt.Errorf("成交额数值异常")
	}
	if days[day] == nil {
		days[day] = map[string]float64{}
	}
	if _, exists := days[day][minute]; exists {
		return fmt.Errorf("分时分钟重复")
	}
	days[day][minute] = amount
	return nil
}
func Validate(days Days) error {
	if len(days) == 0 {
		return fmt.Errorf("分时数据为空")
	}
	expected := Minutes()
	for day, rows := range days {
		if _, err := time.Parse(time.DateOnly, day); err != nil {
			return err
		}
		if len(rows) == 0 || len(rows) > len(expected) {
			return fmt.Errorf("分时数量异常")
		}
		previous := 0.
		for _, minute := range expected[:len(rows)] {
			amount, ok := rows[minute]
			if !ok {
				return fmt.Errorf("分时缺少开盘或中间分钟 %s %s", day, minute)
			}
			if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < previous {
				return fmt.Errorf("累计成交额无效或倒退")
			}
			previous = amount
		}
	}
	return nil
}

// Combine aligns both exchanges by exact minute. No mixed-source or nearby-minute sums.
func Combine(sh, sz Days) (Days, error) {
	if err := Validate(sh); err != nil {
		return nil, err
	}
	if err := Validate(sz); err != nil {
		return nil, err
	}
	out := Days{}
	for day, rows := range sh {
		for minute, amount := range rows {
			if other, ok := sz[day][minute]; ok {
				if err := insert(out, day, minute, amount+other); err != nil {
					return nil, err
				}
			}
		}
	}
	return out, Validate(out)
}
func (s State) View(calendar []string, at time.Time) market.TurnoverView {
	at = at.In(trading.Shanghai)
	today, currentMinute := at.Format(time.DateOnly), at.Format("15:04")
	day, minute := "", ""
	for candidate := range s.Days {
		if candidate <= today && candidate > day && slices.Contains(calendar, candidate) {
			day = candidate
		}
	}
	for candidate := range s.Days[day] {
		if candidate > minute && (day != today || candidate <= currentMinute) {
			minute = candidate
		}
	}
	v := market.TurnoverView{Provider: s.Provider, CapturedAt: s.CapturedAt, Error: s.Error, Stale: true}
	previous := trading.Previous(calendar, day)
	if previous != "" {
		v.PreviousDay = &previous
	}
	if minute != "" {
		v.Amount = market.Ptr(s.Days[day][minute])
		v.SourceTime = market.Ptr(day + "T" + minute + ":00+08:00")
	}
	if amount, ok := s.Days[previous][minute]; ok && amount > 0 {
		v.PreviousSameAmount = &amount
	}
	v.Stale = trading.Stale(v.SourceTime, day, calendar, at)
	if !v.Stale && s.Error == nil && v.Amount != nil && v.PreviousSameAmount != nil {
		ratio := *v.Amount / *v.PreviousSameAmount
		v.VolumeChange = market.Ptr((ratio - 1) * 100)
		if total, ok := s.Days[previous]["15:00"]; ok {
			v.Forecast = market.Ptr(ratio * total)
		}
	}
	return v
}
