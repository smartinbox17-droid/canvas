package outbound

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

var ErrBlockedPrivateAddress = errors.New("outbound request to private or loopback address is blocked")

type GuardConfig struct {
	AllowPrivateUpstreams bool
	AllowedPrivateHosts   map[string]struct{}
	Timeout               time.Duration
}

func NewGuardConfig(allowPrivate bool, allowedHosts []string) GuardConfig {
	m := make(map[string]struct{}, len(allowedHosts))
	for _, h := range allowedHosts {
		m[strings.ToLower(strings.TrimSpace(h))] = struct{}{}
	}
	return GuardConfig{
		AllowPrivateUpstreams: allowPrivate,
		AllowedPrivateHosts:   m,
		Timeout:               30 * time.Second,
	}
}

func IsDisallowedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}

func NewSafeHTTPClient(cfg GuardConfig) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			lowerHost := strings.ToLower(host)
			_, hostAllowed := cfg.AllowedPrivateHosts[lowerHost]
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, ipAddr := range ips {
				if !cfg.AllowPrivateUpstreams && !hostAllowed && IsDisallowedIP(ipAddr.IP) {
					return nil, fmt.Errorf("%w: %s (%s)", ErrBlockedPrivateAddress, host, ipAddr.IP.String())
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &http.Client{Transport: transport, Timeout: timeout}
}
