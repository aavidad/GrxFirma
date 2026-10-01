package sign

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/digitorus/pdfsign/revocation"
	"golang.org/x/crypto/ocsp"
)

const (
	maxOCSPResponseSize = 1 << 20  // 1 MiB
	maxCRLResponseSize  = 16 << 20 // 16 MiB

	defaultRevocationTimeout       = 15 * time.Second
	defaultRevocationDialTimeout   = 5 * time.Second
	defaultRevocationHeaderTimeout = 5 * time.Second
	defaultRevocationTLSDeadline   = 5 * time.Second
	defaultRevocationRedirects     = 3
	revocationClockSkew            = 5 * time.Minute
	maxOCSPAgeWithoutNextUpdate    = 24 * time.Hour
)

var cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

var blockedRevocationPrefixes = [...]struct {
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

type revocationFetcher struct {
	lookupIP            func(context.Context, string) ([]net.IPAddr, error)
	dialContext         func(context.Context, string, string) (net.Conn, error)
	timeout             time.Duration
	responseHeaderLimit time.Duration
	tlsHandshakeLimit   time.Duration
	maxRedirects        int
}

func defaultRevocationFetcher() revocationFetcher {
	dialer := &net.Dialer{
		Timeout:   defaultRevocationDialTimeout,
		KeepAlive: 30 * time.Second,
	}
	return revocationFetcher{
		lookupIP:            net.DefaultResolver.LookupIPAddr,
		dialContext:         dialer.DialContext,
		timeout:             defaultRevocationTimeout,
		responseHeaderLimit: defaultRevocationHeaderTimeout,
		tlsHandshakeLimit:   defaultRevocationTLSDeadline,
		maxRedirects:        defaultRevocationRedirects,
	}
}

func (f revocationFetcher) fetch(
	ctx context.Context,
	method string,
	rawURL string,
	contentType string,
	payload []byte,
	maxSize int64,
) ([]byte, error) {
	target, err := parseRevocationURL(rawURL)
	if err != nil {
		return nil, err
	}
	if maxSize <= 0 {
		return nil, errors.New("revocation response limit must be positive")
	}

	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           f.dialPublicContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   durationOrDefault(f.tlsHandshakeLimit, defaultRevocationTLSDeadline),
		ResponseHeaderTimeout: durationOrDefault(f.responseHeaderLimit, defaultRevocationHeaderTimeout),
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
	defer transport.CloseIdleConnections()

	maxRedirects := f.maxRedirects
	if maxRedirects <= 0 {
		maxRedirects = defaultRevocationRedirects
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   durationOrDefault(f.timeout, defaultRevocationTimeout),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return fmt.Errorf("too many revocation redirects (maximum %d)", maxRedirects)
			}
			if _, err := parseRevocationURL(req.URL.String()); err != nil {
				return fmt.Errorf("unsafe revocation redirect: %w", err)
			}
			if len(via) == 0 || !sameRevocationOrigin(via[0].URL, req.URL) {
				return errors.New("revocation redirect changed origin or scheme")
			}
			if req.Method != via[0].Method {
				return errors.New("revocation redirect changed HTTP method")
			}
			return nil
		},
	}

	var requestBody io.Reader
	if len(payload) > 0 {
		requestBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), requestBody)
	if err != nil {
		return nil, fmt.Errorf("create revocation request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if method == http.MethodPost {
		req.Header.Set("Accept", "application/ocsp-response")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request revocation data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("revocation endpoint returned HTTP status %d", resp.StatusCode)
	}
	if resp.ContentLength > maxSize {
		return nil, fmt.Errorf("revocation response exceeds %d bytes", maxSize)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("read revocation response: %w", err)
	}
	if int64(len(body)) > maxSize {
		return nil, fmt.Errorf("revocation response exceeds %d bytes", maxSize)
	}
	return body, nil
}

