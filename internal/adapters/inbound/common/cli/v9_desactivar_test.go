// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func activarTSAParaDesactivarV9(t *testing.T, dir string) {
	t.Helper()
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	if _, err := excepciones.ActivateLocal(excepciones.ForLocalCLI(context.Background()), dir, excepciones.ActivationRequest{ID: "tsa_http", Accepted: true, AcceptanceVersion: excepciones.AcceptanceVersion}); err != nil {
		t.Fatal(err)
	}
}
func TestV9DesactivarCLIYAliasSinAceptacionConAuditoria(t *testing.T) {
	for _, flag := range []string{"-desactivar-excepcion", "-disable-unsafe-exception"} {
		t.Run(flag, func(t *testing.T) {
			dir := t.TempDir()
			activarTSAParaDesactivarV9(t, dir)
			a, out, stderr := cliExcepcionPrueba(dir, "", false)
			a.esInteractivo = func() bool { t.Fatal("desactivar no debe pedir aceptación"); return false }
			if rc := a.Run(context.Background(), []string{flag, "tsa_http"}); rc != 0 || excepcionActivaEn(dir, "tsa_http") {
				t.Fatalf("desactivar: rc=%d activa=%v stderr=%q", rc, excepcionActivaEn(dir, "tsa_http"), stderr.String())
			}
			if !strings.Contains(out.String(), "tsa_http") || strings.Contains(out.String(), excepciones.AcceptanceVersion) {
				t.Fatal(out.String())
			}
			data, err := os.ReadFile(filepath.Join(dir, "unsafe-exceptions-audit.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
				var event struct {
					Type, ID  string
					Enabled   *bool
					Timestamp time.Time
				}
				if err := json.Unmarshal(line, &event); err != nil {
					t.Fatal(err)
				}
				if event.Type == "unsafe_exception.preference" && event.ID == "tsa_http" && event.Enabled != nil && !*event.Enabled && !event.Timestamp.IsZero() {
					found = true
				}
			}
			if !found {
				t.Fatal("falta auditoría de desactivación", string(data))
			}
		})
	}
}
func TestV9DesactivarCLIPoliticaFijaNoMienteNiSugiereActivar(t *testing.T) {
	for _, value := range []bool{false, true} {
		t.Run(map[bool]string{false: "prohibida", true: "fijada_activa"}[value], func(t *testing.T) {
			dir := t.TempDir()
			activarTSAParaDesactivarV9(t, dir)
			machinepolicy.SetForTest(t, "permitir_tsa_http", value)
			a, out, stderr := cliExcepcionPrueba(dir, "", false)
			if rc := a.Run(context.Background(), []string{"-desactivar-excepcion", "tsa_http"}); rc != 1 || out.Len() != 0 {
				t.Fatalf("valor fijo: rc=%d out=%q stderr=%q", rc, out.String(), stderr.String())
			}
			if strings.Contains(stderr.String(), "-activar-excepcion") || strings.Contains(stderr.String(), "-acepto-el-aviso") {
				t.Fatal(stderr.String())
			}
		})
	}
}
func TestV9DesactivarCLIConPoliticaIlegibleGuardaRenunciaLocal(t *testing.T) {
	dir := t.TempDir()
	activarTSAParaDesactivarV9(t, dir)
	machinepolicy.ErrorForTest(t, "permitir_tsa_http", errors.New("ilegible"))
	a, _, stderr := cliExcepcionPrueba(dir, "", false)
	if rc := a.Run(context.Background(), []string{"-desactivar-excepcion", "tsa_http"}); rc != 0 {
		t.Fatalf("no permite renunciar localmente: rc=%d %q", rc, stderr.String())
	}
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	if excepcionActivaEn(dir, "tsa_http") {
		t.Fatal("reanuda la excepción al poder leer de nuevo la directiva")
	}
}
