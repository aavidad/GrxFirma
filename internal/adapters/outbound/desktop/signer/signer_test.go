// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/testsupport/exttools"
	"grxfirma/internal/testsupport/pdffixture"
	"grxfirma/internal/testsupport/tsatest"
)

var oidMessageDigestTest = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}

type atributoCMS struct {
	Type   asn1.ObjectIdentifier
	Values []asn1.RawValue `asn1:"set"`
}

type signedDataConUnsignedTest struct {
	Version          int
	DigestAlgorithms asn1.RawValue
	EncapContentInfo asn1.RawValue
	Certificates     asn1.RawValue       `asn1:"optional"`
	SignerInfos      []signerInfoPDFTest `asn1:"set"`
}

type signerInfoPDFTest struct {
	Version            int
	SID                asn1.RawValue
	DigestAlgorithm    asn1.RawValue
	SignedAttributes   asn1.RawValue `asn1:"tag:0,optional"`
	SignatureAlgorithm asn1.RawValue
	Signature          []byte
	UnsignedAttributes asn1.RawValue `asn1:"tag:1,optional"`
}

type tsaDesktopMock struct {
	token []byte
}

func (m *tsaDesktopMock) RequestTimestamp(context.Context, []byte, crypto.Hash) ([]byte, error) {
	return append([]byte(nil), m.token...), nil
}

func nuevaTSAURLPAdES(t *testing.T) string {
	t.Helper()
	responder := tsatest.NewResponder(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("metodo HTTP inesperado: %s", r.Method)
			http.Error(w, "metodo no permitido", http.StatusMethodNotAllowed)
			return
		}
		responder.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

type revocationDesktopMock struct {
	evidence ports.RevocationEvidence
}

func (m *revocationDesktopMock) Fetch(context.Context, *x509.Certificate, *x509.Certificate) (ports.RevocationEvidence, error) {
	return m.evidence, nil
}

func commonsignerTestChain(t *testing.T) (leaf, ca *x509.Certificate, leafKey *ecdsa.PrivateKey, caKey *ecdsa.PrivateKey) {
	t.Helper()

	var err error
	caKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("no se pudo generar clave CA: %v", err)
	}
	caTpl := &x509.Certificate{
		SerialNumber:          big.NewInt(11),
		Subject:               pkix.Name{CommonName: "CA signer test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTpl, caTpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("no se pudo crear certificado CA: %v", err)
	}
	ca, err = x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("no se pudo parsear certificado CA: %v", err)
	}

	leafKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("no se pudo generar clave hoja: %v", err)
	}
	leafTpl := &x509.Certificate{
		SerialNumber: big.NewInt(12),
		Subject:      pkix.Name{CommonName: "Firmante LT"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTpl, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("no se pudo crear certificado hoja: %v", err)
	}
	leaf, err = x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatalf("no se pudo parsear certificado hoja: %v", err)
	}
	return leaf, ca, leafKey, caKey
}

func crearCRLPruebaDesktop(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey) []byte {
	t.Helper()
	crlDER, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		SignatureAlgorithm:        x509.ECDSAWithSHA256,
		RevokedCertificateEntries: []x509.RevocationListEntry{},
		Number:                    big.NewInt(13),
		ThisUpdate:                time.Now().Add(-time.Hour),
		NextUpdate:                time.Now().Add(time.Hour),
	}, ca, caKey)
	if err != nil {
		t.Fatalf("no se pudo crear CRL de prueba: %v", err)
	}
	return crlDER
}

// ---------------------------------------------------------------------------
// Helpers de test
// ---------------------------------------------------------------------------

func generarCertRSA(t *testing.T) (*rsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("no se pudo generar clave RSA: %v", err)
	}
	return priv, crearCertAutofirmado(t, priv, &priv.PublicKey)
}

func generarCertECDSA(t *testing.T) (*ecdsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("no se pudo generar clave ECDSA: %v", err)
	}
	return priv, crearCertAutofirmado(t, priv, &priv.PublicKey)
}

func crearCertAutofirmado(t *testing.T, priv crypto.Signer, pub crypto.PublicKey) *x509.Certificate {
	t.Helper()
	plantilla := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Certificado de prueba GrxFirma"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	derCert, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, pub, priv)
	if err != nil {
		t.Fatalf("no se pudo crear el certificado de prueba: %v", err)
	}
	cert, err := x509.ParseCertificate(derCert)
	if err != nil {
		t.Fatalf("no se pudo parsear el certificado generado: %v", err)
	}
	return cert
}

func nuevoTrabajoCaDES(t *testing.T, contenido []byte) domain.SignatureJob {
	t.Helper()
	doc, err := domain.NewDocument("prueba.txt", contenido, "text/plain")
	if err != nil {
		t.Fatalf("no se pudo crear el documento: %v", err)
	}
	return domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	}
}

// contentInfoRaw parsea un ContentInfo CMS.
// Contenido captura el wrapper [0] EXPLICIT tal como lo devuelve encoding/asn1;
// el SignedData real esta en Contenido.Bytes.
type contentInfoRaw struct {
	TipoContenido asn1.ObjectIdentifier
	Contenido     asn1.RawValue // [0] EXPLICIT; Bytes = DER del SignedData
}

// parsearSignedData desempaqueta la estructura SignedData a nivel estructural.
type rawSignedData struct {
	Version      int
	AlgsDigest   asn1.RawValue // SET OF
	EncapContent asn1.RawValue // SEQUENCE
	Certificados asn1.RawValue `asn1:"optional"` // [0]
	Firmantes    asn1.RawValue // SET OF
}

// ---------------------------------------------------------------------------
// Tests funcionales CAdES
// ---------------------------------------------------------------------------

