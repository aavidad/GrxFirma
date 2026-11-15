// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"testing"

	"github.com/digitorus/pkcs7"

	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

// Lote 39a, punto 3: al verificar con una excepción activa, el aviso se metía
// solo en result.Warnings («Advertencias» plegadas en Qt) y nunca en el
// registro de la operación, que es lo que las interfaces enseñan en el aviso
// común. Cada uso permitido debe anotarlo una vez por el mismo camino que
// excepciones.Require; un uso bloqueado no lo anota.

func cadesSHA1V39(t *testing.T) []byte {
	t.Helper()
	priv, cert := certForTest(t, "CAdES SHA-1 lote 39a")
	datos, err := pkcs7.NewSignedData([]byte("documento firmado en 2012"))
	if err != nil {
		t.Fatal(err)
	}
	datos.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA1)
	if err := datos.AddSigner(cert, priv, pkcs7.SignerInfoConfig{}); err != nil {
		t.Fatal(err)
	}
	cms, err := datos.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return cms
}

func vecesV39(claves []string, clave string) int {
	n := 0
	for _, c := range claves {
		if c == clave {
			n++
		}
	}
	return n
}

func TestV39VerificarSHA1ConAjusteAnotaElAvisoUnaVez(t *testing.T) {
	activarExcepcionPrueba(t, "sha1_leer")
	ctx, avisos := domain.ConAvisosOperacion(context.Background())
	doc := domain.Document{Name: "antigua.csig", Content: cadesSHA1V39(t)}
	if _, _, err := NewMultiVerifier().Verify(ctx, doc, domain.CertificateChain{}); err != nil {
		t.Fatal(err)
	}
	if n := vecesV39(avisos.Claves(), excepciones.Warning("sha1_leer")); n != 1 {
		t.Fatalf("el aviso SHA-1 debe anotarse una vez en la operación: %v", avisos.Claves())
	}
}

func TestV39VerificarSHA1SinAjusteNoAnotaElAvisoDeUso(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_sha1_leer")
	excepciones.SetForTest(t, "sha1_leer", false)
	ctx, avisos := domain.ConAvisosOperacion(context.Background())
	doc := domain.Document{Name: "antigua.csig", Content: cadesSHA1V39(t)}
	if _, _, err := NewMultiVerifier().Verify(ctx, doc, domain.CertificateChain{}); err != nil {
		t.Fatal(err)
	}
	if vecesV39(avisos.Claves(), excepciones.Warning("sha1_leer")) != 0 {
		t.Fatalf("un uso bloqueado no puede decir que se comprobó: %v", avisos.Claves())
	}
}

func TestV39VerificarXMLConC14NAntiguaAnotaElAviso(t *testing.T) {
	activarExcepcionPrueba(t, "xml_c14n_antigua")
	ctx, avisos := domain.ConAvisosOperacion(context.Background())
	doc := domain.Document{Name: "firma.xsig", MIMEType: "application/xml", Content: leerFirmaGrxFirmaAnterior(t, "atributos_tab_lf.xsig")}
	if _, _, err := NewXAdESVerifier().Verify(ctx, doc, domain.CertificateChain{}); err != nil {
		t.Fatal(err)
	}
	if n := vecesV39(avisos.Claves(), excepciones.Warning("xml_c14n_antigua")); n != 1 {
		t.Fatalf("el aviso de canonicalización antigua debe anotarse una vez: %v", avisos.Claves())
	}
}
