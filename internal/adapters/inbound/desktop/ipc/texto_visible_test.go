// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/json"
	"testing"
	"time"
	"unicode"

	"grxfirma/internal/adapters/outbound/desktop/cscremota"
	"grxfirma/internal/domain"
)

// sinCaracteresDeFormato falla si el JSON que recibe la interfaz lleva un
// control de dirección, uno de anchura cero o cualquier otro carácter Cf.
func sinCaracteresDeFormato(t *testing.T, que string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var texto string
	// Se decodifica para ver los caracteres y no los escapes \uXXXX.
	var generico any
	if err := json.Unmarshal(data, &generico); err != nil {
		t.Fatal(err)
	}
	var recorrer func(any)
	recorrer = func(x any) {
		switch y := x.(type) {
		case string:
			texto += y + "\n"
		case []any:
			for _, e := range y {
				recorrer(e)
			}
		case map[string]any:
			for _, e := range y {
				recorrer(e)
			}
		}
	}
	recorrer(generico)
	for _, r := range texto {
		if r != '\n' && (unicode.IsControl(r) || unicode.Is(unicode.Cf, r)) {
			t.Fatalf("%s lleva el carácter U+%04X: %s", que, r, data)
		}
	}
}

const (
	cnConDireccion = "CN=Ana\u202eaicraG"
	emisorRaro     = "CN=Emisor\u200b de\u2066 pruebas\u2069"
)

// TestCertificatesIPCQuitaControlesDeDireccion: ID-05. Ni el titular ni el
// emisor de un certificado llegan a la interfaz con controles de dirección o
// de anchura cero; el identificador se conserva tal cual para firmar.
func TestCertificatesIPCQuitaControlesDeDireccion(t *testing.T) {
	m := &Manejador{Catalogo: &stubCatalogo{certs: []domain.CertificateRef{{
		ID:           "id-\u202e-opaco",
		Subject:      cnConDireccion,
		Issuer:       emisorRaro,
		Organizacion: "Org\u202d",
		NIF:          "IDCES-1\u200d2",
		NotAfter:     time.Now().Add(48 * time.Hour),
		Fingerprint:  "fp",
	}}}}
	resp := m.despachar(context.Background(), peticion{Action: "certificates"})
	if !resp.OK {
		t.Fatalf("certificates: %+v", resp)
	}
	certs := resp.Data.([]certJSON)
	if len(certs) != 1 {
		t.Fatalf("certificados: %+v", certs)
	}
	c := certs[0]
	if c.ID != "id-\u202e-opaco" {
		t.Fatalf("el identificador no debe tocarse: %q", c.ID)
	}
	c.ID = ""
	sinCaracteresDeFormato(t, "certificates", c)
	if c.SubjectName != "CN=Ana?aicraG" || c.Issuer != "CN=Emisor? de? pruebas?" {
		t.Fatalf("titular o emisor sin sanear: %q %q", c.SubjectName, c.Issuer)
	}
}

// TestDescribirCredencialCSCQuitaControlesDeDireccion: ID-05 en csc_connect,
// que es por donde llega el nombre que pone el prestador remoto.
func TestDescribirCredencialCSCQuitaControlesDeDireccion(t *testing.T) {
	d := describirCredencialCSC(cscremota.CredencialRemota{Ref: domain.CertificateRef{
		ID: "abc", Subject: cnConDireccion, Issuer: emisorRaro,
	}})
	sinCaracteresDeFormato(t, "csc_connect", d)
	if d.CertificateID != "abc" || d.Subject != "CN=Ana?aicraG" {
		t.Fatalf("credencial: %+v", d)
	}
}

// TestFirmantesVerificacionIPCQuitaControlesDeDireccion: el nombre de quien
// firmó un documento también viene de fuera y se pinta en la misma pantalla.
func TestFirmantesVerificacionIPCQuitaControlesDeDireccion(t *testing.T) {
	out := verificacionFirmantesIPC([]domain.VerificationSignerSummary{{
		ID: "f1", Subject: cnConDireccion, Issuer: emisorRaro,
	}})
	sinCaracteresDeFormato(t, "verify", out)
}
