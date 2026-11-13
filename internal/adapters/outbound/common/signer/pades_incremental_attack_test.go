// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
)

func firmarPDFParaAtaque(t *testing.T) []byte {
	t.Helper()
	return firmarPDFPrueba(t, documentoPDFRealPrueba(t))
}

func firmarPDFPrueba(t *testing.T, doc domain.Document) []byte {
	t.Helper()
	return firmarPDFPruebaConOpciones(t, doc, nil)
}

func firmarPDFPruebaConOpciones(t *testing.T, doc domain.Document, opciones map[string]string) []byte {
	t.Helper()
	priv, cert := generarCertRSAPrueba(t)
	firmado, err := desktopsigner.NuevoMotorFirmaGo(nil).Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
		Options:  opciones,
	}, desktopsigner.NuevaClaveLocal(priv, cert))
	if err != nil {
		t.Fatalf("no se pudo firmar el PDF: %v", err)
	}
	return firmado.Data
}

var reContenidoPDF = regexp.MustCompile(`/Contents\s+(\d+)\s+0\s+R`)

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

// insertarEnDiccionario añade claves al final de un diccionario «<< ... >>».
func insertarEnDiccionario(t *testing.T, cuerpo, claves string) string {
	t.Helper()
	i := strings.LastIndex(cuerpo, ">>")
	if !strings.HasPrefix(strings.TrimSpace(cuerpo), "<<") || i < 0 {
		t.Fatalf("no es un diccionario: %q", cuerpo)
	}
	return cuerpo[:i] + " " + claves + cuerpo[i:]
}

func flujoPDFPrueba(contenido string) string {
	return fmt.Sprintf("<</Length %d>>\nstream\n%s\nendstream", len(contenido), contenido)
}

const textoAtacantePrueba = "IMPORTE: 1.000.000 EUR"

// exigirContenidoAtacante comprueba que el documento atacante está bien
// formado: un lector que sigue la xref pinta el texto del atacante.
func exigirContenidoAtacante(t *testing.T, data []byte) {
	t.Helper()
	if !strings.Contains(contenidoVisiblePrueba(t, data), textoAtacantePrueba) {
		t.Fatal("el documento atacante no muestra el contenido sustituido; la prueba no reproduce el ataque")
	}
}

func contenidoDePaginaPrueba(t *testing.T, pagina string) string {
	t.Helper()
	m := reContenidoPDF.FindStringSubmatch(pagina)
	if m == nil {
		t.Skip("la página de prueba no usa un único flujo /Contents")
	}
	return m[1]
}

// Redefinir el flujo de contenido de la página tras la firma es el ataque
// clásico de actualización incremental: la firma sigue cuadrando, pero el
// visor muestra otro texto.
func TestPAdESVerifier_DetectaContenidoDePaginaRedefinido(t *testing.T) {
	firmado := firmarPDFParaAtaque(t)
	_, pagina := paginaPDFPrueba(t, firmado)
	var n int
	fmt.Sscan(contenidoDePaginaPrueba(t, pagina), &n)
	atacado := anexarActualizacionPrueba(t, firmado, []objetoPDFPrueba{
		{numero: n, cuerpo: flujoPDFPrueba("BT /F1 24 Tf 72 700 Td (" + textoAtacantePrueba + ") Tj ET")},
	}, nil)
	exigirContenidoAtacante(t, atacado)

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
	n, pagina := paginaPDFPrueba(t, firmado)
	atacado := anexarActualizacionPrueba(t, firmado, []objetoPDFPrueba{
		{numero: n, cuerpo: insertarEnDiccionario(t, pagina, "/Rotate 180")},
	}, nil)

	if vr := verificarPDF(t, atacado); vr.Valid || vr.Integrity.Status != domain.VerificationStatusInvalid {
		t.Fatalf("un PDF con la página alterada no debe ser válido: %s", vr.Reason)
	}
}

func TestPAdESVerifier_DetectaAnotacionNoDeFirmaAnadida(t *testing.T) {
	firmado := firmarPDFParaAtaque(t)
	n, pagina := paginaPDFPrueba(t, firmado)
	pagina = regexp.MustCompile(`/Annots\s*\[[^\]]*\]`).ReplaceAllString(pagina, "")
	atacado := anexarActualizacionPrueba(t, firmado, []objetoPDFPrueba{
		{numero: 100, cuerpo: "<</Type/Annot/Subtype/FreeText/Rect[0 0 500 500]/Contents(ANULADO)>>"},
		{numero: n, cuerpo: insertarEnDiccionario(t, pagina, "/Annots [100 0 R]")},
	}, nil)

	if vr := verificarPDF(t, atacado); vr.Valid || vr.Integrity.Status != domain.VerificationStatusInvalid {
		t.Fatalf("una anotación de texto añadida tras la firma no debe pasar como válida: %s", vr.Reason)
	}
}

