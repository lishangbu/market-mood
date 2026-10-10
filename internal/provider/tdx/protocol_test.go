package tdx

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"io"
	"testing"
)

func encodePrice(n int64) []byte {
	v := uint64(n)
	if n < 0 {
		v = uint64(-n)
	}
	first := byte(v & 63)
	if n < 0 {
		first |= 64
	}
	v >>= 6
	if v > 0 {
		first |= 128
	}
	out := []byte{first}
	for v > 0 {
		b := byte(v & 127)
		v >>= 7
		if v > 0 {
			b |= 128
		}
		out = append(out, b)
	}
	return out
}
func quotePacket() []byte {
	b := []byte{0, 0, 1, 0, 1}
	b = append(b, "600000"...)
	b = append(b, 0, 0)
	for _, n := range []int64{1000, -100, -50, 20, -60, 10300000, -1000, 100, 1} {
		b = append(b, encodePrice(n)...)
	}
	b = binary.LittleEndian.AppendUint32(b, 0)
	b = append(b, 0, 0, 0, 0)
	for i := int64(0); i < 5; i++ {
		for _, n := range []int64{-i, 1 + i, 10, 20} {
			b = append(b, encodePrice(n)...)
		}
	}
	return append(b, make([]byte, 10)...)
}
func TestQuoteContract(t *testing.T) {
	stocks, err := parseQuotes(quotePacket(), "2026-09-07")
	if err != nil {
		t.Fatal(err)
	}
	s := stocks[0]
	if s.Code != "600000" || *s.Price != 10 || *s.PreviousClose != 9 || *s.SourceTime != "2026-09-07T10:30:00+08:00" || s.Bids[0].Shares != 1000 || s.Asks[0].Price != 10.01 {
		t.Fatalf("wrong units or identity: %+v", s)
	}
	if s.SealAmount != nil || s.LimitUp != nil || s.Eligible {
		t.Fatal("unavailable facts fabricated")
	}
	p := quotePacket()
	for i := 0; i < len(p); i++ {
		if _, err := parseQuotes(p[:i], "2026-09-07"); err == nil {
			t.Fatalf("accepted truncation at %d", i)
		}
	}
	p[4] = 0
	if _, err := parseQuotes(p, "2026-09-07"); err == nil {
		t.Fatal("wrong market accepted")
	}
	if _, err := parseQuotes(append(quotePacket(), 0), "2026-09-07"); err == nil {
		t.Fatal("unknown trailer accepted")
	}
	for _, raw := range []int64{-1, 25000000} {
		if _, err := quoteTime(raw); err == nil {
			t.Fatal("invalid time accepted")
		}
	}
}
func TestDailyDeltaAndDates(t *testing.T) {
	p := []byte{2, 0}
	for i, date := range []uint32{20260904, 20260907} {
		p = binary.LittleEndian.AppendUint32(p, date)
		open := int64(10000)
		if i == 1 {
			open = -500
		}
		for _, v := range []int64{open, 100, 200, -100} {
			p = append(p, encodePrice(v)...)
		}
		p = binary.LittleEndian.AppendUint32(p, 0)
		p = binary.LittleEndian.AppendUint32(p, 0)
	}
	rows, err := parseDaily(p)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Open != 10 || rows[0].Close != 10.1 || rows[1].Open != 9.6 || rows[1].Close != 9.7 || rows[1].Day != "2026-09-07" || *rows[1].Amount != 0 {
		t.Fatalf("wrong daily deltas %+v", rows)
	}
	for i := 0; i < len(p); i++ {
		if _, err := parseDaily(p[:i]); err == nil {
			t.Fatalf("accepted truncated daily %d", i)
		}
	}
	d := decoder{body: []byte{0x41}}
	if d.price() != -1 || d.err != nil {
		t.Fatal("signed price")
	}
	d = decoder{body: bytes.Repeat([]byte{0x80}, 10)}
	d.price()
	if d.err == nil {
		t.Fatal("unbounded varint")
	}
}

type exchangeIO struct {
	in  io.Reader
	out bytes.Buffer
}

func (e *exchangeIO) Read(p []byte) (int, error)  { return e.in.Read(p) }
func (e *exchangeIO) Write(p []byte) (int, error) { return e.out.Write(p) }

type fragmented struct{ r *bytes.Reader }

func (f fragmented) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return f.r.Read(p)
}
func TestCompressedAndFragmentedResponses(t *testing.T) {
	raw := bytes.Repeat([]byte{1, 2, 3}, 100)
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	w.Write(raw)
	w.Close()
	for _, compress := range []bool{false, true} {
		body := raw
		if compress {
			body = compressed.Bytes()
		}
		header := make([]byte, 16)
		binary.LittleEndian.PutUint16(header[12:], uint16(len(body)))
		binary.LittleEndian.PutUint16(header[14:], uint16(len(raw)))
		packet := append(header, body...)
		e := &exchangeIO{in: fragmented{bytes.NewReader(packet)}}
		got, err := request(e, []byte{4, 5, 6})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, raw) || !bytes.Equal(e.out.Bytes(), []byte{4, 5, 6}) {
			t.Fatal("partial read/write lost bytes")
		}
		if compress {
			binary.LittleEndian.PutUint16(packet[14:], 1)
			if _, err := request(&exchangeIO{in: bytes.NewReader(packet)}, nil); err == nil {
				t.Fatal("oversize decompression accepted")
			}
		}
	}
}
func FuzzParsers(f *testing.F) {
	f.Add(quotePacket())
	f.Add([]byte{1, 0, 0})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) { parseQuotes(b, "2026-09-07"); parseDaily(b) })
}
