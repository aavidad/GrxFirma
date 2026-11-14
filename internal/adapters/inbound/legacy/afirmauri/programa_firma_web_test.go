// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmauri

import (
	"context"
	"testing"

	"grxfirma/internal/domain"
)

// Lote 15: una web no puede pedir por afirma:// (ni por el WebSocket, que usa
// el mismo intérprete) que cambie el programa que abre sus firmas. Esas
// operaciones no existen en el protocolo; solo las cambian la aplicación de
// escritorio, la CLI local, el instalador y la política de máquina.
func TestAfirmaURI_NoCambiaElProgramaParaFirmarEnLasWebs(t *testing.T) {
	adaptador := New(trustStub{estado: domain.TrustAllowed})
	for _, uri := range []string{
		"afirma://afirma_handler_select?handler=autofirma",
		"afirma://afirma_handler_status",
		"afirma://elegir-programa-firma-web?valor=autofirma",
		"afirma://?op=afirma_handler_select&handler=preguntar",
		"afirma://?op=elegir-programa-firma-web&elegir-programa-firma-web=autofirma",
		"afirma://sign?manejador_afirma=autofirma&handler=autofirma&dat=e30",
	} {
		s, err := adaptador.Parse(context.Background(), uri)
		if err == nil && s.Operacion != "sign" {
			t.Fatalf("afirma:// aceptó %q como operación %q", uri, s.Operacion)
		}
		s, err = adaptador.ParseSocket(context.Background(), uri)
		if err == nil && s.Operacion != "sign" {
			t.Fatalf("WebSocket aceptó %q como operación %q", uri, s.Operacion)
		}
	}
	for _, op := range []string{"afirma_handler_select", "afirma_handler_status", "elegir-programa-firma-web", "manejador_afirma", "programa_firma_web"} {
		if _, _, _, err := normalizarOperacion(op); err == nil {
			t.Fatalf("la operación %q no puede existir en afirma://", op)
		}
	}
}
