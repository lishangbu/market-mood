package tdx

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveDaily(t *testing.T) {
	if os.Getenv("MARKET_MOOD_LIVE_TEST") != "1" {
		t.Skip("explicit live data check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rows, err := New().Daily(ctx, "600000")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("rows=%d last=%+v", len(rows), rows[len(rows)-1])
}