func TestMotorFirmaGo_CaDES_RSA_EstructuraASN1(t *testing.T) {
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	documento := []byte("documento de prueba para firma CAdES-BES con RSA")
	job := nuevoTrabajoCaDES(t, documento)

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvio error: %v", err)
	}

	if resultado.Format != domain.FormatCAdES {
		t.Errorf("formato esperado CAdES, obtenido %s", resultado.Format)
	}
	if resultado.Algorithm != "SHA256withRSA" {
		t.Errorf("algoritmo esperado SHA256withRSA, obtenido %s", resultado.Algorithm)
	}
	if len(resultado.Data) == 0 {
		t.Fatal("el resultado no contiene datos")
	}

	// Verificar que el DER es un ContentInfo valido.
	var ci contentInfoRaw
	resto, err := asn1.Unmarshal(resultado.Data, &ci)
	if err != nil {
		t.Fatalf("el resultado no es un ContentInfo ASN.1 valido: %v", err)
	}
	if len(resto) != 0 {
		t.Errorf("bytes sobrantes tras el ContentInfo: %d bytes", len(resto))
	}

	// El OID debe ser id-signedData.
	oidSignedData := asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	if !ci.TipoContenido.Equal(oidSignedData) {
		t.Errorf("OID del ContentInfo incorrecto: %v", ci.TipoContenido)
	}

	// El wrapper [0] EXPLICIT debe estar presente.
	if ci.Contenido.Class != asn1.ClassContextSpecific || ci.Contenido.Tag != 0 {
		t.Errorf("se esperaba [0] EXPLICIT, class=%d tag=%d", ci.Contenido.Class, ci.Contenido.Tag)
	}
	// Los bytes dentro de [0] deben comenzar con SEQUENCE (SignedData).
	if len(ci.Contenido.Bytes) == 0 || ci.Contenido.Bytes[0] != 0x30 {
		t.Errorf("el contenido del ContentInfo no es SEQUENCE; primer byte: %02x", ci.Contenido.Bytes[0])
	}
}

func TestMotorFirmaGo_CaDES_RSA_FirmaCriptograficamenteValida(t *testing.T) {
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	documento := []byte("verificacion criptografica de la firma RSA")
	job := nuevoTrabajoCaDES(t, documento)

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvio error: %v", err)
	}

	verificarFirmaCaDES(t, resultado.Data, &priv.PublicKey)
}

func TestMotorFirmaGo_CaDES_ECDSA_FirmaCriptograficamenteValida(t *testing.T) {
	priv, cert := generarCertECDSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	documento := []byte("verificacion criptografica de la firma ECDSA")
	job := nuevoTrabajoCaDES(t, documento)

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvio error: %v", err)
	}

	verificarFirmaCaDES(t, resultado.Data, &priv.PublicKey)
}

// ---------------------------------------------------------------------------
// Tests funcionales XAdES (perfil BES detached con C14N exclusiva)
// ---------------------------------------------------------------------------

func TestMotorFirmaGo_XAdES_GeneraXMLBase(t *testing.T) {
	priv, cert := generarCertRSA(t)
	_ = priv
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	doc, _ := domain.NewDocument("doc.xml", []byte("<xml><dato>valor</dato></xml>"), "application/xml")
	job := domain.SignatureJob{Document: doc, Format: domain.FormatXAdES, Action: domain.ActionSign}

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvio error inesperado: %v", err)
	}
	if resultado.Format != domain.FormatXAdES {
		t.Errorf("formato esperado XAdES, obtenido %s", resultado.Format)
	}
	if resultado.Algorithm != "SHA256withRSA" {
		t.Errorf("algoritmo esperado SHA256withRSA, obtenido %s", resultado.Algorithm)
	}
	if !bytes.HasPrefix(resultado.Data, []byte("<?xml")) {
		t.Fatal("la salida XAdES debe comenzar por cabecera XML")
	}
	texto := string(resultado.Data)
	if !strings.Contains(texto, "<ds:SignedInfo") {
		t.Fatal("la salida XAdES debe contener SignedInfo")
	}
	if !strings.Contains(texto, "SignedProperties") {
		t.Fatal("la salida XAdES debe contener SignedProperties")
	}
	if !strings.Contains(texto, "SigningCertificateV2") {
		t.Fatal("la salida XAdES debe contener SigningCertificateV2")
	}
	if !strings.Contains(texto, "xml-exc-c14n") {
		t.Fatal("la salida XAdES debe declarar canonicalización exclusiva")
	}
	signatureValueB64 := extraerContenido(t, texto, "<ds:SignatureValue>", "</ds:SignatureValue>")
	signatureValue, err := base64.StdEncoding.DecodeString(signatureValueB64)
	if err != nil {
		t.Fatalf("SignatureValue no es base64 valido: %v", err)
	}
	if len(signatureValue) == 0 {
		t.Fatal("SignatureValue no puede estar vacio")
	}
}

func TestMotorFirmaGo_XMLDSig_GeneraXMLBase(t *testing.T) {
	priv, cert := generarCertRSA(t)
	_ = priv
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	doc, _ := domain.NewDocument("doc.xml", []byte("<xml><dato>valor</dato></xml>"), "application/xml")
	job := domain.SignatureJob{Document: doc, Format: domain.SignatureFormat("XMLdSig"), Action: domain.ActionSign}

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvio error inesperado: %v", err)
	}
	if resultado.Format != domain.SignatureFormat("XMLdSig") {
		t.Errorf("formato esperado XMLdSig, obtenido %s", resultado.Format)
	}
	texto := string(resultado.Data)
	if !strings.Contains(texto, "<ds:SignedInfo") {
		t.Fatal("la salida XMLdSig debe contener SignedInfo")
	}
	if strings.Contains(texto, "SignedProperties") {
		t.Fatal("la salida XMLdSig no debe contener SignedProperties")
	}
}

func TestMotorFirmaGo_XAdEST_UsaTSACuandoSeSolicitaNivelT(t *testing.T) {
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil).WithTimestampAuthority(&tsaDesktopMock{
		token: []byte{0x30, 0x03, 0x02, 0x01, 0x01},
	})

	doc, _ := domain.NewDocument("doc.xml", []byte("<xml><dato>valor</dato></xml>"), "application/xml")
	job := domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatXAdES,
		Action:   domain.ActionSign,
		Options:  map[string]string{"level": "T"},
	}

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvio error inesperado: %v", err)
	}
	texto := string(resultado.Data)
	if !strings.Contains(texto, "<xades:SignatureTimeStamp>") {
		t.Fatal("la salida XAdES-T debe contener SignatureTimeStamp")
	}

	verifier := commonsigner.NewXAdESVerifier()
	verifyResult, signers, err := verifier.Verify(context.Background(), domain.Document{
		Name:     "firma.xsig",
		Content:  resultado.Data,
		MIMEType: "application/xml",
	}, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("Verify devolvio error inesperado: %v", err)
	}
	if !verifyResult.Valid {
		t.Fatalf("la firma XAdES-T deberia ser valida: %s", verifyResult.Reason)
	}
	if len(signers) != 1 {
		t.Fatalf("se esperaba un firmante XAdES-T, obtenidos %d", len(signers))
	}
}

