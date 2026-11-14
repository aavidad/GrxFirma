// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"encoding/asn1"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

// Lote 17a: muestras oficiales de AutoFirma 1.9 que daban error al
// verificar sin ser un fallo de la firma.

func verificadorCAdESSinRed() *CAdESVerifier {
	return NewCAdESVerifierWithChecker(NewRevocationCheckerOffline())
}

func firmaCAdESPrueba(t *testing.T, datos []byte, implicita bool) []byte {
	t.Helper()
	priv, cert := certForTest(t, "CAdES-17a")
	der, _, err := signCAdESBES(datos, &LocalSigningKey{ID: "k", Signer: priv, Certificate: cert}, cmsHashSpecs[0], implicita)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

// firmaCAdESImplicitaVaciaPrueba firma un eContent presente y vacío, como la
// muestra cades_claimed_role.csig (signCAdESBES no firma datos vacíos).
func firmaCAdESImplicitaVaciaPrueba(t *testing.T) []byte {
	t.Helper()
	priv, cert := certForTest(t, "CAdES-vacia")
	spec := cmsHashSpecs[0]
	attrsDER, attrsRaw, err := buildSignedAttributes(cert, cmsDigest(spec.hash, nil), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	firma, algoritmo, _, err := signAttributes(cmsDigest(spec.hash, attrsDER), priv, spec)
	if err != nil {
		t.Fatal(err)
	}
	certs, err := buildCertificateSet(cert, nil)
	if err != nil {
		t.Fatal(err)
	}
	vacio, _ := asn1.Marshal([]byte{})
	sd := signedData{
		Version:          1,
		DigestAlgorithms: []algorithmIdentifier{{Algorithm: spec.digestOID}},
		EncapContentInfo: encapContentInfo{
			EContentType: oidData,
			EContent:     asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: vacio},
		},
		Certificates: certs,
		SignerInfos: []signerInfo{{
			Version:            1,
			SID:                issuerAndSerialNumber{Issuer: asn1.RawValue{FullBytes: cert.RawIssuer}, SerialNumber: cert.SerialNumber},
			DigestAlgorithm:    algorithmIdentifier{Algorithm: spec.digestOID},
			SignedAttributes:   attrsRaw,
			SignatureAlgorithm: algoritmo,
			Signature:          firma,
		}},
	}
	sdDER, err := asn1.Marshal(sd)
	if err != nil {
		t.Fatal(err)
	}
	out, err := asn1.Marshal(contentInfo{ContentType: oidSignedData, Content: asn1.RawValue{Class: 2, Tag: 0, IsCompound: true, Bytes: sdDER}})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// aBERIndefinido codifica en BER con longitud indefinida todo lo construido y
// trocea los OCTET STRING largos, como BouncyCastle en modo flujo.
func aBERIndefinido(node tlvNode) []byte {
	tag := node.TagClass | byte(node.Tag&0x1f)
	if !node.Constructed && node.TagClass == 0 && node.Tag == 4 && len(node.Content) > 16 {
		out := []byte{tag | 0x20, 0x80}
		for resto := node.Content; len(resto) > 0; {
			n := min(7, len(resto))
			out = append(out, encodeTLV(tlvNode{Tag: 4, Content: resto[:n]})...)
			resto = resto[n:]
		}
		return append(out, 0, 0)
	}
	if !node.Constructed {
		return encodeTLV(node)
	}
	out := []byte{tag | 0x20, 0x80}
	for _, hijo := range node.Children {
		out = append(out, aBERIndefinido(hijo)...)
	}
	return append(out, 0, 0)
}

func cmsEnBER(t *testing.T, der []byte) []byte {
	t.Helper()
	nodo, _, err := decodeTLV(der, 0)
	if err != nil {
		t.Fatal(err)
	}
	ber := aBERIndefinido(nodo)
	if _, err := parseContentInfo(ber); err == nil {
		t.Fatal("la conversión a BER no ha cambiado nada: la prueba no prueba")
	}
	return ber
}

func TestCAdES_ExplicitaSinOriginalEsIntegridadNoComprobable(t *testing.T) {
	datos := []byte("acta del pleno")
	firma := firmaCAdESPrueba(t, datos, false)
	doc := domain.Document{Name: "acta.csig", Content: firma}

	res, firmantes, err := verificadorCAdESSinRed().Verify(context.Background(), doc, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("una CAdES explícita sin el original no es un error del programa: %v", err)
	}
	if res.Valid || res.Integrity.Status != domain.VerificationStatusUnknown || res.Coverage != domain.CoberturaNoAcreditada {
		t.Fatalf("valid=%v integridad=%s cobertura=%s; se esperaba integridad sin comprobar", res.Valid, res.Integrity.Status, res.Coverage)
	}
	if res.Reason != motivoCAdESSinOriginal || !contieneString(res.Warnings, motivoCAdESSinOriginal) {
		t.Fatalf("motivo=%q avisos=%v", res.Reason, res.Warnings)
	}
	if len(firmantes) != 1 {
		t.Fatalf("firmantes = %d; el firmante sí se comprueba", len(firmantes))
	}
	if len(res.Material.HuellasContenidoFirmado) != 0 {
		t.Fatal("sin el original no hay contenido firmado del que dar huella")
	}

	// Con el original, la integridad se comprueba como siempre.
	conOriginal, _, err := verificadorCAdESSinRed().VerifyDetached(context.Background(), doc, domain.Document{Name: "acta.txt", Content: datos}, domain.CertificateChain{})
	if err != nil || conOriginal.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("con el original: err=%v integridad=%s", err, conOriginal.Integrity.Status)
	}
	// Y un original alterado sigue rechazándose.
	if _, _, err := verificadorCAdESSinRed().VerifyDetached(context.Background(), doc, domain.Document{Name: "acta.txt", Content: []byte("acta del pleno cambiada")}, domain.CertificateChain{}); err == nil {
		t.Fatal("un original alterado no puede verificar")
	}
}

// Sin el original, la firma del firmante se sigue comprobando: unos atributos
// firmados alterados son un error, no una «integridad sin comprobar».
func TestCAdES_ExplicitaSinOriginalConAtributosAlteradosEsError(t *testing.T) {
	firma := firmaCAdESPrueba(t, []byte("acta del pleno"), false)
	ci, _ := parseContentInfo(firma)
	sd, _ := parseSignedData(ci.Content.Bytes)
	infos, _ := parseSignerInfos(sd.SignerInfos)
	resumen := infos[0].SignedAttributes.Bytes
	i := bytes.Index(firma, resumen)
	if i < 0 {
		t.Fatal("atributos firmados no localizados")
	}
	alterada := append([]byte(nil), firma...)
	alterada[i+len(resumen)-1] ^= 0x01 // último byte del messageDigest
	_, _, err := verificadorCAdESSinRed().Verify(context.Background(), domain.Document{Name: "a.csig", Content: alterada}, domain.CertificateChain{})
	if err == nil {
		t.Fatal("unos atributos firmados alterados no pueden pasar por «sin original»")
	}
}

// El aviso de SHA-1 dice que el documento no ha cambiado; sin el original eso
// no se sabe, así que no se pone (queda la evidencia del algoritmo).
func TestCAdES_ExplicitaSinOriginalNoDiceQueElDocumentoNoHaCambiado(t *testing.T) {
	res := domain.NewVerificationSuccess("CAdES", "x", nil)
	res = anotarAlgoritmoAntiguo(res, []string{detalleAlgoritmoSHA1})
	if contieneString(res.Warnings, motivoAlgoritmoAntiguo) || len(algoritmosAntiguos(res)) != 1 {
		t.Fatalf("avisos=%v evidencia=%v", res.Warnings, res.Evidence)
	}
}

func TestCAdES_ImplicitaConContenidoVacioSeVerificaConAviso(t *testing.T) {
	firma := firmaCAdESImplicitaVaciaPrueba(t)
	res, _, err := verificadorCAdESSinRed().Verify(context.Background(), domain.Document{Name: "vacia.csig", Content: firma}, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("una firma correcta de un contenido vacío no es un error: %v", err)
	}
	if res.Integrity.Status != domain.VerificationStatusValid || !contieneString(res.Warnings, motivoContenidoFirmadoVacio) {
		t.Fatalf("integridad=%s avisos=%v", res.Integrity.Status, res.Warnings)
	}

	// Vaciar el contenido de otra firma no la deja verificar.
	otra := firmaCAdESPrueba(t, []byte("x"), true)
	vaciada, err := reescribirSignedDataParaPrueba(otra, func(sd *signedDataRaw) error {
		vacio, _ := asn1.Marshal([]byte{})
		der, err := asn1.Marshal(encapContentInfo{EContentType: oidData, EContent: asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: vacio}})
		sd.EncapContentInfo = asn1.RawValue{FullBytes: der}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := verificadorCAdESSinRed().Verify(context.Background(), domain.Document{Name: "v.csig", Content: vaciada}, domain.CertificateChain{}); err == nil {
		t.Fatal("vaciar el contenido de una firma no puede verificar")
	}
}

func TestCAdES_FirmaEnBERSeVerifica(t *testing.T) {
	datos := []byte(strings.Repeat("contenido troceado en BER ", 8))
	t.Run("implícita", func(t *testing.T) {
		ber := cmsEnBER(t, firmaCAdESPrueba(t, datos, true))
		res, _, err := verificadorCAdESSinRed().Verify(context.Background(), domain.Document{Name: "ber.csig", Content: ber}, domain.CertificateChain{})
		if err != nil {
			t.Fatalf("una firma en BER se debe leer: %v", err)
		}
		if res.Integrity.Status != domain.VerificationStatusValid || !contieneString(res.Details, detalleCodificacionBER) {
			t.Fatalf("integridad=%s detalles=%v", res.Integrity.Status, res.Details)
		}
		// Un trozo del contenido alterado sigue saliendo como no íntegro.
		i := bytes.Index(ber, []byte("BER"))
		alterada := append([]byte(nil), ber...)
		alterada[i] = 'X'
		if _, _, err := verificadorCAdESSinRed().Verify(context.Background(), domain.Document{Name: "ber.csig", Content: alterada}, domain.CertificateChain{}); err == nil {
			t.Fatal("un contenido BER alterado no puede verificar")
		}
	})
	t.Run("explícita sin original", func(t *testing.T) {
		ber := cmsEnBER(t, firmaCAdESPrueba(t, datos, false))
		res, _, err := verificadorCAdESSinRed().Verify(context.Background(), domain.Document{Name: "ber.csig", Content: ber}, domain.CertificateChain{})
		if err != nil || res.Integrity.Status != domain.VerificationStatusUnknown {
			t.Fatalf("err=%v integridad=%s", err, res.Integrity.Status)
		}
		con, _, err := verificadorCAdESSinRed().VerifyDetached(context.Background(), domain.Document{Name: "ber.csig", Content: ber}, domain.Document{Name: "o", Content: datos}, domain.CertificateChain{})
		if err != nil || con.Integrity.Status != domain.VerificationStatusValid {
			t.Fatalf("con el original: err=%v integridad=%s", err, con.Integrity.Status)
		}
	})
	t.Run("BER mal formado", func(t *testing.T) {
		ber := cmsEnBER(t, firmaCAdESPrueba(t, datos, true))
		cortado := ber[:len(ber)-4]
		if _, _, err := verificadorCAdESSinRed().Verify(context.Background(), domain.Document{Name: "ber.csig", Content: cortado}, domain.CertificateChain{}); err == nil {
			t.Fatal("un BER cortado no puede verificar")
		}
	})
}
