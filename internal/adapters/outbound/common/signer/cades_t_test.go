// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"crypto"
	"encoding/asn1"
	"testing"

	"grxfirma/internal/domain"
)

// TestSignerCAdEST_AñadeTimestamp verifica que la firma CAdES-T incluye el OID
// id-aa-signatureTimeStampToken en los atributos no firmados del SignerInfo.
func TestSignerCAdEST_AñadeTimestamp(t *testing.T) {
	priv, cert := certForTest(t, "RSA")
	base := NewCAdESBESDetached()
	tsa := &tsaMock{token: buildMinimalTST(t)}
	signer := NewSignerCAdEST(base, tsa)

	doc, err := domain.NewDocument("documento.txt", []byte("contenido a sellar"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}

	result, err := signer.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{
		ID:          "clave-test",
		Signer:      priv,
		Certificate: cert,
	})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if len(result.Data) == 0 {
		t.Fatal("resultado de firma vacío")
	}

	// Verificar que el OID signatureTimeStamp aparece en los atributos no firmados
	verificaTimestampEnSignerInfo(t, result.Data)
}

type tsaMock struct {
	token []byte
	err   error
}

func (m *tsaMock) RequestTimestamp(context.Context, []byte, crypto.Hash) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.token, nil
}

func buildMinimalTST(t *testing.T) []byte {
	t.Helper()
	tstContentInfo := contentInfo{
		ContentType: oidSignedData,
		Content: asn1.RawValue{
			Class:      2,
			Tag:        0,
			IsCompound: true,
			Bytes:      []byte{},
		},
	}
	tstDER, err := asn1.Marshal(tstContentInfo)
	if err != nil {
		t.Fatalf("error construyendo TST minimo: %v", err)
	}
	return tstDER
}

// verificaTimestampEnSignerInfo parsea el CMS y comprueba que el primer SignerInfo
// contiene el atributo no firmado id-aa-signatureTimeStampToken.
func verificaTimestampEnSignerInfo(t *testing.T, cmsData []byte) {
	t.Helper()

	var ci contentInfo
	if _, err := asn1.Unmarshal(cmsData, &ci); err != nil {
		t.Fatalf("error parseando ContentInfo: %v", err)
	}

	var sd signedDataConUnsigned
	if _, err := asn1.Unmarshal(ci.Content.Bytes, &sd); err != nil {
		t.Fatalf("error parseando SignedData: %v", err)
	}

	if len(sd.SignerInfos) == 0 {
		t.Fatal("no hay SignerInfos en el SignedData")
	}

	si := sd.SignerInfos[0]

	// Los atributos no firmados están en UnsignedAttributes [1] IMPLICIT SET OF
	if len(si.UnsignedAttributes.Bytes) == 0 {
		t.Fatal("el SignerInfo no contiene atributos no firmados")
	}

	// Reconstruir el SET DER para parsear los atributos
	setDER := makeSetDER(si.UnsignedAttributes.Bytes)
	var attrs []attribute
	if _, err := asn1.UnmarshalWithParams(setDER, &attrs, "set"); err != nil {
		t.Fatalf("error parseando atributos no firmados: %v", err)
	}

	found := false
	for _, attr := range attrs {
		if attr.Type.Equal(oidSignatureTimeStamp) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no se encontró el OID signatureTimeStamp (%v) en los atributos no firmados", oidSignatureTimeStamp)
	}
}

func TestCAdESTPreservesCoSignersAndCounterSignatures(t *testing.T) {
	original := []byte("documento de prueba")
	first, _ := firmarCAdESParaPrueba(t, original, domain.ActionSign, "A", nil)
	co, _ := firmarCAdESParaPrueba(t, first, domain.ActionCoSign, "B", nil)
	counter, _ := firmarCAdESParaPrueba(t, co, domain.ActionCounterSign, "C", nil)
	tsa := &tsaMock{token: buildMinimalTST(t)}
	engine := NewSignerCAdEST(NewCAdESBESDetached(), tsa)
	for _, data := range [][]byte{co, counter} {
		before, err := parseSignedDataParts(data)
		if err != nil {
			t.Fatal(err)
		}
		stamped, err := engine.addTimestamp(context.Background(), data)
		if err != nil {
			t.Fatal(err)
		}
		after, err := parseSignedDataParts(stamped)
		if err != nil {
			t.Fatal(err)
		}
		if len(after.signerInfos) != len(before.signerInfos) {
			t.Fatal("se perdieron firmantes")
		}
		for _, signerDER := range before.signerInfos {
			elems, err := splitSequence(signerDER)
			if err != nil {
				t.Fatal(err)
			}
			signature, _, err := signerInfoSignatureAndUnsigned(elems)
			if err != nil || !bytes.Contains(stamped, signature) {
				t.Fatal("se altero una firma existente")
			}
		}
		if bytes.Count(stamped, mustMarshalOID(t, oidCounterSignature)) != bytes.Count(data, mustMarshalOID(t, oidCounterSignature)) {
			t.Fatal("se perdieron contrafirmas")
		}
		if bytes.Count(stamped, mustMarshalOID(t, oidSignatureTimeStamp)) != 2 {
			t.Fatal("faltan sellos en las hojas")
		}
		report, signers, err := NewCAdESVerifier().VerifyDetachedCMS(context.Background(), stamped, original)
		if err != nil || report.Integrity.Status != domain.VerificationStatusValid || len(signers) != 2 {
			t.Fatalf("firma sellada no verifica: %v %+v", err, report)
		}
	}
}