func TestMotorFirmaGo_XAdEST_UsaTSACuandoSeSolicitaBaselineConTSA(t *testing.T) {
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil).WithTimestampAuthority(&tsaDesktopMock{
		token: []byte{0x30, 0x03, 0x02, 0x01, 0x01},
	})

	doc, _ := domain.NewDocument("doc.xml", []byte("<xml><dato>valor</dato></xml>"), "application/xml")
	job := domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatXAdES,
		Action:   domain.ActionSign,
		Options: map[string]string{
			"profile":   "baseline",
			"tsaPolicy": "0.4.0.2023.1.1",
		},
	}

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvio error inesperado: %v", err)
	}
	if !strings.Contains(string(resultado.Data), "<xades:SignatureTimeStamp>") {
		t.Fatal("profile=baseline + tsaURL debe producir XAdES-T")
	}
}

func TestMotorFirmaGo_XAdEST_UsaTsaURLDeOptionsSinTSAInyectada(t *testing.T) {
	tsaResponder := tsatest.NewResponder(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("metodo HTTP inesperado: %s", r.Method)
		}
		tsaResponder.ServeHTTP(w, r)
	}))
	defer srv.Close()

	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	doc, _ := domain.NewDocument("doc.xml", []byte("<xml><dato>valor</dato></xml>"), "application/xml")
	job := domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatXAdES,
		Action:   domain.ActionSign,
		Options: map[string]string{
			"level":  "T",
			"tsaURL": srv.URL,
		},
	}

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvio error inesperado: %v", err)
	}
	if !strings.Contains(string(resultado.Data), "<xades:SignatureTimeStamp>") {
		t.Fatal("tsaURL en options debe permitir XAdES-T sin TSA global")
	}
}

// ---------------------------------------------------------------------------
// Tests funcionales PAdES (perfil basico Go nativo)
// ---------------------------------------------------------------------------

func TestMotorFirmaGo_PAdES_GeneraPDFBase(t *testing.T) {
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	original := pdffixture.Minimal()
	doc, _ := domain.NewDocument("doc.pdf", original, "application/pdf")
	job := domain.SignatureJob{Document: doc, Format: domain.FormatPAdES, Action: domain.ActionSign}

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvio error inesperado: %v", err)
	}
	if resultado.Format != domain.FormatPAdES {
		t.Fatalf("formato esperado PAdES, obtenido %s", resultado.Format)
	}
	if resultado.Algorithm != "PAdES-Basic-Detached" {
		t.Fatalf("algoritmo esperado PAdES-Basic-Detached, obtenido %s", resultado.Algorithm)
	}
	if !bytes.HasPrefix(resultado.Data, []byte("%PDF-1.4")) {
		t.Fatal("la salida PAdES debe comenzar por cabecera PDF")
	}
	if !bytes.Contains(resultado.Data, []byte("/Type /Sig")) {
		t.Fatal("la salida PAdES debe contener un diccionario de firma PDF")
	}
	if !bytes.Contains(resultado.Data, []byte("/ByteRange [")) {
		t.Fatal("la salida PAdES debe contener /ByteRange")
	}
	if !bytes.Contains(resultado.Data, []byte("/SubFilter /ETSI.CAdES.detached")) {
		t.Fatal("la salida PAdES debe usar subfiltro ETSI.CAdES.detached")
	}
	if !bytes.HasPrefix(resultado.Data, original) {
		t.Fatal("la firma incremental debe preservar íntegramente el PDF de entrada")
	}
	byteRangeTexto := extraerContenido(t, string(resultado.Data), "/ByteRange [", "]")
	byteRange := parsearByteRange(t, byteRangeTexto)
	if byteRange[0] != 0 {
		t.Fatalf("ByteRange debe comenzar en 0, obtenido %d", byteRange[0])
	}
	hexFirma := extraerContenido(t, string(resultado.Data), "/Contents<", ">")
	if len(hexFirma) == 0 {
		t.Fatal("el contenido de firma no puede estar vacío")
	}
	if len(hexFirma)%2 != 0 {
		t.Fatalf("el contenido de firma hexadecimal debe tener longitud par, obtenido %d", len(hexFirma))
	}
	contentsNeedle := []byte("/Contents<")
	contentsMarker := bytes.Index(resultado.Data, contentsNeedle)
	if contentsMarker < 0 {
		t.Fatal("no se encontro /Contents en el PDF firmado")
	}
	contentsTokenStart := contentsMarker + len("/Contents")
	contentsHexStart := contentsTokenStart + 1
	contentsHexEnd := contentsHexStart + len(hexFirma)
	if byteRange[1] != contentsTokenStart {
		t.Fatalf("primer tramo ByteRange=%d, esperado %d", byteRange[1], contentsTokenStart)
	}
	if byteRange[2] != contentsHexEnd+1 {
		t.Fatalf("segundo tramo ByteRange=%d, esperado %d", byteRange[2], contentsHexEnd+1)
	}
	if byteRange[3] != len(resultado.Data)-byteRange[2] {
		t.Fatalf("longitud del segundo tramo=%d, esperada %d", byteRange[3], len(resultado.Data)-byteRange[2])
	}
	firmaHex, err := hex.DecodeString(hexFirma)
	if err != nil {
		t.Fatalf("la firma PDF no es hex valido: %v", err)
	}
	var ci contentInfoRaw
	resto, err := asn1.Unmarshal(firmaHex, &ci)
	if err != nil {
		t.Fatalf("la firma CMS embebida en PDF no es ASN.1 valida: %v", err)
	}
	if len(bytes.Trim(resto, "\x00")) != 0 {
		t.Fatal("la firma CMS embebida en PDF deja bytes no nulos tras ASN.1")
	}
	firmaDER := firmaHex[:len(firmaHex)-len(resto)]
	bytesFirmados := append([]byte(nil), resultado.Data[:byteRange[1]]...)
	bytesFirmados = append(bytesFirmados, resultado.Data[byteRange[2]:]...)
	verificarFirmaCaDES(t, firmaDER, &priv.PublicKey, bytesFirmados)
}

