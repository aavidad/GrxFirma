// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"testing"
	"time"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
)

// KD-03: otra app o persona comparte un .csig explícito («cofírmalo, ya lo he
// firmado yo») sobre un documento D que la app nunca enseña. Sin el original,
// el motor firmaría el messageDigest que eligió quien mandó el fichero.

var documentoOcultoKD03 = []byte("Reconozco una deuda de 90.000 euros con el portador de este documento")

func firmaExplicitaAtacanteKD03(t *testing.T, datos []byte) []byte {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(4242), Subject: pkix.Name{CommonName: "Atacante"},
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
	doc, err := domain.NewDocument("deuda.txt", datos, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	res, err := commonsigner.NewCAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign, Options: map[string]string{},
	}, &commonsigner.LocalSigningKey{ID: "atacante", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatal(err)
	}
	return res.Data
}

func peticionCofirmaKD03(id, accion string, csig []byte) signRequest {
	return signRequest{
		Name: "firmado.csig", MIMEType: "application/pkcs7-signature", Format: "cades",
		Action: accion, CertificateID: id, ContentBase64: base64.StdEncoding.EncodeToString(csig),
	}
}

func TestSignJSONCofirmaCAdESExplicitaPideElOriginal(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	id := importPKCS12(t, facade, ephemeralRSAPKCS12(t, "victima"), "victima")
	csig := firmaExplicitaAtacanteKD03(t, documentoOcultoKD03)
	for _, accion := range []string{"cosign", "countersign"} {
		t.Run(accion, func(t *testing.T) {
			req := peticionCofirmaKD03(id, accion, csig)
			if _, err := facade.SignJSON(mustJSON(t, req)); err == nil || err.Error() != mobileErrorCofirmaSinOriginal {
				t.Fatalf("sin original ni confirmación debe pedir el original: %v", err)
			}

			ajeno := req
			ajeno.OriginalBase64 = base64.StdEncoding.EncodeToString([]byte("acta de la reunión del lunes"))
			if _, err := facade.SignJSON(mustJSON(t, ajeno)); err == nil || err.Error() != mobileErrorOriginalNoCorresponde {
				t.Fatalf("un original que no corresponde debe rechazarse: %v", err)
			}

			conOriginal := req
			conOriginal.OriginalBase64 = base64.StdEncoding.EncodeToString(documentoOcultoKD03)
			raw, err := facade.SignJSON(mustJSON(t, conOriginal))
			if err != nil {
				t.Fatalf("con el original correcto debe firmar: %v", err)
			}
			var firmado signResponse
			decodeResponse(t, raw, &firmado)
			if len(firmado.Warnings) != 0 {
				t.Fatalf("con el original no hay aviso: %v", firmado.Warnings)
			}

			confirmado := req
			confirmado.ConfirmUnseenContent = true
			raw, err = facade.SignJSON(mustJSON(t, confirmado))
			if err != nil {
				t.Fatalf("con la confirmación expresa debe firmar: %v", err)
			}
			firmado = signResponse{}
			decodeResponse(t, raw, &firmado)
			if len(firmado.Warnings) != 1 || firmado.Warnings[0] != mobileAvisoCofirmaSinOriginal {
				t.Fatalf("la firma confirmada sin ver el documento debe llevar el aviso: %v", firmado.Warnings)
			}
		})
	}
}

// La CAdES implícita lleva el documento dentro: se cofirma como siempre.
func TestSignJSONCofirmaCAdESImplicitaNoPideOriginal(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	id := importPKCS12(t, facade, ephemeralRSAPKCS12(t, "victima"), "victima")
	raw, err := facade.SignJSON(mustJSON(t, signRequest{
		Name: "acta.txt", MIMEType: "text/plain", Format: "cades", Action: "sign", CertificateID: id,
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("acta")), Options: map[string]string{"mode": "implicit"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	var firmado signResponse
	decodeResponse(t, raw, &firmado)
	csig, err := base64.StdEncoding.DecodeString(firmado.SignedContentBase64)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.SignJSON(mustJSON(t, peticionCofirmaKD03(id, "cosign", csig))); err != nil {
		t.Fatalf("la cofirma de una CAdES implícita no necesita el original: %v", err)
	}
}

// En el lote no se puede elegir el original de cada firma: una CAdES
// explícita solo se cofirma con la confirmación expresa, y con su aviso.
func TestProcessBatchCofirmaCAdESExplicitaPideConfirmacion(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	id := importPKCS12(t, facade, ephemeralRSAPKCS12(t, "victima"), "victima")
	csig := firmaExplicitaAtacanteKD03(t, documentoOcultoKD03)
	lote := mobileBatchRequest{
		CertificateID: id,
		Items: []batchItemRequest{{
			Name: "firmado.csig", MIMEType: "application/pkcs7-signature", Format: "cades", Action: "cosign",
			ContentBase64: base64.StdEncoding.EncodeToString(csig),
		}},
	}
	if _, err := facade.ProcessBatchJSON(mustJSON(t, lote)); err == nil || err.Error() != mobileErrorCofirmaSinOriginal {
		t.Fatalf("el lote no debe cofirmar a ciegas una CAdES explícita: %v", err)
	}
	lote.ConfirmUnseenContent = true
	raw, err := facade.ProcessBatchJSON(mustJSON(t, lote))
	if err != nil {
		t.Fatalf("con la confirmación expresa el lote debe firmar: %v", err)
	}
	var respuesta mobileBatchResponse
	decodeResponse(t, raw, &respuesta)
	if !respuesta.OK || len(respuesta.Warnings) != 1 || respuesta.Warnings[0] != mobileAvisoCofirmaSinOriginal {
		t.Fatalf("el lote confirmado debe firmar y llevar el aviso: %+v", respuesta)
	}
}
