// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto/x509"
	"testing"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// Regresión: un certificado vigente en fechas no debe quedar marcado como
// válido cuando la revocación no se comprobó o no concluyó. Antes, una firma
// con un único certificado embebido (o cualquier firma XML) devolvía
// certificate=valid sin ninguna evidencia de revocación.
func TestCAdES_CertificadoSinRevocacionConcluyenteNoEsValido(t *testing.T) {
	leaf, ca, leafKey, caKey := generarCadenaLTPrueba(t)
	doc, _ := domain.NewDocument("doc.txt", []byte("contenido sintético"), "text/plain")
	job := domain.SignatureJob{Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign}
	anclas := domain.CertificateChain{DERCertificates: [][]byte{ca.Raw}}

	casos := []struct {
		nombre      string
		cadena      []*x509.Certificate
		verificador *CAdESVerifier
		lt          bool
		quiere      domain.VerificationAspectStatus
	}{
		{
			nombre:      "solo_certificado_firmante",
			verificador: NewCAdESVerifierWithChecker(NewRevocationCheckerOffline()),
			quiere:      domain.VerificationStatusWarning,
		},
		{
			nombre:      "cadena_con_revocacion_no_concluyente",
			cadena:      []*x509.Certificate{ca},
			verificador: NewCAdESVerifierWithChecker(NewRevocationCheckerOffline()),
			quiere:      domain.VerificationStatusWarning,
		},
		{
			nombre:      "revocacion_embebida_autenticada",
			cadena:      []*x509.Certificate{ca},
			verificador: NewCAdESVerifierWithChecker(NewRevocationCheckerOffline()),
			lt:          true,
			quiere:      domain.VerificationStatusValid,
		},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			key := &LocalSigningKey{ID: "reg", Signer: leafKey, Certificate: leaf, Chain: caso.cadena}
			var firmado []byte
			if caso.lt {
				ltSigner := NewSignerCAdESLT(
					NewSignerCAdEST(NewCAdESBESDetached(), &tsaLTMock{token: []byte{0x30, 0x03, 0x02, 0x01, 0x01}}),
					&revocationProviderMock{evidence: ports.RevocationEvidence{CRLs: [][]byte{crearCRLPruebaLT(t, ca, caKey)}}},
				)
				res, err := ltSigner.Sign(context.Background(), job, key)
				if err != nil {
					t.Fatalf("Sign LT: %v", err)
				}
				firmado = res.Data
			} else {
				res, err := NewCAdESBESDetached().Sign(context.Background(), job, key)
				if err != nil {
					t.Fatalf("Sign: %v", err)
				}
				firmado = res.Data
			}
			verificacion, _, err := caso.verificador.VerifyDetachedCMSWithAnchors(context.Background(), firmado, doc.Content, anclas)
			if err != nil {
				t.Fatalf("verificación: %v", err)
			}
			if verificacion.Integrity.Status != domain.VerificationStatusValid {
				t.Fatalf("integridad=%s, se esperaba válida", verificacion.Integrity.Status)
			}
			if verificacion.Certificate.Status != caso.quiere {
				t.Fatalf("certificate=%s (%s), se esperaba %s", verificacion.Certificate.Status, verificacion.Certificate.Reason, caso.quiere)
			}
			if len(verificacion.Material.Firmas) != 1 || len(verificacion.Material.Firmas[0].ValorFirma) == 0 {
				t.Fatalf("falta el material del firmante: %+v", verificacion.Material)
			}
			if got, want := verificacion.Material.HuellasContenidoFirmado, huellaContenido(doc.Content); len(got) != 1 || got[0] != want {
				t.Fatalf("huella del contenido verificado=%v, se esperaba %s", got, want)
			}
			if caso.lt && len(verificacion.Material.Firmas[0].CRLsEmbebidasDER) != 1 {
				t.Fatal("la CRL embebida debe conservarse en el material")
			}
		})
	}
}
