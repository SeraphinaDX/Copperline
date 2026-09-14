package irc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"
)

const (
	ircDialTimeout    = 5 * time.Second
	ipv4FallbackDelay = 250 * time.Millisecond
)

type lookupIPFunc func(context.Context, string) ([]net.IPAddr, error)
type dialContextFunc func(context.Context, string, string) (net.Conn, error)

// ipv6FirstDialer implements Happy Eyeballs with an explicit IPv6 preference.
// Go's default dialer races address families, but its preferred family follows
// resolver ordering and therefore does not guarantee an IPv6-first attempt.
type ipv6FirstDialer struct {
	lookupIP      lookupIPFunc
	dialContext   dialContextFunc
	timeout       time.Duration
	fallbackDelay time.Duration
}

func newIPv6FirstDialer() *ipv6FirstDialer {
	netDialer := &net.Dialer{}
	return &ipv6FirstDialer{
		lookupIP:      net.DefaultResolver.LookupIPAddr,
		dialContext:   netDialer.DialContext,
		timeout:       ircDialTimeout,
		fallbackDelay: ipv4FallbackDelay,
	}
}

func (d *ipv6FirstDialer) Dial(network, address string) (net.Conn, error) {
	if network != "tcp" {
		ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
		defer cancel()
		return d.dialContext(ctx, network, address)
	}

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("parse IRC server address %q: %w", address, err)
	}
	if ip, parseErr := netip.ParseAddr(host); parseErr == nil {
		family := "tcp6"
		if ip.Is4() {
			family = "tcp4"
		}
		ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
		defer cancel()
		return d.dialContext(ctx, family, address)
	}

	ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
	defer cancel()
	resolved, err := d.lookupIP(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve IRC server %q: %w", host, err)
	}
	var ipv6, ipv4 []string
	for _, candidate := range resolved {
		if candidate.IP == nil {
			continue
		}
		endpoint := net.JoinHostPort(candidate.String(), port)
		if candidate.IP.To4() == nil {
			ipv6 = append(ipv6, endpoint)
		} else {
			ipv4 = append(ipv4, endpoint)
		}
	}
	if len(ipv6) == 0 && len(ipv4) == 0 {
		return nil, fmt.Errorf("IRC server %q resolved without usable IP addresses", host)
	}
	if len(ipv6) == 0 {
		return d.dialAddresses(ctx, "tcp4", ipv4)
	}
	if len(ipv4) == 0 {
		return d.dialAddresses(ctx, "tcp6", ipv6)
	}
	return d.raceFamilies(ctx, ipv6, ipv4)
}

type dialResult struct {
	conn net.Conn
	err  error
}

func (d *ipv6FirstDialer) raceFamilies(ctx context.Context, ipv6, ipv4 []string) (net.Conn, error) {
	results := make(chan dialResult)
	start := func(network string, addresses []string) {
		go func() {
			conn, err := d.dialAddresses(ctx, network, addresses)
			select {
			case results <- dialResult{conn: conn, err: err}:
			case <-ctx.Done():
				if conn != nil {
					_ = conn.Close()
				}
			}
		}()
	}

	start("tcp6", ipv6)
	pending := 1
	ipv4Started := false
	timer := time.NewTimer(d.fallbackDelay)
	defer timer.Stop()
	var dialErrors []error

	for pending > 0 {
		select {
		case result := <-results:
			pending--
			if result.err == nil {
				return result.conn, nil
			}
			dialErrors = append(dialErrors, result.err)
			if !ipv4Started {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				start("tcp4", ipv4)
				pending++
				ipv4Started = true
			}
		case <-timer.C:
			if !ipv4Started {
				start("tcp4", ipv4)
				pending++
				ipv4Started = true
			}
		case <-ctx.Done():
			dialErrors = append(dialErrors, ctx.Err())
			return nil, fmt.Errorf("connect to IRC server: %w", errors.Join(dialErrors...))
		}
	}
	return nil, fmt.Errorf("connect to IRC server: %w", errors.Join(dialErrors...))
}

func (d *ipv6FirstDialer) dialAddresses(ctx context.Context, network string, addresses []string) (net.Conn, error) {
	var dialErrors []error
	for _, address := range addresses {
		conn, err := d.dialContext(ctx, network, address)
		if err == nil {
			return conn, nil
		}
		dialErrors = append(dialErrors, fmt.Errorf("%s %s: %w", network, address, err))
		if ctx.Err() != nil {
			break
		}
	}
	if len(dialErrors) == 0 {
		return nil, errors.New("no IRC server addresses to dial")
	}
	return nil, errors.Join(dialErrors...)
}