func (f revocationFetcher) dialPublicContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid revocation endpoint address: %w", err)
	}
	host = strings.TrimSuffix(host, ".")
	if strings.Contains(host, "%") {
		return nil, errors.New("IPv6 zones are not allowed in revocation endpoints")
	}

	var addresses []net.IPAddr
	if literal := net.ParseIP(host); literal != nil {
		addresses = []net.IPAddr{{IP: literal}}
	} else {
		if f.lookupIP == nil {
			return nil, errors.New("revocation DNS resolver is not configured")
		}
		addresses, err = f.lookupIP(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolve revocation endpoint: %w", err)
		}
	}
	if len(addresses) == 0 {
		return nil, errors.New("revocation endpoint resolved to no addresses")
	}

	for _, resolved := range addresses {
		if err := validatePublicIP(resolved.IP); err != nil {
			return nil, fmt.Errorf("unsafe revocation endpoint address %q: %w", resolved.IP.String(), err)
		}
	}
	if f.dialContext == nil {
		return nil, errors.New("revocation network dialer is not configured")
	}

	var lastErr error
	for _, resolved := range addresses {
		pinnedAddress := net.JoinHostPort(resolved.IP.String(), port)
		conn, dialErr := f.dialContext(ctx, network, pinnedAddress)
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	return nil, fmt.Errorf("connect to revocation endpoint: %w", lastErr)
}

func parseRevocationURL(rawURL string) (*url.URL, error) {
	target, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("parse revocation endpoint: %w", err)
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, errors.New("revocation endpoint must use HTTP or HTTPS")
	}
	if target.User != nil {
		return nil, errors.New("credentials are not allowed in revocation endpoints")
	}
	if target.Hostname() == "" {
		return nil, errors.New("revocation endpoint has no hostname")
	}
	if target.Opaque != "" || target.Fragment != "" {
		return nil, errors.New("opaque URLs and fragments are not allowed in revocation endpoints")
	}
	if strings.Contains(target.Hostname(), "%") {
		return nil, errors.New("IPv6 zones are not allowed in revocation endpoints")
	}
	if port := target.Port(); port != "" {
		if _, err := net.LookupPort("tcp", port); err != nil {
			return nil, errors.New("revocation endpoint has an invalid port")
		}
	}
	if literal := net.ParseIP(target.Hostname()); literal != nil {
		if err := validatePublicIP(literal); err != nil {
			return nil, fmt.Errorf("unsafe revocation endpoint address: %w", err)
		}
	}
	return target, nil
}

func validatePublicIP(ip net.IP) error {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return errors.New("invalid IP address")
	}
	address = address.Unmap()
	if !address.IsValid() || address.IsUnspecified() {
		return errors.New("unspecified address is blocked")
	}
	if address.IsLoopback() {
		return errors.New("loopback address is blocked")
	}
	if address.IsPrivate() {
		return errors.New("private address is blocked")
	}
	if address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() {
		return errors.New("link-local address is blocked")
	}
	if address.IsMulticast() {
		return errors.New("multicast address is blocked")
	}
	if address.Is4() && cgnatPrefix.Contains(address) {
		return errors.New("carrier-grade NAT address is blocked")
	}
	for _, blocked := range blockedRevocationPrefixes {
		if blocked.prefix.Contains(address) {
			return fmt.Errorf("%s address is blocked", blocked.name)
		}
	}
	if !address.IsGlobalUnicast() {
		return errors.New("non-global unicast address is blocked")
	}
	return nil
}

func sameRevocationOrigin(left, right *url.URL) bool {
	if left == nil || right == nil || left.Scheme != right.Scheme {
		return false
	}
	return strings.EqualFold(strings.TrimSuffix(left.Hostname(), "."), strings.TrimSuffix(right.Hostname(), ".")) &&
		effectivePort(left) == effectivePort(right)
}

func effectivePort(target *url.URL) string {
	if port := target.Port(); port != "" {
		return port
	}
	if target.Scheme == "https" {
		return "443"
	}
	return "80"
}

func durationOrDefault(value, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}

func embedOCSPRevocationStatus(cert, issuer *x509.Certificate, info *revocation.InfoArchival) error {
	return embedOCSPRevocationStatusWithFetcher(cert, issuer, info, defaultRevocationFetcher())
}

