// Package tdx implements the TongDaXin wire protocol without a C runtime.
package tdx

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"market-mood/internal/market"
	"math"
	"time"
)

// Response lengths are uint16. Bound decompression by the declared output size.
func request(conn io.ReadWriter, packet []byte) ([]byte, error) {
	if _, err := io.Copy(conn, bytes.NewReader(packet)); err != nil {
		return nil, err
	}
	var header [16]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return nil, err
	}
	size, raw := int(binary.LittleEndian.Uint16(header[12:])), int(binary.LittleEndian.Uint16(header[14:]))
	if size == 0 || raw == 0 {
		return nil, fmt.Errorf("通达信空响应")
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, err
	}
	if size == raw {
		return body, nil
	}
	reader, err := zlib.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	out, err := io.ReadAll(io.LimitReader(reader, int64(raw)+1))
	if err != nil {
		return nil, err
	}
	if len(out) != raw {
		return nil, fmt.Errorf("通达信解压长度异常: got %d want %d", len(out), raw)
	}
	return out, nil
}

// decoder accumulates the first error, so dependent reads cannot overrun a packet.
// First price byte has six magnitude bits, a sign and a continuation bit.
type decoder struct {
	body []byte
	pos  int
	err  error
}

func (d *decoder) take(n int) []byte {
	if d.err != nil {
		return make([]byte, n)
	}
	if n > len(d.body)-d.pos {
		d.err = io.ErrUnexpectedEOF
		return make([]byte, n)
	}
	b := d.body[d.pos : d.pos+n]
	d.pos += n
	return b
}
func (d *decoder) u32() uint32 { return binary.LittleEndian.Uint32(d.take(4)) }
func (d *decoder) price() int64 {
	first := d.take(1)[0]
	b := first
	n := int64(first & 63)
	shift := uint(6)
	for b&128 != 0 && d.err == nil {
		if shift >= 35 {
			d.err = fmt.Errorf("价格编码异常")
			return 0
		}
		b = d.take(1)[0]
		n += int64(b&127) << shift
		shift += 7
	}
	if first&64 != 0 {
		return -n
	}
	return n
}
func volume(v uint32) float64 {
	if v == 0 {
		return 0
	}
	log := int(v >> 24)
	h, mid, low := float64(v>>16&255), float64(v>>8&255), float64(v&255)
	e := log*2 - 134
	high := 0.
	if h > 128 {
		high = math.Ldexp(128, e) + math.Ldexp(h-128, e+1)
	} else {
		high = math.Ldexp(h, int(math.Abs(float64(e))))
	}
	scale := 1.
	if uint32(h)&128 != 0 {
		scale = 2
	}
	return math.Ldexp(1, log*2-127) + high + (math.Ldexp(mid, log*2-142)+math.Ldexp(low, log*2-150))*scale
}
func parseDaily(body []byte) ([]market.Bar, error) {
	d := decoder{body: body}
	count := int(binary.LittleEndian.Uint16(d.take(2)))
	if count > 10 {
		return nil, fmt.Errorf("日线数量异常: %d (响应 %d 字节)", count, len(body))
	}
	rows := make([]market.Bar, 0, count)
	base := int64(0)
	for range count {
		date := d.u32()
		open := base + d.price()
		close := open + d.price()
		high := open + d.price()
		low := open + d.price()
		v, a := volume(d.u32()), volume(d.u32())
		base = close
		rows = append(rows, market.Bar{Day: fmt.Sprintf("%04d-%02d-%02d", date/10000, date/100%100, date%100), Open: float64(open) / 1000, Close: float64(close) / 1000, High: float64(high) / 1000, Low: float64(low) / 1000, Volume: v, Amount: market.Ptr(a)})
	}
	if d.err != nil {
		return nil, fmt.Errorf("日线字段截断: %w", d.err)
	}
	if d.pos != len(body) {
		return nil, fmt.Errorf("日线包存在未解析尾部")
	}
	return rows, market.ValidateBars(rows)
}
func quoteTime(raw int64) (string, error) {
	if raw < 0 {
		return "", fmt.Errorf("行情时间为负")
	}
	hour, tail := raw/1000000, raw%1000000
	minute, second := tail/10000, tail%10000*60/10000
	if minute >= 60 {
		minute, second = tail*60/1000000, (tail*60%1000000)*60/1000000
	}
	if hour >= 24 || minute >= 60 || second >= 60 {
		return "", fmt.Errorf("行情时间异常")
	}
	return fmt.Sprintf("%02d:%02d:%02d", hour, minute, second), nil
}
func validCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
func exchange(code string) byte {
	if code[0] == '6' {
		return 1
	}
	return 0
}
func parseQuotes(body []byte, day string) ([]market.Stock, error) {
	if _, err := time.Parse(time.DateOnly, day); err != nil {
		return nil, err
	}
	d := decoder{body: body}
	head := d.take(4)
	count := int(binary.LittleEndian.Uint16(head[2:]))
	if count > 80 {
		return nil, fmt.Errorf("报价数量异常")
	}
	stocks := make([]market.Stock, 0, count)
	for range count {
		identity := d.take(9)
		code := string(identity[1:7])
		if !validCode(code) || identity[0] != exchange(code) {
			return nil, fmt.Errorf("报价代码或市场不符")
		}
		last := d.price()
		previous := last + d.price()
		open := last + d.price()
		high := last + d.price()
		low := last + d.price()
		clock, err := quoteTime(d.price())
		if err != nil {
			return nil, err
		}
		d.price()
		totalVolume := d.price()
		d.price()
		amount := volume(d.u32())
		for range 4 {
			d.price()
		}
		bids, asks := make([]market.BookLevel, 0, 5), make([]market.BookLevel, 0, 5)
		for range 5 {
			bid, ask := last+d.price(), last+d.price()
			bidLots, askLots := d.price(), d.price()
			if bid < 0 || ask < 0 || bidLots < 0 || askLots < 0 {
				return nil, fmt.Errorf("盘口字段异常")
			}
			bids = append(bids, market.BookLevel{Price: float64(bid) / 100, Shares: float64(bidLots) * 100})
			asks = append(asks, market.BookLevel{Price: float64(ask) / 100, Shares: float64(askLots) * 100})
		}
		d.take(2)
		for range 4 {
			d.price()
		}
		d.take(4)
		if d.err != nil {
			return nil, fmt.Errorf("报价字段截断: %w", d.err)
		}
		if last < 0 || previous < 0 || open < 0 || low < 0 || high < low || totalVolume < 0 || math.IsInf(amount, 0) || math.IsNaN(amount) || amount < 0 {
			return nil, fmt.Errorf("报价数值异常")
		}
		if last > 0 && totalVolume > 0 && (last > high || last < low) {
			return nil, fmt.Errorf("现价超出日内范围")
		}
		s := market.Stock{Code: code, Price: market.Ptr(float64(last) / 100), Open: market.Ptr(float64(open) / 100), PreviousClose: market.Ptr(float64(previous) / 100), Amount: market.Ptr(amount), SourceTime: market.Ptr(day + "T" + clock + "+08:00"), Bids: bids, Asks: asks, FallbackReasons: []string{}}
		if previous > 0 {
			s.Change = market.Ptr((float64(last)/float64(previous) - 1) * 100)
		}
		stocks = append(stocks, s)
	}
	if d.err != nil {
		return nil, d.err
	}
	if d.pos != len(body) {
		return nil, fmt.Errorf("报价包存在未解析尾部")
	}
	return stocks, nil
}
