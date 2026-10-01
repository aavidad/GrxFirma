// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
)

func firmarPDFParaAtaque(t *testing.T) []byte {
	t.Helper()
	priv, cert := generarCertRSAPrueba(t)
	firmado, err := desktopsigner.NuevoMotorFirmaGo(nil).Sign(context.Background(), domain.SignatureJob{
		Document: documentoPDFRealPrueba(t),
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
	}, desktopsigner.NuevaClaveLocal(priv, cert))
	if err != nil {
		t.Fatalf("no se pudo firmar el PDF: %v", err)
	}
	return firmado.Data
}

var (
	rePaginaPDF    = regexp.MustCompile(`(?s)(\d+) 0 obj\s*<<(.*?/Type\s*/Page[^s].*?)>>\s*endobj`)
	reContenidoPDF = regexp.MustCompile(`/Contents\s+(\d+)\s+0\s+R`)
	reRaizPDF      = regexp.MustCompile(`/Root\s+(\d+)\s+0\s+R`)
)

func verificarPDF(t *testing.T, data []byte) domain.VerificationResult {
	t.Helper()
	doc, err := domain.NewDocument("atacado.pdf", data, "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	vr, _, err := commonsigner.NewPAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	return vr
}

func anexar(data []byte, cuerpo string) []byte {
	return append(append([]byte(nil), data...), []byte("\n"+cuerpo+"\n%%EOF\n")...)
}

// Redefinir el flujo de contenido de la página tras la firma es el ataque
// clásico de actualización incremental: la firma sigue cuadrando, pero el
// visor muestra otro texto.
func TestPAdESVerifier_DetectaContenidoDePaginaRedefinido(t *testing.T) {
	firmado := firmarPDFParaAtaque(t)
	pagina := rePaginaPDF.FindSubmatch(firmado)
	if pagina == nil {
		t.Fatal("no se localizó la página en el PDF firmado")
	}
	contenido := reContenidoPDF.FindSubmatch(pagina[2])
	if contenido == nil {
		t.Skip("la página de prueba no usa un único flujo /Contents")
	}
	flujo := "BT /F1 24 Tf 72 700 Td (IMPORTE: 1.000.000 EUR) Tj ET"
	atacado := anexar(firmado, fmt.Sprintf("%s 0 obj\n<</Length %d>>\nstream\n%s\nendstream\nendobj", contenido[1], len(flujo), flujo))

	vr := verificarPDF(t, atacado)
	if vr.Valid {
		t.Fatalf("un PDF con el contenido de página redefinido no debe ser válido: %s", vr.Reason)
	}
	if vr.Integrity.Status != domain.VerificationStatusInvalid {
		t.Fatalf("integrity=%q, want invalid", vr.Integrity.Status)
	}
}

func TestPAdESVerifier_DetectaPaginaConRecursosCambiados(t *testing.T) {
	firmado := firmarPDFParaAtaque(t)
	pagina := rePaginaPDF.FindSubmatch(firmado)
	if pagina == nil {
		t.Fatal("no se localizó la página en el PDF firmado")
	}
	nuevo := string(pagina[2]) + " /Rotate 180"
	atacado := anexar(firmado, fmt.Sprintf("%s 0 obj\n<<%s>>\nendobj", pagina[1], nuevo))

	if vr := verificarPDF(t, atacado); vr.Valid {
		t.Fatalf("un PDF con la página alterada no debe ser válido: %s", vr.Reason)
	}
}

func TestPAdESVerifier_DetectaAnotacionNoDeFirmaAnadida(t *testing.T) {
	firmado := firmarPDFParaAtaque(t)
	pagina := rePaginaPDF.FindSubmatch(firmado)
	if pagina == nil {
		t.Fatal("no se localizó la página en el PDF firmado")
	}
	anotacion := 90001
	dict := regexp.MustCompile(`/Annots\s*\[[^\]]*\]`).ReplaceAllString(string(pagina[2]), "")
	atacado := anexar(firmado, fmt.Sprintf(
		"%d 0 obj\n<</Type/Annot/Subtype/FreeText/Rect[0 0 500 500]/Contents(ANULADO)>>\nendobj\n%s 0 obj\n<<%s /Annots [%d 0 R]>>\nendobj",
		anotacion, pagina[1], dict, anotacion))

	if vr := verificarPDF(t, atacado); vr.Valid {
		t.Fatalf("una anotación de texto añadida tras la firma no debe pasar como válida: %s", vr.Reason)
	}
}

// Añadir material de validación (DSS) tras la firma es legítimo (PAdES-LTA).
func TestPAdESVerifier_AdmiteDSSPosterior(t *testing.T) {
	firmado := firmarPDFParaAtaque(t)
	raices := reRaizPDF.FindAllSubmatch(firmado, -1)
	if len(raices) == 0 {
		t.Fatal("no se localizó /Root")
	}
	raiz := string(raices[len(raices)-1][1])
	catalogo := regexp.MustCompile(`(?s)(?:^|\n)`+raiz+` 0 obj\s*<<(.*?)>>\s*endobj`).FindAllSubmatch(firmado, -1)
	if len(catalogo) == 0 {
		t.Fatal("no se localizó el catálogo")
	}
	cat := string(catalogo[len(catalogo)-1][1])
	atacado := anexar(firmado, fmt.Sprintf(
		"90010 0 obj\n<</Type/DSS/Certs[]>>\nendobj\n%s 0 obj\n<<%s /DSS 90010 0 R>>\nendobj", raiz, cat))

	vr := verificarPDF(t, atacado)
	if !vr.Valid {
		t.Fatalf("añadir un DSS no debe invalidar la firma: %s %v", vr.Reason, vr.Integrity.Details)
	}
	if vr.Coverage != "partial" {
		t.Fatalf("coverage=%q, want partial", vr.Coverage)
	}
}
