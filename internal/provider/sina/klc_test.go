package sina

import (
	"encoding/json"
	"market-mood/internal/market"
	"math"
	"os"
	"testing"
)

func TestKLCGolden(t *testing.T) {
	body, err := os.ReadFile("../../../testdata/sina-sh600000-klc2.js")
	if err != nil {
		t.Fatal(err)
	}
	golden, err := os.ReadFile("../../../testdata/sina-sh600000-golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var want []market.Bar
	if err := json.Unmarshal(golden, &want); err != nil {
		t.Fatal(err)
	}
	got, err := Decode(string(body), "sh600000")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("rows: %d != %d", len(got), len(want))
	}
	for i, row := range got {
		if row.Day != want[i].Day {
			t.Fatalf("row %d day %s != %s", i, row.Day, want[i].Day)
		}
		a, b := []float64{row.Open, row.High, row.Low, row.Close, row.Volume, *row.Amount}, []float64{want[i].Open, want[i].High, want[i].Low, want[i].Close, want[i].Volume, *want[i].Amount}
		for j := range a {
			if math.Abs(a[j]-b[j]) > 1e-8 {
				t.Fatalf("row %d %s field %d %.12f != %.12f", i, row.Day, j, a[j], b[j])
			}
		}
		if (row.Prevclose == nil) != (want[i].Prevclose == nil) {
			t.Fatalf("row %d prevclose presence", i)
		}
		if row.Prevclose != nil && math.Abs(*row.Prevclose-*want[i].Prevclose) > 1e-8 {
			t.Fatalf("row %d prevclose", i)
		}
	}
}
func TestMalformedKLC(t *testing.T) {
	body, err := os.ReadFile("../../../testdata/sina-sh600000-klc2.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(string(body), "sh600001"); err == nil {
		t.Fatal("wrong identity accepted")
	}
	for _, raw := range []string{"", `var KLC_K2_sh600000="?";`, `var KLC_K2_sh600000="AAA";`, `var KLC_K2_sh600000="K2/";`, `var KLC_K2_sh600000=process.exit(1)`} {
		if _, err := Decode(raw, "sh600000"); err == nil {
			t.Fatalf("invalid payload accepted %q", raw)
		}
	}
	b, _ := newBits("////")
	b.read(25, false)
	if b.err == nil {
		t.Fatal("bit read beyond payload")
	}
	b, _ = newBits("////")
	b.read(-1, false)
	if b.err == nil {
		t.Fatal("negative bit width")
	}
}
func FuzzDecode(f *testing.F) {
	f.Add(`var KLC_K2_sh600000="K2/";`)
	f.Add("")
	f.Fuzz(func(t *testing.T, body string) { Decode(body, "sh600000") })
}
