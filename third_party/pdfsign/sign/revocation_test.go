package sign

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/digitorus/pdfsign/revocation"
)

// El certificado se genera en cada ejecución; el árbol no almacena identidades ni claves privadas.
func TestEmbedRevocationStatusWithoutEndpoints(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	certTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Pruebas Ficticio"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, certTemplate, certTemplate, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	var archival revocation.InfoArchival
	if err := DefaultEmbedRevocationStatusFunction(cert, cert, &archival); err != nil {
		t.Fatalf("certificado sin puntos de revocación: %v", err)
	}
	if len(archival.OCSP) != 0 || len(archival.CRL) != 0 {
		t.Fatalf("respuesta inesperada para certificado sin puntos de revocación: OCSP=%d CRL=%d", len(archival.OCSP), len(archival.CRL))
	}
}
