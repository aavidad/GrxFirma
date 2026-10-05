// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

// La fecha de cada firma llega al resumen del firmante con su origen.
func TestVerificacionCMS_FechaDeFirmaDelAtributoFirmado(t *testing.T) {
	contenido := []byte("contenido con fecha de firma")
	antes := time.Now().UTC().Add(-time.Minute)
	firma, _ := firmarCAdESParaPrueba(t, contenido, domain.ActionSign, "Firmante A", nil)
	cofirma, _ := firmarCAdESParaPrueba(t, firma, domain.ActionCoSign, "Cofirmante B", nil)
	despues := time.Now().UTC().Add(time.Minute)

	vr, firmantes, err := NewCAdESVerifier().VerifyDetachedCMS(context.Background(), cofirma, contenido)
	if err != nil {
		t.Fatal(err)
	}
	if len(vr.SigningTimes) != 2 {
		t.Fatalf("fechas = %d, want 2", len(vr.SigningTimes))
	}
	resumen := vr.WithSignerSummaries(firmantes).SignerSummaries
	if len(resumen) != 2 {
		t.Fatalf("firmantes = %d, want 2", len(resumen))
	}
	for _, f := range resumen {
		if f.SigningTimeSource != domain.SigningTimeSourceSignedAttribute {
			t.Fatalf("origen = %q", f.SigningTimeSource)
		}
		fecha, err := time.Parse(time.RFC3339, f.SigningTime)
		if err != nil || fecha.Before(antes.Truncate(time.Second)) || fecha.After(despues) {
			t.Fatalf("fecha = %q (%v)", f.SigningTime, err)
		}
	}
}

// Un sello de tiempo que no se puede comprobar no da fecha: se usa el
// atributo firmado.
func TestVerificacionCMS_SelloNoComprobableNoDaFecha(t *testing.T) {
	priv, cert := certForTest(t, "RSA")
	doc, err := domain.NewDocument("documento.txt", []byte("contenido a sellar"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	res, err := NewSignerCAdEST(NewCAdESBESDetached(), &tsaMock{token: buildMinimalTST(t)}).Sign(
		context.Background(),
		domain.SignatureJob{Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign},
		&LocalSigningKey{ID: "clave", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatal(err)
	}
	vr, _, err := NewCAdESVerifier().VerifyDetachedCMS(context.Background(), res.Data, doc.Content)
	if err != nil {
		t.Fatal(err)
	}
	if len(vr.SigningTimes) != 1 || vr.SigningTimes[0].Source != domain.SigningTimeSourceSignedAttribute {
		t.Fatalf("fechas = %+v", vr.SigningTimes)
	}
}

func TestAtributoSigningTime_SinAtributoNoInventaFecha(t *testing.T) {
	if _, ok := atributoSigningTime(nil); ok {
		t.Fatal("sin atributos no debe haber fecha")
	}
	if _, _, ok := fechaFirmaCMS(signerInfoRaw{}); ok {
		t.Fatal("un SignerInfo vacío no debe dar fecha")
	}
}
