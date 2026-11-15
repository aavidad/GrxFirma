// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

func cliExcepcionPrueba(dir, input string, interactive bool) (*Adaptador, *strings.Builder, *strings.Builder) {
	var stdout, stderr strings.Builder
	a := New(nil, nil).WithConfigDir(dir).WithLocalizador(localizador.Para("es"))
	a.Stdout, a.Stderr, a.Stdin = &stdout, &stderr, strings.NewReader(input)
	a.esInteractivo = func() bool { return interactive }
	return a, &stdout, &stderr
}

func excepcionActivaEn(dir, id string) bool {
	for _, state := range excepciones.ListAt(dir) {
		if state.ID == id {
			return state.Enabled
		}
	}
	return false
}

func TestActivarExcepcionCLIExigeConfirmacionYVersion(t *testing.T) {
	const id = "tsa_http"
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	dir := t.TempDir()
	a, stdout, stderr := cliExcepcionPrueba(dir, "", false)
	if rc := a.Run(context.Background(), []string{"-activar-excepcion", id}); rc != 1 || excepcionActivaEn(dir, id) {
		t.Fatalf("sin confirmación: rc=%d activa=%v", rc, excepcionActivaEn(dir, id))
	}
	if !strings.Contains(stdout.String(), excepciones.AcceptanceVersion) || !strings.Contains(stdout.String(), "HTTP") {
		t.Fatalf("no mostró riesgo y versión completos: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "grxfirma -activar-excepcion tsa_http -acepto-el-aviso") {
		t.Fatalf("falta orden exacta para automatización: %q", stderr.String())
	}
	a, stdout, stderr = cliExcepcionPrueba(dir, "", false)
	if rc := a.Run(context.Background(), []string{"-enable-unsafe-exception", id, "-accept-warning"}); rc != 0 || !excepcionActivaEn(dir, id) {
		t.Fatalf("con aceptación: rc=%d activa=%v stderr=%q", rc, excepcionActivaEn(dir, id), stderr.String())
	}
	if !strings.Contains(stdout.String(), id) {
		t.Fatalf("falta confirmación de activación: %q", stdout.String())
	}
	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Acceptances map[string]excepciones.Acceptance `json:"excepciones_inseguras_aceptaciones"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if acceptance, ok := saved.Acceptances[id]; !ok || acceptance.TextVersion != excepciones.AcceptanceVersion || acceptance.Channel != "cli" {
		t.Fatalf("falta aceptación versionada del canal local: %+v", acceptance)
	}
}

func TestActivarExcepcionCLIInteractivaRespetaRespuesta(t *testing.T) {
	const id = "tsa_http"
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	dir := t.TempDir()
	a, _, _ := cliExcepcionPrueba(dir, "n\n", true)
	if rc := a.Run(context.Background(), []string{"-activar-excepcion", id}); rc != 1 || excepcionActivaEn(dir, id) {
		t.Fatalf("respuesta negativa activó excepción: rc=%d", rc)
	}
	a, _, stderr := cliExcepcionPrueba(dir, "s\n", true)
	if rc := a.Run(context.Background(), []string{"-activar-excepcion", id}); rc != 0 || !excepcionActivaEn(dir, id) {
		t.Fatalf("respuesta afirmativa no activó: rc=%d stderr=%q", rc, stderr.String())
	}
}

func TestActivarExcepcionCLIDirectivaProhibeOIlegible(t *testing.T) {
	const id = "tsa_http"
	for _, tc := range []struct {
		name string
		set  func(*testing.T)
	}{
		{"prohibida", func(t *testing.T) { machinepolicy.SetForTest(t, "permitir_tsa_http", false) }},
		{"ilegible", func(t *testing.T) { machinepolicy.ErrorForTest(t, "permitir_tsa_http", errors.New("ilegible")) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.set(t)
			dir := t.TempDir()
			a, _, stderr := cliExcepcionPrueba(dir, "", false)
			if rc := a.Run(context.Background(), []string{"-activar-excepcion", id, "-acepto-el-aviso"}); rc != 1 || excepcionActivaEn(dir, id) {
				t.Fatalf("directiva %s activó excepción: rc=%d", tc.name, rc)
			}
			if !strings.Contains(stderr.String(), id) {
				t.Fatalf("falta identificador del bloqueo: %q", stderr.String())
			}
		})
	}
}

func TestActivarExcepcionCLINoAceptaIDVacioNiOtraOperacion(t *testing.T) {
	for _, args := range [][]string{
		{"-activar-excepcion", ""},
		{"-activar-excepcion", "tsa_http", "-operacion", "firmar"},
	} {
		if _, err := parsearArgs(args); err == nil {
			t.Fatalf("se aceptó activación ambigua: %q", args)
		}
	}
}
