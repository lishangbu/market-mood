package store

import (
	"context"
	"fmt"
	"market-mood/internal/catalog"
	"strings"
	"testing"
)

func directory(t *testing.T, shCount int) catalog.Result {
	t.Helper()
	r := catalog.Result{FallbackReasons: []string{"首源失败"}, ExchangeChecks: map[string]string{"sh": "已核对", "sz": "已核对", "bj": "未核对"}}
	for _, exchange := range []string{"sh", "sz", "bj"} {
		n, prefix := shCount, 600000
		if exchange == "sz" {
			n, prefix = 1000, 0
		}
		if exchange == "bj" {
			n, prefix = 100, 920000
		}
		for i := 0; i < n; i++ {
			s, err := catalog.NewSecurity(fmt.Sprintf("%06d", prefix+i), fmt.Sprintf("公司%d", i), exchange, "test")
			if err != nil {
				t.Fatal(err)
			}
			r.Securities = append(r.Securities, s)
		}
	}
	return r
}
func TestCatalogAtomicReplaceAndLiteralSearch(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	result := directory(t, 1100)
	result.Securities[0].Name = "特别%公司"
	if err := s.ReplaceCatalog(ctx, result); err != nil {
		t.Fatal(err)
	}
	page, err := s.Securities(ctx, catalog.Query{Q: "%", Page: 1, PageSize: 20})
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].Code != "600000" {
		t.Fatal("literal search", page, err)
	}
	page, err = s.Securities(ctx, catalog.Query{Exchange: "sh", Page: 2, PageSize: 100})
	if err != nil || page.Total != 1100 || page.Items[0].Code != "600100" {
		t.Fatal("pagination", page, err)
	}
	first, err := s.CatalogStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.Counts["bj"] != 100 || len(first.FallbackReasons) != 1 {
		t.Fatal("catalog status", first)
	}
	if err := s.SaveCatalogError(ctx, "网络失败"); err != nil {
		t.Fatal(err)
	}
	failed, err := s.CatalogStatus(ctx)
	if err != nil || failed.Error == nil || *failed.UpdatedAt != *first.UpdatedAt {
		t.Fatal("failed refresh erased success", failed, err)
	}
	smaller := directory(t, 1000)
	if err := s.ReplaceCatalog(ctx, smaller); err == nil || !strings.Contains(err.Error(), "异常下降") {
		t.Fatal("shrink accepted", err)
	}
	all, err := s.AllSecurities(ctx)
	if err != nil || len(all) != 2200 {
		t.Fatal("failed replacement modified catalog", len(all), err)
	}
	result.Securities[0].Name = "新名称"
	if err := s.ReplaceCatalog(ctx, result); err != nil {
		t.Fatal(err)
	}
	updated, err := s.Security(ctx, "600000")
	if err != nil || updated.Name != "新名称" {
		t.Fatal(updated, err)
	}
	var historical string
	if err := s.db.QueryRowContext(ctx, "SELECT name FROM catalog_revision_securities WHERE revision=1 AND code='600000'").Scan(&historical); err != nil || historical != "特别%公司" {
		t.Fatal("historical identity overwritten", historical, err)
	}
	status, _ := s.CatalogStatus(ctx)
	if status.Error != nil {
		t.Fatal("success didn't clear error")
	}
}
