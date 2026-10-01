// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package revocationclient

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
	"sync"
	"time"
)

const defaultHTTPTimeout = 15 * time.Second

type lookupIPFunc func(context.Context, string) ([]net.IPAddr, error)
type dialContextFunc func(context.Context, string, string) (net.Conn, error)

type endpointPolicy struct {
	lookupIP     lookupIPFunc
	allowPrivate bool
}

func defaultEndpointPolicy() endpointPolicy {
	resolver := net.DefaultResolver
	return endpointPolicy{lookupIP: resolver.LookupIPAddr}
}

func hardenHTTPClient(base *http.Client, policy endpointPolicy) *http.Client {
	if base == nil {
		base = &http.Client{}
	}
	hardened := *base
	if hardened.Timeout <= 0 {
		hardened.Timeout = defaultHTTPTimeout
	}

	transport := base.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	httpTransport, ok := transport.(*http.Transport)
	if !ok {
		hardened.Transport = errorRoundTripper{
			err: errors.New("revocación: transporte HTTP personalizado no soportado por la política SSRF"),
		}
	} else {
		clone := httpTransport.Clone()
		baseDial := clone.DialContext
		if baseDial == nil {
			dialer := &net.Dialer{}
			baseDial = dialer.DialContext
		}
		trustedProxies := newTrustedProxyRegistry()
		if baseProxy := clone.Proxy; baseProxy != nil {
			clone.Proxy = func(req *http.Request) (*url.URL, error) {
				proxyURL, err := baseProxy(req)
				if err != nil || proxyURL == nil {
					return proxyURL, err
				}
				if err := trustedProxies.add(proxyURL); err != nil {
					return nil, err
				}
				return proxyURL, nil
			}
		}
		clone.DialContext = policy.secureDialContextWithTrustedProxy(baseDial, trustedProxies.contains)
		// Force TLS connections through the guarded DialContext. TLS options
		// (roots, client certificates, versions) remain on TLSClientConfig.
		//lint:ignore SA1019 Hay que anular también el hook legado para que no eluda la política SSRF.
		clone.DialTLS = nil
		clone.DialTLSContext = nil
		hardened.Transport = endpointGuardRoundTripper{
			base:   clone,
			policy: policy,
		}
	}

	previousRedirectPolicy := base.CheckRedirect
	hardened.CheckRedirect = policy.redirectPolicy(previousRedirectPolicy)
	return &hardened
}

type errorRoundTripper struct {
	err error
}

func (r errorRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, r.err
}

type endpointGuardRoundTripper struct {
	base   *http.Transport
	policy endpointPolicy
}

func (r endpointGuardRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("revocación: petición HTTP nula")
	}
	if err := r.policy.validateEndpoint(req.Context(), req.URL); err != nil {
		return nil, err
	}
	return r.base.RoundTrip(req)
}

func (p endpointPolicy) validateEndpoint(ctx context.Context, endpoint *url.URL) error {
	_, _, err := p.resolveEndpoint(ctx, endpoint)
	return err
}

func (p endpointPolicy) resolveEndpoint(ctx context.Context, endpoint *url.URL) (string, []net.IP, error) {
	host, err := validateEndpointURL(endpoint)
	if err != nil {
		return "", nil, err
	}
	addresses, err := p.resolveHost(ctx, host)
	if err != nil {
		return "", nil, err
	}
	return host, addresses, nil
}

func validateEndpointURL(endpoint *url.URL) (string, error) {
	if endpoint == nil {
		return "", errors.New("revocación: URL nula")
	}
	scheme := strings.ToLower(strings.TrimSpace(endpoint.Scheme))
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("revocación: esquema URL no permitido: %q", endpoint.Scheme)
	}
	if !endpoint.IsAbs() || endpoint.Opaque != "" || endpoint.Host == "" {
		return "", errors.New("revocación: URL HTTP(S) absoluta inválida")
	}
	if endpoint.User != nil {
		return "", errors.New("revocación: las credenciales en URL no están permitidas")
	}
	host := strings.TrimSuffix(strings.TrimSpace(endpoint.Hostname()), ".")
	if host == "" || strings.Contains(host, "%") {
		return "", errors.New("revocación: host vacío o con zona IPv6 no permitida")
	}
	if endpoint.Port() != "" {
		port, err := strconv.Atoi(endpoint.Port())
		if err != nil || port < 1 || port > 65535 {
			return "", errors.New("revocación: puerto URL inválido")
		}
	}
	return host, nil
}

