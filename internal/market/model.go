// Package market defines normalized market data and pure sentiment calculations.
// Pointer fields represent unknown values; they must remain JSON null, never zero.
package market

type BookLevel struct {
	Price  float64 `json:"price"`
	Shares float64 `json:"shares"`
}

type Stock struct {
	Bids            []BookLevel `json:"bids"`
	Asks            []BookLevel `json:"asks"`
	Code            string      `json:"code"`
	Name            string      `json:"name"`
	Industry        string      `json:"industry"`
	Price           *float64    `json:"price"`
	Change          *float64    `json:"change"`
	Amount          *float64    `json:"amount"`
	SealAmount      *float64    `json:"seal_amount"`
	BreakCount      *uint32     `json:"break_count"`
	FloatCap        *float64    `json:"float_cap"`
	MarketCap       *float64    `json:"market_cap"`
	TurnoverRate    *float64    `json:"turnover_rate"`
	FirstSealTime   *string     `json:"first_seal_time"`
	LastSealTime    *string     `json:"last_seal_time"`
	LimitStats      *LimitStats `json:"limit_stats"`
	Open            *float64    `json:"open"`
	PreviousClose   *float64    `json:"previous_close"`
	SourceTime      *string     `json:"source_time"`
	Provider        *string     `json:"provider"`
	FallbackReasons []string    `json:"fallback_reasons"`
	LimitUp         *float64    `json:"limit_up"`
	LimitDown       *float64    `json:"limit_down"`
	Boards          *uint32     `json:"boards"`
	Sealed          *bool       `json:"sealed"`
	Eligible        bool        `json:"eligible"`
}

type LimitStats struct {
	Days  uint32 `json:"days"`
	Count uint32 `json:"count"`
}

type MarketFrame struct {
	Day             string   `json:"day"`
	CapturedAt      string   `json:"captured_at"`
	SourceTime      *string  `json:"source_time"`
	Provider        *string  `json:"provider"`
	FallbackReasons []string `json:"fallback_reasons"`
	Stocks          []Stock  `json:"stocks"`
	Complete        bool     `json:"complete"`
}

type LimitFrame struct {
	Day        string  `json:"day"`
	CapturedAt string  `json:"captured_at"`
	Sealed     []Stock `json:"sealed"`
	Broken     []Stock `json:"broken"`
}

type CloseFrame struct {
	Market MarketFrame `json:"market"`
	Limits LimitFrame  `json:"limits"`
}

type Pool struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Ready       bool     `json:"ready"`
	Stocks      []Stock  `json:"stocks"`
	Count       int      `json:"count"`
	ValidCount  int      `json:"valid_count"`
	Average     *float64 `json:"average"`
	YangRate    *float64 `json:"yang_rate"`
}

type Factor struct {
	Name   string     `json:"name"`
	Weight float64    `json:"weight"`
	Value  *float64   `json:"value"`
	Score  *float64   `json:"score"`
	Unit   string     `json:"unit"`
	Range  [2]float64 `json:"range"`
}

type Score struct {
	Version string   `json:"version"`
	Total   *float64 `json:"total"`
	Factors []Factor `json:"factors"`
}

type Breadth struct {
	Up                 int      `json:"up"`
	Down               int      `json:"down"`
	Flat               int      `json:"flat"`
	Yang               int      `json:"yang"`
	Yin                int      `json:"yin"`
	Valid              int      `json:"valid"`
	Excluded           int      `json:"excluded"`
	Amount             *float64 `json:"amount"`
	PreviousSameAmount *float64 `json:"previous_same_amount"`
	VolumeChange       *float64 `json:"volume_change"`
	Forecast           *float64 `json:"forecast"`
}

type Sealing struct {
	Sealed int      `json:"sealed"`
	Broken int      `json:"broken"`
	Rate   *float64 `json:"rate"`
	Height *uint32  `json:"height"`
}

type SourceStatus struct {
	Name            string   `json:"name"`
	CapturedAt      *string  `json:"captured_at"`
	SourceTime      *string  `json:"source_time"`
	Provider        *string  `json:"provider"`
	FallbackReasons []string `json:"fallback_reasons"`
	Error           *string  `json:"error"`
	Stale           bool     `json:"stale"`
}

type Dashboard struct {
	Top50Rebuild     *Top50State     `json:"top50_rebuild"`
	YesterdayRebuild *YesterdayState `json:"yesterday_rebuild"`
	Turnover         *TurnoverView   `json:"turnover"`
	Day              *string         `json:"day"`
	GeneratedAt      string          `json:"generated_at"`
	ObservedAt       *string         `json:"observed_at"`
	Session          string          `json:"session"`
	Sources          []SourceStatus  `json:"sources"`
	Pools            []Pool          `json:"pools"`
	Score            Score           `json:"score"`
	Breadth          *Breadth        `json:"breadth"`
	Capacity         *Sealing        `json:"capacity"`
	Sealing          *Sealing        `json:"sealing"`
	PreviousDay      *string         `json:"previous_day"`
	Notes            []string        `json:"notes"`
}

type Watch struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

type RuntimeData struct {
	PreviousTop50  *Top50State  `json:"previous_top50"`
	PreviousLimits *LimitFrame  `json:"previous_limits"`
	Market         *MarketFrame `json:"market"`
	Limits         *LimitFrame  `json:"limits"`
	Previous       *CloseFrame  `json:"previous"`
	Calendar       []string     `json:"calendar"`
	MarketError    *string      `json:"market_error"`
	LimitError     *string      `json:"limit_error"`
	CalendarError  *string      `json:"calendar_error"`
	WatchQuotes    []Stock      `json:"watch_quotes"`
	WatchError     *string      `json:"watch_error"`
}

type Top50State struct {
	Version   uint32  `json:"version"`
	Day       string  `json:"day"`
	Checked   int     `json:"checked"`
	Total     int     `json:"total"`
	Excluded  int     `json:"excluded"`
	Failed    int     `json:"failed"`
	Complete  bool    `json:"complete"`
	Running   bool    `json:"running"`
	Error     *string `json:"error"`
	UpdatedAt string  `json:"updated_at"`
	Stocks    []Stock `json:"stocks"`
}

type Bar struct {
	Day       string   `json:"day"`
	Open      float64  `json:"open"`
	Close     float64  `json:"close"`
	High      float64  `json:"high"`
	Low       float64  `json:"low"`
	Volume    float64  `json:"volume"`
	Amount    *float64 `json:"amount"`
	Prevclose *float64 `json:"prevclose"`
}

type YesterdayState struct {
	Version   uint32      `json:"version"`
	Day       string      `json:"day"`
	Checked   int         `json:"checked"`
	Total     int         `json:"total"`
	Excluded  int         `json:"excluded"`
	Failed    int         `json:"failed"`
	Complete  bool        `json:"complete"`
	Running   bool        `json:"running"`
	Error     *string     `json:"error"`
	UpdatedAt string      `json:"updated_at"`
	Limits    *LimitFrame `json:"limits"`
	TdxCount  int         `json:"tdx_count"`
}

type TurnoverView struct {
	Amount             *float64 `json:"amount"`
	PreviousSameAmount *float64 `json:"previous_same_amount"`
	VolumeChange       *float64 `json:"volume_change"`
	Forecast           *float64 `json:"forecast"`
	SourceTime         *string  `json:"source_time"`
	PreviousDay        *string  `json:"previous_day"`
	Provider           string   `json:"provider"`
	CapturedAt         *string  `json:"captured_at"`
	Stale              bool     `json:"stale"`
	Error              *string  `json:"error"`
}
