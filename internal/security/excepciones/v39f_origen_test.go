// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package excepciones

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/security/machinepolicy"
)

// Lote 39f: el recibo guarda dónde aceptó la persona, con un valor cerrado
// por canal. Antes todos decían «aceptado desde el aviso», también al activar
// desde Configuración o desde la línea de órdenes.
func TestV39FReciboGuardaElOrigenReal(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	cases := []struct {
		name   string
		issuer func(context.Context) context.Context
		origin string
		want   string
	}{
		{"escritorio desde el aviso", ForLocalDesktop, OriginWarning, "aviso"},
		{"escritorio desde ajustes", ForLocalDesktop, OriginSettings, "ajustes"},
		{"android desde ajustes", ForLocalAndroid, OriginSettings, "ajustes"},
		{"android desde el aviso", ForLocalAndroid, OriginWarning, "aviso"},
		{"línea de órdenes sin declarar", ForLocalCLI, "", "linea_ordenes"},
		{"línea de órdenes declarada", ForLocalCLI, OriginCLI, "linea_ordenes"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			receipt, err := ActivateLocal(c.issuer(context.Background()), dir, ActivationRequest{
				ID: "tsa_http", Accepted: true, AcceptanceVersion: AcceptanceVersion, Origin: c.origin,
			})
			if err != nil {
				t.Fatal(err)
			}
			if receipt.Origin != c.want {
				t.Fatalf("origen %q, se esperaba %q", receipt.Origin, c.want)
			}
			stored := readAcceptance(dir, "tsa_http")
			if stored == nil || stored.Origin != c.want {
				t.Fatalf("el recibo guardado no conserva el origen: %+v", stored)
			}
			audit, err := os.ReadFile(filepath.Join(dir, "unsafe-exceptions-audit.jsonl"))
			if err != nil || !strings.Contains(string(audit), `"origin":"`+c.want+`"`) {
				t.Fatalf("la auditoría no lleva el origen: %s %v", audit, err)
			}
			if Check("tsa_http", WithConfigDir(context.Background(), dir)) != nil {
				t.Fatal("la aceptación con origen no activa")
			}
		})
	}
}

func TestV39FOrigenDesconocidoOAjenoSeRechazaSinEscribir(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	cases := []struct {
		name   string
		issuer func(context.Context) context.Context
		origin string
	}{
		{"escritorio sin origen", ForLocalDesktop, ""},
		{"android sin origen", ForLocalAndroid, ""},
		{"escritorio dice línea de órdenes", ForLocalDesktop, OriginCLI},
		{"línea de órdenes dice aviso", ForLocalCLI, OriginWarning},
		{"línea de órdenes dice ajustes", ForLocalCLI, OriginSettings},
		{"texto antiguo", ForLocalDesktop, legacyAcceptanceOrigin},
		{"nombre de canal", ForLocalAndroid, "android"},
		{"mayúsculas", ForLocalDesktop, "Aviso"},
		{"desconocido", ForLocalDesktop, "portal"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			_, err := ActivateLocal(c.issuer(context.Background()), dir, ActivationRequest{
				ID: "tsa_http", Accepted: true, AcceptanceVersion: AcceptanceVersion, Origin: c.origin,
			})
			if !errors.Is(err, ErrAcceptanceOrigin) {
				t.Fatalf("se esperaba el rechazo del origen, llegó %v", err)
			}
			for _, name := range []string{"settings.json", "unsafe-exceptions-audit.jsonl"} {
				if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
					t.Fatalf("%s escrito con un origen rechazado: %v", name, err)
				}
			}
		})
	}
}

// Los recibos viejos se siguen leyendo; uno con un origen inventado o que no
// cuadra con su canal no activa nada.
func TestV39FLecturaDeRecibosPorOrigen(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	cases := []struct {
		origin, channel string
		readable        bool
	}{
		{legacyAcceptanceOrigin, "cli", true},
		{legacyAcceptanceOrigin, "desktop", true},
		{legacyAcceptanceOrigin, "android", true},
		{legacyAcceptanceOrigin, "protocol_dialog", true},
		{OriginWarning, "desktop", true},
		{OriginSettings, "android", true},
		{OriginCLI, "cli", true},
		{OriginWarning, "cli", false},
		{OriginSettings, "protocol_dialog", false},
		{OriginCLI, "desktop", false},
		{"portal", "desktop", false},
		{"", "cli", false},
		{"Aviso", "desktop", false},
	}
	for _, c := range cases {
		t.Run(c.origin+"/"+c.channel, func(t *testing.T) {
			dir := t.TempDir()
			r := Acceptance{ID: "tsa_http", AcceptedAt: time.Now().UTC().Add(-time.Minute), TextVersion: AcceptanceVersion, Origin: c.origin, Channel: c.channel}
			v3Write(t, dir, "settings.json", &r)
			enabled := Check("tsa_http", WithConfigDir(context.Background(), dir)) == nil
			if enabled != c.readable || (readAcceptance(dir, "tsa_http") != nil) != c.readable {
				t.Fatalf("legible=%v activa=%v, se esperaba %v", readAcceptance(dir, "tsa_http") != nil, enabled, c.readable)
			}
		})
	}
	raw, err := json.Marshal(Acceptance{Origin: OriginWarning})
	if err != nil || !strings.Contains(string(raw), `"origin":"aviso"`) {
		t.Fatalf("nombre del campo: %s", raw)
	}
}