func (p endpointPolicy) resolveHost(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if err := p.validateIP(ip); err != nil {
			return nil, err
		}
		return []net.IP{ip}, nil
	}
	if p.lookupIP == nil {
		return nil, errors.New("revocación: resolver DNS no configurado")
	}
	addresses, err := p.lookupIP(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("revocación: no se pudo resolver %q: %w", host, err)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("revocación: %q no resolvió ninguna dirección", host)
	}
	resolved := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		if err := p.validateIP(address.IP); err != nil {
			return nil, fmt.Errorf("revocación: host %q no permitido: %w", host, err)
		}
		resolved = append(resolved, address.IP)
	}
	return resolved, nil
}

func (p endpointPolicy) validateIP(ip net.IP) error {
	if ip == nil {
		return errors.New("dirección IP inválida")
	}
	if p.allowPrivate {
		return nil
	}
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return errors.New("dirección IP inválida")
	}
	address = address.Unmap()
	switch {
	case address.IsLoopback():
		return fmt.Errorf("dirección loopback bloqueada (%s)", address)
	case address.IsLinkLocalUnicast(), address.IsLinkLocalMulticast():
		return fmt.Errorf("dirección link-local bloqueada (%s)", address)
	case address.IsPrivate():
		return fmt.Errorf("dirección privada bloqueada (%s)", address)
	case cgnatPrefix.Contains(address):
		return fmt.Errorf("dirección compartida CGNAT bloqueada (%s)", address)
	case address.IsUnspecified():
		return fmt.Errorf("dirección no especificada bloqueada (%s)", address)
	case address.IsMulticast():
		return fmt.Errorf("dirección multicast bloqueada (%s)", address)
	}
	for _, blocked := range blockedEndpointPrefixes {
		if blocked.prefix.Contains(address) {
			return fmt.Errorf("dirección %s bloqueada (%s)", blocked.name, address)
		}
	}
	if !address.IsGlobalUnicast() {
		return fmt.Errorf("dirección no global bloqueada (%s)", address)
	}
	return nil
}

var cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

var blockedEndpointPrefixes = [...]struct {
	prefix netip.Prefix
	name   string
}{
	{netip.MustParsePrefix("0.0.0.0/8"), "current-network"},
	{netip.MustParsePrefix("192.0.0.0/24"), "IETF protocol assignment"},
	{netip.MustParsePrefix("192.0.2.0/24"), "TEST-NET-1"},
	{netip.MustParsePrefix("192.88.99.0/24"), "deprecated 6to4 relay"},
	{netip.MustParsePrefix("198.18.0.0/15"), "benchmark"},
	{netip.MustParsePrefix("198.51.100.0/24"), "TEST-NET-2"},
	{netip.MustParsePrefix("203.0.113.0/24"), "TEST-NET-3"},
	{netip.MustParsePrefix("240.0.0.0/4"), "reserved"},
	{netip.MustParsePrefix("::/96"), "IPv4-compatible"},
	{netip.MustParsePrefix("64:ff9b::/96"), "NAT64 well-known"},
	{netip.MustParsePrefix("64:ff9b:1::/48"), "NAT64 local-use"},
	{netip.MustParsePrefix("100::/64"), "discard-only"},
	{netip.MustParsePrefix("2001::/23"), "IPv6 special-purpose"},
	{netip.MustParsePrefix("2001:db8::/32"), "documentation"},
	{netip.MustParsePrefix("2002::/16"), "6to4"},
	{netip.MustParsePrefix("3fff::/20"), "documentation"},
	{netip.MustParsePrefix("5f00::/16"), "segment-routing local-use"},
	{netip.MustParsePrefix("fec0::/10"), "deprecated site-local"},
}

func (p endpointPolicy) secureDialContext(baseDial dialContextFunc) dialContextFunc {
	return p.secureDialContextWithTrustedProxy(baseDial, nil)
}

func (p endpointPolicy) secureDialContextWithTrustedProxy(baseDial dialContextFunc, isTrustedProxy func(string) bool) dialContextFunc {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		// A proxy is explicit application configuration, not a destination
		// controlled by the certificate. The endpoint itself has already been
		// resolved and checked by endpointGuardRoundTripper. Allowing the proxy
		// address preserves enterprise proxy support without allowing a
		// certificate URL to reach a private address directly.
		if isTrustedProxy != nil && isTrustedProxy(address) {
			return baseDial(ctx, network, address)
		}
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("revocación: destino de conexión inválido: %w", err)
		}
		host = strings.TrimSuffix(host, ".")

		addresses, err := p.resolveForConnection(ctx, network, host)
		if err != nil {
			return nil, err
		}
		var dialErrors []error
		for _, ip := range addresses {
			conn, dialErr := baseDial(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			dialErrors = append(dialErrors, dialErr)
		}
		return nil, fmt.Errorf("revocación: no se pudo conectar con %q: %w", host, errors.Join(dialErrors...))
	}
}

