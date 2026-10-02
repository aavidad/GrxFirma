// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func escribirParTLSPropio(t *testing.T, notBefore, notAfter time.Time) (string, string, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(7),
		Subject:      pkix.Name{CommonName: "validador.prueba"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "srv.crt.pem"), filepath.Join(dir, "srv.key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	return certFile, keyFile, cert
}

func TestStartTLS13ServerWithKeyPairUsaCertificadoPropio(t *testing.T) {
	certFile, keyFile, cert := escribirParTLSPropio(t, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, err := StartTLS13ServerWithKeyPair(ctx, "127.0.0.1:0", http.NotFoundHandler(), certFile, keyFile)
	if err != nil {
		t.Fatalf("arranque con par propio: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	conn, err := tls.Dial("tcp", srv.Addr, &tls.Config{RootCAs: pool, ServerName: "localhost", MinVersion: tls.VersionTLS13})
	if err != nil {
		t.Fatalf("el cliente no acepta el certificado propio: %v", err)
	}
	defer conn.Close()
	if got := conn.ConnectionState().PeerCertificates[0].SerialNumber.Int64(); got != 7 {
		t.Fatalf("certificado servido inesperado: serie %d", got)
	}
	if _, err := tls.Dial("tcp", srv.Addr, &tls.Config{RootCAs: pool, ServerName: "localhost", MaxVersion: tls.VersionTLS12}); err == nil {
		t.Fatal("el validador con par propio acepta TLS 1.2")
	}
}

func TestStartTLS13ServerWithKeyPairRechazaCertificadoNoVigente(t *testing.T) {
	certFile, keyFile, _ := escribirParTLSPropio(t, time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour))
	_, err := StartTLS13ServerWithKeyPair(context.Background(), "127.0.0.1:0", http.NotFoundHandler(), certFile, keyFile)
	if err == nil || !strings.Contains(err.Error(), "vigente") {
		t.Fatalf("se esperaba rechazo por vigencia, obtenido %v", err)
	}
}

func TestStartTLS13ServerWithKeyPairRechazaClaveAjena(t *testing.T) {
	certFile, _, _ := escribirParTLSPropio(t, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	_, otraClave, _ := escribirParTLSPropio(t, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if _, err := StartTLS13ServerWithKeyPair(context.Background(), "127.0.0.1:0", http.NotFoundHandler(), certFile, otraClave); err == nil {
		t.Fatal("se aceptó una clave que no corresponde al certificado")
	}
}
