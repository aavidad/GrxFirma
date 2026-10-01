package sign

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/digitorus/pdfsign/revocation"
	"golang.org/x/crypto/ocsp"
)

const testPublicAddress = "93.184.216.34"

func TestParseRevocationURLRejectsUnsafeTargets(t *testing.T) {
	t.Parallel()

	testCases := []string{
		"file:///tmp/revocation",
		"http://user:password@example.com/status",
		"http://127.0.0.1/status",
		"http://172.16.0.1/status",
		"http://169.254.169.254/latest/meta-data",
		"http://100.64.0.1/status",
		"http://224.0.0.1/status",
		"http://[::1]/status",
		"http://[fe80::1]/status",
		"http://[ff02::1]/status",
		"http://example.com/status#fragment",
	}

	for _, rawURL := range testCases {
		rawURL := rawURL
		t.Run(rawURL, func(t *testing.T) {
			t.Parallel()
			if _, err := parseRevocationURL(rawURL); err == nil {
				t.Fatalf("parseRevocationURL(%q) accepted an unsafe target", rawURL)
			}
		})
	}
}

func TestValidatePublicIPBlocksSpecialUseRanges(t *testing.T) {
	t.Parallel()

	blocked := []string{
		"0.1.2.3",
		"192.0.0.1",
		"192.0.2.1",
		"192.88.99.1",
		"198.18.0.1",
		"198.51.100.1",
		"203.0.113.1",
		"240.0.0.1",
		"255.255.255.255",
		"::192.0.2.1",
		"::ffff:127.0.0.1",
		"64:ff9b::c000:201",
		"64:ff9b:1::1",
		"100::1",
		"2001::1",
		"2001:1::1",
		"2001:2::1",
		"2001:10::1",
		"2001:20::1",
		"2001:db8::1",
		"2002:c000:201::1",
		"3fff::1",
		"5f00::1",
		"fec0::1",
	}
	for _, rawIP := range blocked {
		rawIP := rawIP
		t.Run("blocked_"+rawIP, func(t *testing.T) {
			t.Parallel()
			if err := validatePublicIP(net.ParseIP(rawIP)); err == nil {
				t.Fatalf("validatePublicIP(%q) accepted a special-use address", rawIP)
			}
		})
	}

	allowed := []string{"8.8.8.8", "93.184.216.34", "2606:4700:4700::1111"}
	for _, rawIP := range allowed {
		rawIP := rawIP
		t.Run("allowed_"+rawIP, func(t *testing.T) {
			t.Parallel()
			if err := validatePublicIP(net.ParseIP(rawIP)); err != nil {
				t.Fatalf("validatePublicIP(%q) error = %v", rawIP, err)
			}
		})
	}
}

func TestRevocationFetcherBlocksDNSRebindingBeforeDial(t *testing.T) {
	t.Parallel()

	var dialed atomic.Bool
	fetcher := revocationFetcher{
		lookupIP: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("192.0.2.10")}}, nil
		},
		dialContext: func(context.Context, string, string) (net.Conn, error) {
			dialed.Store(true)
			return nil, errors.New("unexpected dial")
		},
		timeout:             time.Second,
		responseHeaderLimit: time.Second,
		tlsHandshakeLimit:   time.Second,
		maxRedirects:        1,
	}

	_, err := fetcher.fetch(context.Background(), http.MethodGet, "http://rebind.test/status", "", nil, 32)
	if err == nil || !strings.Contains(err.Error(), "address is blocked") {
		t.Fatalf("fetch() error = %v, want reserved-address rejection", err)
	}
	if dialed.Load() {
		t.Fatal("network dial occurred after DNS resolved to a reserved address")
	}
}

func TestRevocationFetcherAllowsSameOriginRedirect(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/final", http.StatusTemporaryRedirect)
		case "/final":
			_, _ = io.WriteString(w, "verified")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	fetcher, logicalURL := fetcherForServer(t, server)
	body, err := fetcher.fetch(context.Background(), http.MethodGet, logicalURL+"/start", "", nil, 32)
	if err != nil {
		t.Fatalf("fetch() error = %v", err)
	}
	if string(body) != "verified" {
		t.Fatalf("fetch() body = %q, want verified", body)
	}
}

