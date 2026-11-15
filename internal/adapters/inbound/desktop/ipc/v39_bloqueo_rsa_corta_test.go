// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"grxfirma/internal/testsupport/pdffixture"
)

// Lote 39a, puntos 2 y 3: firma con una clave RSA de 1024 bits y la petición
// que arma Qt (signFileAdvanced), contra el motor de firma real. Sin el
// ajuste es un fallo tipado sin diagnóstico de avería (el asistente guiado no
// debe abrirse encima del aviso); con el ajuste, el aviso de uso llega en los
// warnings de la respuesta, que es lo que Qt pinta en el aviso común.
func firmaRSACortaQtV39(t *testing.T) (*Manejador, context.Context, peticion) {
	t.Helper()
	machinepolicy.UnsetForTest(t, "permitir_rsa_corta")
	key, err := rsa.GenerateKey(rand.Reader, 1024) // nosemgrep: go.lang.security.audit.crypto.use_of_weak_rsa_key.use-of-weak-rsa-key -- la prueba necesita una clave RSA corta para el bloqueo rsa_corta.
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1024), Subject: pkix.Name{CommonName: "Firmante RSA corta"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	ref := domain.CertificateRef{ID: "rsa-corta", DER: der, NotAfter: template.NotAfter, HasSigningKey: true}
	catalog := &stubCatalogo{certs: []domain.CertificateRef{ref}}
	useCase := application.NuevoSignDocumentUseCase(catalog, &stubClaves{key: desktopsigner.NuevaClaveLocal(key, cert)}, desktopsigner.NuevoMotorFirmaGo(nil), aprobacionFirmaContrato{}, nil, nil)
	dir := t.TempDir()
	input := filepath.Join(dir, "documento.pdf")
	if err := os.WriteFile(input, pdffixture.Minimal(), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &Manejador{ConfigDir: dir, Catalogo: catalog, Firmar: useCase}
	ctx := withUnsafeRetryOwner(conEstadoInterfaz(context.Background(), interfazVerificada))
	t.Cleanup(func() { m.clearUnsafeRetries(ctx) })
	if resp := m.handleCertificados(ctx, "certificates"); !resp.OK {
		t.Fatalf("certificates: %s", resp.Error)
	}
	output := filepath.Join(dir, "documento_firmado.pdf")
	raw := []byte(`{"inputPath":` + jsonString(input) + `,"outputPath":` + jsonString(output) + `,"certificateIndex":0,"certificateId":"rsa-corta","format":"pades","action":"sign","strictCompat":false,"overwrite":"rename","saveToDisk":true,"returnSignatureB64":false,"qrContent":"","reason":"","location":"","contactInfo":"","extraOptions":{"sealLanguage":"es"}}`)
	return m, ctx, peticion{Action: "sign", Params: raw}
}

func TestV39FirmaRSACortaBloqueadaSinDiagnosticoDeAveria(t *testing.T) {
	excepciones.SetForTest(t, "rsa_corta", false)
	m, ctx, p := firmaRSACortaQtV39(t)
	resp := m.despachar(ctx, p)
	if resp.OK || resp.ErrorCode != "unsafe_exception_blocked" || resp.ExceptionID != "rsa_corta" ||
		resp.Activable == nil || !*resp.Activable {
		t.Fatalf("la firma con RSA corta debe llegar como bloqueo activable: %+v", resp)
	}
	if resp.Diagnostic != nil {
		t.Fatalf("un bloqueo no es una avería del certificado: %+v", resp.Diagnostic)
	}
}

func TestV39FirmaRSACortaConAjusteLlevaElAvisoComun(t *testing.T) {
	excepciones.SetForTest(t, "rsa_corta", true)
	m, ctx, p := firmaRSACortaQtV39(t)
	resp := m.despachar(ctx, p)
	if !resp.OK {
		t.Fatalf("con el ajuste debe firmar: %+v", resp)
	}
	if n := countString(resp.WarningKeys, excepciones.Warning("rsa_corta")); n != 1 {
		t.Fatalf("el aviso de RSA corta debe ir una vez en warnings: %v", resp.WarningKeys)
	}
}
