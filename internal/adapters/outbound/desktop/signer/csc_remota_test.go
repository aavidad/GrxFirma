// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"context"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/csc"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/testsupport/csctest"
	"grxfirma/internal/testsupport/pdffixture"
)

// TestMotorFirmaConClaveCSCRemota recorre el camino que usa la CLI: la
// credencial remota se envuelve en una ClaveLocal y el motor firma PAdES,
// CAdES y XAdES sin saber que la clave está en un servicio CSC. La firma la
// verifica el propio verificador del motor con la CA simulada como ancla.
func TestMotorFirmaConClaveCSCRemota(t *testing.T) {
	for _, credencial := range []string{csctest.CredencialRSA, csctest.CredencialEC} {
		t.Run(credencial, func(t *testing.T) {
			s := csctest.Nuevo(t)
			s.Configurar(func(s *csctest.Servidor) { s.SCAL = "2" })
			ctx := context.Background()
			cliente, err := csc.Nuevo(csc.Opciones{
				URLServicio:        s.URL,
				ClientID:           csctest.ClientID,
				HTTP:               s.Client(),
				AbrirNavegador:     s.Navegador(),
				EsperaAutorizacion: 10 * time.Second,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = cliente.Close() }()
			if err := cliente.Autorizar(ctx); err != nil {
				t.Fatalf("Autorizar: %v", err)
			}
			cred, err := cliente.Credencial(ctx, credencial)
			if err != nil {
				t.Fatalf("Credencial: %v", err)
			}
			firmante, err := cliente.Firmante(ctx, cred)
			if err != nil {
				t.Fatal(err)
			}
			clave := signer.NuevaClaveLocalConCadena(firmante, cred.Certificado, cred.Cadena)
			motor := signer.NuevoMotorFirmaGo(nil)
			anclas := domain.CertificateChain{DERCertificates: [][]byte{s.CA.Raw}}

			pdf := pdffixture.Minimal()
			texto := []byte("contenido firmado con una credencial remota")
			xml := []byte(`<?xml version="1.0" encoding="UTF-8"?><documento><dato>remoto</dato></documento>`)
			casos := []struct {
				formato domain.SignatureFormat
				nombre  string
				datos   []byte
				tipo    string
			}{
				{domain.FormatPAdES, "doc.pdf", pdf, "application/pdf"},
				{domain.FormatCAdES, "doc.txt", texto, "text/plain"},
			}
			// El motor XAdES-BES solo admite claves RSA (limitación del motor,
			// no del cliente CSC).
			if credencial == csctest.CredencialRSA {
				casos = append(casos, struct {
					formato domain.SignatureFormat
					nombre  string
					datos   []byte
					tipo    string
				}{domain.FormatXAdES, "doc.xml", xml, "application/xml"})
			}
			for _, caso := range casos {
				doc, err := domain.NewDocument(caso.nombre, caso.datos, caso.tipo)
				if err != nil {
					t.Fatal(err)
				}
				job := domain.SignatureJob{Document: doc, Format: caso.formato, Action: domain.ActionSign}
				res, err := motor.Sign(ctx, job, clave)
				if err != nil {
					t.Fatalf("%s: Sign: %v", caso.formato, err)
				}
				firmado, err := domain.NewDocument("firmado", res.Data, "")
				if err != nil {
					t.Fatal(err)
				}
				verificador := commonsigner.NewMultiVerifier()
				vr, firmantes, err := verificador.Verify(ctx, firmado, anclas)
				if err != nil || vr.Integrity.Status != domain.VerificationStatusValid {
					// Las firmas separadas se comprueban contra el original.
					vr, firmantes, err = verificador.VerifyDetached(ctx, firmado, doc, anclas)
				}
				if err != nil || vr.Integrity.Status != domain.VerificationStatusValid || len(firmantes) != 1 {
					t.Fatalf("%s: verificación: err=%v integridad=%+v firmantes=%d", caso.formato, err, vr.Integrity, len(firmantes))
				}
				if firmantes[0].Fingerprint != cred.Referencia().Fingerprint {
					t.Fatalf("%s: el firmante verificado no es el certificado remoto: %+v", caso.formato, firmantes[0])
				}
			}

			// El servicio solo recibió resúmenes del tamaño de SHA-256/384/512,
			// nunca el documento.
			s.Leer(func(s *csctest.Servidor) {
				if len(s.ResumenesFirmados) != len(casos) {
					t.Fatalf("firmas remotas = %d, se esperaban %d", len(s.ResumenesFirmados), len(casos))
				}
				for _, r := range s.ResumenesFirmados {
					if len(r) != 32 && len(r) != 48 && len(r) != 64 {
						t.Fatalf("llegó al servicio algo que no es un resumen (%d bytes)", len(r))
					}
				}
			})
		})
	}
}
