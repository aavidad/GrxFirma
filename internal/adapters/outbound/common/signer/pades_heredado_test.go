// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" // #nosec G505 -- vectores de prueba de firmas PDF antiguas.
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

// Lote 17a: subfiltros PDF antiguos que AutoFirma verificaba (muestras
// CONVENIO.pdf y firma_CM.pdf con adbe.x509.rsa_sha1).

const huecoContentsPrueba = 8192

// pdfFirmaHeredadaPrueba arma un PDF de una revisión con un diccionario de
// firma del subfiltro indicado. firmar recibe los bytes del rango y devuelve
// el contenido DER de /Contents.
func pdfFirmaHeredadaPrueba(t *testing.T, subFilter, cert string, firmar func([]byte) []byte) []byte {
	t.Helper()
	return pdfFirmaHeredadaSobre(t, "%PDF-1.4\n", 1, subFilter, cert, firmar)
}

// pdfFirmaHeredadaSobre añade a previo una revisión con el diccionario de
// firma como objeto numero; la firma cubre todo, previo incluido.
func pdfFirmaHeredadaSobre(t *testing.T, previo string, numero int, subFilter, cert string, firmar func([]byte) []byte) []byte {
	t.Helper()
	const marcaRango = "0000000000 0000000000 0000000000 0000000000"
	cabecera := previo + fmt.Sprintf("%d 0 obj\n", numero) + "<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /" + subFilter + cert +
		" /M (D:20140101120000Z) /ByteRange [" + marcaRango + "] /Contents <"
	cola := fmt.Sprintf("> >>\nendobj\ntrailer\n<< /Root %d 0 R >>\n%%%%EOF\n", numero)
	raw := []byte(cabecera + strings.Repeat("0", huecoContentsPrueba) + cola)
	desde := len(cabecera) - 1
	hasta := desde + 1 + huecoContentsPrueba + 1
	rango := fmt.Sprintf("%010d %010d %010d %010d", 0, desde, hasta, len(raw)-hasta)
	i := bytes.LastIndex(raw, []byte(marcaRango))
	copy(raw[i:], rango)
	firmados := append(append([]byte(nil), raw[:desde]...), raw[hasta:]...)
	contenido := hex.EncodeToString(firmar(firmados))
	if len(contenido) > huecoContentsPrueba {
		t.Fatal("la firma no cabe en /Contents")
	}
	copy(raw[desde+1:], contenido)
	return raw
}

func firmaPKCS1Prueba(priv *rsa.PrivateKey, hash crypto.Hash) func([]byte) []byte {
	return func(datos []byte) []byte {
		firma, err := rsa.SignPKCS1v15(rand.Reader, priv, hash, cmsDigest(hash, datos))
		if err != nil {
			panic(err)
		}
		der, _ := asn1.Marshal(firma)
		return der
	}
}

func certHexPDF(certs ...*x509.Certificate) string {
	var b strings.Builder
	b.WriteString(" /Cert [")
	for _, c := range certs {
		b.WriteString("<" + hex.EncodeToString(c.Raw) + ">")
	}
	b.WriteString("]")
	return b.String()
}