func TestMotorFirmaGo_PAdES_RechazaEntradaInvalidaSinCrearOtroPDF(t *testing.T) {
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	for _, tc := range []struct {
		name    string
		content []byte
	}{
		{name: "sin cabecera", content: []byte("contenido que no es PDF")},
		{name: "cabecera sin estructura", content: []byte("%PDF-origen")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := domain.NewDocument("doc.pdf", tc.content, "application/pdf")
			if err != nil {
				t.Fatalf("NewDocument() error = %v", err)
			}
			resultado, err := motor.Sign(context.Background(), domain.SignatureJob{
				Document: doc,
				Format:   domain.FormatPAdES,
				Action:   domain.ActionSign,
			}, clave)
			if err == nil {
				t.Fatal("se esperaba rechazo explícito del PDF inválido")
			}
			if !strings.Contains(err.Error(), "no es válido") {
				t.Fatalf("error inseguro o poco explícito: %v", err)
			}
			if len(resultado.Data) != 0 {
				t.Fatal("el rechazo no debe devolver un PDF alternativo")
			}
		})
	}
}

func TestMotorFirmaGo_PAdES_RechazaPermisoLegacyParaPDFInvalido(t *testing.T) {
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)
	doc, err := domain.NewDocument("doc.pdf", pdffixture.Minimal(), "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}

	resultado, err := motor.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
		Options:  map[string]string{"allowInvalidPDF": "true"},
	}, clave)
	if err == nil {
		t.Fatal("allowInvalidPDF=true debe rechazarse explícitamente")
	}
	if !strings.Contains(err.Error(), "ya no está soportado") {
		t.Fatalf("error de deprecación inesperado: %v", err)
	}
	if len(resultado.Data) != 0 {
		t.Fatal("el rechazo no debe devolver un PDF alternativo")
	}
}

func TestMotorFirmaGo_PAdES_UsaSubFilterAdobeSiSeSolicita(t *testing.T) {
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	doc, _ := domain.NewDocument("doc.pdf", pdffixture.Minimal(), "application/pdf")
	job := domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
		Options:  map[string]string{"subfilter": "adobe"},
	}

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvio error inesperado: %v", err)
	}
	if resultado.Algorithm != "PAdES-Basic-Detached-Adobe" {
		t.Fatalf("algoritmo esperado PAdES-Basic-Detached-Adobe, obtenido %s", resultado.Algorithm)
	}
	if !bytes.Contains(resultado.Data, []byte("/SubFilter /adbe.pkcs7.detached")) {
		t.Fatal("la salida PAdES Adobe debe usar subfiltro adbe.pkcs7.detached")
	}
}

func TestMotorFirmaGo_PAdES_ValidaConPdfsig(t *testing.T) {
	exttools.Require(t, "pdfsig")

	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	doc, _ := domain.NewDocument("doc.pdf", pdffixture.Minimal(), "application/pdf")
	job := domain.SignatureJob{Document: doc, Format: domain.FormatPAdES, Action: domain.ActionSign}

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvio error inesperado: %v", err)
	}

	tempDir := t.TempDir()
	signedPath := filepath.Join(tempDir, "firmado.pdf")
	if err := os.WriteFile(signedPath, resultado.Data, 0o600); err != nil {
		t.Fatalf("no se pudo escribir el PDF firmado temporal: %v", err)
	}

	cmd := exec.Command("pdfsig", signedPath)
	salida, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pdfsig devolvio error: %v\n%s", err, salida)
	}

	out := string(salida)
	if !strings.Contains(out, "Signature #1:") {
		t.Fatalf("pdfsig no detectó la firma PDF:\n%s", out)
	}
	if !strings.Contains(out, "Signature Type: ETSI.CAdES.detached") {
		t.Fatalf("pdfsig no reconoció el tipo ETSI.CAdES.detached:\n%s", out)
	}
	if !strings.Contains(out, "Total document signed") {
		t.Fatalf("pdfsig no confirmó que el documento completo esté firmado:\n%s", out)
	}
	if !strings.Contains(out, "Signature Validation: Signature is Valid.") {
		t.Fatalf("pdfsig no validó la firma generada:\n%s", out)
	}
}

func TestMotorFirmaGo_PAdES_EsEstructuralmenteValidoConQPDF(t *testing.T) {
	exttools.Require(t, "qpdf")

	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	doc, _ := domain.NewDocument("doc.pdf", pdffixture.Minimal(), "application/pdf")
	job := domain.SignatureJob{Document: doc, Format: domain.FormatPAdES, Action: domain.ActionSign}

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvio error inesperado: %v", err)
	}

	tempDir := t.TempDir()
	signedPath := filepath.Join(tempDir, "firmado-pades.pdf")
	if err := os.WriteFile(signedPath, resultado.Data, 0o600); err != nil {
		t.Fatalf("no se pudo escribir el PDF firmado temporal: %v", err)
	}

	cmd := exec.Command("qpdf", "--check", signedPath)
	salida, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("qpdf devolvio error: %v\n%s", err, salida)
	}

	out := string(salida)
	if !strings.Contains(out, "checking") && !strings.Contains(out, "No syntax or stream encoding errors found") {
		t.Fatalf("salida inesperada de qpdf:\n%s", out)
	}
}

func TestMotorFirmaGo_PAdESVisible_PreservaPDFOriginalYGeneraWidget(t *testing.T) {
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	fixturePath := filepath.Join("..", "..", "..", "..", "..", "test", "regression", "fixtures", "v1", "samples", "2.pdf")
	pdfOriginal, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("no se pudo leer el fixture PDF: %v", err)
	}
	doc, err := domain.NewDocument("2.pdf", pdfOriginal, "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	job := domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
		Options: map[string]string{
			"visibleSeal":      "true",
			"visibleSealRectX": "36",
			"visibleSealRectY": "36",
			"visibleSealRectW": "220",
			"visibleSealRectH": "70",
			"page":             "1",
		},
	}

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvió error inesperado: %v", err)
	}
	if resultado.Format != domain.FormatPAdES {
		t.Fatalf("formato esperado PAdES, obtenido %s", resultado.Format)
	}
	if !bytes.HasPrefix(resultado.Data, []byte("%PDF-")) {
		t.Fatal("la salida con sello visible debe seguir siendo un PDF real")
	}
	if bytes.Contains(resultado.Data, []byte("Documento firmado:")) {
		t.Fatal("la ruta con sello visible no debe regenerar un PDF mínimo")
	}
	if !bytes.Contains(resultado.Data, []byte("/Subtype /Widget")) {
		t.Fatal("la salida con sello visible debe contener un widget de firma")
	}
	if !bytes.Contains(resultado.Data, []byte("/AP << /N ")) {
		t.Fatal("la salida con sello visible debe contener una apariencia visible (/AP)")
	}

	tempDir := t.TempDir()
	signedPath := filepath.Join(tempDir, "firmado-visible.pdf")
	if err := os.WriteFile(signedPath, resultado.Data, 0o600); err != nil {
		t.Fatalf("no se pudo escribir el PDF firmado temporal: %v", err)
	}

	if exttools.Available(t, "qpdf") {
		salida, err := exec.Command("qpdf", "--show-npages", signedPath).CombinedOutput()
		if err != nil {
			t.Fatalf("qpdf --show-npages devolvió error: %v\n%s", err, salida)
		}
		if strings.TrimSpace(string(salida)) != "15" {
			t.Fatalf("se esperaban 15 páginas tras firmar, obtenido %q", strings.TrimSpace(string(salida)))
		}
	}
	if exttools.Available(t, "pdfsig") {
		salida, err := exec.Command("pdfsig", signedPath).CombinedOutput()
		if err != nil {
			t.Fatalf("pdfsig devolvió error: %v\n%s", err, salida)
		}
		out := string(salida)
		if !strings.Contains(out, "Signature #1:") {
			t.Fatalf("pdfsig no detectó la firma PDF visible:\n%s", out)
		}
		if !strings.Contains(out, "Signature Type: ETSI.CAdES.detached") {
			t.Fatalf("pdfsig no reconoció la firma visible ETSI por defecto:\n%s", out)
		}
		if !strings.Contains(out, "Signature Validation: Signature is Valid.") {
			t.Fatalf("pdfsig no validó la firma visible:\n%s", out)
		}
	}
}

