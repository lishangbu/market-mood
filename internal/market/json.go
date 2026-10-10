// JSON methods preserve the existing explicit-null and non-null-array contract.
package market

import "encoding/json"

func (v *BookLevel) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"price", "shares"}, []string{}); err != nil {
		return err
	}
	type wire BookLevel
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*v = BookLevel(parsed)
	return nil
}

func (v Stock) MarshalJSON() ([]byte, error) {
	type wire Stock
	if v.Bids == nil {
		v.Bids = []BookLevel{}
	}
	if v.Asks == nil {
		v.Asks = []BookLevel{}
	}
	if v.FallbackReasons == nil {
		v.FallbackReasons = []string{}
	}
	return json.Marshal(wire(v))
}

func (v *Stock) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"bids", "asks", "code", "name", "industry", "price", "change", "amount", "seal_amount", "break_count", "float_cap", "market_cap", "turnover_rate", "first_seal_time", "last_seal_time", "limit_stats", "open", "previous_close", "source_time", "provider", "fallback_reasons", "limit_up", "limit_down", "boards", "sealed", "eligible"}, []string{"price", "change", "amount", "seal_amount", "break_count", "float_cap", "market_cap", "turnover_rate", "first_seal_time", "last_seal_time", "limit_stats", "open", "previous_close", "source_time", "provider", "limit_up", "limit_down", "boards", "sealed"}); err != nil {
		return err
	}
	type wire Stock
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*v = Stock(parsed)
	return nil
}

func (v *LimitStats) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"days", "count"}, []string{}); err != nil {
		return err
	}
	type wire LimitStats
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*v = LimitStats(parsed)
	return nil
}

func (v MarketFrame) MarshalJSON() ([]byte, error) {
	type wire MarketFrame
	if v.FallbackReasons == nil {
		v.FallbackReasons = []string{}
	}
	if v.Stocks == nil {
		v.Stocks = []Stock{}
	}
	return json.Marshal(wire(v))
}

func (v *MarketFrame) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"day", "captured_at", "source_time", "provider", "fallback_reasons", "stocks", "complete"}, []string{"source_time", "provider"}); err != nil {
		return err
	}
	type wire MarketFrame
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*v = MarketFrame(parsed)
	return nil
}

func (v LimitFrame) MarshalJSON() ([]byte, error) {
	type wire LimitFrame
	if v.Sealed == nil {
		v.Sealed = []Stock{}
	}
	if v.Broken == nil {
		v.Broken = []Stock{}
	}
	return json.Marshal(wire(v))
}

func (v *LimitFrame) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"day", "captured_at", "sealed", "broken"}, []string{}); err != nil {
		return err
	}
	type wire LimitFrame
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*v = LimitFrame(parsed)
	return nil
}

func (v *CloseFrame) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"market", "limits"}, []string{}); err != nil {
		return err
	}
	type wire CloseFrame
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*v = CloseFrame(parsed)
	return nil
}

func (v Pool) MarshalJSON() ([]byte, error) {
	type wire Pool
	if v.Stocks == nil {
		v.Stocks = []Stock{}
	}
	return json.Marshal(wire(v))
}

func (v *Pool) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"id", "title", "description", "ready", "stocks", "count", "valid_count", "average", "yang_rate"}, []string{"average", "yang_rate"}); err != nil {
		return err
	}
	type wire Pool
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*v = Pool(parsed)
	return nil
}

func (v *Factor) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"name", "weight", "value", "score", "unit", "range"}, []string{"value", "score"}); err != nil {
		return err
	}
	type wire Factor
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*v = Factor(parsed)
	return nil
}

func (v Score) MarshalJSON() ([]byte, error) {
	type wire Score
	if v.Factors == nil {
		v.Factors = []Factor{}
	}
	return json.Marshal(wire(v))
}

func (v *Score) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"version", "total", "factors"}, []string{"total"}); err != nil {
		return err
	}
	type wire Score
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*v = Score(parsed)
	return nil
}

func (v *Breadth) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"up", "down", "flat", "yang", "yin", "valid", "excluded", "amount", "previous_same_amount", "volume_change", "forecast"}, []string{"amount", "previous_same_amount", "volume_change", "forecast"}); err != nil {
		return err
	}
	type wire Breadth
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*v = Breadth(parsed)
	return nil
}