func embedOCSPRevocationStatusWithFetcher(
	cert, issuer *x509.Certificate,
	info *revocation.InfoArchival,
	fetcher revocationFetcher,
) error {
	if cert == nil || issuer == nil {
		return errors.New("certificate and issuer are required for OCSP")
	}
	if info == nil {
		return errors.New("revocation information destination is required")
	}
	if len(cert.OCSPServer) == 0 {
		return errors.New("certificate has no OCSP endpoint")
	}
	if err := cert.CheckSignatureFrom(issuer); err != nil {
		return fmt.Errorf("certificate was not issued by the supplied OCSP issuer: %w", err)
	}

	request, err := ocsp.CreateRequest(cert, issuer, nil)
	if err != nil {
		return fmt.Errorf("create OCSP request: %w", err)
	}
	body, err := fetcher.fetch(
		context.Background(),
		http.MethodPost,
		cert.OCSPServer[0],
		"application/ocsp-request",
		request,
		maxOCSPResponseSize,
	)
	if err != nil {
		return fmt.Errorf("download OCSP response: %w", err)
	}

	response, err := ocsp.ParseResponseForCert(body, cert, issuer)
	if err != nil {
		return fmt.Errorf("validate OCSP response: %w", err)
	}
	now := time.Now()
	if err := validateOCSPFreshness(response, now); err != nil {
		return err
	}
	if err := validateOCSPResponder(response, issuer, now); err != nil {
		return err
	}
	if response.Status != ocsp.Good && response.Status != ocsp.Revoked {
		return fmt.Errorf("OCSP responder returned unsupported certificate status %d", response.Status)
	}
	return info.AddOCSP(body)
}

func validateOCSPFreshness(response *ocsp.Response, now time.Time) error {
	if response == nil {
		return errors.New("OCSP response is nil")
	}
	if response.ThisUpdate.IsZero() {
		return errors.New("OCSP response has no thisUpdate")
	}
	if response.ProducedAt.IsZero() {
		return errors.New("OCSP response has no producedAt")
	}
	if response.ThisUpdate.After(now.Add(revocationClockSkew)) {
		return errors.New("OCSP response thisUpdate is in the future")
	}
	if response.ProducedAt.After(now.Add(revocationClockSkew)) {
		return errors.New("OCSP response producedAt is in the future")
	}
	if response.ThisUpdate.After(response.ProducedAt.Add(revocationClockSkew)) {
		return errors.New("OCSP response thisUpdate is after producedAt")
	}
	if response.NextUpdate.IsZero() {
		if response.ThisUpdate.Before(now.Add(-maxOCSPAgeWithoutNextUpdate - revocationClockSkew)) {
			return fmt.Errorf("OCSP response without nextUpdate is older than %s", maxOCSPAgeWithoutNextUpdate)
		}
		return nil
	}
	if response.NextUpdate.Before(response.ThisUpdate) {
		return errors.New("OCSP response nextUpdate is before thisUpdate")
	}
	if response.NextUpdate.Before(now.Add(-revocationClockSkew)) {
		return errors.New("OCSP response is stale")
	}
	return nil
}

func validateOCSPResponder(response *ocsp.Response, issuer *x509.Certificate, now time.Time) error {
	if response == nil || issuer == nil {
		return errors.New("OCSP response and issuer are required")
	}
	responder := response.Certificate
	if responder == nil {
		// ParseResponseForCert already verified the response directly with the
		// issuer key when no delegated responder certificate was embedded.
		return nil
	}
	if now.Add(revocationClockSkew).Before(responder.NotBefore) ||
		now.Add(-revocationClockSkew).After(responder.NotAfter) {
		return errors.New("OCSP responder certificate is outside its validity period")
	}
	if responder.Equal(issuer) {
		return nil
	}

	roots := x509.NewCertPool()
	roots.AddCert(issuer)
	if _, err := responder.Verify(x509.VerifyOptions{
		Roots:       roots,
		CurrentTime: now,
		KeyUsages:   []x509.ExtKeyUsage{x509.ExtKeyUsageOCSPSigning},
	}); err != nil {
		return fmt.Errorf("verify delegated OCSP responder certificate: %w", err)
	}
	return nil
}

