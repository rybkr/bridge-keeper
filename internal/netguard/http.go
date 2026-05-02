package netguard

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

var metadataHosts = map[string]struct{}{
	"metadata":                 {},
	"metadata.google.internal": {},
	"metadata.goog":            {},
}

// ValidateHTTPURL parses an HTTP(S) URL and rejects literal local, private, or
// cloud metadata targets that should never be reachable through agent HTTP tools.
func ValidateHTTPURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("URL must use http or https")
	}
	if parsed.Hostname() == "" {
		return nil, fmt.Errorf("URL must include a host")
	}
	if reason, blocked := BlockedHostReason(parsed.Hostname()); blocked {
		return nil, fmt.Errorf("%s", reason)
	}
	return parsed, nil
}

// BlockedHostReason reports whether a host is an unsafe direct HTTP target.
// It handles ordinary IP literals plus legacy IPv4 spellings accepted by many
// network stacks, such as 127.1, 0177.0.0.1, 0x7f000001, and 2130706433.
func BlockedHostReason(host string) (string, bool) {
	normalized := normalizeHost(host)
	if normalized == "" {
		return "host is empty", true
	}

	if _, ok := metadataHosts[strings.TrimSuffix(normalized, ".")]; ok {
		return fmt.Sprintf("host %q is a blocked metadata endpoint", host), true
	}
	if strings.TrimSuffix(normalized, ".") == "localhost" || strings.HasSuffix(strings.TrimSuffix(normalized, "."), ".localhost") {
		return fmt.Sprintf("host %q is a blocked local endpoint", host), true
	}

	if addr, ok := parseHostAddr(normalized); ok {
		if reason, blocked := BlockedAddrReason(addr); blocked {
			return fmt.Sprintf("host %q resolves to blocked %s address %s", host, reason, addr), true
		}
	}
	return "", false
}

// BlockedAddrReason reports whether an IP address is unsafe for agent-managed
// HTTP requests.
func BlockedAddrReason(addr netip.Addr) (string, bool) {
	if addr.Is4In6() {
		addr = addr.Unmap()
	}

	if addr == netip.MustParseAddr("100.100.100.200") {
		return "metadata", true
	}
	switch {
	case addr.IsLoopback():
		return "loopback", true
	case addr.IsPrivate():
		return "private", true
	case addr.IsLinkLocalUnicast():
		return "link-local", true
	case addr.IsLinkLocalMulticast():
		return "link-local multicast", true
	case addr.IsUnspecified():
		return "unspecified", true
	case addr.IsMulticast():
		return "multicast", true
	default:
		return "", false
	}
}

// NewGuardedHTTPClient returns a client that validates redirects and checks DNS
// answers at dial time before connecting.
func NewGuardedHTTPClient(base *http.Client) *http.Client {
	client := &http.Client{}
	if base != nil {
		*client = *base
	}

	client.Transport = NewGuardedTransport(client.Transport)

	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if _, err := ValidateHTTPURL(req.URL.String()); err != nil {
			return fmt.Errorf("redirect blocked: %w", err)
		}
		if base != nil && base.CheckRedirect != nil {
			return base.CheckRedirect(req, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}

	return client
}

// NewGuardedTransport wraps the default transport with a DNS-aware dial guard.
// Custom transports are returned as-is because their dialing behavior is opaque.
func NewGuardedTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	transport, ok := base.(*http.Transport)
	if !ok {
		return base
	}

	clone := transport.Clone()
	dialer := &net.Dialer{}

	clone.Proxy = nil
	clone.DialTLSContext = nil
	clone.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addr, err := resolveDialAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
	}
	return clone
}

func resolveDialAddr(ctx context.Context, host string) (netip.Addr, error) {
	addrs, err := lookupHostAddrs(ctx, host)
	if err != nil {
		return netip.Addr{}, err
	}
	for _, addr := range addrs {
		if reason, blocked := BlockedAddrReason(addr); blocked {
			return netip.Addr{}, fmt.Errorf("DNS for host %q returned blocked %s address %s", host, reason, addr)
		}
	}
	if len(addrs) == 0 {
		return netip.Addr{}, fmt.Errorf("DNS for host %q returned no addresses", host)
	}
	return addrs[0], nil
}

func lookupHostAddrs(ctx context.Context, host string) ([]netip.Addr, error) {
	if addr, ok := parseHostAddr(host); ok {
		return []netip.Addr{addr}, nil
	}

	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	return ips, nil
}

func parseHostAddr(host string) (netip.Addr, bool) {
	host = normalizeHost(host)
	host = strings.TrimSuffix(host, ".")
	if strings.Contains(host, "%") {
		host = strings.SplitN(host, "%", 2)[0]
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr, true
	}
	if addr, ok := parseLegacyIPv4(host); ok {
		return addr, true
	}
	return netip.Addr{}, false
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	if unescaped, err := url.PathUnescape(host); err == nil {
		host = unescaped
	}
	return host
}

func parseLegacyIPv4(host string) (netip.Addr, bool) {
	if host == "" || strings.Contains(host, ":") {
		return netip.Addr{}, false
	}
	parts := strings.Split(host, ".")
	if len(parts) > 4 {
		return netip.Addr{}, false
	}

	values := make([]uint64, len(parts))
	for i, part := range parts {
		if part == "" || strings.HasPrefix(part, "+") || strings.HasPrefix(part, "-") {
			return netip.Addr{}, false
		}
		value, err := strconv.ParseUint(part, 0, 32)
		if err != nil {
			return netip.Addr{}, false
		}
		values[i] = value
	}

	var packed uint64
	switch len(values) {
	case 1:
		packed = values[0]
	case 2:
		if values[0] > 0xff || values[1] > 0xffffff {
			return netip.Addr{}, false
		}
		packed = values[0]<<24 | values[1]
	case 3:
		if values[0] > 0xff || values[1] > 0xff || values[2] > 0xffff {
			return netip.Addr{}, false
		}
		packed = values[0]<<24 | values[1]<<16 | values[2]
	case 4:
		for _, value := range values {
			if value > 0xff {
				return netip.Addr{}, false
			}
			packed = packed<<8 | value
		}
	}
	if packed > 0xffffffff {
		return netip.Addr{}, false
	}

	return netip.AddrFrom4([4]byte{
		byte(packed >> 24),
		byte(packed >> 16),
		byte(packed >> 8),
		byte(packed),
	}), true
}