func (v *Sealing) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"sealed", "broken", "rate", "height"}, []string{"rate", "height"}); err != nil {
		return err
	}
	type wire Sealing
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*v = Sealing(parsed)
	return nil
}

func (v SourceStatus) MarshalJSON() ([]byte, error) {
	type wire SourceStatus
	if v.FallbackReasons == nil {
		v.FallbackReasons = []string{}
	}
	return json.Marshal(wire(v))
}

func (v *SourceStatus) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"name", "captured_at", "source_time", "provider", "fallback_reasons", "error", "stale"}, []string{"captured_at", "source_time", "provider", "error"}); err != nil {
		return err
	}
	type wire SourceStatus
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*v = SourceStatus(parsed)
	return nil
}

func (v Dashboard) MarshalJSON() ([]byte, error) {
	type wire Dashboard
	if v.Sources == nil {
		v.Sources = []SourceStatus{}
	}
	if v.Pools == nil {
		v.Pools = []Pool{}
	}
	if v.Notes == nil {
		v.Notes = []string{}
	}
	return json.Marshal(wire(v))
}

func (v *Dashboard) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"top50_rebuild", "yesterday_rebuild", "turnover", "day", "generated_at", "observed_at", "session", "sources", "pools", "score", "breadth", "capacity", "sealing", "previous_day", "notes"}, []string{"top50_rebuild", "yesterday_rebuild", "turnover", "day", "observed_at", "breadth", "capacity", "sealing", "previous_day"}); err != nil {
		return err
	}
	type wire Dashboard
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*v = Dashboard(parsed)
	return nil
}

func (v *Watch) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"code", "name", "created_at"}, []string{}); err != nil {
		return err
	}
	type wire Watch
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*v = Watch(parsed)
	return nil
}

func (v RuntimeData) MarshalJSON() ([]byte, error) {
	type wire RuntimeData
	if v.Calendar == nil {
		v.Calendar = []string{}
	}
	if v.WatchQuotes == nil {
		v.WatchQuotes = []Stock{}
	}
	return json.Marshal(wire(v))
}

func (v *RuntimeData) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"previous_top50", "previous_limits", "market", "limits", "previous", "calendar", "market_error", "limit_error", "calendar_error", "watch_quotes", "watch_error"}, []string{"previous_top50", "previous_limits", "market", "limits", "previous", "market_error", "limit_error", "calendar_error", "watch_error"}); err != nil {
		return err
	}
	type wire RuntimeData
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*v = RuntimeData(parsed)
	return nil
}

func (v Top50State) MarshalJSON() ([]byte, error) {
	type wire Top50State
	if v.Stocks == nil {
		v.Stocks = []Stock{}
	}
	return json.Marshal(wire(v))
}

func (v *Top50State) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"version", "day", "checked", "total", "excluded", "failed", "complete", "running", "error", "updated_at", "stocks"}, []string{"error"}); err != nil {
		return err
	}
	type wire Top50State
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*v = Top50State(parsed)
	return nil
}

func (v *Bar) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"day", "open", "close", "high", "low", "volume", "amount", "prevclose"}, []string{"amount", "prevclose"}); err != nil {
		return err
	}
	type wire Bar
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*v = Bar(parsed)
	return nil
}

func (v *YesterdayState) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"version", "day", "checked", "total", "excluded", "failed", "complete", "running", "error", "updated_at", "limits", "tdx_count"}, []string{"error", "limits"}); err != nil {
		return err
	}
	type wire YesterdayState
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*v = YesterdayState(parsed)
	return nil
}

func (v *TurnoverView) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"amount", "previous_same_amount", "volume_change", "forecast", "source_time", "previous_day", "provider", "captured_at", "stale", "error"}, []string{"amount", "previous_same_amount", "volume_change", "forecast", "source_time", "previous_day", "captured_at", "error"}); err != nil {
		return err
	}
	type wire TurnoverView
	var parsed wire
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*v = TurnoverView(parsed)
	return nil
}
