package healthcheck

import (
	"net"
	"testing"
	"time"
)

// startUDPResponder starts a UDP server that listens on 127.0.0.1:0 and responds
// to every packet with at least 12 bytes where the first two bytes mirror the
// request ID (bytes 0 and 1). It returns the address string and a shutdown
// function.
func startUDPResponder(t *testing.T) (string, func()) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start UDP server: %v", err)
	}

	done := make(chan struct{})
	go func() {
		buf := make([]byte, 512)
		for {
			select {
			case <-done:
				return
			default:
			}
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return // likely closed
			}
			if n < 2 {
				continue
			}
			// Build minimal DNS response: copy ID, pad to 12 bytes.
			resp := make([]byte, 12)
			resp[0] = buf[0]
			resp[1] = buf[1]
			conn.WriteTo(resp, addr) // ignore error
		}
	}()

	shutdown := func() {
		close(done)
		conn.Close()
	}

	return conn.LocalAddr().String(), shutdown
}

func TestDNSHealthChecker_CheckOnce_Success(t *testing.T) {
	addr, shutdown := startUDPResponder(t)
	defer shutdown()

	checker := NewDNSHealthChecker(addr, 1*time.Second, 1, 0)
	ok, _, err := checker.CheckOnce()
	if !ok || err != nil {
		t.Fatalf("expected successful health check, got ok=%v err=%v", ok, err)
	}
}

func TestDNSHealthChecker_CheckOnce_Failure(t *testing.T) {
	// Use an address where nothing is listening.
	checker := NewDNSHealthChecker("127.0.0.1:65534", 100*time.Millisecond, 1, 0)
	ok, _, err := checker.CheckOnce()
	if ok || err == nil {
		t.Fatalf("expected health check failure, got ok=%v err=%v", ok, err)
	}
}

func TestDNSHealthChecker_WaitHealthy(t *testing.T) {
	// Test WaitHealthy with a server that starts responding after some attempts
	addr, shutdown := startUDPResponder(t)
	defer shutdown()

	// Use a checker with enough retries and short delays for a quick test
	checker := NewDNSHealthChecker(addr, 200*time.Millisecond, 5, 50*time.Millisecond)

	// This should succeed quickly since the server is already running
	success := checker.WaitHealthy()
	if !success {
		t.Fatalf("WaitHealthy should succeed with a running server")
	}
}

func TestDNSHealthChecker_WaitHealthy_Failure(t *testing.T) {
	// Test WaitHealthy failure when server never responds
	checker := NewDNSHealthChecker("127.0.0.1:65534", 50*time.Millisecond, 2, 10*time.Millisecond)

	success := checker.WaitHealthy()
	if success {
		t.Fatalf("WaitHealthy should fail when server is not reachable")
	}
}

// fakeChecker is a simple implementation of Checker for testing Aggregate.
type fakeChecker struct {
	name string
	ok   bool
}

func (f *fakeChecker) Name() string                            { return f.name }
func (f *fakeChecker) CheckOnce() (bool, time.Duration, error) { return f.ok, 0, nil }
func (f *fakeChecker) WaitHealthy() bool                       { return f.ok }

func TestAggregate(t *testing.T) {
	results, all := Aggregate(&fakeChecker{"a", true}, &fakeChecker{"b", false})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if all {
		t.Fatalf("expected overall health to be false when at least one checker fails")
	}
	if !results["a"].Healthy || results["b"].Healthy {
		t.Fatalf("unexpected result health status %+v", results)
	}
}
