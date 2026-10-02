// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

func syntheticPublicCertificate(t *testing.T, usage x509.KeyUsage, expired bool) ([]byte, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	start, end := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	if expired {
		start, end = time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Destinatario sintético"}, NotBefore: start, NotAfter: end, KeyUsage: usage}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return der, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

func TestLibroDestinatariosPublicos(t *testing.T) {
	ctx := context.Background()
	book := NuevoLocalPublicRecipientBook(t.TempDir())
	der, private := syntheticPublicCertificate(t, x509.KeyUsageKeyEncipherment, false)
	recipient, err := book.Import(ctx, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	if err != nil {
		t.Fatal(err)
	}
	if recipient.Origin != "importado" {
		t.Fatalf("origen = %q", recipient.Origin)
	}
	if _, err := book.Import(ctx, der); err != nil {
		t.Fatal(err)
	}
	list, err := book.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("listado = %d, %v", len(list), err)
	}
	resolved, err := book.Resolve(ctx, []string{recipient.ID})
	if err != nil || len(resolved) != 1 {
		t.Fatalf("resolución = %d, %v", len(resolved), err)
	}
	info, err := os.Stat(filepath.Join(book.dir(), recipient.ID+".der"))
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		t.Fatalf("permisos = %v, %v", info, err)
	}
	if _, err := book.Import(ctx, append(der, private...)); err == nil || !strings.Contains(err.Error(), "clave privada") {
		t.Fatalf("clave privada aceptada: %v", err)
	}
	if err := book.Remove(ctx, recipient.ID); err != nil {
		t.Fatal(err)
	}
	list, err = book.List(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("baja = %d, %v", len(list), err)
	}
}

func TestLibroDestinatariosRechazaCertificadosNoAptos(t *testing.T) {
	book := NuevoLocalPublicRecipientBook(t.TempDir())
	for _, test := range []struct {
		name    string
		usage   x509.KeyUsage
		expired bool
	}{
		{"caducado", x509.KeyUsageKeyEncipherment, true},
		{"solo firma", x509.KeyUsageDigitalSignature, false},
		{"solo acuerdo", x509.KeyUsageKeyAgreement, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			der, _ := syntheticPublicCertificate(t, test.usage, test.expired)
			if _, err := book.Import(context.Background(), der); err == nil {
				t.Fatal("certificado no apto aceptado")
			}
		})
	}
}

func TestDestinatariosDuplicadosPriorizanElPropio(t *testing.T) {
	der, _ := syntheticPublicCertificate(t, x509.KeyUsageKeyEncipherment, false)
	propio := domain.ProtectionRecipient{ID: "propio", Origin: "propio", CertificateDER: der}
	importado := domain.ProtectionRecipient{ID: "importado", Origin: "importado", CertificateDER: der}
	got := dedupeRecipients([]domain.ProtectionRecipient{propio, importado})
	if len(got) != 1 || got[0].ID != "propio" {
		t.Fatalf("duplicado: %+v", got)
	}
}

func TestLibroOmiteCaducadoGuardadoYPermiteRetirarlo(t *testing.T) {
	book := NuevoLocalPublicRecipientBook(t.TempDir())
	der, _ := syntheticPublicCertificate(t, x509.KeyUsageKeyEncipherment, true)
	sum := sha256.Sum256(der)
	id := "x509-" + hex.EncodeToString(sum[:])
	if err := os.MkdirAll(book.dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(book.dir(), id+".der"), der, 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := book.List(context.Background())
	if err != nil || len(list) != 0 {
		t.Fatalf("caducado visible: %d, %v", len(list), err)
	}
	if err := book.Remove(context.Background(), id); err != nil {
		t.Fatal(err)
	}
}
