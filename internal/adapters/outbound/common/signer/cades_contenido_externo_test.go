// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"regexp"
	"strconv"
	"testing"

	"grxfirma/internal/adapters/outbound/common/asiccontainer"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
)

// firmaCAdESImplicitaPrueba devuelve un CMS con el contenido encapsulado
// (CAdES implícito, como los de AutoFirma con mode=implicit).
func firmaCAdESImplicitaPrueba(t *testing.T, priv *rsa.PrivateKey, cert *x509.Certificate, datos []byte) []byte {
	t.Helper()
	doc, err := domain.NewDocument("victima.txt", datos, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	res, err := commonsigner.NewCAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign,
		Options: map[string]string{"mode": "implicit"},
	}, &commonsigner.LocalSigningKey{ID: "k", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatalf("firma implícita: %v", err)
	}
	return res.Data
}

var reByteRangePrueba = regexp.MustCompile(`/ByteRange\s*\[\s*(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s*\]`)

// sustituirContentsPDF pone otro CMS en el hueco de /Contents sin tocar
// ByteRange ni xref: el PDF sigue siendo estructuralmente correcto.
func sustituirContentsPDF(t *testing.T, pdf, cms []byte) []byte {
	t.Helper()
	m := reByteRangePrueba.FindSubmatch(pdf)
	if m == nil {
		t.Fatal("PDF firmado sin ByteRange")
	}
	inicio, _ := strconv.Atoi(string(m[2]))
	fin, _ := strconv.Atoi(string(m[3]))
	if pdf[inicio] != '<' || pdf[fin-1] != '>' {
		t.Fatalf("el hueco de ByteRange no es /Contents: %q %q", pdf[inicio], pdf[fin-1])
	}
	hueco := fin - inicio - 2
	cifrado := hex.EncodeToString(cms)
	if len(cifrado) > hueco {
		t.Fatalf("el CMS no cabe en /Contents: %d > %d", len(cifrado), hueco)
	}
	out := append([]byte(nil), pdf...)
	copy(out[inicio+1:], cifrado)
	for i := inicio + 1 + len(cifrado); i < fin-1; i++ {
		out[i] = '0'
	}
	if hueco%2 == 1 {
		out[fin-2] = ' ' // el relleno de ceros tiene que ser de bytes completos
	}
	return out
}

// E2-01 (a): un PDF cuyo /Contents es un CAdES implícito válido sobre otros
// datos no puede salir válido; lo firmado es el ByteRange del PDF.
func TestPAdESVerifier_RechazaCMSImplicitoSobreOtrosDatos(t *testing.T) {
	priv, cert := generarCertRSAPrueba(t)
	firmado, err := desktopsigner.NuevoMotorFirmaGo(nil).Sign(context.Background(), domain.SignatureJob{
		Document: documentoPDFRealPrueba(t), Format: domain.FormatPAdES, Action: domain.ActionSign,
	}, desktopsigner.NuevaClaveLocal(priv, cert))
	if err != nil {
		t.Fatalf("firmar PDF: %v", err)
	}
	ajeno := firmaCAdESImplicitaPrueba(t, priv, cert, []byte("Autorizo el pago de 10 EUR"))
	atacado := sustituirContentsPDF(t, firmado.Data, ajeno)
	doc, _ := domain.NewDocument("atacado.pdf", atacado, "application/pdf")
	vr, _, err := commonsigner.NewPAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
	if err == nil && (vr.Valid || vr.Integrity.Status == domain.VerificationStatusValid) {
		t.Fatalf("un CMS implícito ajeno al PDF no puede validar el PDF: %s %+v", vr.Reason, vr.Integrity)
	}
}

