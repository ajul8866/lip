// Package egress enforces the outbound HTTP policy. It is deliberately the
// only way LIP creates outbound connections: proxy environment variables are
// ignored, every redirect is re-validated, and the actual dialed IP is checked
// after DNS resolution (so an allowlisted name cannot rebind to a blocked IP).
//
// The policy is fail-closed: with no matching rule, nothing is allowed. Safe
// local-only mode additionally permits only loopback/RFC1918/ULA destinations.
package egress

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var ErrDenied = errors.New("egress denied")

type Rule struct {
	Host     string
	Port     string
	Wildcard bool
	Any      bool
}

type Guard struct {
	safe     bool
	rules    []Rule
	resolver *net.Resolver
	dialer   *net.Dialer
}

type Options struct {
	Safe      bool
	Allowlist []string
	Resolver  *net.Resolver
	Timeout   time.Duration
}

func New(opts Options) (*Guard, error) {
	if opts.Resolver == nil {
		opts.Resolver = net.DefaultResolver
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	g := &Guard{safe: opts.Safe, resolver: opts.Resolver}
	for _, raw := range opts.Allowlist {
		rule, err := parseRule(raw, opts.Safe)
		if err != nil {
			return nil, fmt.Errorf("invalid egress allowlist entry %q: %w", raw, err)
		}
		g.rules = append(g.rules, rule)
	}
	g.dialer = &net.Dialer{Timeout: opts.Timeout, KeepAlive: 30 * time.Second, Control: g.control}
	return g, nil
}

func parseRule(raw string, safe bool) (Rule, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "*" {
		if safe {
			return Rule{}, errors.New("wildcard egress is not permitted in safe local-only mode")
		}
		return Rule{Any: true}, nil
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		return Rule{}, errors.New("expected host:port")
	}
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		return Rule{}, errors.New("empty host")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return Rule{}, errors.New("invalid port")
	}
	rule := Rule{Host: host, Port: port}
	if strings.HasPrefix(host, "*.") {
		if safe {
			return Rule{}, errors.New("wildcard hosts are not permitted in safe local-only mode")
		}
		base := strings.TrimPrefix(host, "*.")
		if base == "" || strings.Contains(base, "*") {
			return Rule{}, errors.New("invalid wildcard host")
		}
		rule.Wildcard = true
		rule.Host = base
	} else if strings.Contains(host, "*") {
		return Rule{}, errors.New("invalid host")
	}
	return rule, nil
}

func (g *Guard) Safe() bool { return g.safe }

func (g *Guard) RuleCount() int { return len(g.rules) }

// AllowsHost performs the static allowlist check without DNS resolution. It is
// exposed for configuration diagnostics; network calls must use CheckTarget.
func (g *Guard) AllowsHost(host, port string) bool {
	host = normalizeHost(host)
	for _, r := range g.rules {
		if r.Any {
			return true
		}
		if r.Port != port {
			continue
		}
		if r.Wildcard {
			if strings.HasSuffix(host, "."+r.Host) {
				return true
			}
			continue
		}
		if host == r.Host {
			return true
		}
	}
	return false
}

// CheckTarget validates an allowlisted host and, in safe mode, every IP the
// host resolves to. Resolution failures deny the request.
func (g *Guard) CheckTarget(ctx context.Context, host, port string) error {
	host = normalizeHost(host)
	if host == "" {
		return fmt.Errorf("%w: empty host", ErrDenied)
	}
	if !g.AllowsHost(host, port) {
		return fmt.Errorf("%w: %s:%s is not allowlisted", ErrDenied, host, port)
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return g.checkIP(net.IP(ip.AsSlice()))
	}
	if g.safe {
		addrs, err := g.resolver.LookupIPAddr(ctx, host)
		if err != nil {
			return fmt.Errorf("%w: resolve %s: %v", ErrDenied, host, err)
		}
		if len(addrs) == 0 {
			return fmt.Errorf("%w: %s resolved to no addresses", ErrDenied, host)
		}
		for _, addr := range addrs {
			if err := g.checkIP(addr.IP); err != nil {
				return fmt.Errorf("%w (resolved %s)", err, addr.IP)
			}
		}
	}
	return nil
}

func (g *Guard) checkIP(ip net.IP) error {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return fmt.Errorf("%w: invalid IP", ErrDenied)
	}
	addr = addr.Unmap()
	if isMetadata(addr) || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return fmt.Errorf("%w: blocked address %s", ErrDenied, addr)
	}
	if g.safe {
		if !addr.IsLoopback() && !addr.IsPrivate() {
			return fmt.Errorf("%w: safe local-only blocks public address %s", ErrDenied, addr)
		}
	}
	return nil
}

func isMetadata(addr netip.Addr) bool {
	addr = addr.Unmap()
	if addr == netip.MustParseAddr("169.254.169.254") ||
		addr == netip.MustParseAddr("169.254.170.2") ||
		addr == netip.MustParseAddr("fd00:ec2::254") {
		return true
	}
	return false
}

// CheckURL validates scheme, credentials, and destination policy.
func (g *Guard) CheckURL(ctx context.Context, raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid URL", ErrDenied)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%w: only http/https is permitted", ErrDenied)
	}
	if u.User != nil {
		return nil, fmt.Errorf("%w: URL credentials are not permitted", ErrDenied)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("%w: URL has no host", ErrDenied)
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	if err := g.CheckTarget(ctx, u.Hostname(), port); err != nil {
		return nil, err
	}
	return u, nil
}

func (g *Guard) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid dial address", ErrDenied)
	}
	if err := g.CheckTarget(ctx, host, port); err != nil {
		return nil, err
	}
	return g.dialer.DialContext(ctx, network, address)
}

func (g *Guard) control(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: invalid dial address", ErrDenied)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("%w: non-IP dial target", ErrDenied)
	}
	return g.checkIP(ip)
}

// Client returns an HTTP client that cannot bypass the guard through proxy
// environment variables or redirects.
func (g *Guard) Client(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           g.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("%w: too many redirects", ErrDenied)
			}
			_, err := g.CheckURL(req.Context(), req.URL.String())
			return err
		},
	}
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.TrimSuffix(host, ".")
	host = strings.Trim(host, "[]")
	return host
}

// ErrorMessage produces a stable, non-secret reason for API responses.
func ErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.Contains(msg, "not allowlisted") {
		return "destination is not allowlisted"
	}
	if strings.Contains(msg, "safe local-only") {
		return "safe local-only mode blocks this destination"
	}
	return "destination is blocked by egress policy"
}
