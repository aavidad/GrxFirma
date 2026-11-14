// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/localizador"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type catalogoUnCert struct{ ref domain.CertificateRef }

func (c catalogoUnCert) List(context.Context) ([]domain.CertificateRef, error) {
	return []domain.CertificateRef{c.ref}, nil
}

type clavesLocales struct{ clave *commonsigner.LocalSigningKey }

func (c clavesLocales) KeyFor(context.Context, domain.CertificateRef) (ports.SigningKey, error) {
	return c.clave, nil
}

type aprobarSiempre struct{}

func (aprobarSiempre) Request(context.Context, string) (bool, error) { return true, nil }

func claveLocalPrueba(t *testing.T, nombre string) *commonsigner.LocalSigningKey {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(16), Subject: pkix.Name{CommonName: nombre},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &commonsigner.LocalSigningKey{ID: "cert-b", Signer: priv, Certificate: cert}
}

// Lote 16c (KD-03 en la CLI): cofirmar una CAdES explícita sin el documento
// sigue funcionando, y el aviso ya no se queda solo en el registro: sale en
// la salida de error con el texto del catálogo. Recorre el caso de uso y el
// motor CAdES reales.
func TestRunFirma_CofirmaSinDocumentoAvisaEnLaSalidaDeError(t *testing.T) {
	tmp := t.TempDir()
	entrada := filepath.Join(tmp, "firmado.csig")
	if err := os.WriteFile(entrada, firmaCAdESPrueba(t, []byte("documento que no se enseña"), false), 0o600); err != nil {
		t.Fatal(err)
	}
	clave := claveLocalPrueba(t, "Cofirmante B")
	uc := application.NuevoSignDocumentUseCase(
		catalogoUnCert{ref: domain.CertificateRef{ID: "cert-b", Subject: "CN=Cofirmante B"}},
		clavesLocales{clave: clave},
		commonsigner.NewCAdESBESDetached(),
		aprobarSiempre{},
		nil,
		nil,
	)
	loc := localizador.Para("es")
	var stdout, stderr strings.Builder
	a := New(uc, nil).WithLocalizador(loc)
	a.Stdout, a.Stderr = &stdout, &stderr
	rc := a.Run(context.Background(), []string{
		"-entrada", entrada, "-salida", filepath.Join(tmp, "cofirmado.csig"),
		"-formato", "CAdES", "-accion", "cosign", "-certificado", "cert-b",
	})
	if rc != 0 {
		t.Fatalf("la cofirma debe hacerse como antes: rc=%d stderr=%s", rc, stderr.String())
	}
	aviso := loc.T(domain.AvisoCofirmaSinOriginal)
	if !strings.Contains(stderr.String(), loc.T("cli.aviso.prefix")+" "+aviso) {
		t.Fatalf("falta el aviso en la salida de error: %q", stderr.String())
	}
}

type loteConAvisos struct{}

func (loteConAvisos) Execute(_ context.Context, cmd application.ProcessBatchCommand) (application.BatchResult, error) {
	res := application.BatchResult{Errores: map[int]error{}}
	for i, job := range cmd.Jobs {
		r := application.SignResult{Result: domain.SignatureResult{Format: job.Format, Data: []byte("firmado")}}
		if i == 1 {
			r.Warnings = []string{domain.AvisoCRLNoAplicable}
		}
		res.Results = append(res.Results, r)
	}
	return res, nil
}

// El lote de la CLI dice qué documento lleva cada aviso (14-5: la CRL que no
// cubre al firmante).
func TestRunLote_AvisosPorDocumentoEnLaSalidaDeError(t *testing.T) {
	tmp := t.TempDir()
	for _, nombre := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(tmp, nombre), []byte(nombre), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	loc := localizador.Para("es")
	var stdout, stderr strings.Builder
	a := New(nil, loteConAvisos{}).WithLocalizador(loc)
	a.Stdout, a.Stderr = &stdout, &stderr
	rc := a.Run(context.Background(), []string{"-lote", tmp, "-salida", t.TempDir(), "-formato", "CAdES"})
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%s", rc, stderr.String())
	}
	want := loc.T("cli.aviso.prefix") + " b.txt: " + loc.T(domain.AvisoCRLNoAplicable)
	if !strings.Contains(stderr.String(), want) || strings.Contains(stderr.String(), "a.txt:") {
		t.Fatalf("salida de error = %q; se esperaba %q", stderr.String(), want)
	}
}