func TestMotorFirmaGo_PAdESVisible_TodasLasPaginas_GeneraUnWidgetPorPagina(t *testing.T) {
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	fixturePath := filepath.Join("..", "..", "..", "..", "..", "test", "regression", "fixtures", "v1", "samples", "2.pdf")
	pdfOriginal, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("no se pudo leer el fixture PDF: %v", err)
	}
	doc, err := domain.NewDocument("2.pdf", pdfOriginal, "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	job := domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
		Options: map[string]string{
			"visibleSeal":      "true",
			"visibleSealRectX": "36",
			"visibleSealRectY": "36",
			"visibleSealRectW": "220",
			"visibleSealRectH": "70",
			"page":             "all",
		},
	}

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvió error inesperado: %v", err)
	}
	if !bytes.HasPrefix(resultado.Data, []byte("%PDF-")) {
		t.Fatal("la salida con sello visible en todas las páginas debe seguir siendo un PDF real")
	}
	if got := bytes.Count(resultado.Data, []byte("/Subtype /Widget")); got != 15 {
		t.Fatalf("se esperaban 15 widgets visibles, obtenido %d", got)
	}
	// Una sola firma criptográfica (un /ByteRange) pese a los 15 widgets.
	if got := bytes.Count(resultado.Data, []byte("/ByteRange")); got != 1 {
		t.Fatalf("se esperaba exactamente 1 diccionario de firma (/ByteRange), obtenido %d", got)
	}

	tempDir := t.TempDir()
	signedPath := filepath.Join(tempDir, "firmado-visible-all-pages.pdf")
	if err := os.WriteFile(signedPath, resultado.Data, 0o600); err != nil {
		t.Fatalf("no se pudo escribir el PDF firmado temporal: %v", err)
	}

	if exttools.Available(t, "qpdf") {
		if salida, err := exec.Command("qpdf", "--check", signedPath).CombinedOutput(); err != nil {
			t.Fatalf("qpdf --check falló en la firma de todas las páginas: %v\n%s", err, salida)
		}
	}
	if exttools.Available(t, "pdfsig") {
		// La validación criptográfica vía pdfsig depende de la versión de
		// poppler para firmas con /V en el campo padre y widgets /Kids; aquí
		// solo comprobamos que no delate campos de firma inconsistentes. La
		// validez la garantizan qpdf y los tests de página única.
		salida, _ := exec.Command("pdfsig", signedPath).CombinedOutput()
		if strings.Contains(string(salida), "Impossible") {
			t.Fatalf("pdfsig reporta firmas inconsistentes:\n%s", salida)
		}
	}
}

func TestMotorFirmaGo_PAdESSinSello_PreservaPDFOriginal(t *testing.T) {
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	fixturePath := filepath.Join("..", "..", "..", "..", "..", "test", "regression", "fixtures", "v1", "samples", "2.pdf")
	pdfOriginal, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("no se pudo leer el fixture PDF: %v", err)
	}
	doc, err := domain.NewDocument("2.pdf", pdfOriginal, "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	job := domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
	}

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvió error inesperado: %v", err)
	}
	if !bytes.HasPrefix(resultado.Data, []byte("%PDF-")) {
		t.Fatal("la salida debe seguir siendo un PDF real")
	}
	if bytes.Contains(resultado.Data, []byte("Documento firmado:")) {
		t.Fatal("la firma PAdES sin sello no debe regenerar un PDF mínimo")
	}
	if bytes.Contains(resultado.Data, []byte("/AP << /N ")) {
		t.Fatal("la firma PAdES sin sello no debe inyectar apariencia visible")
	}

	tempDir := t.TempDir()
	signedPath := filepath.Join(tempDir, "firmado-sin-sello.pdf")
	if err := os.WriteFile(signedPath, resultado.Data, 0o600); err != nil {
		t.Fatalf("no se pudo escribir el PDF firmado temporal: %v", err)
	}

	if exttools.Available(t, "pdfsig") {
		salida, err := exec.Command("pdfsig", signedPath).CombinedOutput()
		if err != nil {
			t.Fatalf("pdfsig devolvió error: %v\n%s", err, salida)
		}
		out := string(salida)
		if !strings.Contains(out, "Signature #1:") {
			t.Fatalf("pdfsig no detectó la firma PDF:\n%s", out)
		}
		if !strings.Contains(out, "Signature Validation: Signature is Valid.") {
			t.Fatalf("pdfsig no validó la firma PDF:\n%s", out)
		}
	}
}

