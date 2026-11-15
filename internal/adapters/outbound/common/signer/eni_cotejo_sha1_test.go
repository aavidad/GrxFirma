// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/digitorus/pkcs7"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

// cadesSHA1Prueba firma datos con SHA-1, como AutoFirma hasta 2014. Con
// separada=true el CMS no lleva el contenido (CAdES explícita).
func cadesSHA1Prueba(t *testing.T, datos []byte, separada bool) []byte {
	t.Helper()
	priv, cert := generarCertRSAPrueba(t)
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

// Lote 16c (KD-05 y SHA-1): una CAdES con SHA-1 cuyo contenido corresponde
// coteja con el centinela propio; con otro contenido sigue sin corresponder.
func TestCotejarCAdESConSHA1CorrespondeConAviso(t *testing.T) {
	machinepolicy.UnsetForTest(t, machinepolicy.PermitirSHA1Legacy)
	excepciones.SetForTest(t, "sha1_conservar", true)
	original := []byte("acta firmada en 2012 con AutoFirma")
	otro := []byte("acta firmada en 2012 con AutoFirma, cambiada")
	for nombre, separada := range map[string]bool{"implícita": false, "explícita": true} {
		t.Run(nombre, func(t *testing.T) {
			cms := cadesSHA1Prueba(t, original, separada)
			err := commonsigner.CotejarCAdESExplicita(cms, original)
			if !errors.Is(err, commonsigner.ErrFirmaCorrespondeConSHA1) {
				t.Fatalf("una firma SHA-1 que corresponde debe dar el centinela SHA-1: %v", err)
			}
			if aviso, err := commonsigner.AdmitirCotejoSHA1(err, "prueba"); err != nil || !aviso {
				t.Fatalf("sin política que lo prohíba se admite con aviso: aviso=%v err=%v", aviso, err)
			}
			for caso, datos := range map[string][]byte{
				"otro documento":   otro,
				"un byte cambiado": append(bytes.Clone(original[:len(original)-1]), 'X'),
				"vacío":            nil,
			} {
				err := commonsigner.CotejarCAdESExplicita(cms, datos)
				if !errors.Is(err, commonsigner.ErrFirmaNoCorrespondeOriginal) {
					t.Errorf("%s: un contenido distinto nunca corresponde: %v", caso, err)
				}
				if aviso, err := commonsigner.AdmitirCotejoSHA1(err, "prueba"); err == nil || aviso {
					t.Errorf("%s: AdmitirCotejoSHA1 no puede dar por buena una firma que no corresponde", caso)
				}
			}
		})
	}
	// El eContent de una implícita cambiado después de firmar no corresponde
	// a su propio messageDigest.
	cms := cadesSHA1Prueba(t, original, false)
	i := bytes.Index(cms, []byte("2012"))
	if i < 0 {
		t.Fatal("no se encontró el contenido encapsulado")
	}
	cms[i] = '3'
	contenido, implicita, err := commonsigner.ContenidoCAdESImplicito(cms)
	if err != nil || !implicita {
		t.Fatalf("ContenidoCAdESImplicito: implicita=%v err=%v", implicita, err)
	}
	if err := commonsigner.CotejarCAdESExplicita(cms, contenido); !errors.Is(err, commonsigner.ErrFirmaNoCorrespondeOriginal) {
		t.Fatalf("una CAdES SHA-1 alterada no corresponde: %v", err)
	}
}

func TestCotejarXAdESConSHA1CorrespondeConAviso(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_sha1_leer")
	excepciones.SetForTest(t, "sha1_leer", true)
	machinepolicy.UnsetForTest(t, "permitir_xml_c14n_antigua")
	excepciones.SetForTest(t, "xml_c14n_antigua", true)
	machinepolicy.SetForTest(t, machinepolicy.PermitirSHA1Legacy, true) // solo para generar la firma de prueba
	priv, cert := generarCertRSAPrueba(t)
	original := []byte(`<?xml version="1.0"?><datos><importe>10</importe></datos>`)
	doc, _ := domain.NewDocument("datos.xml", original, "application/xml")
	res, err := commonsigner.NewXAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatXAdES, Action: domain.ActionSign,
		Options: map[string]string{"algorithm": "SHA1withRSA"},
	}, &commonsigner.LocalSigningKey{ID: "k", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatal(err)
	}
	machinepolicy.UnsetForTest(t, machinepolicy.PermitirSHA1Legacy)
	if err := commonsigner.CotejarXAdESSeparada(res.Data, original); !errors.Is(err, commonsigner.ErrFirmaCorrespondeConSHA1) {
		t.Fatalf("una XAdES SHA-1 que corresponde debe dar el centinela SHA-1: %v", err)
	}
	if err := commonsigner.CotejarXAdESSeparada(res.Data, []byte(`<datos><importe>99</importe></datos>`)); !errors.Is(err, commonsigner.ErrFirmaNoCorrespondeOriginal) {
		t.Fatalf("un original ajeno no corresponde, con SHA-1 o sin él: %v", err)
	}
}

// La SHA-256 no cambia: coteja sin aviso.
func TestCotejarCAdESSHA256SinAviso(t *testing.T) {
	priv, cert := generarCertRSAPrueba(t)
	original := []byte("acta de 2026")
	doc, _ := domain.NewDocument("acta.txt", original, "text/plain")
	res, err := commonsigner.NewCAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign, Options: map[string]string{},
	}, &commonsigner.LocalSigningKey{ID: "k", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatal(err)
	}
	err = commonsigner.CotejarCAdESExplicita(res.Data, original)
	if err != nil {
		t.Fatalf("una SHA-256 que corresponde coteja sin aviso: %v", err)
	}
	if aviso, err := commonsigner.AdmitirCotejoSHA1(err, "prueba"); err != nil || aviso {
		t.Fatalf("sin SHA-1 no hay aviso: aviso=%v err=%v", aviso, err)
	}
}

// La política de máquina con permitir_sha1_legacy deshabilitada cierra el
// cotejo SHA-1 aunque el contenido corresponda.
func TestAdmitirCotejoSHA1LoCierraLaPolitica(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirSHA1Legacy, false)
	original := []byte("acta firmada en 2012 con AutoFirma")
	err := commonsigner.CotejarCAdESExplicita(cadesSHA1Prueba(t, original, false), original)
	aviso, err := commonsigner.AdmitirCotejoSHA1(err, "prueba")
	if !errors.Is(err, commonsigner.ErrCotejoSHA1Prohibido) || aviso {
		t.Fatalf("con SHA-1 prohibido por política debe rechazarse: aviso=%v err=%v", aviso, err)
	}
	machinepolicy.SetForTest(t, machinepolicy.PermitirSHA1Legacy, true)
	if aviso, err := commonsigner.AdmitirCotejoSHA1(commonsigner.ErrFirmaCorrespondeConSHA1, "prueba"); err != nil || !aviso {
		t.Fatalf("con SHA-1 permitido sigue el aviso: aviso=%v err=%v", aviso, err)
	}
}
