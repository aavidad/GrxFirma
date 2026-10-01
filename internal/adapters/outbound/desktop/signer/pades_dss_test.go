// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/digitorus/pdf"

	"crypto/x509"

	"grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/testsupport/exttools"
)

// abrirDSS parsea el PDF firmado y retorna el diccionario /DSS del catálogo.
func abrirDSS(t *testing.T, data []byte) pdf.Value {
	t.Helper()
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("el PDF firmado no se pudo parsear: %v", err)
	}
	dss := reader.Trailer().Key("Root").Key("DSS")
	if dss.IsNull() {
		t.Fatal("el catálogo no contiene /DSS")
	}
	return dss
}

func firmarPAdESConLTV(t *testing.T, contenido []byte, evidencia ports.RevocationEvidence) []byte {
	t.Helper()

	leaf, ca, leafKey, _ := commonsignerTestChain(t)
	clave := signer.NuevaClaveLocalConCadena(leafKey, leaf, []*x509.Certificate{ca})
	motor := signer.NuevoMotorFirmaGo(nil)
	motor.WithRevocationProvider(&revocationDesktopMock{evidence: evidencia})

	doc, err := domain.NewDocument("doc.pdf", contenido, "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	resultado, err := motor.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
		Options:  map[string]string{"ltv": "true"},
	}, clave)
	if err != nil {
		t.Fatalf("Sign con LTV devolvió error: %v", err)
	}
	if !strings.Contains(resultado.Algorithm, "LTV") && !strings.Contains(resultado.Algorithm, "B-LT") {
		t.Errorf("el algoritmo debería reflejar LTV, obtenido %q", resultado.Algorithm)
	}
	return resultado.Data
}

// TestMotorFirmaGo_PAdES_LTV_DSS verifica que con la opción "ltv" el PDF
// firmado incrusta un Document Security Store (/DSS) con la cadena de
// certificados y las evidencias OCSP/CRL, y sigue siendo estructuralmente
// válido según qpdf.
func TestMotorFirmaGo_PAdES_LTV_DSS(t *testing.T) {
	original, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "test", "prueba1.pdf"))
	if err != nil {
		t.Fatalf("no se pudo leer el fixture: %v", err)
	}

	_, ca, _, caKey := commonsignerTestChain(t)
	evidencia := ports.RevocationEvidence{
		OCSPResponses: [][]byte{[]byte("respuesta-ocsp-de-prueba")},
		CRLs:          [][]byte{crearCRLPruebaDesktop(t, ca, caKey)},
	}
	firmado := firmarPAdESConLTV(t, original, evidencia)

	dss := abrirDSS(t, firmado)
	if got := dss.Key("Certs").Len(); got != 2 {
		t.Errorf("/DSS /Certs debería tener 2 streams (hoja+CA), tiene %d", got)
	}
	if got := dss.Key("OCSPs").Len(); got != 1 {
		t.Errorf("/DSS /OCSPs debería tener 1 stream, tiene %d", got)
	}
	if got := dss.Key("CRLs").Len(); got != 1 {
		t.Errorf("/DSS /CRLs debería tener 1 stream, tiene %d", got)
	}
	// El material debe ser recuperable desde el stream (DER = SEQUENCE 0x30).
	leido := new(bytes.Buffer)
	if _, err := leido.ReadFrom(dss.Key("CRLs").Index(0).Reader()); err != nil {
		t.Fatalf("no se pudo leer el stream de CRL: %v", err)
	}
	if leido.Len() == 0 || leido.Bytes()[0] != 0x30 {
		t.Error("el stream de CRL no contiene DER válido")
	}

	if exttools.Available(t, "qpdf") {
		ruta := filepath.Join(t.TempDir(), "ltv.pdf")
		if err := os.WriteFile(ruta, firmado, 0o600); err != nil {
			t.Fatalf("no se pudo escribir el PDF temporal: %v", err)
		}
		if salida, err := exec.Command("qpdf", "--check", ruta).CombinedOutput(); err != nil {
			t.Fatalf("qpdf --check falló sobre el PDF con /DSS: %v\n%s", err, salida)
		}
	}
}

