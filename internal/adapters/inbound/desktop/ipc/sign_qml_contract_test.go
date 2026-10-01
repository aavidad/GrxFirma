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
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/testsupport/pdffixture"
)

type aprobacionFirmaContrato struct{}

func (aprobacionFirmaContrato) Request(context.Context, string) (bool, error) { return true, nil }

// Reproduce el objeto params que construyen main.qml/buildSignPayload e
// ipcbridge.cpp/signFileAdvanced, con las opciones PAdES predeterminadas.
func TestFirmaPAdESPeticionQtContraMotorReal(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(96), Subject: pkix.Name{CommonName: "Firma sintética"},
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
	ref := domain.CertificateRef{ID: "synthetic", DER: der, NotAfter: template.NotAfter, HasSigningKey: true}
	catalog := &stubCatalogo{certs: []domain.CertificateRef{ref}}
	useCase := application.NuevoSignDocumentUseCase(catalog, &stubClaves{key: desktopsigner.NuevaClaveLocal(key, cert)}, desktopsigner.NuevoMotorFirmaGo(nil), aprobacionFirmaContrato{}, nil, nil)
	dir := t.TempDir()
	input := filepath.Join(dir, "synthetic.pdf")
	if err := os.WriteFile(input, pdffixture.Minimal(), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		seal string
	}{
		{"sin_sello", ""},
		{"con_sello", `,"visibleSeal":{"x":0.62,"y":0.04,"w":0.34,"h":0.12,"pageWidth":595,"pageHeight":842,"page":1,"rotation":0,"keepText":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &Manejador{Catalogo: catalog, Firmar: useCase}
			if resp := m.handleCertificados(context.Background(), "certificates"); !resp.OK {
				t.Fatalf("certificates: %s", resp.Error)
			}
			output := filepath.Join(dir, tc.name+".pdf")
			raw := []byte(`{"inputPath":` + jsonString(input) + `,"outputPath":` + jsonString(output) + `,"certificateIndex":0,"certificateId":"synthetic","format":"pades","action":"sign","strictCompat":false,"overwrite":"rename","saveToDisk":true,"returnSignatureB64":false,"qrContent":"","reason":"","location":"","contactInfo":"","extraOptions":{"profile":"baseline","subfilter":"etsi","visibleSealLogo":"institucional"}` + tc.seal + `}`)
			resp := m.despachar(context.Background(), peticion{Action: "sign", Params: raw})
			if !resp.OK {
				t.Fatalf("sign IPC: %s", resp.Error)
			}
			if got := resp.Data.(resultadoFirma).OutputPath; got != output {
				t.Fatalf("ruta devuelta %q, want %q", got, output)
			}
			if _, err := os.Stat(output); err != nil {
				t.Fatalf("salida: %v", err)
			}
		})
	}
	// El camino CLI crea el mismo comando sin los campos de interfaz.
	cli, err := application.NewSignCommand("synthetic.pdf", pdffixture.Minimal(), "application/pdf", "pades", "sign", ref.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := useCase.Execute(context.Background(), cli); err != nil {
		t.Fatalf("firma directa CLI: %v", err)
	}
}

func jsonString(value string) string { data, _ := json.Marshal(value); return string(data) }
