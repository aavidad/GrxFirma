// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"os"
	"testing"
)

// Las incidencias y el registro de depuración pueden llevar datos personales.
// Sin carpeta personal válida no se escriben ni se leen en la carpeta
// temporal compartida.
func TestSinCarpetaPersonalLasIncidenciasNoVanALaTemporal(t *testing.T) {
	temporal := t.TempDir()
	for _, variable := range []string{"HOME", "USERPROFILE", "LOCALAPPDATA", "GRXFIRMA_LOG_FILE", "GRXFIRMA_DEBUG_LOG_FILE"} {
		t.Setenv(variable, "")
	}
	t.Setenv("TMPDIR", temporal)
	t.Setenv("TMP", temporal)
	t.Setenv("TEMP", temporal)

	if dir, err := protocolIncidentDir(); err == nil {
		t.Fatalf("protocolIncidentDir sin perfil = %q", dir)
	}
	if ruta, err := writeProtocolIncidentFiles(protocolIncident{}); err == nil {
		t.Fatalf("se escribió la incidencia en %q", ruta)
	}
	if ruta := resolveProtocolDebugLogPath(); ruta != "" {
		t.Fatalf("registro de depuración sin perfil = %q", ruta)
	}
	if cola := buildProtocolIncidentLogTail(); cola != "" {
		t.Fatalf("se leyó un registro sin perfil: %q", cola)
	}
	if entradas, _ := os.ReadDir(temporal); len(entradas) != 0 {
		t.Fatalf("se escribió en la carpeta temporal: %v", entradas)
	}
}
