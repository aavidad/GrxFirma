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

	"grxfirma/internal/adapters/outbound/common/eni"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type catalogoExpediente struct{ ref domain.CertificateRef }

func (c catalogoExpediente) List(context.Context) ([]domain.CertificateRef, error) {
	return []domain.CertificateRef{c.ref}, nil
}

type clavesExpediente struct{ clave *commonsigner.LocalSigningKey }

func (c clavesExpediente) KeyFor(context.Context, domain.CertificateRef) (ports.SigningKey, error) {
	return c.clave, nil
}

func TestRunGenerarExpediente_FirmaElIndice(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(11), Subject: pkix.Name{CommonName: "Firmante expediente"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
	}
	der, _ := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	cert, _ := x509.ParseCertificate(der)
	ref := domain.CertificateRef{ID: "c1", Subject: "CN=Firmante expediente", HasSigningKey: true, NotAfter: cert.NotAfter}

	dir := t.TempDir()
	for i, contenido := range []string{"%PDF-1.7 a", "%PDF-1.7 b"} {
		x, err := eni.Generar(eni.Documento{
			Contenido: []byte(contenido), NombreFormato: "PDF",
			Metadatos: eni.Metadatos{Organos: []string{"L01180877"}, EstadoElaboracion: "EE01", TipoDocumental: "TD99"},
			Firmas:    []eni.Firma{{Tipo: eni.FirmaPAdES}},
		}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(dir, "doc"+string(rune('1'+i))+".xml"), x, 0o600)
	}
	salida := filepath.Join(t.TempDir(), "expediente.xml")
	a := New(nil, nil).WithCatalogo(catalogoExpediente{ref}).WithClaves(clavesExpediente{&commonsigner.LocalSigningKey{ID: "c1", Signer: priv, Certificate: cert}})
	var stdout, stderr strings.Builder
	a.Stdout, a.Stderr = &stdout, &stderr
	rc := a.Run(context.Background(), []string{"-operacion", "generar-expediente", "-lote", dir, "-salida", salida,
		"-opcion", "exp.organo=L01180877", "-opcion", "exp.clasificacion=L01180877_PRO_LICENCIAS", "-opcion", "exp.estado=E02"})
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%s", rc, stderr.String())
	}
	exp, err := os.ReadFile(salida)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(exp), "<eniconexpind:DocumentoIndizado") != 2 || !strings.Contains(string(exp), "<ds:Signature") {
		t.Fatalf("expediente inesperado:\n%s", exp)
	}
	doc, _ := domain.NewDocument("expediente.xml", exp, "application/xml")
	res, _, err := commonsigner.NewXAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
	if err != nil || res.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("la firma del índice debe verificarse: %v %+v", err, res.Integrity)
	}
	stdout.Reset()
	stderr.Reset()
	if rc := a.Run(context.Background(), []string{"-operacion", "validar-eni", "-entrada", salida}); rc != 0 {
		t.Fatalf("la validación CLI del expediente generado falló: rc=%d %s %s", rc, stdout.String(), stderr.String())
	}
	if d := os.Getenv("GRXFIRMA_ENI_SALIDA"); d != "" {
		_ = os.WriteFile(filepath.Join(d, "cli-expediente.xml"), exp, 0o600)
	}
}
