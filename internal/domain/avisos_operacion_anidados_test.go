// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain_test

import (
	"context"
	"slices"
	"testing"

	"grxfirma/internal/domain"
)

// Tras unir 16c y 16d: el IPC abre un registro por petición y el caso de uso
// de firma abre otro dentro. Lo anotado dentro llega a los dos; lo de una
// operación hermana no se mezcla.
func TestAvisosOperacionAnidadosLleganAlPadre(t *testing.T) {
	ctxPeticion, peticion := domain.ConAvisosOperacion(context.Background())
	ctxFirma, firma := domain.ConAvisosOperacion(ctxPeticion)
	_, hermana := domain.ConAvisosOperacion(context.Background())

	domain.AnotarAviso(ctxFirma, domain.AvisoCofirmaSinOriginal)
	domain.AnotarAviso(ctxFirma, domain.AvisoCofirmaSinOriginal)
	domain.AnotarAviso(ctxPeticion, domain.AvisoCRLNoAplicable)

	if got := firma.Claves(); !slices.Equal(got, []string{domain.AvisoCofirmaSinOriginal}) {
		t.Fatalf("firma = %q", got)
	}
	if got := peticion.Claves(); !slices.Equal(got, []string{domain.AvisoCofirmaSinOriginal, domain.AvisoCRLNoAplicable}) {
		t.Fatalf("petición = %q", got)
	}
	if got := hermana.Claves(); got != nil {
		t.Fatalf("una operación hermana no debe recibir avisos: %q", got)
	}
}