func TestRevocationFetcherRejectsCrossOriginAndDowngradeRedirects(t *testing.T) {
	t.Parallel()

	var redirected atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "http://other.test"+r.URL.Path, http.StatusTemporaryRedirect)
			return
		}
		redirected.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	fetcher, logicalURL := fetcherForServer(t, server)
	_, err := fetcher.fetch(context.Background(), http.MethodGet, logicalURL+"/start", "", nil, 32)
	if err == nil || !strings.Contains(err.Error(), "changed origin or scheme") {
		t.Fatalf("fetch() error = %v, want cross-origin rejection", err)
	}
	if redirected.Load() {
		t.Fatal("cross-origin redirect reached the target handler")
	}

	httpsURL, _ := url.Parse("https://revocation.example/status")
	httpURL, _ := url.Parse("http://revocation.example/status")
	if sameRevocationOrigin(httpsURL, httpURL) {
		t.Fatal("HTTPS-to-HTTP redirect was considered same-origin")
	}
}

func TestRevocationFetcherLimitsRedirects(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/again", http.StatusTemporaryRedirect)
	}))
	defer server.Close()

	fetcher, logicalURL := fetcherForServer(t, server)
	fetcher.maxRedirects = 2
	_, err := fetcher.fetch(context.Background(), http.MethodGet, logicalURL+"/again", "", nil, 32)
	if err == nil || !strings.Contains(err.Error(), "too many revocation redirects") {
		t.Fatalf("fetch() error = %v, want redirect limit", err)
	}
}

func TestRevocationFetcherEnforcesStatusAndSize(t *testing.T) {
	t.Parallel()

	t.Run("status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "failure", http.StatusBadGateway)
		}))
		defer server.Close()

		fetcher, logicalURL := fetcherForServer(t, server)
		_, err := fetcher.fetch(context.Background(), http.MethodGet, logicalURL, "", nil, 32)
		if err == nil || !strings.Contains(err.Error(), "HTTP status 502") {
			t.Fatalf("fetch() error = %v, want HTTP status rejection", err)
		}
	})

	t.Run("streamed body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "")
			_, _ = io.WriteString(w, strings.Repeat("x", 33))
		}))
		defer server.Close()

		fetcher, logicalURL := fetcherForServer(t, server)
		_, err := fetcher.fetch(context.Background(), http.MethodGet, logicalURL, "", nil, 32)
		if err == nil || !strings.Contains(err.Error(), "exceeds 32 bytes") {
			t.Fatalf("fetch() error = %v, want size rejection", err)
		}
	})
}

func TestRevocationFetcherEnforcesTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	fetcher, logicalURL := fetcherForServer(t, server)
	fetcher.timeout = 50 * time.Millisecond
	fetcher.responseHeaderLimit = time.Second
	_, err := fetcher.fetch(context.Background(), http.MethodGet, logicalURL, "", nil, 32)
	if err == nil || !strings.Contains(err.Error(), "Client.Timeout") {
		t.Fatalf("fetch() error = %v, want client timeout", err)
	}
}

func TestRevocationFreshnessValidation(t *testing.T) {
	t.Parallel()

	now := time.Now()
	if err := validateOCSPFreshness(&ocsp.Response{
		ThisUpdate: now.Add(2 * revocationClockSkew),
		ProducedAt: now,
		NextUpdate: now.Add(time.Hour),
	}, now); err == nil {
		t.Fatal("OCSP response with future thisUpdate was accepted")
	}
	if err := validateOCSPFreshness(&ocsp.Response{
		ThisUpdate: now.Add(-time.Hour),
		ProducedAt: now.Add(-time.Hour),
		NextUpdate: now.Add(-2 * revocationClockSkew),
	}, now); err == nil {
		t.Fatal("stale OCSP response was accepted")
	}
	if err := validateOCSPFreshness(&ocsp.Response{
		ThisUpdate: now.Add(-maxOCSPAgeWithoutNextUpdate - 2*revocationClockSkew),
		ProducedAt: now.Add(-maxOCSPAgeWithoutNextUpdate - 2*revocationClockSkew),
	}, now); err == nil || !strings.Contains(err.Error(), "older than") {
		t.Fatalf("old OCSP response without nextUpdate error = %v, want replay rejection", err)
	}
	if err := validateOCSPFreshness(&ocsp.Response{
		ThisUpdate: now,
		ProducedAt: now.Add(-2 * revocationClockSkew),
		NextUpdate: now.Add(time.Hour),
	}, now); err == nil {
		t.Fatal("OCSP response with thisUpdate after producedAt was accepted")
	}
	if err := validateOCSPFreshness(&ocsp.Response{
		ThisUpdate: now,
		ProducedAt: now,
		NextUpdate: now.Add(-time.Minute),
	}, now); err == nil {
		t.Fatal("OCSP response with nextUpdate before thisUpdate was accepted")
	}
	if err := validateCRLFreshness(&x509.RevocationList{
		ThisUpdate: now.Add(-time.Hour),
	}, now); err == nil {
		t.Fatal("CRL without nextUpdate was accepted")
	}
	if err := validateCRLFreshness(&x509.RevocationList{
		ThisUpdate: now.Add(time.Minute),
		NextUpdate: now,
	}, now); err == nil {
		t.Fatal("CRL with nextUpdate before thisUpdate was accepted")
	}
}

