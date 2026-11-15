// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/digitorus/pkcs7"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

// cadesSHA1Movil firma datos con SHA-1, como AutoFirma hasta 2014.
func cadesSHA1Movil(t *testing.T, datos []byte, separada bool) []byte {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(2012), Subject: pkix.Name{CommonName: "Firmante de 2012"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	firmado, err := pkcs7.NewSignedData(datos)
	if err != nil {
		t.Fatal(err)
	}
	firmado.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA1)
	if err := firmado.AddSigner(cert, priv, pkcs7.SignerInfoConfig{}); err != nil {
		t.Fatal(err)
	}
	if separada {
		firmado.Detach()
	}
	cms, err := firmado.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return cms
}

func peticionENISHA1(cms []byte) eniDocumentRequest {
	return eniDocumentRequest{
		SignatureBase64: base64.StdEncoding.EncodeToString(cms),
		Organs:          []string{"L01180877"},
		Origin:          "administracion",
		State:           "EE01",
		DocumentType:    "TD10",
		CaptureDate:     "2026-10-05T10:00:00+02:00",
		ContentFormat:   "TXT",
	}
}

// Lote 16c (KD-05 y SHA-1): antes de KD-05 una CAdES implícita con SHA-1 se
// envolvía en el ENI sin cotejar; con KD-05 se rechazaba por ser SHA-1. Ahora
// se coteja igual y, si corresponde, el ENI sale con el aviso.
func TestCreateENIDocumentCAdESSHA1QueCorrespondeLlevaAviso(t *testing.T) {
	machinepolicy.UnsetForTest(t, machinepolicy.PermitirSHA1Legacy)
	excepciones.SetForTest(t, "sha1_conservar", true)
	facade := newAndroidFacadeForTest(t)
	original := []byte("acta de la sesión de 2012 con importe 100")
	cms := cadesSHA1Movil(t, original, false)

	raw, err := facade.CreateENIDocumentJSON(mustJSON(t, peticionENISHA1(cms)))
	if err != nil {
		t.Fatalf("una CAdES SHA-1 que corresponde debe envolverse: %v", err)
	}
	var respuesta eniDocumentResponse
	decodeResponse(t, raw, &respuesta)
	if respuesta.ContentBase64 == "" || len(respuesta.Warnings) != 1 || respuesta.Warnings[0] != commonsigner.AvisoFirmaSHA1Corresponde {
		t.Fatalf("el ENI debe salir con el aviso de SHA-1: %+v", respuesta.Warnings)
	}

	// Explícita con su original: igual.
	separada := cadesSHA1Movil(t, original, true)
	explicita := peticionENISHA1(separada)
	explicita.OriginalBase64 = base64.StdEncoding.EncodeToString(original)
	if raw, err = facade.CreateENIDocumentJSON(mustJSON(t, explicita)); err != nil {
		t.Fatalf("una CAdES explícita SHA-1 con su original debe envolverse: %v", err)
	}
	decodeResponse(t, raw, &respuesta)
	if len(respuesta.Warnings) != 1 || respuesta.Warnings[0] != commonsigner.AvisoFirmaSHA1Corresponde {
		t.Fatalf("falta el aviso de SHA-1 en la explícita: %+v", respuesta.Warnings)
	}
	explicita.OriginalBase64 = base64.StdEncoding.EncodeToString([]byte("otra acta"))
	if _, err := facade.CreateENIDocumentJSON(mustJSON(t, explicita)); err == nil || err.Error() != eniErrorMismatch {
		t.Fatalf("una CAdES SHA-1 con otro original no corresponde: %v", err)
	}

	// Implícita alterada después de firmar: no corresponde.
	i := bytes.Index(cms, []byte("importe 100"))
	if i < 0 {
		t.Fatal("no se encontró el contenido encapsulado")
	}
	cms[i+len("importe 1")] = '9'
	if _, err := facade.CreateENIDocumentJSON(mustJSON(t, peticionENISHA1(cms))); err == nil || err.Error() != eniErrorMismatch {
		t.Fatalf("una CAdES SHA-1 alterada debe rechazarse: %v", err)
	}
}

