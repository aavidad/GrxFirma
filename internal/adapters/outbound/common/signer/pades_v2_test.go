// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"context"
	"testing"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/testsupport/pdffixture"
)

func TestInspeccionarPAdESV2_FirmasEnRevisionesActivas(t *testing.T) {
	doc, err := domain.NewDocument("original.pdf", pdffixture.Minimal(), "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	motor := desktopsigner.NuevoMotorFirmaGo(nil)
	priv1, cert1 := generarCertRSAPruebaConCN(t, "Primera")
	first, err := motor.Sign(context.Background(), domain.SignatureJob{Document: doc, Format: domain.FormatPAdES, Action: domain.ActionSign}, desktopsigner.NuevaClaveLocal(priv1, cert1))
	if err != nil {
		t.Fatal(err)
	}
	priv2, cert2 := generarCertRSAPruebaConCN(t, "Segunda")
	firstDoc, _ := domain.NewDocument("firmado.pdf", first.Data, "application/pdf")
	second, err := motor.Sign(context.Background(), domain.SignatureJob{Document: firstDoc, Format: domain.FormatPAdES, Action: domain.ActionSign}, desktopsigner.NuevaClaveLocal(priv2, cert2))
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := commonsigner.InspeccionarPAdESV2(second.Data, 20, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Firmas) != 2 || len(inspection.Revisiones) < 3 {
		t.Fatalf("inspección incompleta: firmas=%d revisiones=%d", len(inspection.Firmas), len(inspection.Revisiones))
	}
	if inspection.Firmas[0].RevisionLongitud != len(first.Data) || inspection.Firmas[1].RevisionLongitud != len(second.Data) {
		t.Fatalf("revisiones firmadas mal ordenadas: %+v", inspection.Firmas)
	}
	if inspection.Firmas[0].RevisionHuellaSHA256 == inspection.Firmas[0].ContenidoHuellaSHA256 {
		t.Fatal("las dos huellas representan bytes distintos")
	}
	for i, sig := range inspection.Firmas {
		if sig.CambiosDesdeAnterior.Estado != "permitidos" {
			t.Fatalf("firma %d: cambio=%+v", i+1, sig.CambiosDesdeAnterior)
		}
	}
	if inspection.CambiosPosteriores.Estado != "ninguno" {
		t.Fatalf("cambios posteriores=%+v", inspection.CambiosPosteriores)
	}
}
