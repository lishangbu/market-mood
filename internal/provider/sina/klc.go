package sina

import (
	"encoding/json"
	"fmt"
	"market-mood/internal/market"
	"math"
	"strings"
	"time"
)

const maxPayload = 4 << 20
const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

type bits struct {
	values []byte
	pos    int
	err    error
}

func newBits(payload string) (*bits, error) {
	if len(payload) == 0 || len(payload) > maxPayload {
		return nil, fmt.Errorf("KLC 数据长度无效")
	}
	b := &bits{values: make([]byte, len(payload))}
	for i := range payload {
		v := strings.IndexByte(alphabet, payload[i])
		if v < 0 {
			return nil, fmt.Errorf("KLC 非法字符")
		}
		b.values[i] = byte(v)
	}
	return b, nil
}
func (b *bits) read(width int, signed bool) int64 {
	if b.err != nil {
		return 0
	}
	if width < 0 || width > 53 || b.pos+width > len(b.values)*6 {
		b.err = fmt.Errorf("KLC 位宽或剩余数据无效")
		return 0
	}
	var value int64
	for i := 0; i < width; i++ {
		value |= int64((b.values[b.pos/6]>>uint(b.pos%6))&1) << uint(i)
		b.pos++
	}
	if signed && width > 0 && value&(int64(1)<<uint(width-1)) != 0 {
		value -= int64(1) << uint(width)
	}
	return value
}
func (b *bits) flag() bool { return b.read(1, false) != 0 }
func (b *bits) delta() int {
	sign := b.flag()
	n := 1
	for b.flag() {
		n++
		if n > 64 {
			b.err = fmt.Errorf("KLC 长度增量无效")
			return 0
		}
	}
	if sign {
		return n
	}
	return -n
}

// precision factors encode powers of two and five, in the same order as the
// reference KLC decoder. Round-to-even is required when precision decreases.
func precision(p int) [2]int {
	if p < 0 {
		x := precision(-p)
		return [2]int{-x[0], -x[1]}
	}
	q, r := p/3, p%3
	x := [2]int{q, q}
	if r > 0 {
		x[r-1]++
	}
	return x
}
func convert(v float64, from [2]int, to int) float64 {
	target := precision(to)
	a, c := target[0]-from[0], target[1]-from[1]
	if a > c {
		v *= math.Pow(2, float64(a-c))
	} else if c > a {
		v *= math.Pow(5, float64(c-a))
	}
	scale := int(math.Min(float64(a), float64(c)))
	if scale < 0 {
		return math.RoundToEven(v / math.Pow10(-scale))
	}
	return v * math.Pow10(scale)
}
func number(v float64, p int) float64 {
	x := precision(p)
	diff := x[0] - x[1]
	if diff > 0 {
		v *= math.Pow(5, float64(diff))
	} else if diff < 0 {
		v *= math.Pow(2, float64(-diff))
	}
	return v * math.Pow10(-int(math.Max(float64(x[0]), float64(x[1]))))
}

