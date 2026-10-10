package tdx

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"market-mood/internal/market"
	"net"
	"strings"
	"sync"
	"time"
)

// Client shares no connections between calls. Each host attempt uses one deadline
// for dialing, handshake and all batches. A failed host never contributes partial rows.
type Client struct {
	Hosts    []string
	Dialer   net.Dialer
	mu       sync.Mutex
	failures map[string]hostFailure
	now      func() time.Time
}
type hostFailure struct {
	until time.Time
	err   error
}

// A broken endpoint must not be retried for every stock in a full-market rebuild.
// Failures are local to this client, retain the reason, and are retried after a minute.
func (c *Client) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}
func (c *Client) cooling(host string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	f, ok := c.failures[host]
	if ok && c.clock().Before(f.until) {
		return fmt.Errorf("节点暂不可用，稍后重试: %w", f.err)
	}
	delete(c.failures, host)
	return nil
}
func (c *Client) failed(ctx context.Context, host string, err error) {
	if ctx.Err() != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failures == nil {
		c.failures = map[string]hostFailure{}
	}
	c.failures[host] = hostFailure{c.clock().Add(time.Minute), err}
}

func New() *Client { return &Client{Hosts: []string{"115.238.90.165:7709", "180.153.18.170:7709"}} }
func (c *Client) connect(ctx context.Context, host string) (net.Conn, func(), error) {
	conn, err := c.Dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, nil, err
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	cleanup := func() { stop(); conn.Close() }
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			cleanup()
			return nil, nil, err
		}
	}
	for _, raw := range []string{"0c0218930001030003000d0001", "0c0218940001030003000d0002", "0c031899000120002000db0fd5d0c9ccd6a4a8af0000008fc22540130000d500c9ccbdf0d7ea00000002"} {
		packet, err := hex.DecodeString(raw)
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		if _, err := request(conn, packet); err != nil {
			cleanup()
			return nil, nil, err
		}
	}
	return conn, cleanup, nil
}
func dailyPacket(code string, ex byte, count uint16) []byte {
	p := []byte{0x0c, 1, 8, 0x64, 1, 1, 0x1c, 0, 0x1c, 0, 0x2d, 5}
	p = binary.LittleEndian.AppendUint16(p, uint16(ex))
	p = append(p, code...)
	for _, n := range []uint16{9, 1, 0, count} {
		p = binary.LittleEndian.AppendUint16(p, n)
	}
	return append(p, make([]byte, 10)...)
}
func (c *Client) Daily(ctx context.Context, code string) ([]market.Bar, error) {
	if !validCode(code) {
		return nil, fmt.Errorf("股票代码异常")
	}
	var failures []error
	for _, host := range c.Hosts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := c.cooling(host); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", host, err))
			continue
		}
		rows, err := func() ([]market.Bar, error) {
			attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			conn, close, err := c.connect(attempt, host)
			if err != nil {
				return nil, err
			}
			defer close()
			body, err := request(conn, dailyPacket(code, exchange(code), 10))
			if err != nil {
				return nil, err
			}
			return parseDaily(body)
		}()
		if err == nil {
			return rows, nil
		}
		c.failed(ctx, host, err)
		failures = append(failures, fmt.Errorf("%s: %w", host, err))
	}
	return nil, fmt.Errorf("通达信日线不可用: %w", errors.Join(failures...))
}
func (c *Client) Quotes(ctx context.Context, codes []string) ([]market.Stock, error) {
	if len(codes) == 0 {
		return nil, fmt.Errorf("报价代码为空")
	}
	expected := map[string]bool{}
	for _, code := range codes {
		if !validCode(code) || strings.HasPrefix(code, "92") || expected[code] {
			return nil, fmt.Errorf("通达信仅支持不重复的沪深报价代码")
		}
		expected[code] = true
	}
	var failures []error
	for _, host := range c.Hosts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := c.cooling(host); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", host, err))
			continue
		}
		stocks, err := func() ([]market.Stock, error) {
			attempt, cancel := context.WithTimeout(ctx, 25*time.Second)
			defer cancel()
			conn, close, err := c.connect(attempt, host)
			if err != nil {
				return nil, err
			}
			defer close()
			body, err := request(conn, dailyPacket("000001", 1, 1))
			if err != nil {
				return nil, err
			}
			bars, err := parseDaily(body)
			if err != nil {
				return nil, err
			}
			day := bars[len(bars)-1].Day
			stocks := make([]market.Stock, 0, len(codes))
			seen := map[string]bool{}
			for start := 0; start < len(codes); start += 80 {
				chunk := codes[start:min(start+80, len(codes))]
				size := uint16(len(chunk)*7 + 12)
				p := []byte{0x0c, 1, 0x20, 0x63, 0, 2}
				p = binary.LittleEndian.AppendUint16(p, size)
				p = binary.LittleEndian.AppendUint16(p, size)
				p = binary.LittleEndian.AppendUint32(p, 0x0005053e)
				p = binary.LittleEndian.AppendUint32(p, 0)
				p = binary.LittleEndian.AppendUint16(p, 0)
				p = binary.LittleEndian.AppendUint16(p, uint16(len(chunk)))
				for _, code := range chunk {
					p = append(p, exchange(code))
					p = append(p, code...)
				}
				body, err := request(conn, p)
				if err != nil {
					return nil, err
				}
				rows, err := parseQuotes(body, day)
				if err != nil {
					return nil, err
				}
				for _, s := range rows {
					if !expected[s.Code] || seen[s.Code] {
						return nil, fmt.Errorf("通达信报价身份不完整或重复")
					}
					seen[s.Code] = true
					stocks = append(stocks, s)
				}
			}
			if len(stocks) != len(codes) {
				return nil, fmt.Errorf("通达信报价缺股")
			}
			return stocks, nil
		}()
		if err == nil {
			return stocks, nil
		}
		c.failed(ctx, host, err)
		failures = append(failures, fmt.Errorf("%s: %w", host, err))
	}
	return nil, fmt.Errorf("通达信报价不可用: %w", errors.Join(failures...))
}