func TestEmbedOCSPRevocationStatusValidatesBeforeAdding(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	issuer, issuerKey, cert := newRevocationCertificateChain(t, now)
	responseDER, err := ocsp.CreateResponse(issuer, issuer, ocsp.Response{
		Status:       ocsp.Good,
		SerialNumber: cert.SerialNumber,
		ThisUpdate:   now.Add(-time.Minute),
		NextUpdate:   now.Add(time.Hour),
		ProducedAt:   now.Add(-time.Minute),
	}, issuerKey)
	if err != nil {
		t.Fatalf("create OCSP response: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("OCSP method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/ocsp-response")
		_, _ = w.Write(responseDER)
	}))
	defer server.Close()

	fetcher, logicalURL := fetcherForServer(t, server)
	cert.OCSPServer = []string{logicalURL}
	var info revocation.InfoArchival
	if err := embedOCSPRevocationStatusWithFetcher(cert, issuer, &info, fetcher); err != nil {
		t.Fatalf("embed OCSP: %v", err)
	}
	if len(info.OCSP) != 1 {
		t.Fatalf("embedded OCSP responses = %d, want 1", len(info.OCSP))
	}

	info.OCSP = nil
	cert.SerialNumber = new(big.Int).Add(cert.SerialNumber, big.NewInt(1))
	if err := embedOCSPRevocationStatusWithFetcher(cert, issuer, &info, fetcher); err == nil {
		t.Fatal("OCSP response for another certificate was accepted")
	}
	if len(info.OCSP) != 0 {
		t.Fatal("invalid OCSP response was embedded")
	}
}

func TestEmbedCRLRevocationStatusVerifiesIssuerBeforeAdding(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	issuer, issuerKey, cert := newRevocationCertificateChain(t, now)
	crlDER := createCRL(t, issuer, issuerKey, now)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(crlDER)
	}))
	defer server.Close()

	fetcher, logicalURL := fetcherForServer(t, server)
	cert.CRLDistributionPoints = []string{logicalURL}
	var info revocation.InfoArchival
	if err := embedCRLRevocationStatusWithFetcher(cert, issuer, &info, fetcher); err != nil {
		t.Fatalf("embed CRL: %v", err)
	}
	if len(info.CRL) != 1 {
		t.Fatalf("embedded CRLs = %d, want 1", len(info.CRL))
	}

	info.CRL = nil
	crlDER[len(crlDER)-1] ^= 0xff
	if err := embedCRLRevocationStatusWithFetcher(cert, issuer, &info, fetcher); err == nil {
		t.Fatal("CRL with an invalid signature was accepted")
	}
	if len(info.CRL) != 0 {
		t.Fatal("unverified CRL was embedded")
	}
}

func TestEmbedCRLRevocationStatusRequiresIssuer(t *testing.T) {
	t.Parallel()

	var info revocation.InfoArchival
	err := embedCRLRevocationStatusWithFetcher(
		&x509.Certificate{CRLDistributionPoints: []string{"https://example.com/list.crl"}},
		nil,
		&info,
		revocationFetcher{},
	)
	if err == nil || !strings.Contains(err.Error(), "issuer") {
		t.Fatalf("embed CRL error = %v, want issuer requirement", err)
	}
	if len(info.CRL) != 0 {
		t.Fatal("CRL was embedded without an issuer")
	}
}

