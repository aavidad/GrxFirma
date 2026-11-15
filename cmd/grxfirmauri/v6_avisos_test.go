// SPDX-License-Identifier: EUPL-1.2
package main

import (
	"context"
	"errors"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/application"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"grxfirma/internal/testsupport/perfil"
	"slices"
	"strings"
	"testing"
)

type v6SignConAvisos struct{ result application.SignResult }

func (s v6SignConAvisos) Execute(ctx context.Context, _ application.SignCommand) (application.SignResult, error) {
	for _, id := range []string{"tsa_http", "trifasica_http"} {
		if err := excepciones.Require(ctx, id); err != nil {
			return application.SignResult{}, err
		}
	}
	return s.result, nil
}
func TestV6WSSDevuelveAvisosSinColectorDelLlamante(t *testing.T) {
	for _, id := range []string{"tsa_http", "trifasica_http"} {
		e, _ := excepciones.Find(id)
		machinepolicy.UnsetForTest(t, e.PolicyKey)
		excepciones.SetForTest(t, id, true)
	}
	perfil.Nuevo(t)
	h, solicitud := newSignAndSaveFlowTest(t, acceptLegacySaveDefaultForTest)
	base := h.signUC.(*signExecutorStub).result
	h.signUC = v6SignConAvisos{result: base}
	solicitud.Operacion = afirmauri.OperacionFirma
	result, handled, err := h.HandleLegacy(context.Background(), "", solicitud)
	if err != nil || !handled {
		t.Fatalf("WSS: %v handled=%v", err, handled)
	}
	for _, id := range []string{"tsa_http", "trifasica_http"} {
		if !slices.Contains(result.Warnings, excepciones.Warning(id)) {
			t.Fatalf("WSS pierde %s: %v", id, result.Warnings)
		}
	}
	excepciones.SetForTest(t, "tsa_http", false)
	_, _, err = h.HandleLegacy(context.Background(), "", solicitud)
	var block *excepciones.Block
	if !errors.As(err, &block) || block.ID != "tsa_http" {
		t.Fatalf("WSS no devuelve bloqueo: %v", err)
	}
	if strings.Contains(err.Error(), "-activar-excepcion") || strings.Contains(err.Error(), "-acepto-el-aviso") {
		t.Fatal("WSS envía instrucciones de terminal al portal")
	}

}
