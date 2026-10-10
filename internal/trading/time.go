// Package trading centralizes exchange time and freshness rules.
package trading

import (
	"slices"
	"time"
)

var Shanghai = time.FixedZone("Asia/Shanghai", 8*60*60)

func Now() time.Time { return time.Now().In(Shanghai) }

// Previous returns the immediately preceding verified trading date.
func Previous(calendar []string, day string) string {
	var previous string
	for _, candidate := range calendar {
		if candidate < day && candidate > previous {
			previous = candidate
		}
	}
	return previous
}

func Session(calendar []string, at time.Time) string {
	at = at.In(Shanghai)
	day := at.Format(time.DateOnly)
	if !slices.Contains(calendar, day) {
		if at.Weekday() == time.Saturday || at.Weekday() == time.Sunday || (len(calendar) > 0 && slices.Max(calendar) > day) {
			return "休市"
		}
		return "交易日待核验"
	}
	m := at.Hour()*60 + at.Minute()
	switch {
	case m < 555:
		return "盘前"
	case m < 570:
		return "集合竞价"
	case m < 690:
		return "交易中"
	case m < 780:
		return "午间休市"
	case m < 900:
		return "交易中"
	default:
		return "已收盘"
	}
}

// Stale uses source time, never fetch time. A successful HTTP request does not
// prove freshness; lunch and close require observations reaching their boundary.
func Stale(sourceTime *string, day string, calendar []string, at time.Time) bool {
	if sourceTime == nil || !slices.Contains(calendar, day) {
		return true
	}
	observed, err := time.Parse(time.RFC3339, *sourceTime)
	if err != nil || observed.After(at) {
		return true
	}
	at, observed = at.In(Shanghai), observed.In(Shanghai)
	// A timestamp belonging to another day cannot validate this batch.
	if observed.Format(time.DateOnly) != day {
		return true
	}
	today := at.Format(time.DateOnly)
	m := at.Hour()*60 + at.Minute()
	expected := Previous(calendar, today)
	if m >= 555 && slices.Contains(calendar, today) {
		expected = today
	}
	if expected != day {
		return true
	}
	state := Session(calendar, at)
	if state == "交易日待核验" && m >= 555 {
		return true
	}
	if state == "交易中" || state == "集合竞价" {
		return day != today || at.Sub(observed) > 180*time.Second
	}
	boundary := 900
	if state == "午间休市" {
		boundary = 690
	}
	return observed.Hour()*60+observed.Minute() < boundary
}

// ActiveWindow controls polling frequency only; the calendar validates dates.
func ActiveWindow(at time.Time) bool {
	at = at.In(Shanghai)
	m := at.Hour()*60 + at.Minute()
	return at.Weekday() != time.Saturday && at.Weekday() != time.Sunday && ((m >= 555 && m <= 691) || (m >= 780 && m <= 905))
}

func Backoff(base time.Duration, failures uint32) time.Duration {
	if base <= 0 {
		return time.Second
	}
	multiplier := time.Duration(1 << min(failures, 6))
	if base >= 15*time.Minute/multiplier {
		return 15 * time.Minute
	}
	return base * multiplier
}

// PoolStale handles a date-identified historical pool re-requested after close.
// Unlike quote timestamps, a pool's CapturedAt is the request time. A confirmed
// past trading-date response can be fetched during a weekend or before opening;
// it must still identify the latest expected trading date. Same-day observations
// retain the intraday/lunch/close boundaries and timeout rules.
func PoolStale(capturedAt, day string, calendar []string, at time.Time) bool {
	captured, err := time.Parse(time.RFC3339, capturedAt)
	if err != nil || captured.After(at) || !slices.Contains(calendar, day) {
		return true
	}
	captured, at = captured.In(Shanghai), at.In(Shanghai)
	if captured.Format(time.DateOnly) <= day {
		return Stale(&capturedAt, day, calendar, at)
	}
	today := at.Format(time.DateOnly)
	expected := Previous(calendar, today)
	if at.Hour()*60+at.Minute() >= 555 && slices.Contains(calendar, today) {
		expected = today
	}
	if Session(calendar, at) == "交易日待核验" && at.Hour()*60+at.Minute() >= 555 {
		return true
	}
	return expected != day
}
