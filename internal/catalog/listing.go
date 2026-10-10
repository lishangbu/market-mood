package catalog

import (
	"context"
	"fmt"
	"market-mood/internal/provider/transport"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Verified against the issuer's announcement, section 一, page 1. The company
// profile currently has no listing date; this is evidence, not an inferred date.
var verifiedListingDates = map[string]struct{ date, source string }{
	"sz301286": {"2022-06-14", "https://static.cninfo.com.cn/finalpage/2023-06-16/1217087681.PDF"},
}

var companyTable = regexp.MustCompile(`(?s)<table\s+id="comInfo1"[^>]*>(.*?)</table>`)
var listingLink = regexp.MustCompile(`stockid=([0-9]{6})&(?:amp;)?InMarketDate=[0-9]+"[^>]*>\s*([0-9]{4}-[0-9]{2}-[0-9]{2})\s*</a>`)

func parseSinaListing(body []byte, row Security) (string, error) {
	page := string(body)
	if !strings.Contains(page, "/realstock/company/"+row.Exchange+row.Code+"/nc.shtml") {
		return "", fmt.Errorf("公司简介股票身份不一致")
	}
	table := companyTable.FindStringSubmatch(page)
	if len(table) != 2 {
		return "", fmt.Errorf("公司简介表缺失")
	}
	links := listingLink.FindAllStringSubmatch(table[1], -1)
	if len(links) != 1 || links[0][1] != row.Code {
		return "", fmt.Errorf("上市日期身份缺失或重复")
	}
	date := normalizedDate(links[0][2])
	if date == nil || *date > time.Now().Format(time.DateOnly) {
		return "", fmt.Errorf("上市日期无效")
	}
	return *date, nil
}

// SupplementListingDates fills only missing dates from identified company
// profiles. It never interprets incomplete official lists as delisting evidence.
func (c *Client) SupplementListingDates(ctx context.Context, rows []Security) map[string]string {
	type job struct {
		index int
		row   Security
	}
	jobs := make(chan job)
	var wg sync.WaitGroup
	var failed atomic.Int32
	counts := map[string]int{}
	var mu sync.Mutex
	for range 4 {
		wg.Go(func() {
			for j := range jobs {
				if ctx.Err() != nil || failed.Load() >= 20 {
					continue
				}
				if verified, ok := verifiedListingDates[j.row.Exchange+j.row.Code]; ok {
					date := verified.date
					rows[j.index].ListingDate = &date
					rows[j.index].Provider = j.row.Provider + "（上市日期：公司公告 " + verified.source + "）"
					mu.Lock()
					counts[j.row.Exchange]++
					mu.Unlock()
					continue
				}
				path := c.SinaBase + "/corp/go.php/vCI_CorpInfo/stockid/" + j.row.Code + ".phtml"
				body, err := transport.Get(ctx, c.HTTP, path, "https://finance.sina.com.cn/", 1<<20)
				date := ""
				if err == nil {
					date, err = parseSinaListing(body, j.row)
				}
				if err != nil {
					failed.Add(1)
					continue
				}
				rows[j.index].ListingDate = &date
				// Preserve the directory source; make the field's additional source visible.
				rows[j.index].Provider = j.row.Provider + "（上市日期：新浪公司资料）"
				mu.Lock()
				counts[j.row.Exchange]++
				mu.Unlock()
			}
		})
	}
	for i, row := range rows {
		if row.ListingDate != nil || (row.Exchange != "sh" && row.Exchange != "sz") {
			continue
		}
		select {
		case jobs <- job{i, row}:
		case <-ctx.Done():
		}
	}
	close(jobs)
	wg.Wait()
	checks := map[string]string{}
	for _, exchange := range []string{"sh", "sz"} {
		missing := 0
		for _, row := range rows {
			if row.Exchange == exchange && row.ListingDate == nil {
				missing++
			}
		}
		checks[exchange] = fmt.Sprintf("公司资料及公告补充上市日期%d只，仍待核验%d只", counts[exchange], missing)
	}
	return checks
}