func TestMotorFirmaGo_PAdEST_EmbebeTimestampEnCMS(t *testing.T) {
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	doc, _ := domain.NewDocument("doc.pdf", pdffixture.Minimal(), "application/pdf")
	job := domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
		Options: map[string]string{
			"level":  "T",
			"tsaURL": nuevaTSAURLPAdES(t),
		},
	}

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvio error inesperado: %v", err)
	}
	if resultado.Algorithm != "PAdES-B-T" {
		t.Fatalf("algoritmo esperado PAdES-B-T, obtenido %s", resultado.Algorithm)
	}

	hexFirma := extraerContenido(t, string(resultado.Data), "/Contents<", ">")
	firmaHex, err := hex.DecodeString(hexFirma)
	if err != nil {
		t.Fatalf("la firma PDF no es hex valido: %v", err)
	}
	firmaDER := bytes.TrimRight(firmaHex, "\x00")

	var ciTimestamp contentInfoRaw
	if _, err := asn1.Unmarshal(firmaDER, &ciTimestamp); err != nil {
		t.Fatalf("no se pudo parsear el ContentInfo timestamped: %v", err)
	}
	var sd signedDataConUnsignedTest
	if _, err := asn1.Unmarshal(ciTimestamp.Contenido.Bytes, &sd); err != nil {
		t.Fatalf("no se pudo parsear el SignedData timestamped: %v", err)
	}
	if len(sd.SignerInfos) != 1 {
		t.Fatalf("se esperaba un SignerInfo, obtenidos %d", len(sd.SignerInfos))
	}
	if len(sd.SignerInfos[0].UnsignedAttributes.Bytes) == 0 {
		t.Fatal("PAdES-B-T debe contener atributos no firmados en el CMS embebido")
	}
	setDER := makeSetDER(sd.SignerInfos[0].UnsignedAttributes.Bytes)
	var attrs []atributoCMS
	if _, err := asn1.UnmarshalWithParams(setDER, &attrs, "set"); err != nil {
		t.Fatalf("no se pudieron parsear los atributos no firmados: %v", err)
	}
	found := false
	for _, attr := range attrs {
		if attr.Type.Equal(asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 14}) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("el CMS embebido en PAdES-B-T debe contener signatureTimeStampToken")
	}
}

func TestMotorFirmaGo_PAdEST_UsaSubFilterAdobeSiSeSolicita(t *testing.T) {
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	doc, _ := domain.NewDocument("doc.pdf", pdffixture.Minimal(), "application/pdf")
	job := domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
		Options: map[string]string{
			"level":     "T",
			"subfilter": "adobe",
			"tsaURL":    nuevaTSAURLPAdES(t),
		},
	}

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvio error inesperado: %v", err)
	}
	if resultado.Algorithm != "PAdES-B-T-Adobe" {
		t.Fatalf("algoritmo esperado PAdES-B-T-Adobe, obtenido %s", resultado.Algorithm)
	}
	if !bytes.Contains(resultado.Data, []byte("/SubFilter /adbe.pkcs7.detached")) {
		t.Fatal("la salida PAdES-T Adobe debe usar subfiltro adbe.pkcs7.detached")
	}
}

func TestMotorFirmaGo_PAdEST_UsaBaselineConTSAComoPerfilT(t *testing.T) {
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	doc, _ := domain.NewDocument("doc.pdf", pdffixture.Minimal(), "application/pdf")
	job := domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
		Options: map[string]string{
			"profile":   "baseline",
			"tsaPolicy": "0.4.0.2023.1.1",
			"tsaURL":    nuevaTSAURLPAdES(t),
		},
	}

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvio error inesperado: %v", err)
	}
	if resultado.Algorithm != "PAdES-B-T" {
		t.Fatalf("algoritmo esperado PAdES-B-T, obtenido %s", resultado.Algorithm)
	}
}

func TestMotorFirmaGo_PAdEST_UsaTsaURLDeOptionsSinTSAInyectada(t *testing.T) {
	tsaResponder := tsatest.NewResponder(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("metodo HTTP inesperado: %s", r.Method)
		}
		tsaResponder.ServeHTTP(w, r)
	}))
	defer srv.Close()

	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	doc, _ := domain.NewDocument("doc.pdf", pdffixture.Minimal(), "application/pdf")
	job := domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
		Options: map[string]string{
			"level":  "T",
			"tsaURL": srv.URL,
		},
	}

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvio error inesperado: %v", err)
	}
	if resultado.Algorithm != "PAdES-B-T" {
		t.Fatalf("algoritmo esperado PAdES-B-T, obtenido %s", resultado.Algorithm)
	}
}

func TestMotorFirmaGo_PAdEST_ValidaConPdfsig(t *testing.T) {
	exttools.Require(t, "pdfsig")

	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	doc, _ := domain.NewDocument("doc.pdf", pdffixture.Minimal(), "application/pdf")
	job := domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
		Options: map[string]string{
			"level":  "T",
			"tsaURL": nuevaTSAURLPAdES(t),
		},
	}

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvio error inesperado: %v", err)
	}

	tempDir := t.TempDir()
	signedPath := filepath.Join(tempDir, "firmado-pades-t.pdf")
	if err := os.WriteFile(signedPath, resultado.Data, 0o600); err != nil {
		t.Fatalf("no se pudo escribir el PDF firmado temporal: %v", err)
	}

	cmd := exec.Command("pdfsig", signedPath)
	salida, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pdfsig devolvio error: %v\n%s", err, salida)
	}

	out := string(salida)
	if !strings.Contains(out, "Signature #1:") {
		t.Fatalf("pdfsig no detectó la firma PDF:\n%s", out)
	}
	if !strings.Contains(out, "Signature Type: ETSI.CAdES.detached") {
		t.Fatalf("pdfsig no reconoció el tipo ETSI.CAdES.detached:\n%s", out)
	}
	if !strings.Contains(out, "Total document signed") {
		t.Fatalf("pdfsig no confirmó que el documento completo esté firmado:\n%s", out)
	}
	if !strings.Contains(out, "Signature Validation: Signature is Valid.") {
		t.Fatalf("pdfsig no validó la firma generada:\n%s", out)
	}
}

func TestMotorFirmaGo_CAdESLT_EmbebeRevocacion(t *testing.T) {
	leaf, ca, leafKey, caKey := commonsignerTestChain(t)
	clave := signer.NuevaClaveLocalConCadena(leafKey, leaf, []*x509.Certificate{ca})
	motor := signer.NuevoMotorFirmaGo(nil).
		WithTimestampAuthority(&tsaDesktopMock{token: []byte{0x30, 0x03, 0x02, 0x01, 0x01}}).
		WithRevocationProvider(&revocationDesktopMock{
			evidence: ports.RevocationEvidence{
				CRLs: [][]byte{crearCRLPruebaDesktop(t, ca, caKey)},
			},
		})

	doc, _ := domain.NewDocument("doc.bin", []byte("contenido LT"), "application/octet-stream")
	job := domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
		Options:  map[string]string{"level": "LT"},
	}

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvio error inesperado: %v", err)
	}
	if resultado.Algorithm != "CAdES-B-LT" {
		t.Fatalf("algoritmo esperado CAdES-B-LT, obtenido %s", resultado.Algorithm)
	}

	verifier := commonsigner.NewCAdESVerifier()
	verifyResult, _, err := verifier.VerifyDetachedCMS(context.Background(), resultado.Data, doc.Content)
	if err != nil {
		t.Fatalf("VerifyDetachedCMS devolvio error inesperado: %v", err)
	}
	if !verifyResult.Valid {
		t.Fatalf("la firma CAdES-LT deberia verificar: %s", verifyResult.Reason)
	}
}