// TestMotorFirmaGo_PAdES_LTV_CofirmaFusionaDSS verifica que al cofirmar un PDF
// que ya tiene /DSS, las referencias previas se conservan junto a las nuevas y
// la firma anterior sigue intacta.
func TestMotorFirmaGo_PAdES_LTV_CofirmaFusionaDSS(t *testing.T) {
	original, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "test", "prueba1.pdf"))
	if err != nil {
		t.Fatalf("no se pudo leer el fixture: %v", err)
	}

	_, ca, _, caKey := commonsignerTestChain(t)
	evidencia := ports.RevocationEvidence{
		CRLs: [][]byte{crearCRLPruebaDesktop(t, ca, caKey)},
	}

	primera := firmarPAdESConLTV(t, original, evidencia)
	segunda := firmarPAdESConLTV(t, primera, evidencia)

	if got := bytes.Count(segunda, []byte("/ByteRange")); got < 2 {
		t.Fatalf("la cofirma debería preservar la firma anterior (2 ByteRange), hay %d", got)
	}

	dss := abrirDSS(t, segunda)
	if got := dss.Key("Certs").Len(); got != 4 {
		t.Errorf("/DSS /Certs debería fusionar 2 previos + 2 nuevos = 4, tiene %d", got)
	}
	if got := dss.Key("CRLs").Len(); got != 2 {
		t.Errorf("/DSS /CRLs debería fusionar 1 previo + 1 nuevo = 2, tiene %d", got)
	}

	if exttools.Available(t, "qpdf") {
		ruta := filepath.Join(t.TempDir(), "ltv2.pdf")
		if err := os.WriteFile(ruta, segunda, 0o600); err != nil {
			t.Fatalf("no se pudo escribir el PDF temporal: %v", err)
		}
		if salida, err := exec.Command("qpdf", "--check", ruta).CombinedOutput(); err != nil {
			t.Fatalf("qpdf --check falló sobre la cofirma con /DSS fusionado: %v\n%s", err, salida)
		}
	}
}

// TestMotorFirmaGo_PAdES_LTV_SinProveedorFalla verifica que pedir LTV sin
// proveedor de revocación falla con un error claro en vez de degradar en
// silencio a una firma sin material de validación.
func TestMotorFirmaGo_PAdES_LTV_SinProveedorFalla(t *testing.T) {
	original, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "test", "prueba1.pdf"))
	if err != nil {
		t.Fatalf("no se pudo leer el fixture: %v", err)
	}

	leaf, ca, leafKey, _ := commonsignerTestChain(t)
	clave := signer.NuevaClaveLocalConCadena(leafKey, leaf, []*x509.Certificate{ca})
	motor := signer.NuevoMotorFirmaGo(nil) // sin WithRevocationProvider

	doc, err := domain.NewDocument("doc.pdf", original, "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	_, err = motor.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
		Options:  map[string]string{"ltv": "true"},
	}, clave)
	if err == nil || !strings.Contains(err.Error(), "PAdES-LTV") {
		t.Fatalf("se esperaba error PAdES-LTV sin proveedor, obtenido: %v", err)
	}
}

// TestMotorFirmaGo_PAdES_PerfilT_SinURLTSARechazaSinSustituirPDF verifica que
// una TSA de interfaz que no expone URL RFC 3161 no activa un generador de PDF
// alternativo. La operación debe fallar de forma explícita.
func TestMotorFirmaGo_PAdES_PerfilT_SinURLTSARechazaSinSustituirPDF(t *testing.T) {
	original, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "test", "prueba1.pdf"))
	if err != nil {
		t.Fatalf("no se pudo leer el fixture: %v", err)
	}

	leaf, ca, leafKey, _ := commonsignerTestChain(t)
	clave := signer.NuevaClaveLocalConCadena(leafKey, leaf, []*x509.Certificate{ca})
	motor := signer.NuevoMotorFirmaGo(nil).WithTimestampAuthority(&tsaDesktopMock{
		token: []byte{0x30, 0x03, 0x02, 0x01, 0x01},
	})

	doc, err := domain.NewDocument("doc.pdf", original, "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	resultado, err := motor.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
		Options:  map[string]string{"level": "T"},
	}, clave)
	if err == nil {
		t.Fatal("se esperaba error al no disponer de URL RFC 3161 para PAdES-T")
	}
	if !strings.Contains(err.Error(), "requiere una TSA RFC 3161 configurada mediante URL") {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(resultado.Data) != 0 {
		t.Fatal("el error no debe devolver un PDF alternativo")
	}
}
