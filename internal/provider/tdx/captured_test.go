package tdx

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestDailyPacketMatchesDocumentedPytdxRequest(t *testing.T) {
	// pytdx GetSecurityBarsCmd's published market=0, code=000001, day=9,
	// start=0, count=10 packet. This validates byte offsets, not just length.
	want := "0c01086401011c001c002d0500003030303030310900010000000a0000000000000000000000"
	if got := hex.EncodeToString(dailyPacket("000001", 0, 10)); got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

type capturedConnection struct {
	*bytes.Reader
	sent bytes.Buffer
}

func (c *capturedConnection) Write(p []byte) (int, error) { return c.sent.Write(p) }
func TestCapturedMalformedDailyResponsesAreRejected(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/validation/2026-10-09-tdx-wire-probe.json")
	if err != nil {
		t.Fatal(err)
	}
	var captures []struct {
		Header string `json:"header_hex"`
		Body   string `json:"body_prefix_hex"`
		Code   string `json:"code"`
		Market byte   `json:"market"`
		Count  uint16 `json:"requested_count"`
		Length int    `json:"body_length"`
	}
	if err := json.Unmarshal(raw, &captures); err != nil {
		t.Fatal(err)
	}
	if len(captures) != 8 {
		t.Fatal("missing host/count combinations", len(captures))
	}
	for _, capture := range captures {
		header, err := hex.DecodeString(capture.Header)
		if err != nil {
			t.Fatal(err)
		}
		body, err := hex.DecodeString(capture.Body)
		if err != nil {
			t.Fatal(err)
		}
		if len(body) != capture.Length {
			t.Fatal("not a complete captured response")
		}
		conn := &capturedConnection{Reader: bytes.NewReader(append(header, body...))}
		decoded, err := request(conn, dailyPacket(capture.Code, capture.Market, capture.Count))
		if err != nil {
			t.Fatal("valid framing rejected", err)
		}
		if _, err := parseDaily(decoded); err == nil {
			t.Fatal("malformed remote data accepted")
		}
	}
}
