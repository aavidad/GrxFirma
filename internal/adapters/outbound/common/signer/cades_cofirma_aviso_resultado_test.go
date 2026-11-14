// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"testing"

	"grxfirma/internal/domain"
)

// Lote 16c: el aviso de cofirma sin documento va también al registro de la
// operación (contexto), de donde lo toman el resultado de la firma y la CLI.
func TestCAdES_CofirmaExplicitaAnotaElAvisoEnLaOperacion(t *testing.T) {
	explicita, _ := firmarCAdESParaPrueba(t, []byte("documento que no se enseña"), domain.ActionSign, "Firmante A", nil)
	implicita, _ := firmarCAdESParaPrueba(t, []byte("documento dentro"), domain.ActionSign, "Firmante A",
		map[string]string{"mode": "implicit"})
	for _, caso := range []struct {
		nombre string
		firma  []byte
		accion domain.SignatureAction
		aviso  bool
	}{
		{"cofirma_explicita", explicita, domain.ActionCoSign, true},
		{"contrafirma_explicita", explicita, domain.ActionCounterSign, true},
		{"cofirma_implicita", implicita, domain.ActionCoSign, false},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			priv, cert := certForTest(t, "Cofirmante B")
			doc, err := domain.NewDocument("firma.csig", caso.firma, "application/pkcs7-signature")
			if err != nil {
				t.Fatal(err)
			}
			ctx, avisos := domain.ConAvisosOperacion(context.Background())
			if _, err := NewCAdESBESDetached().Sign(ctx, domain.SignatureJob{
				Document: doc, Format: domain.FormatCAdES, Action: caso.accion, Options: map[string]string{},
			}, &LocalSigningKey{ID: "b", Signer: priv, Certificate: cert}); err != nil {
				t.Fatal(err)
			}
			claves := avisos.Claves()
			hay := len(claves) == 1 && claves[0] == domain.AvisoCofirmaSinOriginal
			if hay != caso.aviso || (!caso.aviso && len(claves) != 0) {
				t.Fatalf("avisos de la operación = %q, want aviso=%v", claves, caso.aviso)
			}
		})
	}
}