func TestCreateENIDocumentCAdESSHA1LoCierraLaPolitica(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirSHA1Legacy, false)
	facade := newAndroidFacadeForTest(t)
	cms := cadesSHA1Movil(t, []byte("acta de 2012"), false)
	if _, err := facade.CreateENIDocumentJSON(mustJSON(t, peticionENISHA1(cms))); !isSHA1PolicyBlock(err) {
		t.Fatalf("con SHA-1 prohibido por política el ENI se rechaza: %v", err)
	}
}

// La cofirma con el original de una CAdES explícita SHA-1: si el original
// corresponde, la comprobación sigue con el aviso de SHA-1 en vez de decir
// que el original no corresponde. La firma completa la prueba
// TestSignJSONCofirmaCAdESExplicitaSHA1ConOriginal (lote 17b).
func TestComprobarOriginalCofirmaSHA1(t *testing.T) {
	machinepolicy.UnsetForTest(t, machinepolicy.PermitirSHA1Legacy)
	excepciones.SetForTest(t, "sha1_conservar", true)
	original := []byte("convenio firmado en 2012")
	csig := cadesSHA1Movil(t, original, true)
	avisos, err := comprobarOriginalCofirma(csig, original, false)
	if err != nil || len(avisos) != 1 || avisos[0] != commonsigner.AvisoFirmaSHA1Corresponde {
		t.Fatalf("el original de una CAdES SHA-1 corresponde, con aviso: avisos=%v err=%v", avisos, err)
	}
	if _, err := comprobarOriginalCofirma(csig, []byte("otro convenio"), false); err == nil || err.Error() != mobileErrorOriginalNoCorresponde {
		t.Fatalf("un original ajeno sigue sin corresponder: %v", err)
	}
	machinepolicy.SetForTest(t, machinepolicy.PermitirSHA1Legacy, false)
	if _, err := comprobarOriginalCofirma(csig, original, false); !isSHA1PolicyBlock(err) {
		t.Fatalf("con SHA-1 prohibido por política se rechaza: %v", err)
	}
}

// Lote 17b (3c): la fachada pasa el original al motor. Una CAdES explícita
// cuyos firmantes solo usan SHA-1 se cofirma con su original y el aviso de
// SHA-1; la cofirma resultante (mixta, SHA-1 y SHA-256) también.
func TestSignJSONCofirmaCAdESExplicitaSHA1ConOriginal(t *testing.T) {
	machinepolicy.UnsetForTest(t, machinepolicy.PermitirSHA1Legacy)
	excepciones.SetForTest(t, "sha1_conservar", true)
	facade := newAndroidFacadeForTest(t)
	id := importPKCS12(t, facade, ephemeralRSAPKCS12(t, "cofirmante"), "cofirmante")
	original := []byte("convenio firmado en 2012")
	firma := cadesSHA1Movil(t, original, true)
	for _, paso := range []string{"solo_sha1", "mixta"} {
		req := peticionCofirmaKD03(id, "cosign", firma)
		req.OriginalBase64 = base64.StdEncoding.EncodeToString(original)
		raw, err := facade.SignJSON(mustJSON(t, req))
		if err != nil {
			t.Fatalf("%s: con el original la cofirma debe hacerse: %v", paso, err)
		}
		var firmado signResponse
		decodeResponse(t, raw, &firmado)
		if len(firmado.Warnings) != 1 || firmado.Warnings[0] != commonsigner.AvisoFirmaSHA1Corresponde {
			t.Fatalf("%s: la cofirma debe llevar solo el aviso de SHA-1: %v", paso, firmado.Warnings)
		}
		if firma, err = base64.StdEncoding.DecodeString(firmado.SignedContentBase64); err != nil {
			t.Fatal(err)
		}
		if err := commonsigner.CotejarCAdESExplicita(firma, original); !errors.Is(err, commonsigner.ErrFirmaCorrespondeConSHA1) {
			t.Fatalf("%s: todos los firmantes deben cubrir el original: %v", paso, err)
		}
	}
}

// La política prohíbe el caso de conservación, no un fallo criptográfico.
func isSHA1PolicyBlock(err error) bool {
	var b *excepciones.Block
	return errors.As(err, &b) && b.ID == "sha1_conservar" && b.BlockKey == "excepciones.sha1_conservar.bloqueo" && !b.Activable && b.OrganizationKey != ""
}