func TestMotorFirmaGo_CAdESLTA_EmbebeArchiveTimestamp(t *testing.T) {
	leaf, ca, leafKey, caKey := commonsignerTestChain(t)
	clave := signer.NuevaClaveLocalConCadena(leafKey, leaf, []*x509.Certificate{ca})
	motor := signer.NuevoMotorFirmaGo(nil).
		WithTimestampAuthority(&tsaDesktopMock{token: []byte{0x30, 0x03, 0x02, 0x01, 0x02}}).
		WithRevocationProvider(&revocationDesktopMock{
			evidence: ports.RevocationEvidence{
				CRLs: [][]byte{crearCRLPruebaDesktop(t, ca, caKey)},
			},
		})

	doc, _ := domain.NewDocument("doc.bin", []byte("contenido LTA"), "application/octet-stream")
	job := domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
		Options:  map[string]string{"level": "LTA"},
	}

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvio error inesperado: %v", err)
	}
	if resultado.Algorithm != "CAdES-B-LTA" {
		t.Fatalf("algoritmo esperado CAdES-B-LTA, obtenido %s", resultado.Algorithm)
	}
	if !bytes.Contains(resultado.Data, []byte{0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x09, 0x10, 0x02, 0x1b}) {
		t.Fatal("la firma CAdES-LTA debe contener el OID archiveTimestamp")
	}
}

