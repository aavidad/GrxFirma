// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"context"
	"strings"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

// Veri*Factu no pasa por la verificación de firmas: lo aceptado que se aplica
// allí (SHA-1) también queda en la auditoría.
func TestGestionConfianzaForzada_RegistrarAplicadasAudita(t *testing.T) {
	logger := &loggerMock{}
	gestion := application.NuevoGestionConfianzaForzadaUseCase(&almacenConfianzaMock{}, application.NuevoAuditUseCase(relojMock{}, logger))

	gestion.RegistrarAplicadas(context.Background(), "verifactu_validar", nil)
	if len(logger.registros) != 0 {
		t.Fatalf("sin nada aplicado no se audita: %v", logger.registros)
	}
	gestion.RegistrarAplicadas(context.Background(), "verifactu_validar", []domain.ObstaculoConfianza{{Tipo: domain.ConfianzaAlgoritmoSHA1}})
	if len(logger.registros) != 1 {
		t.Fatalf("falta el registro de auditoría: %v", logger.registros)
	}
	payload := string(logger.registros[0].Payload)
	for _, esperado := range []string{"verifactu_validar", "confianza_forzada=algoritmo_sha1"} {
		if !strings.Contains(payload, esperado) {
			t.Fatalf("la auditoría no contiene %q: %s", esperado, payload)
		}
	}
	var nilGestion *application.GestionConfianzaForzadaUseCase
	nilGestion.RegistrarAplicadas(context.Background(), "verifactu_validar", []domain.ObstaculoConfianza{{Tipo: domain.ConfianzaAlgoritmoSHA1}})
}