func TestDefaultEmbedRevocationStatusSkipsTrustAnchorWithoutIssuer(t *testing.T) {
	t.Parallel()

	trustAnchor := &x509.Certificate{
		IsCA:                  true,
		BasicConstraintsValid: true,
		OCSPServer:            []string{"http://127.0.0.1/ocsp"},
		CRLDistributionPoints: []string{"http://127.0.0.1/root.crl"},
	}
	var info revocation.InfoArchival
	if err := DefaultEmbedRevocationStatusFunction(trustAnchor, nil, &info); err != nil {
		t.Fatalf("trust anchor revocation collection error = %v, want skip", err)
	}
	if len(info.OCSP) != 0 || len(info.CRL) != 0 {
		t.Fatal("revocation evidence was added for a trust anchor without issuer")
	}
}

func TestEmbedRevocationRejectsUnrelatedIssuerBeforeNetwork(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	_, _, cert := newRevocationCertificateChain(t, now)
	unrelatedIssuer, _, unrelatedCert := newRevocationCertificateChain(t, now)
	if cert.SerialNumber.Cmp(unrelatedCert.SerialNumber) != 0 {
		t.Fatal("test setup requires unrelated certificates with the same serial number")
	}
	cert.OCSPServer = []string{"http://revocation.test/ocsp"}
	cert.CRLDistributionPoints = []string{"http://revocation.test/list.crl"}

	var dialed atomic.Bool
	fetcher := revocationFetcher{
		lookupIP: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP(testPublicAddress)}}, nil
		},
		dialContext: func(context.Context, string, string) (net.Conn, error) {
			dialed.Store(true)
			return nil, errors.New("unexpected dial")
		},
	}

	var info revocation.InfoArchival
	if err := embedOCSPRevocationStatusWithFetcher(cert, unrelatedIssuer, &info, fetcher); err == nil ||
		!strings.Contains(err.Error(), "not issued") {
		t.Fatalf("OCSP unrelated-issuer error = %v, want issuer rejection", err)
	}
	if err := embedCRLRevocationStatusWithFetcher(cert, unrelatedIssuer, &info, fetcher); err == nil ||
		!strings.Contains(err.Error(), "not issued") {
		t.Fatalf("CRL unrelated-issuer error = %v, want issuer rejection", err)
	}
	if dialed.Load() {
		t.Fatal("network dial occurred before rejecting an unrelated issuer")
	}
	if len(info.OCSP) != 0 || len(info.CRL) != 0 {
		t.Fatal("revocation evidence was embedded for an unrelated issuer")
	}
}

func fetcherForServer(t *testing.T, server *httptest.Server) (revocationFetcher, string) {
	t.Helper()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	testAddress := serverURL.Host
	dialer := &net.Dialer{Timeout: time.Second}
	return revocationFetcher{
		lookupIP: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP(testPublicAddress)}}, nil
		},
		dialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, testAddress)
		},
		timeout:             time.Second,
		responseHeaderLimit: time.Second,
		tlsHandshakeLimit:   time.Second,
		maxRedirects:        2,
	}, "http://revocation.test:" + serverURL.Port()
}

func newRevocationCertificateChain(
	t *testing.T,
	now time.Time,
) (*x509.Certificate, crypto.Signer, *x509.Certificate) {
	t.Helper()

	issuerKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate issuer key: %v", err)
	}
	issuerTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Revocation Test Issuer"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
	}
	issuerDER, err := x509.CreateCertificate(rand.Reader, issuerTemplate, issuerTemplate, issuerKey.Public(), issuerKey)
	if err != nil {
		t.Fatalf("create issuer certificate: %v", err)
	}
	issuer, err := x509.ParseCertificate(issuerDER)
	if err != nil {
		t.Fatalf("parse issuer certificate: %v", err)
	}

	certKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	certTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "Revocation Test Leaf"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(12 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, certTemplate, issuer, certKey.Public(), issuerKey)
	if err != nil {
		t.Fatalf("create leaf certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("parse leaf certificate: %v", err)
	}
	return issuer, issuerKey, cert
}

func createCRL(t *testing.T, issuer *x509.Certificate, issuerKey crypto.Signer, now time.Time) []byte {
	t.Helper()

	crlDER, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number:     big.NewInt(1),
		ThisUpdate: now.Add(-time.Minute),
		NextUpdate: now.Add(time.Hour),
	}, issuer, issuerKey)
	if err != nil {
		t.Fatalf("create CRL: %v", err)
	}
	return crlDER
}
