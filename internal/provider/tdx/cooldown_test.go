package tdx

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// Exercise the real TCP/handshake/parser seam, including remote malformed data.
func dailyServer(t *testing.T, body []byte) (string, *atomic.Int32) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	count := new(atomic.Int32)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			count.Add(1)
			func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(time.Second))
				for i, size := range []int{13, 13, 42, 38} {
					if _, err := io.ReadFull(conn, make([]byte, size)); err != nil {
						return
					}
					response := []byte{1}
					if i == 3 {
						response = body
					}
					header := make([]byte, 16)
					binary.LittleEndian.PutUint16(header[12:], uint16(len(response)))
					binary.LittleEndian.PutUint16(header[14:], uint16(len(response)))
					if _, err := conn.Write(append(header, response...)); err != nil {
						return
					}
				}
			}()
		}
	}()
	t.Cleanup(func() { listener.Close(); <-done })
	return listener.Addr().String(), count
}

func TestMalformedHostIsNotRetriedForEverySecurity(t *testing.T) {
	host, calls := dailyServer(t, []byte{0x20, 3})
	c := New()
	c.Hosts = []string{host}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, code := range []string{"600000", "000001", "600001"} {
		if _, err := c.Daily(ctx, code); err == nil {
			t.Fatal("malformed response accepted")
		}
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("invalid host contacted %d times; want one then cooldown", n)
	}
}

func validDailyBody() []byte {
	body := []byte{1, 0}
	body = binary.LittleEndian.AppendUint32(body, 20261009)
	for _, n := range []int64{9700, -160, 40, -160} {
		body = append(body, encodePrice(n)...)
	}
	body = binary.LittleEndian.AppendUint32(body, 0)
	return binary.LittleEndian.AppendUint32(body, 0)
}
func TestCooldownKeepsFallbackAndRetriesAfterExpiry(t *testing.T) {
	bad, badCalls := dailyServer(t, []byte{0x20, 3})
	good, goodCalls := dailyServer(t, validDailyBody())
	c := New()
	c.Hosts = []string{bad, good}
	now := time.Now()
	c.now = func() time.Time { return now }
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for range 2 {
		rows, err := c.Daily(ctx, "600000")
		if err != nil || len(rows) != 1 || rows[0].Close != 9.54 {
			t.Fatalf("fallback lost: %+v %v", rows, err)
		}
	}
	if badCalls.Load() != 1 || goodCalls.Load() != 2 {
		t.Fatal("wrong host selection", badCalls.Load(), goodCalls.Load())
	}
	now = now.Add(time.Minute)
	if _, err := c.Daily(ctx, "600000"); err != nil {
		t.Fatal(err)
	}
	if badCalls.Load() != 2 {
		t.Fatal("unhealthy endpoint never retried")
	}
}
func TestCallerCancellationDoesNotQuarantineHost(t *testing.T) {
	host, calls := dailyServer(t, validDailyBody())
	c := New()
	c.Hosts = []string{host}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Daily(ctx, "600000"); err == nil {
		t.Fatal("cancel ignored")
	}
	if _, err := c.Daily(context.Background(), "600000"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("canceled caller made a connection")
	}
}