// E2-01 (b): un ASiC-S cuyo signature.p7s es un CMS implícito sobre otros
// datos no puede validar el dato del contenedor.
func TestASiCCAdES_RechazaCMSImplicitoSobreOtrosDatos(t *testing.T) {
	priv, cert := generarCertRSAPrueba(t)
	ajeno := firmaCAdESImplicitaPrueba(t, priv, cert, []byte("Autorizo el pago de 10 EUR"))
	contenedor, err := asiccontainer.CreateCAdESContainer(ajeno, []byte("Autorizo el pago de 1.000.000 EUR"), "contrato.txt")
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := domain.NewDocument("contrato.asics", contenedor, "application/vnd.etsi.asic-s+zip")
	vr, _, err := commonsigner.NewMultiVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
	if err == nil && (vr.Valid || vr.Integrity.Status == domain.VerificationStatusValid) {
		t.Fatalf("un CMS implícito ajeno al dato del ASiC no puede validarlo: %s %+v", vr.Reason, vr.Integrity)
	}
}

// E2-01: con original aportado, el resumen se comprueba siempre contra ese
// original. Un CAdES implícito solo vale si su contenido es el mismo.
func TestCAdESVerifier_DetachedConImplicitoExigeMismoContenido(t *testing.T) {
	priv, cert := generarCertRSAPrueba(t)
	datos := []byte("contenido firmado de verdad")
	implicita := firmaCAdESImplicitaPrueba(t, priv, cert, datos)
	firmado, _ := domain.NewDocument("firma.csig", implicita, "application/pkcs7-signature")
	verifier := commonsigner.NewCAdESVerifierWithChecker(commonsigner.NewRevocationCheckerOffline())

	otro, _ := domain.NewDocument("otro.txt", []byte("contenido que nadie firmó"), "text/plain")
	vr, _, err := verifier.VerifyDetached(context.Background(), firmado, otro, domain.CertificateChain{})
	if err == nil && (vr.Valid || vr.Integrity.Status == domain.VerificationStatusValid) {
		t.Fatalf("un original distinto del encapsulado no puede validar: %s %+v", vr.Reason, vr.Integrity)
	}

	mismo, _ := domain.NewDocument("mismo.txt", append([]byte(nil), datos...), "text/plain")
	vr, _, err = verifier.VerifyDetached(context.Background(), firmado, mismo, domain.CertificateChain{})
	if err != nil || vr.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("CAdES implícito con su mismo original debe seguir verificando: %v %+v", err, vr.Integrity)
	}

	vr, _, err = verifier.Verify(context.Background(), firmado, domain.CertificateChain{})
	if err != nil || vr.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("CAdES implícito sin original debe seguir verificando: %v %+v", err, vr.Integrity)
	}
}

// E2-01: la API detached estricta (PAdES, ASiC, identidad) rechaza un CMS
// con contenido encapsulado aunque coincida con el externo, y sigue
// aceptando el CAdES explícito.
func TestCAdESVerifier_DetachedCMSEstrictoRechazaEncapsulado(t *testing.T) {
	priv, cert := generarCertRSAPrueba(t)
	datos := []byte("reto canónico")
	verifier := commonsigner.NewCAdESVerifierWithChecker(commonsigner.NewRevocationCheckerOffline())
	implicita := firmaCAdESImplicitaPrueba(t, priv, cert, datos)
	if _, _, err := verifier.VerifyDetachedCMS(context.Background(), implicita, datos); err == nil {
		t.Fatal("la verificación detached estricta debe rechazar contenido encapsulado")
	}

	doc, _ := domain.NewDocument("reto.txt", datos, "text/plain")
	explicita, err := commonsigner.NewCAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign,
	}, &commonsigner.LocalSigningKey{ID: "k", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatal(err)
	}
	vr, _, err := verifier.VerifyDetachedCMS(context.Background(), explicita.Data, datos)
	if err != nil || vr.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("CAdES explícito debe verificar: %v %+v", err, vr.Integrity)
	}
	if _, _, err := verifier.VerifyDetachedCMS(context.Background(), explicita.Data, bytes.ToUpper(datos)); err == nil {
		t.Fatal("CAdES explícito con contenido distinto debe fallar")
	}
}
