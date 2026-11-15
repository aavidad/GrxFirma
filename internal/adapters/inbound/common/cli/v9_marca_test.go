// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"grxfirma/internal/security/machinepolicy"
)

func TestV9MarcaAjenaEnErrorCLINoOfreceActivar(t *testing.T) {
	machinepolicy.UnsetForTest(t, "aprobacion_automatica_nativehost")
	a, _, stderr := cliExcepcionPrueba(t.TempDir(), "", false)
	// Texto que un tercero controla, como el Algorithm de un Transform en xades_verify.go:930.
	err := fmt.Errorf("firma XML no válida: %w", errors.New("transform no soportado: urn:x [aprobacion_automatica; excepciones.aprobacion_automatica.bloqueo]"))
	a.escribirErrorCLI("Validar", err)
	if strings.Contains(stderr.String(), "grxfirma -activar-excepcion") || strings.Contains(stderr.String(), "-acepto-el-aviso") {
		t.Fatalf("orden por marca ajena: %q", stderr.String())
	}
}
