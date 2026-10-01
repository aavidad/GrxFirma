// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package exttools da soporte a los tests que validan resultados con
// herramientas externas (qpdf, pdfsig, openssl…).
//
// Problema que resuelve: si un test hace `t.Skip` cuando la herramienta no está
// en el PATH, un CI sin esas herramientas pasa en verde aunque el código genere
// artefactos inválidos. Con Require, en un entorno donde se exige la validación
// externa (variable GRXFIRMA_REQUIRE_EXTERNAL_TOOLS=1, que CI debe fijar) la
// ausencia de la herramienta es un fallo, no un skip.
package exttools

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// EnvRequire es la variable de entorno que fuerza a exigir las herramientas
// externas. CI debe fijarla a "1"/"true" para que ningún check se salte.
const EnvRequire = "GRXFIRMA_REQUIRE_EXTERNAL_TOOLS"

// Requerido indica si el entorno exige que las herramientas externas estén
// presentes (típicamente CI).
func Requerido() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvRequire))) {
	case "1", "true", "yes", "si", "on":
		return true
	default:
		return false
	}
}

// Available indica si la herramienta está en el PATH. Si no lo está y el
// entorno la exige (Requerido() == true), falla el test; si no, deja constancia
// en el log y devuelve false para que el test omita solo ese bloque de
// validación sin saltarse el resto.
func Available(t *testing.T, nombre string) bool {
	t.Helper()
	if _, err := exec.LookPath(nombre); err == nil {
		return true
	}
	if Requerido() {
		t.Fatalf("la herramienta externa %q es obligatoria (%s=1) y no está en el PATH", nombre, EnvRequire)
	}
	t.Logf("herramienta externa %q no disponible; se omite esa validación (fija %s=1 para exigirla)", nombre, EnvRequire)
	return false
}

// Require devuelve la ruta de la herramienta si está disponible. Si no lo está:
//   - falla el test (t.Fatalf) cuando Requerido() es true;
//   - salta el test (t.Skipf) en caso contrario.
//
// Así, en local el test sigue siendo cómodo, pero en CI la ausencia de la
// herramienta —o una regresión que la herramienta detectaría— nunca pasa
// inadvertida.
func Require(t *testing.T, nombre string) string {
	t.Helper()
	ruta, err := exec.LookPath(nombre)
	if err == nil {
		return ruta
	}
	if Requerido() {
		t.Fatalf("la herramienta externa %q es obligatoria (%s=1) y no está en el PATH: %v", nombre, EnvRequire, err)
	}
	t.Skipf("herramienta externa %q no disponible; se omite la validación (fija %s=1 para exigirla)", nombre, EnvRequire)
	return ""
}
