package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
)

//go:embed data/listing-dates.json
var listingDateData []byte

type listingEvidence struct {
	Date     string `json:"date"`
	Source   string `json:"source"`
	Evidence string `json:"evidence"`
}

// Only individually verified issuer disclosures belong in this supplemental
// dataset. Existing directory dates remain authoritative and are not replaced.
var verifiedListingDates = loadListingEvidence()

func loadListingEvidence() map[string]listingEvidence {
	var rows map[string]listingEvidence
	if err := json.Unmarshal(listingDateData, &rows); err != nil {
		panic(fmt.Errorf("invalid embedded listing-date data: %w", err))
	}
	for symbol, row := range rows {
		source, err := url.Parse(row.Source)
		if len(symbol) != 8 || normalizedDate(row.Date) == nil || row.Evidence == "" || err != nil || source.Scheme != "https" || source.Host == "" {
			panic(fmt.Errorf("invalid embedded listing-date evidence for %s", symbol))
		}
		if _, err := NewSecurity(symbol[2:], symbol, symbol[:2], "公司公告"); err != nil {
			panic(err)
		}
	}
	return rows
}
