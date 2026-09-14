package irc

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func successfulTestConn() net.Conn {
	client, peer := net.Pipe()
	_ = peer.Close()
	return client
}

func TestIPv6FirstDialerPrefersAAAARegardlessOfResolverOrder(t *testing.T) {
	var calls []string
	d := &ipv6FirstDialer{
		lookupIP: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("192.0.2.1")}, {IP: net.ParseIP("2001:db8::1")}}, nil
		},
		dialContext: func(_ context.Context, network, address string) (net.Conn, error) {
			calls = append(calls, network+" "+address)
			return successfulTestConn(), nil
		},
		timeout:       time.Second,
		fallbackDelay: time.Second,
	}
	conn, err := d.Dial("tcp", "irc.example:6697")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if len(calls) != 1 || !strings.HasPrefix(calls[0], "tcp6 ") {
		t.Fatalf("dial calls = %#v, want one IPv6 attempt", calls)
	}
}

func TestIPv6FirstDialerFallsBackWithoutWaitingForIPv6Timeout(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	d := &ipv6FirstDialer{
		lookupIP: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("2001:db8::1")}, {IP: net.ParseIP("192.0.2.1")}}, nil
		},
		dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			mu.Lock()
			calls = append(calls, network+" "+address)
			mu.Unlock()
			if network == "tcp6" {
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return successfulTestConn(), nil
		},
		timeout:       time.Second,
		fallbackDelay: 10 * time.Millisecond,
	}
	started := time.Now()
	conn, err := d.Dial("tcp", "irc.example:6697")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("IPv4 fallback took %s", elapsed)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 || !strings.HasPrefix(calls[0], "tcp6 ") || !strings.HasPrefix(calls[1], "tcp4 ") {
		t.Fatalf("dial calls = %#v, want IPv6 then IPv4", calls)
	}
}

func TestIPv6FailureStartsIPv4Immediately(t *testing.T) {
	d := &ipv6FirstDialer{
		lookupIP: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("2001:db8::1")}, {IP: net.ParseIP("192.0.2.1")}}, nil
		},
		dialContext: func(_ context.Context, network, _ string) (net.Conn, error) {
			if network == "tcp6" {
				return nil, errors.New("IPv6 unreachable")
			}
			return successfulTestConn(), nil
		},
		timeout:       time.Second,
		fallbackDelay: time.Second,
	}
	started := time.Now()
	conn, err := d.Dial("tcp", "irc.example:6697")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("immediate IPv4 fallback took %s", elapsed)
	}
}

func TestIPv4OnlyHostDoesNotWaitForFallbackTimer(t *testing.T) {
	var networkUsed string
	d := &ipv6FirstDialer{
		lookupIP: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("192.0.2.1")}}, nil
		},
		dialContext: func(_ context.Context, network, _ string) (net.Conn, error) {
			networkUsed = network
			return successfulTestConn(), nil
		},
		timeout:       time.Second,
		fallbackDelay: time.Second,
	}
	conn, err := d.Dial("tcp", "irc.example:6697")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if networkUsed != "tcp4" {
		t.Fatalf("network = %q, want tcp4", networkUsed)
	}
}

func TestLiteralIPv6AddressBypassesDNS(t *testing.T) {
	lookupCalled := false
	var networkUsed string
	d := &ipv6FirstDialer{
		lookupIP: func(context.Context, string) ([]net.IPAddr, error) {
			lookupCalled = true
			return nil, nil
		},
		dialContext: func(_ context.Context, network, _ string) (net.Conn, error) {
			networkUsed = network
			return successfulTestConn(), nil
		},
		timeout: time.Second,
	}
	conn, err := d.Dial("tcp", "[2001:db8::1]:6697")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if lookupCalled || networkUsed != "tcp6" {
		t.Fatalf("lookup=%v network=%q, want no lookup and tcp6", lookupCalled, networkUsed)
	}
}
