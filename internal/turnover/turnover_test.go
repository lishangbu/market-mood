package turnover

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func instant(s string) time.Time {
	at, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return at
}
func syntheticWindow(t *testing.T) Days {
	t.Helper()
	sh, _ := os.ReadFile("../../testdata/turnover-synthetic-sh.json")
	sz, _ := os.ReadFile("../../testdata/turnover-synthetic-sz.json")
	a, err := ParseEastmoney(sh, "000001")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseEastmoney(sz, "399001")
	if err != nil {
		t.Fatal(err)
	}
	out, err := Combine(a, b)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestSyntheticWindowProvidesYesterday(t *testing.T) {
	s := State{Days: syntheticWindow(t), Provider: "东方财富"}
	if len(s.Days["2026-09-07"]) != 241 {
		t.Fatal("full day minutes")
	}
	cal := []string{"2026-09-04", "2026-09-07", "2026-09-08"}
	v := s.View(cal, instant("2026-09-08T10:20:30+08:00"))
	if v.Stale || v.PreviousDay == nil || *v.PreviousDay != "2026-09-07" || v.Amount == nil || v.PreviousSameAmount == nil || v.VolumeChange == nil || v.Forecast == nil || *v.Forecast <= *v.Amount {
		t.Fatalf("invalid view %+v", v)
	}
	if *v.VolumeChange != (*v.Amount / *v.PreviousSameAmount - 1)*100 {
		t.Fatal("same minute ratio")
	}
	v = s.View(cal, instant("2026-09-08T11:00:00+08:00"))
	if !v.Stale || v.Amount == nil || v.VolumeChange != nil || v.Forecast != nil {
		t.Fatal("stale calculations")
	}
	failure := "failed"
	s.Error = &failure
	v = s.View(cal, instant("2026-09-08T10:20:30+08:00"))
	if v.VolumeChange != nil || v.Forecast != nil || v.Amount == nil {
		t.Fatal("failed source calculations")
	}
}
func TestSourcesNormalizeAndRejectGaps(t *testing.T) {
	em := `{"rc":0,"data":{"code":"000001","trends":["2026-09-07 09:30,0,0,0,0,0,100","2026-09-07 09:31,0,0,0,0,0,25"]}}`
	tx := `{"code":0,"data":{"sh000001":{"data":[{"date":"20260907","data":["0930 1 1 100","0931 1 1 125"]}]}}}`
	a, err := ParseEastmoney([]byte(em), "000001")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseTencent([]byte(tx), "sh000001")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("minute amounts != cumulative amounts")
	}
	for _, bad := range []string{strings.Replace(em, "09:31", "09:32", 1), strings.Replace(em, "09:31", "09:30", 1), strings.Replace(em, "000001", "399001", 1), strings.Replace(em, ",25", ",-1", 1)} {
		if _, err := ParseEastmoney([]byte(bad), "000001"); err == nil {
			t.Fatal("bad sequence accepted")
		}
	}
	if _, err := ParseTencent([]byte(strings.Replace(tx, "1 125", "1 99", 1)), "sh000001"); err == nil {
		t.Fatal("decreasing cumulative amount")
	}
}
func TestExactMinuteAlignment(t *testing.T) {
	sh := Days{"2026-09-08": {"09:30": 100, "09:31": 200}}
	sz := Days{"2026-09-08": {"09:30": 50}}
	days, err := Combine(sh, sz)
	if err != nil {
		t.Fatal(err)
	}
	if len(days["2026-09-08"]) != 1 || days["2026-09-08"]["09:30"] != 150 {
		t.Fatal("unaligned markets added")
	}
	s := State{Days: days}
	cal := []string{"2026-09-07", "2026-09-08"}
	at := instant("2026-09-08T09:31:00+08:00")
	v := s.View(cal, at)
	if v.VolumeChange != nil {
		t.Fatal("missing history substituted")
	}
	s.Days["2026-09-07"] = map[string]float64{"09:30": 0}
	if s.View(cal, at).VolumeChange != nil {
		t.Fatal("zero denominator")
	}
	s.Days["2026-09-07"] = map[string]float64{"09:29": 100}
	if s.View(cal, at).VolumeChange != nil {
		t.Fatal("nearby minute substituted")
	}
}