// embedCRLRevocationStatus requires the issuer because unverified CRLs must
// never be embedded in a signed document.
func embedCRLRevocationStatus(cert, issuer *x509.Certificate, info *revocation.InfoArchival) error {
	return embedCRLRevocationStatusWithFetcher(cert, issuer, info, defaultRevocationFetcher())
}

func embedCRLRevocationStatusWithFetcher(
	cert, issuer *x509.Certificate,
	info *revocation.InfoArchival,
	fetcher revocationFetcher,
) error {
	if cert == nil || issuer == nil {
		return errors.New("certificate and issuer are required for CRL verification")
	}
	if info == nil {
		return errors.New("revocation information destination is required")
	}
	if len(cert.CRLDistributionPoints) == 0 {
		return errors.New("certificate has no CRL distribution point")
	}
	if err := cert.CheckSignatureFrom(issuer); err != nil {
		return fmt.Errorf("certificate was not issued by the supplied CRL issuer: %w", err)
	}

	body, err := fetcher.fetch(
		context.Background(),
		http.MethodGet,
		cert.CRLDistributionPoints[0],
		"",
		nil,
		maxCRLResponseSize,
	)
	if err != nil {
		return fmt.Errorf("download CRL: %w", err)
	}

	list, err := x509.ParseRevocationList(body)
	if err != nil {
		return fmt.Errorf("parse CRL: %w", err)
	}
	if err := list.CheckSignatureFrom(issuer); err != nil {
		return fmt.Errorf("verify CRL signature: %w", err)
	}
	// Some valid producers encode an equivalent distinguished name with a
	// different DER string type. The signatures above prove the key
	// relationship; the parsed-name fallback preserves interoperability while
	// still rejecting a CRL that names another issuer.
	if !bytes.Equal(list.RawIssuer, issuer.RawSubject) &&
		list.Issuer.String() != issuer.Subject.String() {
		return errors.New("CRL issuer does not match the certificate issuer")
	}
	if err := validateCRLFreshness(list, time.Now()); err != nil {
		return err
	}
	return info.AddCRL(body)
}

func validateCRLFreshness(list *x509.RevocationList, now time.Time) error {
	if list == nil {
		return errors.New("CRL is nil")
	}
	if list.ThisUpdate.IsZero() {
		return errors.New("CRL has no thisUpdate")
	}
	if list.ThisUpdate.After(now.Add(revocationClockSkew)) {
		return errors.New("CRL thisUpdate is in the future")
	}
	if list.NextUpdate.IsZero() {
		return errors.New("CRL has no nextUpdate")
	}
	if list.NextUpdate.Before(list.ThisUpdate) {
		return errors.New("CRL nextUpdate is before thisUpdate")
	}
	if list.NextUpdate.Before(now.Add(-revocationClockSkew)) {
		return errors.New("CRL is stale")
	}
	return nil
}

func DefaultEmbedRevocationStatusFunction(cert, issuer *x509.Certificate, info *revocation.InfoArchival) error {
	if cert == nil {
		return errors.New("certificate is required")
	}
	if info == nil {
		return errors.New("revocation information destination is required")
	}
	// The signing pipeline invokes this callback for every certificate in the
	// chain, including the trust anchor. Without an issuer, neither OCSP nor a
	// CRL can be authenticated, so do not perform network access for that item.
	if issuer == nil {
		return nil
	}

	// This callback is an LTV evidence collector, not an authorization policy.
	// A cryptographically valid Revoked response is intentionally embedded so
	// verifiers can evaluate it. The signing/validation layer decides whether
	// that status must reject the operation.
	//
	// OCSP requires the issuer certificate, both to create the request and to
	// verify that the response belongs to this certificate.
	if len(cert.OCSPServer) > 0 {
		if err := embedOCSPRevocationStatus(cert, issuer, info); err != nil {
			return err
		}
	}

	// A CRL without its issuer cannot be authenticated and is therefore not
	// safe to embed.
	if len(cert.CRLDistributionPoints) > 0 {
		if err := embedCRLRevocationStatus(cert, issuer, info); err != nil {
			return err
		}
	}
	return nil
}