// E2-02 (a): un widget nuevo con /FT /Sig y apariencia propia que tapa la
// página («Sneaky Signature»). No hay firma nueva que lo respalde.
func TestPAdESVerifier_DetectaWidgetFirmaConAparienciaAnadido(t *testing.T) {
	firmado := firmarPDFParaAtaque(t)
	n, pagina := paginaPDFPrueba(t, firmado)
	cat, catalogo := catalogoPDFPrueba(t, firmado)
	if !regexp.MustCompile(`/Fields\s*\[`).MatchString(catalogo) {
		t.Fatalf("el catálogo firmado no tiene /Fields: %s", catalogo)
	}
	catalogo = regexp.MustCompile(`/Fields\s*\[`).ReplaceAllString(catalogo, "/Fields [100 0 R ")
	if regexp.MustCompile(`/Annots\s*\[`).MatchString(pagina) {
		pagina = regexp.MustCompile(`/Annots\s*\[`).ReplaceAllString(pagina, "/Annots [100 0 R ")
	} else {
		pagina = insertarEnDiccionario(t, pagina, "/Annots [100 0 R]")
	}
	apariencia := "1 1 1 rg 0 0 612 792 re f 1 0 0 rg BT /Helv 48 Tf 100 400 Td (ANULADO) Tj ET"
	atacado := anexarActualizacionPrueba(t, firmado, []objetoPDFPrueba{
		{numero: 100, cuerpo: fmt.Sprintf("<</Type/Annot/Subtype/Widget/FT/Sig/T(Firma 2)/F 4/Rect[0 0 612 792]/P %d 0 R/AP<</N 101 0 R>>>>", n)},
		{numero: 101, cuerpo: fmt.Sprintf("<</Type/XObject/Subtype/Form/BBox[0 0 612 792]/Length %d>>\nstream\n%s\nendstream", len(apariencia), apariencia)},
		{numero: n, cuerpo: pagina},
		{numero: cat, cuerpo: catalogo},
	}, nil)

	if vr := verificarPDF(t, atacado); vr.Valid || vr.Integrity.Status != domain.VerificationStatusInvalid {
		t.Fatalf("un widget con apariencia añadido tras la firma no debe pasar como válido: %s", vr.Reason)
	}
}