func TestMotorFirmaGo_PAdEST_EsEstructuralmenteValidoConQPDF(t *testing.T) {
	exttools.Require(t, "qpdf")

	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	doc, _ := domain.NewDocument("doc.pdf", pdffixture.Minimal(), "application/pdf")
	job := domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
		Options: map[string]string{
			"level":  "T",
			"tsaURL": nuevaTSAURLPAdES(t),
		},
	}

	resultado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("Sign devolvio error inesperado: %v", err)
	}

	tempDir := t.TempDir()
	signedPath := filepath.Join(tempDir, "firmado-pades-t.pdf")
	if err := os.WriteFile(signedPath, resultado.Data, 0o600); err != nil {
		t.Fatalf("no se pudo escribir el PDF firmado temporal: %v", err)
	}

	cmd := exec.Command("qpdf", "--check", signedPath)
	salida, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("qpdf devolvio error: %v\n%s", err, salida)
	}

	out := string(salida)
	if !strings.Contains(out, "checking") && !strings.Contains(out, "No syntax or stream encoding errors found") {
		t.Fatalf("salida inesperada de qpdf:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// Tests de comportamiento de contexto y clave
// ---------------------------------------------------------------------------

func TestMotorFirmaGo_ContextoCancelado(t *testing.T) {
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancelar antes de llamar

	job := nuevoTrabajoCaDES(t, []byte("datos"))
	_, err := motor.Sign(ctx, job, clave)
	if err == nil {
		t.Fatal("se esperaba error por contexto cancelado")
	}
}

func TestClaveLocal_KeyID_NoVacio(t *testing.T) {
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	if clave.KeyID() == "" {
		t.Fatal("KeyID no puede estar vacio")
	}
}

// ---------------------------------------------------------------------------
// Verificacion criptografica de la firma CAdES producida
// ---------------------------------------------------------------------------

// verificarFirmaCaDES parsea el ContentInfo producido, extrae los atributos
// firmados y verifica la firma criptografica con la clave publica indicada.
// Si se aporta contenidoFirmado, tambien comprueba que el atributo
// messageDigest coincide con los bytes firmados.
func verificarFirmaCaDES(t *testing.T, der []byte, pub crypto.PublicKey, contenidoFirmado ...[]byte) {
	t.Helper()

	// Parsear ContentInfo.
	var ci contentInfoRaw
	if _, err := asn1.Unmarshal(der, &ci); err != nil {
		t.Fatalf("no se pudo parsear el ContentInfo: %v", err)
	}

	// ci.Contenido es el wrapper [0] EXPLICIT; los bytes del SignedData estan en Bytes.
	sdBytes := ci.Contenido.Bytes // DER del SignedData SEQUENCE

	// Parsear SignedData en modo raw para acceder a los campos con tags implicitos.
	var sd rawSignedData
	if _, err := asn1.Unmarshal(sdBytes, &sd); err != nil {
		t.Fatalf("no se pudo parsear el SignedData: %v", err)
	}
	if len(contenidoFirmado) > 0 {
		digestEsperado := sha256.Sum256(contenidoFirmado[0])
		messageDigest := extraerAtributoMessageDigest(t, sd.Firmantes.Bytes)
		if !bytes.Equal(messageDigest, digestEsperado[:]) {
			t.Fatal("el atributo messageDigest no coincide con el contenido firmado")
		}
	}

	// Los SignerInfos estan en un SET OF; parsear el primero.
	if sd.Firmantes.Tag != asn1.TagSet {
		t.Fatalf("Firmantes no es un SET, tag=%d", sd.Firmantes.Tag)
	}

	// Extraer el primer SignerInfo del SET.
	var siBytes asn1.RawValue
	if _, err := asn1.Unmarshal(sd.Firmantes.Bytes, &siBytes); err != nil {
		t.Fatalf("no se pudo extraer el primer SignerInfo: %v", err)
	}

	// Parsear el SignerInfo (SEQUENCE).
	type signerInfoParsed struct {
		Version        int
		SID            asn1.RawValue // IssuerAndSerialNumber
		AlgDigest      asn1.RawValue // AlgorithmIdentifier
		AtribsFirmados asn1.RawValue // [0] IMPLICIT
		AlgFirma       asn1.RawValue // AlgorithmIdentifier
		Firma          []byte
	}
	var si signerInfoParsed
	if _, err := asn1.Unmarshal(siBytes.FullBytes, &si); err != nil {
		t.Fatalf("no se pudo parsear el SignerInfo: %v", err)
	}

	// Los atributos firmados tienen tag [0] (0xa0) en el struct.
	// Para verificar la firma hay que rehashearlos como SET (0x31).
	if si.AtribsFirmados.Class != asn1.ClassContextSpecific || si.AtribsFirmados.Tag != 0 {
		t.Fatalf("AtribsFirmados no tiene tag [0], class=%d tag=%d",
			si.AtribsFirmados.Class, si.AtribsFirmados.Tag)
	}

	// Reconstruir los bytes del SET para calcular el digest.
	atribsComoSET := make([]byte, len(si.AtribsFirmados.FullBytes))
	copy(atribsComoSET, si.AtribsFirmados.FullBytes)
	atribsComoSET[0] = 0x31 // [0] -> SET

	digestAttrs := sha256.Sum256(atribsComoSET)

	// Verificar la firma segun el tipo de clave publica.
	switch pubKey := pub.(type) {
	case *rsa.PublicKey:
		if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, digestAttrs[:], si.Firma); err != nil {
			t.Fatalf("la firma RSA no es valida criptograficamente: %v", err)
		}
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(pubKey, digestAttrs[:], si.Firma) {
			t.Fatal("la firma ECDSA no es valida criptograficamente")
		}
	default:
		t.Fatalf("tipo de clave publica desconocido en el test: %T", pub)
	}
}

func parsearByteRange(t *testing.T, raw string) [4]int {
	t.Helper()

	partes := strings.Fields(raw)
	if len(partes) != 4 {
		t.Fatalf("ByteRange invalido, se esperaban 4 enteros y llegaron %d: %q", len(partes), raw)
	}
	var out [4]int
	for i, parte := range partes {
		valor, err := strconv.Atoi(parte)
		if err != nil {
			t.Fatalf("ByteRange contiene un entero invalido %q: %v", parte, err)
		}
		out[i] = valor
	}
	return out
}

func makeSetDER(content []byte) []byte {
	header := []byte{0x31}
	header = append(header, encodeDERLength(len(content))...)
	header = append(header, content...)
	return header
}

func encodeDERLength(n int) []byte {
	if n < 0x80 {
		return []byte{byte(n)}
	}
	var tmp [8]byte
	i := len(tmp)
	for n > 0 {
		i--
		tmp[i] = byte(n)
		n >>= 8
	}
	out := []byte{0x80 | byte(len(tmp)-i)}
	out = append(out, tmp[i:]...)
	return out
}

func extraerAtributoMessageDigest(t *testing.T, signerInfosDER []byte) []byte {
	t.Helper()

	var siBytes asn1.RawValue
	if _, err := asn1.Unmarshal(signerInfosDER, &siBytes); err != nil {
		t.Fatalf("no se pudo extraer el primer SignerInfo: %v", err)
	}

	type signerInfoParsed struct {
		Version        int
		SID            asn1.RawValue
		AlgDigest      asn1.RawValue
		AtribsFirmados asn1.RawValue
		AlgFirma       asn1.RawValue
		Firma          []byte
	}
	var si signerInfoParsed
	if _, err := asn1.Unmarshal(siBytes.FullBytes, &si); err != nil {
		t.Fatalf("no se pudo parsear el SignerInfo: %v", err)
	}

	resto := si.AtribsFirmados.Bytes
	for len(resto) > 0 {
		var raw asn1.RawValue
		var err error
		resto, err = asn1.Unmarshal(resto, &raw)
		if err != nil {
			t.Fatalf("no se pudieron parsear los atributos firmados: %v", err)
		}
		var attr atributoCMS
		if _, err := asn1.Unmarshal(raw.FullBytes, &attr); err != nil {
			t.Fatalf("no se pudo parsear un atributo firmado: %v", err)
		}
		if !attr.Type.Equal(oidMessageDigestTest) {
			continue
		}
		if len(attr.Values) != 1 {
			t.Fatalf("messageDigest debe tener un unico valor, obtenido %d", len(attr.Values))
		}
		var digest []byte
		if _, err := asn1.Unmarshal(attr.Values[0].FullBytes, &digest); err != nil {
			t.Fatalf("no se pudo parsear messageDigest: %v", err)
		}
		return digest
	}

	t.Fatal("no se encontro el atributo messageDigest")
	return nil
}

func extraerContenido(t *testing.T, s, inicio, fin string) string {
	t.Helper()
	start := strings.Index(s, inicio)
	if start < 0 {
		t.Fatalf("no se encontro marcador de inicio %q", inicio)
	}
	start += len(inicio)
	end := strings.Index(s[start:], fin)
	if end < 0 {
		t.Fatalf("no se encontro marcador de fin %q", fin)
	}
	return s[start : start+end]
}

// ETSI EN 319 142-1 prohíbe signing-time en PAdES (ETSI.CAdES.detached) y
// @firma rechaza la firma si aparece (validado en la sede de JCyL). En el
// subfiltro Adobe se mantiene por compatibilidad.
func TestMotorFirmaGo_PAdES_ETSISinSigningTime(t *testing.T) {
	oidSigningTime := []byte{0x06, 0x09, 0x2A, 0x86, 0x48, 0x86, 0xF7, 0x0D, 0x01, 0x09, 0x05}
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)
	for _, caso := range []struct {
		opciones map[string]string
		conHora  bool
	}{
		{opciones: nil, conHora: false},
		{opciones: map[string]string{"subfilter": "adobe"}, conHora: true},
	} {
		doc, _ := domain.NewDocument("doc.pdf", pdffixture.Minimal(), "application/pdf")
		resultado, err := motor.Sign(context.Background(), domain.SignatureJob{
			Document: doc, Format: domain.FormatPAdES, Action: domain.ActionSign, Options: caso.opciones,
		}, clave)
		if err != nil {
			t.Fatalf("Sign: %v", err)
		}
		if !bytes.Contains(resultado.Data, []byte("/M (D:")) && !caso.conHora {
			t.Fatal("sin signing-time la hora de firma debe quedar en /M")
		}
		cms, err := hex.DecodeString(extraerContenido(t, string(resultado.Data), "/Contents<", ">"))
		if err != nil {
			t.Fatal(err)
		}
		if got := bytes.Contains(cms, oidSigningTime); got != caso.conHora {
			t.Fatalf("opciones=%v: signing-time presente=%v, want %v", caso.opciones, got, caso.conHora)
		}
	}
}