// Decode implements KLC2 daily format 3466/v0 in native Go. The only accepted
// envelope is a JSON string assigned to the requested security; it never runs JS.
// Algorithm reference: AKShare akshare/stock/cons.py hk_js_decode (MIT).
func Decode(body, sym string) ([]market.Bar, error) {
	if len(sym) != 8 || (sym[:2] != "sh" && sym[:2] != "sz") {
		return nil, fmt.Errorf("KLC 证券代码无效")
	}
	for _, c := range sym[2:] {
		if c < '0' || c > '9' {
			return nil, fmt.Errorf("KLC 证券代码无效")
		}
	}
	if len(body) > maxPayload+1024 {
		return nil, fmt.Errorf("KLC 响应超限")
	}
	prefix := "var KLC_K2_" + sym + "="
	raw := strings.TrimSpace(body)
	// Sina may append an opaque base64 block comment after the assignment.
	// Accept only that bounded inert suffix; reject additional statements.
	if at := strings.Index(raw, ";"); at >= 0 {
		tail := strings.TrimSpace(raw[at+1:])
		if strings.HasPrefix(tail, "/*") && strings.HasSuffix(tail, "*/") {
			comment := strings.TrimSpace(tail[2 : len(tail)-2])
			valid := true
			for _, c := range comment {
				if c != ' ' && c != '\n' && c != '\r' && c != '\t' && !strings.ContainsRune(alphabet, c) {
					valid = false
					break
				}
			}
			if valid {
				raw = raw[:at+1]
			}
		}
	}
	if !strings.HasPrefix(raw, prefix) || !strings.HasSuffix(raw, ";") {
		return nil, fmt.Errorf("KLC 响应身份或格式无效")
	}
	var payload string
	if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(raw, prefix), ";"))), &payload); err != nil {
		return nil, fmt.Errorf("KLC 字符串: %w", err)
	}
	b, err := newBits(payload)
	if err != nil {
		return nil, err
	}
	if b.read(12, false) != 3466 || b.read(6, false) != 63 {
		return nil, fmt.Errorf("KLC 格式或版本不支持")
	}
	p := map[byte]int{'p': 6, 'v': 0, 'a': 0, 'e': 0, 't': 0}
	widths := map[byte]int{'o': 3, 'h': 3, 'l': 3, 'c': 3, 'v': 5, 'a': 5, 'e': 3, 't': 0}
	accumulated := map[byte]float64{'p': 0, 'v': 0, 'a': 0, 'o': 0, 'h': 0, 'l': 0, 'c': 0, 'e': 0, 't': 0}
	average, separate, post, postExtra := true, false, false, false
	weekday, day := int64(62), int64(0)
	rows := make([]market.Bar, 0, 7000)
	dateStep := func() int {
		d := int(b.read(3, false))
		if d == 1 {
			day = b.read(18, true)
			return 0
		}
		if d == 0 {
			return int(b.read(6, false))
		}
		return d
	}
	for len(rows) < 30000 {
		if b.err != nil {
			return nil, b.err
		}
		if b.pos >= len(b.values)*6 {
			return nil, fmt.Errorf("KLC 终止标记缺失")
		}
		step, controls, previous := 1, 0, float64(0)
		if b.flag() {
			if b.flag() {
				if b.flag() {
					controls++
					oldAverage := average
					if b.flag() {
						if b.flag() {
							average = !average
						}
						if b.flag() {
							post = !post
						}
						if b.flag() {
							postExtra = !postExtra
						}
						oldSeparate := separate
						if b.flag() {
							separate = !separate
						}
						if b.flag() {
							weekday = b.read(7, false)
							if weekday == 0 {
								b.err = fmt.Errorf("KLC 交易周掩码为空")
							}
						}
						if oldSeparate != separate {
							if oldSeparate {
								accumulated['p'] = accumulated['c']
							} else {
								for _, k := range "ohlc" {
									accumulated[byte(k)] = accumulated['p']
								}
							}
						}
					}
					count := 3
					if post {
						count = 5
					}
					for i := 0; i < count; i++ {
						if b.flag() {
							k := "pvaet"[i]
							old := p[k]
							p[k] += b.delta()
							if p[k] < -60 || p[k] > 60 {
								b.err = fmt.Errorf("KLC 精度超限")
								break
							}
							accumulated[k] = convert(accumulated[k], precision(old), p[k])
							if separate && i == 0 {
								for _, c := range "ohlc" {
									accumulated[byte(c)] = convert(accumulated[byte(c)], precision(old), p['p'])
								}
							}
						}
					}
					if !average && oldAverage {
						amount := 0.
						if len(rows) > 0 {
							amount = *rows[len(rows)-1].Amount
						}
						accumulated['a'] = convert(amount, [2]int{}, p['a'])
					}
				}
				if b.flag() {
					controls++
					count := 7
					if post {
						count++
					}
					if postExtra {
						count++
					}
					for i := 0; i < count; i++ {
						if b.flag() {
							if i == 6 {
								step = dateStep()
							} else {
								k := "ohlcva*et"[i]
								widths[k] += b.delta()
							}
						}
					}
				}
				if b.flag() {
					controls++
					width := widths['o']
					if b.flag() {
						width += b.delta()
					}
					value := float64(b.read(3*width, true))
					if separate {
						previous = accumulated['c'] + value
					} else {
						accumulated['p'] += value
						previous = accumulated['p']
					}
				}
				if controls == 0 {
					if b.err != nil {
						return nil, b.err
					}
					if err := market.ValidateBars(rows); err != nil {
						return nil, err
					}
					return rows, nil
				}
			} else {
				if b.flag() {
					if b.flag() {
						if b.flag() {
							step = dateStep()
						} else {
							widths['v'] += b.delta()
						}
					} else {
						if post && b.flag() {
							k := byte('e')
							if postExtra && b.flag() {
								k = 't'
							}
							widths[k] += b.delta()
						} else {
							widths['a'] += b.delta()
						}
					}
				} else {
					k := "ohlc"[b.read(2, false)]
					widths[k] += b.delta()
				}
			}
		}
		values := map[byte]float64{}
		count := 6
		if post {
			count++
		}
		if postExtra {
			count++
		}
		mask := 185
		if separate {
			mask = 191
		}
		for i := 0; i < count; i++ {
			k := "ohlcvaet"[i]
			values[k] = float64(b.read(3*widths[k], (mask>>uint(i))&1 != 0))
		}
		if b.err != nil {
			return nil, b.err
		}
		if step < 0 || step > 63 {
			return nil, fmt.Errorf("KLC 日期增量无效")
		}
		for i := 0; i < step; i++ {
			for j := 0; ; j++ {
				day++
				if j >= 7 {
					return nil, fmt.Errorf("KLC 交易周无有效日期")
				}
				if weekday&(1<<uint((day%7+10)%7)) != 0 {
					break
				}
			}
		}
		if day < -7657 || day > 50000 {
			return nil, fmt.Errorf("KLC 日期范围无效")
		}
		row := market.Bar{Day: time.Unix((7657+day)*86400, 0).UTC().Format(time.DateOnly)}
		if previous != 0 {
			row.Prevclose = market.Ptr(number(previous, p['p']))
		}
		averagePrice := 0.
		if separate {
			prices := [4]*float64{&row.Open, &row.High, &row.Low, &row.Close}
			for i, k := range "ohlc" {
				accumulated[byte(k)] += values[byte(k)]
				*prices[i] = number(accumulated[byte(k)], p['p'])
				averagePrice += accumulated[byte(k)] / 4
			}
		} else {
			open := accumulated['p'] + values['o']
			row.Open = number(open, p['p'])
			row.High = number(open+values['h'], p['p'])
			row.Low = number(open-values['l'], p['p'])
			accumulated['p'] = open + values['c']
			row.Close = number(accumulated['p'], p['p'])
			averagePrice = open + (values['h']-values['l']+values['c'])/4
		}
		accumulated['v'] += values['v']
		row.Volume = number(accumulated['v'], p['v'])
		amount := 0.
		if average {
			pricePrecision, volumePrecision := precision(p['p']), precision(p['v'])
			both := [2]int{pricePrecision[0] + volumePrecision[0], pricePrecision[1] + volumePrecision[1]}
			amount = number(convert(math.Floor(averagePrice*accumulated['v']+.5), both, p['a'])+values['a'], p['a'])
		} else {
			accumulated['a'] += values['a']
			amount = number(accumulated['a'], p['a'])
		}
		row.Amount = market.Ptr(amount)
		rows = append(rows, row)
	}
	return nil, fmt.Errorf("KLC 行数超限")
}