// E2-02 (b): sin objetos nuevos, solo una xref y un trailer que reasignan
// el flujo de contenido de la página a un objeto huérfano que ya estaba
// dentro de los bytes firmados.
func TestPAdESVerifier_DetectaReasignacionXRefAObjetoHuerfano(t *testing.T) {
	base := documentoPDFRealPrueba(t).Content
	_, pagina := paginaPDFPrueba(t, base)
	var n int
	fmt.Sscan(contenidoDePaginaPrueba(t, pagina), &n)
	huerfano := fmt.Sprintf("%d 0 obj\n%s\nendobj\n", n, flujoPDFPrueba("BT /F1 24 Tf 72 700 Td ("+textoAtacantePrueba+") Tj ET"))
	preparado := anexarActualizacionPrueba(t, base, []objetoPDFPrueba{
		{numero: 9000, cuerpo: "<<>>", prefijo: huerfano},
	}, nil)
	offset := bytes.LastIndex(preparado, []byte(huerfano))
	if offset < 0 {
		t.Fatal("no se localizó el objeto huérfano")
	}
	doc, err := domain.NewDocument("preparado.pdf", preparado, "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	firmado := firmarPDFPrueba(t, doc)
	if !bytes.Equal(firmado[:offset+len(huerfano)], preparado[:offset+len(huerfano)]) {
		t.Fatal("el objeto huérfano no quedó dentro de los bytes firmados")
	}
	if vr := verificarPDF(t, firmado); !vr.Valid {
		t.Fatalf("el PDF preparado y firmado debe verificar antes del ataque: %s", vr.Reason)
	}
	atacado := anexarActualizacionPrueba(t, firmado, nil, map[int]int{n: offset})
	exigirContenidoAtacante(t, atacado)

	if vr := verificarPDF(t, atacado); vr.Valid || vr.Integrity.Status != domain.VerificationStatusInvalid {
		t.Fatalf("una xref que reasigna el contenido a un objeto huérfano no debe pasar como válida: %s", vr.Reason)
	}
}

// E2-02 (c): el objeto redefinido va precedido de un carácter que no es
// espacio («]N 0 obj»). El lector llega por el desplazamiento de la xref.
func TestPAdESVerifier_DetectaObjetoRedefinidoSinEspacioPrevio(t *testing.T) {
	firmado := firmarPDFParaAtaque(t)
	_, pagina := paginaPDFPrueba(t, firmado)
	var n int
	fmt.Sscan(contenidoDePaginaPrueba(t, pagina), &n)
	atacado := anexarActualizacionPrueba(t, firmado, []objetoPDFPrueba{
		{numero: n, prefijo: "]", cuerpo: flujoPDFPrueba("BT /F1 24 Tf 72 700 Td (" + textoAtacantePrueba + ") Tj ET")},
	}, nil)
	exigirContenidoAtacante(t, atacado)

	if vr := verificarPDF(t, atacado); vr.Valid || vr.Integrity.Status != domain.VerificationStatusInvalid {
		t.Fatalf("un objeto redefinido tras «]» no debe pasar como válido: %s", vr.Reason)
	}
}

// Añadir material de validación (DSS) tras la firma es legítimo (PAdES-LTA).
func TestPAdESVerifier_AdmiteDSSPosterior(t *testing.T) {
	firmado := firmarPDFParaAtaque(t)
	cat, catalogo := catalogoPDFPrueba(t, firmado)
	atacado := anexarActualizacionPrueba(t, firmado, []objetoPDFPrueba{
		{numero: 102, cuerpo: "<</Type/DSS/Certs[]>>"},
		{numero: cat, cuerpo: insertarEnDiccionario(t, catalogo, "/DSS 102 0 R")},
	}, nil)

	vr := verificarPDF(t, atacado)
	if !vr.Valid {
		t.Fatalf("añadir un DSS no debe invalidar la firma: %s %v", vr.Reason, vr.Integrity.Details)
	}
	if vr.Coverage != "partial" {
		t.Fatalf("coverage=%q, want partial", vr.Coverage)
	}
}

// Una segunda firma legítima en otra revisión no deja la primera en duda.
func TestPAdESVerifier_AdmiteSegundaFirmaPosterior(t *testing.T) {
	primera := firmarPDFParaAtaque(t)
	doc, err := domain.NewDocument("firmado.pdf", primera, "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	segunda := firmarPDFPrueba(t, doc)
	vr := verificarPDF(t, segunda)
	if !vr.Valid || vr.Coverage != "full" {
		t.Fatalf("dos firmas sucesivas deben verificar: valid=%v coverage=%q %s %v", vr.Valid, vr.Coverage, vr.Reason, vr.Integrity.Details)
	}
}

// Dos firmas visibles de GrxFirma: la segunda añade su widget a /Annots de
// la página y escribe objetos nuevos para la apariencia. La primera firma
// sigue siendo válida (el análisis v2 lo daba por contenido modificado).
func TestPAdESVerifier_AdmiteSegundaFirmaVisible(t *testing.T) {
	opciones := map[string]string{"visibleSeal": "true", "visibleSealRectX": "36", "visibleSealRectY": "36", "visibleSealRectW": "220", "visibleSealRectH": "70"}
	primera := firmarPDFPruebaConOpciones(t, documentoPDFRealPrueba(t), opciones)
	doc, err := domain.NewDocument("firmado.pdf", primera, "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	segunda := firmarPDFPruebaConOpciones(t, doc, opciones)
	vr := verificarPDF(t, segunda)
	if !vr.Valid || vr.Coverage != "full" {
		t.Fatalf("dos firmas visibles sucesivas deben verificar: valid=%v coverage=%q %s %v", vr.Valid, vr.Coverage, vr.Reason, vr.Integrity.Details)
	}
	inspeccion, err := commonsigner.InspeccionarPAdESV2(segunda, 20, 20)
	if err != nil {
		t.Fatal(err)
	}
	if cambio := inspeccion.Firmas[1].CambiosDesdeAnterior; cambio.Estado == "no_permitidos" {
		t.Fatalf("una firma visible añadida no es una modificación prohibida: %+v", cambio)
	}
}

// Un documento preparado con una referencia colgante (/Annots [50 0 R] sin
// objeto 50) y firmado. Después, junto a un DSS legítimo, se añade el objeto
// 50 disfrazado de /Type /XRef: el visor lo pinta como anotación.
func TestPAdESVerifier_DetectaObjetoXRefFalsoEnReferenciaColgante(t *testing.T) {
	base := documentoPDFRealPrueba(t).Content
	n, pagina := paginaPDFPrueba(t, base)
	preparado := anexarActualizacionPrueba(t, base, []objetoPDFPrueba{
		{numero: n, cuerpo: insertarEnDiccionario(t, pagina, "/Annots [50 0 R]")},
	}, nil)
	doc, err := domain.NewDocument("preparado.pdf", preparado, "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	firmado := firmarPDFPrueba(t, doc)
	if vr := verificarPDF(t, firmado); !vr.Valid {
		t.Fatalf("el PDF preparado y firmado debe verificar antes del ataque: %s", vr.Reason)
	}
	cat, catalogo := catalogoPDFPrueba(t, firmado)
	atacado := anexarActualizacionPrueba(t, firmado, []objetoPDFPrueba{
		{numero: 50, cuerpo: "<</Type/XRef/Subtype/FreeText/Rect[0 0 595 842]/Contents(ANULADO)/DA(/Helv 48 Tf 1 0 0 rg)/Length 0>>\nstream\n\nendstream"},
		{numero: 102, cuerpo: "<</Type/DSS/Certs[]>>"},
		{numero: cat, cuerpo: insertarEnDiccionario(t, catalogo, "/DSS 102 0 R")},
	}, nil)
	if got := anotacionVisiblePrueba(t, atacado); got != "ANULADO" {
		t.Fatalf("el documento atacante no muestra la anotación; la prueba no reproduce el ataque: %q", got)
	}

	if vr := verificarPDF(t, atacado); vr.Valid {
		t.Fatalf("un objeto nuevo que ocupa una referencia colgante no debe pasar como válido: %s", vr.Reason)
	}
}
