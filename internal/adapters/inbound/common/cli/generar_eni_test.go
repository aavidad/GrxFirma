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
	"encoding/base64"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
)

func firmaCAdESPrueba(t *testing.T, datos []byte, implicita bool) []byte {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "Prueba ENI"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	doc, _ := domain.NewDocument("doc.pdf", datos, "application/pdf")
	opciones := map[string]string{}
	if implicita {
		opciones["mode"] = "implicit"
	}
	res, err := commonsigner.NewCAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign, Options: opciones,
	}, &commonsigner.LocalSigningKey{ID: "k", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatal(err)
	}
	return res.Data
}

func TestRunGenerarENI_CAdESImplicitaYExplicita(t *testing.T) {
	tmp := t.TempDir()
	documento := []byte("%PDF-1.7\ncontenido del documento")
	casos := []struct {
		nombre    string
		implicita bool
		original  bool
		tipo      string
	}{
		{"implicita", true, false, "TF05"},
		{"explicita", false, true, "TF04"},
	}
	for _, c := range casos {
		firma := filepath.Join(tmp, c.nombre+".csig")
		if err := os.WriteFile(firma, firmaCAdESPrueba(t, documento, c.implicita), 0o600); err != nil {
			t.Fatal(err)
		}
		args := []string{"-operacion", "generar-eni", "-entrada", firma,
			"-opcion", "eni.organo=L01180877", "-opcion", "eni.origen=ciudadano", "-opcion", "eni.tipoDocumental=TD14"}
		if c.original {
			orig := filepath.Join(tmp, "doc.pdf")
			_ = os.WriteFile(orig, documento, 0o600)
			args = append(args, "-original", orig)
		}
		var stdout, stderr strings.Builder
		a := New(nil, nil)
		a.Stdout, a.Stderr = &stdout, &stderr
		if rc := a.Run(context.Background(), args); rc != 0 {
			t.Fatalf("%s: rc=%d stderr=%s", c.nombre, rc, stderr.String())
		}
		xmlENI, err := os.ReadFile(filepath.Join(tmp, c.nombre+"_eni.xml"))
		if err != nil {
			t.Fatal(err)
		}
		texto := string(xmlENI)
		for _, esperado := range []string{
			"<enids:TipoFirma>" + c.tipo + "</enids:TipoFirma>",
			"<enifile:ValorBinario>" + base64.StdEncoding.EncodeToString(documento) + "</enifile:ValorBinario>",
			"<enifile:NombreFormato>PDF</enifile:NombreFormato>",
		} {
			if !strings.Contains(texto, esperado) {
				t.Errorf("%s: falta %s", c.nombre, esperado)
			}
		}
		if dir := os.Getenv("GRXFIRMA_ENI_SALIDA"); dir != "" {
			_ = os.WriteFile(filepath.Join(dir, "cli-"+c.nombre+".xml"), xmlENI, 0o600)
		}
	}

	// Una firma explícita sin el documento original se rechaza con motivo.
	firma := filepath.Join(tmp, "explicita.csig")
	var stdout, stderr strings.Builder
	a := New(nil, nil)
	a.Stdout, a.Stderr = &stdout, &stderr
	if rc := a.Run(context.Background(), []string{"-operacion", "generar-eni", "-entrada", firma, "-opcion", "eni.organo=L01180877", "-opcion", "eni.origen=ciudadano", "-sobrescribir", "forzar"}); rc == 0 || !strings.Contains(stderr.String(), "-original") {
		t.Fatalf("rc=%d stderr=%s", rc, stderr.String())
	}
}