// certLiteralPDF escribe /Cert como cadena literal con escapes, como
// firma_CM.pdf.
func certLiteralPDF(cert *x509.Certificate) string {
	var b strings.Builder
	b.WriteString(" /Cert(")
	for _, c := range cert.Raw {
		switch c {
		case '(', ')', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '\r':
			b.WriteString(`\r`)
		case '\n':
			b.WriteString(`\n`)
		case 0x00:
			b.WriteString(`\000`)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteString(")")
	return b.String()
}

func verificarPDFHeredado(t *testing.T, pdf []byte) (domain.VerificationResult, []domain.CertificateRef, error) {
	t.Helper()
	return NewPAdESVerifierWithCAdES(NewCAdESVerifierWithChecker(NewRevocationCheckerOffline())).Verify(context.Background(), domain.Document{Name: "f.pdf", Content: pdf}, domain.CertificateChain{})
}

func exigirIntegraConSHA1(t *testing.T, res domain.VerificationResult, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("una firma PDF antigua correcta se debe verificar: %v", err)
	}
	if res.Valid || res.Integrity.Status != domain.VerificationStatusValid || len(algoritmosAntiguos(res)) == 0 || !contieneString(res.Warnings, motivoAlgoritmoAntiguo) {
		t.Fatalf("valid=%v integridad=%s evidencia=%v avisos=%v", res.Valid, res.Integrity.Status, res.Evidence, res.Warnings)
	}
}

func TestPAdES_X509RSASHA1SeVerificaComoSHA1(t *testing.T) {
	priv, cert := certForTest(t, "PDF-PKCS1")
	for nombre, certPDF := range map[string]string{"hexadecimal": certHexPDF(cert), "literal": certLiteralPDF(cert)} {
		t.Run(nombre, func(t *testing.T) {
			pdf := pdfFirmaHeredadaPrueba(t, subFiltroX509RSASHA1, certPDF, firmaPKCS1Prueba(priv, crypto.SHA1))
			res, firmantes, err := verificarPDFHeredado(t, pdf)
			exigirIntegraConSHA1(t, res, err)
			if len(firmantes) != 1 || firmantes[0].Fingerprint != certificateFingerprint(cert) {
				t.Fatalf("firmantes=%+v", firmantes)
			}
		})
	}
	// Un DigestInfo con SHA-256 (PDF 1.7) también vale, y sigue marcado.
	pdf := pdfFirmaHeredadaPrueba(t, subFiltroX509RSASHA1, certHexPDF(cert), firmaPKCS1Prueba(priv, crypto.SHA256))
	res, _, err := verificarPDFHeredado(t, pdf)
	exigirIntegraConSHA1(t, res, err)
}

func TestPAdES_X509RSASHA1AlteradoNoEsIntegro(t *testing.T) {
	priv, cert := certForTest(t, "PDF-PKCS1")
	pdf := pdfFirmaHeredadaPrueba(t, subFiltroX509RSASHA1, certHexPDF(cert), firmaPKCS1Prueba(priv, crypto.SHA1))

	alterado := bytes.Replace(pdf, []byte("/Root 1 0 R"), []byte("/Root 2 0 R"), 1)
	if _, _, err := verificarPDFHeredado(t, alterado); err == nil {
		t.Fatal("un PDF alterado dentro del rango firmado no puede verificar")
	}

	// Otro certificado en /Cert: su clave no verifica la firma.
	_, otro := certForTest(t, "Otro")
	ajeno := pdfFirmaHeredadaPrueba(t, subFiltroX509RSASHA1, certHexPDF(otro), firmaPKCS1Prueba(priv, crypto.SHA1))
	if _, _, err := verificarPDFHeredado(t, ajeno); err == nil {
		t.Fatal("una firma que no verifica ningún certificado de /Cert no puede pasar")
	}

	sinCert := pdfFirmaHeredadaPrueba(t, subFiltroX509RSASHA1, "", firmaPKCS1Prueba(priv, crypto.SHA1))
	if _, _, err := verificarPDFHeredado(t, sinCert); err == nil {
		t.Fatal("sin /Cert no hay firmante")
	}

	// /Contents que no es un OCTET STRING.
	basura := pdfFirmaHeredadaPrueba(t, subFiltroX509RSASHA1, certHexPDF(cert), func([]byte) []byte { return []byte{0x30, 0x00} })
	if _, _, err := verificarPDFHeredado(t, basura); err == nil {
		t.Fatal("un /Contents que no es la firma no puede pasar")
	}
}

func TestPAdES_PKCS7SHA1SeVerificaComoSHA1(t *testing.T) {
	priv, cert := certForTest(t, "PDF-PKCS7-SHA1")
	clave := &LocalSigningKey{ID: "k", Signer: priv, Certificate: cert}
	firmarResumen := func(datos []byte) []byte {
		resumen := sha1.Sum(datos) // #nosec G401 -- vector de prueba.
		der, _, err := signCAdESBES(resumen[:], clave, cmsHashSpecs[0], true)
		if err != nil {
			panic(err)
		}
		return der
	}
	pdf := pdfFirmaHeredadaPrueba(t, subFiltroPKCS7SHA1, "", firmarResumen)
	res, _, err := verificarPDFHeredado(t, pdf)
	exigirIntegraConSHA1(t, res, err)

	alterado := bytes.Replace(pdf, []byte("/Root 1 0 R"), []byte("/Root 2 0 R"), 1)
	if _, _, err := verificarPDFHeredado(t, alterado); err == nil {
		t.Fatal("un PDF adbe.pkcs7.sha1 alterado no puede verificar")
	}

	// El eContent tiene que ser el resumen del rango: un CMS que firma otro
	// resumen no vale.
	otro := pdfFirmaHeredadaPrueba(t, subFiltroPKCS7SHA1, "", func(datos []byte) []byte {
		return firmarResumen(append([]byte("otro"), datos...))
	})
	if _, _, err := verificarPDFHeredado(t, otro); err == nil {
		t.Fatal("un adbe.pkcs7.sha1 con el resumen de otros bytes no puede verificar")
	}
}

// Retrocompatibilidad: antes, una firma antigua junto a otras se contaba como
// no verificada y el PDF se abría sin salir válido. Si ahora no verifica (aquí,
// la primera, alterada por la segunda revisión), sigue igual: aviso, no error.
func TestPAdES_FirmaAntiguaQueNoVerificaJuntoAOtrasEsAviso(t *testing.T) {
	priv, cert := certForTest(t, "PDF-PKCS1")
	primera := pdfFirmaHeredadaPrueba(t, subFiltroX509RSASHA1, certHexPDF(cert), firmaPKCS1Prueba(priv, crypto.SHA1))
	primera = bytes.Replace(primera, []byte("D:20140101120000Z"), []byte("D:20150101120000Z"), 1)
	pdf := pdfFirmaHeredadaSobre(t, string(primera), 2, subFiltroX509RSASHA1, certHexPDF(cert), firmaPKCS1Prueba(priv, crypto.SHA1))

	res, firmantes, err := verificarPDFHeredado(t, pdf)
	if err != nil {
		t.Fatalf("una firma antigua que no verifica junto a otra no debe impedir abrir el PDF: %v", err)
	}
	if res.Valid || !contieneString(res.Warnings, motivoFirmasPDFNoVerificadas) ||
		!contieneString(res.Details, "firma_no_verificada subfilter="+subFiltroX509RSASHA1) || !contieneString(res.Details, "firmas_pdf=2") {
		t.Fatalf("valid=%v avisos=%v detalles=%v", res.Valid, res.Warnings, res.Details)
	}
	if len(firmantes) != 1 {
		t.Fatalf("firmantes=%+v", firmantes)
	}
}

// La inspección v2 y la detección de firmas no cuentan los subfiltros
// antiguos: solo los sabe verificar PAdESVerifier.Verify.
func TestPAdES_SubfiltrosAntiguosSoloEnElVerificador(t *testing.T) {
	priv, cert := certForTest(t, "PDF-PKCS1")
	pdf := pdfFirmaHeredadaPrueba(t, subFiltroX509RSASHA1, certHexPDF(cert), firmaPKCS1Prueba(priv, crypto.SHA1))
	if _, err := extractPDFEmbeddedSignatures(pdf); err == nil {
		t.Fatal("la extracción general no debe aceptar adbe.x509.rsa_sha1")
	}
}

func TestPAdES_PDFSinFirmasLoDice(t *testing.T) {
	_, _, err := verificarPDFHeredado(t, []byte("%PDF-1.4\n1 0 obj\n<< /FT /Sig /T (Campo vacío) >>\nendobj\n%%EOF\n"))
	if err == nil || !strings.Contains(err.Error(), "no contiene ninguna firma") {
		t.Fatalf("err = %v", err)
	}
}

func TestLeerCadenaLiteralPDF(t *testing.T) {
	casos := map[string]string{
		`(a\(b\)c)`:      "a(b)c",
		`(a(b)c)`:        "a(b)c",
		`(\n\r\t\b\f\\)`: "\n\r\t\b\f\\",
		`(\101\0611)`:    "A11",
		"(a\\\r\nb)":     "ab",
		"(a\r\nb)":       "a\nb",
		`(\q)`:           "q",
	}
	for entrada, want := range casos {
		got, fin, err := leerCadenaLiteralPDF(entrada, 0)
		if err != nil || string(got) != want || fin != len(entrada) {
			t.Fatalf("%q: %q, %d, %v; want %q", entrada, got, fin, err, want)
		}
	}
	if _, _, err := leerCadenaLiteralPDF(`(sin cerrar`, 0); err == nil {
		t.Fatal("una cadena sin cerrar es un error")
	}
	if _, err := extraerCertificadosPDF("<< /Cert <00> /Cert <01> >>"); err == nil {
		t.Fatal("dos /Cert en el mismo diccionario es un error")
	}
}
