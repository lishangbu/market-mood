package trading

import (
	"testing"
	"time"
)

func instant(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
func TestFreshness(t *testing.T) {
	cal := []string{"2026-09-04", "2026-09-07"}
	cases := []struct {
		name, source, day, now string
		stale                  bool
	}{
		{"close rejects morning", "2026-09-07T10:00:00+08:00", "2026-09-07", "2026-09-07T15:30:00+08:00", true},
		{"close accepts closing quote", "2026-09-07T15:00:00+08:00", "2026-09-07", "2026-09-07T15:30:00+08:00", false},
		{"lunch requires boundary", "2026-09-07T11:00:00+08:00", "2026-09-07", "2026-09-07T12:00:00+08:00", true},
		{"lunch accepts boundary", "2026-09-07T11:30:00+08:00", "2026-09-07", "2026-09-07T12:00:00+08:00", false},
		{"future rejected", "2026-09-07T10:01:00+08:00", "2026-09-07", "2026-09-07T10:00:00+08:00", true},
		{"180 seconds accepted", "2026-09-07T10:00:00+08:00", "2026-09-07", "2026-09-07T10:03:00+08:00", false},
		{"181 seconds rejected", "2026-09-07T10:00:00+08:00", "2026-09-07", "2026-09-07T10:03:01+08:00", true},
		{"preopen Friday close", "2026-09-04T15:00:00+08:00", "2026-09-04", "2026-09-07T08:00:00+08:00", false},
		{"unknown weekday preopen", "2026-09-07T15:00:00+08:00", "2026-09-07", "2026-09-08T08:00:00+08:00", false},
		{"unknown weekday trading", "2026-09-07T15:00:00+08:00", "2026-09-07", "2026-09-08T10:00:00+08:00", true},
		{"timestamp must match batch", "2026-09-07T08:00:00+08:00", "2026-09-04", "2026-09-07T08:01:00+08:00", true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := Stale(&tt.source, tt.day, cal, instant(tt.now)); got != tt.stale {
				t.Fatalf("stale=%v want %v", got, tt.stale)
			}
		})
	}
	if !Stale(nil, "2026-09-07", cal, instant("2026-09-07T10:00:00+08:00")) {
		t.Fatal("missing source accepted")
	}
}
func TestSessionsAndPolling(t *testing.T) {
	cal := []string{"2026-09-07"}
	for _, tt := range []struct {
		at, state string
		active    bool
	}{
		{"2026-09-07T01:14:59Z", "盘前", false}, {"2026-09-07T01:15:00Z", "集合竞价", true},
		{"2026-09-07T04:00:00Z", "午间休市", false}, {"2026-09-07T05:00:00Z", "交易中", true},
		{"2026-09-06T02:00:00Z", "休市", false}, {"2026-09-08T02:00:00Z", "交易日待核验", true},
	} {
		at := instant(tt.at)
		if got := Session(cal, at); got != tt.state {
			t.Errorf("%s: %s", tt.at, got)
		}
		if ActiveWindow(at) != tt.active {
			t.Errorf("window %s", tt.at)
		}
	}
	if got := Session(nil, instant("2026-09-07T10:00:00+08:00")); got != "交易日待核验" {
		t.Fatal(got)
	}
	for _, tt := range []struct {
		base time.Duration
		fail uint32
		want time.Duration
	}{
		{10 * time.Second, 0, 10 * time.Second}, {60 * time.Second, 1, 120 * time.Second}, {60 * time.Second, ^uint32(0), 900 * time.Second}, {time.Duration(1<<63 - 1), 6, 900 * time.Second},
	} {
		if got := Backoff(tt.base, tt.fail); got != tt.want {
			t.Fatalf("backoff %v", got)
		}
	}
}
