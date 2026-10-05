package sign

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/digitorus/pdf"
	"github.com/mattetti/filebuffer"
)

// countingSigner counts how many times the PDF is signed: with a remote key
// every call is a new authorization, so the placeholder must fit the first
// signature.
type countingSigner struct {
	crypto.Signer
	calls atomic.Int32
}

func (c *countingSigner) Sign(r io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	c.calls.Add(1)
	return c.Signer.Sign(r, digest, opts)
}

func certificateFor(t *testing.T, key crypto.Signer) *x509.Certificate {
	t.Helper()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(7),
		Subject:      pkix.Name{CommonName: "Firmante con nombre largo para ocupar espacio en los atributos firmados"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func TestPlaceholderFitsFirstSignature(t *testing.T) {
	rsa4096, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		t.Fatal(err)
	}
	p521, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../testfiles/testfile20.pdf")
	if err != nil {
		t.Fatal(err)
	}
	for name, key := range map[string]crypto.Signer{"rsa4096": rsa4096, "p521": p521} {
		t.Run(name, func(t *testing.T) {
			signer := &countingSigner{Signer: key}
			input := filebuffer.New(data)
			rdr, err := pdf.NewReader(input, int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			err = Sign(input, io.Discard, rdr, int64(len(data)), SignData{
				Signature: SignDataSignature{
					Info:     SignDataSignatureInfo{Name: "Prueba", Date: time.Now()},
					CertType: ApprovalSignature,
				},
				DigestAlgorithm: crypto.SHA512,
				Signer:          signer,
				Certificate:     certificateFor(t, key),
			})
			if err != nil {
				t.Fatalf("Sign: %v", err)
			}
			if n := signer.calls.Load(); n != 1 {
				t.Fatalf("the PDF was signed %d times; the placeholder must fit the first signature", n)
			}
		})
	}
}
