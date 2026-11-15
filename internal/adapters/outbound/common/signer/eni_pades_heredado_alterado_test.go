// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"testing"

	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

// Revisión del lote 17a: una firma PDF antigua (adbe.x509.rsa_sha1) cuyo
// documento se alteró después, con otra firma encima, solo deja el PDF «no
// verificado» al verificar; el cotejo previo al ENI tiene que rechazarlo.
func TestComprobarIntegridadPAdESRechazaFirmaAntiguaAlterada(t *testing.T) {
	activarExcepcionPrueba(t, "sha1_leer")
	priv, cert := certForTest(t, "PDF-PKCS1")
	primera := pdfFirmaHeredadaPrueba(t, subFiltroX509RSASHA1, certHexPDF(cert), firmaPKCS1Prueba(priv, crypto.SHA1))
	primera = bytes.Replace(primera, []byte("D:20140101120000Z"), []byte("D:20150101120000Z"), 1)

	priv2, cert2 := certForTest(t, "Encima")
	clave := &LocalSigningKey{ID: "k", Signer: priv2, Certificate: cert2}
	moderna := pdfFirmaHeredadaSobre(t, string(primera), 2, "adbe.pkcs7.detached", "", func(datos []byte) []byte {
		der, _, err := signCAdESBES(datos, clave, cmsHashSpecs[0], false)
		if err != nil {
			t.Fatal(err)
		}
		return der
	})
	antigua := pdfFirmaHeredadaSobre(t, string(primera), 2, subFiltroX509RSASHA1, certHexPDF(cert2), firmaPKCS1Prueba(priv2, crypto.SHA256))

	for nombre, pdf := range map[string][]byte{"moderna_encima": moderna, "antigua_encima": antigua} {
		if res, _, err := verificarPDFHeredado(t, pdf); err == nil && res.Valid {
			t.Fatalf("%s: un PDF con una firma alterada no puede salir válido", nombre)
		}
		if err := ComprobarIntegridadPAdES(context.Background(), pdf); !errors.Is(err, ErrFirmaNoCorrespondeOriginal) {
			t.Fatalf("%s: el cotejo ENI debe rechazar, err=%v", nombre, err)
		}
	}
}

// La firma antigua íntegra sigue pasando el cotejo, con el aviso de SHA-1.
func TestComprobarIntegridadPAdESFirmaAntiguaIntegraAvisaSHA1(t *testing.T) {
	activarExcepcionPrueba(t, "sha1_leer")
	priv, cert := certForTest(t, "PDF-PKCS1")
	pdf := pdfFirmaHeredadaPrueba(t, subFiltroX509RSASHA1, certHexPDF(cert), firmaPKCS1Prueba(priv, crypto.SHA1))
	if err := ComprobarIntegridadPAdES(context.Background(), pdf); !errors.Is(err, ErrFirmaCorrespondeConSHA1) {
		t.Fatalf("err=%v, quiere ErrFirmaCorrespondeConSHA1", err)
	}
}

func TestComprobarIntegridadPAdESSHA1SinAjusteNoProduceENI(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_sha1_leer")
	excepciones.SetForTest(t, "sha1_leer", false)
	priv, cert := certForTest(t, "PDF-PKCS1")
	pdf := pdfFirmaHeredadaPrueba(t, subFiltroX509RSASHA1, certHexPDF(cert), firmaPKCS1Prueba(priv, crypto.SHA1))
	var block *excepciones.Block
	if err := ComprobarIntegridadPAdES(context.Background(), pdf); !errors.As(err, &block) || block.ID != "sha1_leer" {
		t.Fatalf("el cotejo ENI debe bloquear la firma no evaluada: %v", err)
	}
}