const maxTrustedProxyEndpoints = 16

type trustedProxyRegistry struct {
	mu        sync.RWMutex
	addresses map[string]struct{}
}

func newTrustedProxyRegistry() *trustedProxyRegistry {
	return &trustedProxyRegistry{addresses: make(map[string]struct{})}
}

func (r *trustedProxyRegistry) add(proxyURL *url.URL) error {
	address, err := proxyDialAddress(proxyURL)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.addresses[address]; exists {
		return nil
	}
	if len(r.addresses) >= maxTrustedProxyEndpoints {
		return errors.New("revocación: demasiados endpoints proxy distintos")
	}
	r.addresses[address] = struct{}{}
	return nil
}

func (r *trustedProxyRegistry) contains(address string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.addresses[address]
	return ok
}

func proxyDialAddress(proxyURL *url.URL) (string, error) {
	if proxyURL == nil {
		return "", errors.New("revocación: URL de proxy nula")
	}
	host := proxyURL.Hostname()
	if host == "" {
		return "", errors.New("revocación: proxy sin host")
	}
	port := proxyURL.Port()
	if port == "" {
		switch strings.ToLower(proxyURL.Scheme) {
		case "http":
			port = "80"
		case "https":
			port = "443"
		case "socks5", "socks5h":
			port = "1080"
		default:
			return "", fmt.Errorf("revocación: esquema de proxy no soportado: %q", proxyURL.Scheme)
		}
	}
	return net.JoinHostPort(host, port), nil
}

func (p endpointPolicy) resolveForConnection(ctx context.Context, network, host string) ([]net.IP, error) {
	if literal := net.ParseIP(host); literal != nil {
		if err := p.validateIP(literal); err != nil {
			return nil, fmt.Errorf("revocación: conexión bloqueada: %w", err)
		}
		return []net.IP{literal}, nil
	}
	if p.lookupIP == nil {
		return nil, errors.New("revocación: resolver DNS no configurado")
	}
	resolved, err := p.lookupIP(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("revocación: no se pudo resolver %q al conectar: %w", host, err)
	}
	var addresses []net.IP
	for _, candidate := range resolved {
		if err := p.validateIP(candidate.IP); err != nil {
			return nil, fmt.Errorf("revocación: conexión con %q bloqueada: %w", host, err)
		}
		if network == "tcp4" && candidate.IP.To4() == nil {
			continue
		}
		if network == "tcp6" && candidate.IP.To4() != nil {
			continue
		}
		addresses = append(addresses, candidate.IP)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("revocación: %q no resolvió direcciones compatibles y permitidas", host)
	}
	return addresses, nil
}

func (p endpointPolicy) redirectPolicy(previous func(*http.Request, []*http.Request) error) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if req == nil || req.URL == nil {
			return errors.New("revocación: redirect sin URL")
		}
		if len(via) >= 10 {
			return errors.New("revocación: demasiados redirects")
		}
		if err := p.validateEndpoint(req.Context(), req.URL); err != nil {
			return fmt.Errorf("revocación: redirect bloqueado: %w", err)
		}
		if len(via) > 0 {
			previousURL := via[len(via)-1].URL
			if strings.EqualFold(previousURL.Scheme, "https") && strings.EqualFold(req.URL.Scheme, "http") {
				return errors.New("revocación: downgrade HTTPS a HTTP bloqueado")
			}
			if !sameOrigin(previousURL, req.URL) {
				return errors.New("revocación: redirect a otro origen bloqueado")
			}
		}
		if previous != nil {
			return previous(req, via)
		}
		return nil
	}
}

func sameOrigin(left, right *url.URL) bool {
	if left == nil || right == nil {
		return false
	}
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(canonicalHost(left), canonicalHost(right)) &&
		effectivePort(left) == effectivePort(right)
}

func canonicalHost(endpoint *url.URL) string {
	return strings.TrimSuffix(strings.TrimSpace(endpoint.Hostname()), ".")
}

func effectivePort(endpoint *url.URL) string {
	if port := endpoint.Port(); port != "" {
		return port
	}
	switch strings.ToLower(endpoint.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}
