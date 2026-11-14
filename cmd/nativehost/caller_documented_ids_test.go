// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const adrDesktopSecurityPath = "../../docs/ADR-desktop-security.md"

var adrCallerIDPattern = regexp.MustCompile("`([^`]+)`")

// documentedCallerRows devuelve las filas de la tabla «Llamantes autorizados
// del host nativo» de la ADR de escritorio: navegador, ID, titular y estado.
func documentedCallerRows(t *testing.T) [][4]string {
	t.Helper()
	raw, err := os.ReadFile(adrDesktopSecurityPath)
	if err != nil {
		t.Fatalf("no se puede leer la ADR de escritorio: %v", err)
	}
	// Con core.autocrlf (CI de Windows) el fichero llega con CRLF.
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	start := strings.Index(text, "\n## Llamantes autorizados del host nativo\n")
	if start < 0 {
		t.Fatal("docs/ADR-desktop-security.md no tiene la sección «Llamantes autorizados del host nativo»")
	}
	section := text[start+1:]
	if next := strings.Index(section[3:], "\n## "); next >= 0 {
		section = section[:next+3]
	}
	var rows [][4]string
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "|---") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) != 5 {
			t.Fatalf("fila de la tabla de llamantes con %d columnas, se esperaban 5: %q", len(cells), line)
		}
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		if cells[0] == "Navegador" {
			continue
		}
		match := adrCallerIDPattern.FindStringSubmatch(cells[1])
		if match == nil {
			t.Fatalf("la fila %q no da el ID entre comillas invertidas", line)
		}
		rows = append(rows, [4]string{cells[0], match[1], cells[3], cells[4]})
	}
	if len(rows) == 0 {
		t.Fatal("la tabla de llamantes autorizados está vacía")
	}
	return rows
}

func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// JD-02: los únicos llamantes integrados en el binario son los que figuran,
// con titular y estado, en la ADR de escritorio. Los ID anteriores
// (`extension@dipgra.es`) y los de portafirmas no se aceptan aunque un
// manifiesto del propio host los liste.
func TestCallerPolicyOnlyDocumentedIDs(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	executable := filepath.Join(dir, "bin", "grxfirma-nativehost")
	writeTestFile(t, executable, []byte("host"), 0o700)

	policy := newNativeCallerPolicy(executable, home, false)
	gotChromium := sortedKeys(policy.chromiumIDs)
	gotFirefox := sortedKeys(policy.firefoxIDs)
	if strings.Join(gotChromium, ",") != officialChromiumExtensionID {
		t.Fatalf("sin manifiestos, los ID Chromium integrados deben ser solo el oficial; hay %v", gotChromium)
	}
	if strings.Join(gotFirefox, ",") != officialFirefoxExtensionID {
		t.Fatalf("sin manifiestos, los ID Firefox integrados deben ser solo el oficial; hay %v", gotFirefox)
	}

	documented := map[string]map[string]struct{}{"Chromium": {}, "Firefox": {}}
	for _, row := range documentedCallerRows(t) {
		browser, id, owner, state := row[0], row[1], row[2], row[3]
		ids, ok := documented[browser]
		if !ok {
			t.Fatalf("navegador desconocido en la tabla de llamantes: %q", browser)
		}
		if owner == "" || state == "" {
			t.Errorf("la fila de %s no dice titular y estado", id)
		}
		ids[id] = struct{}{}
	}
	if got := strings.Join(sortedKeys(documented["Chromium"]), ","); got != strings.Join(gotChromium, ",") {
		t.Errorf("la ADR documenta los ID Chromium %q y el binario integra %q", got, strings.Join(gotChromium, ","))
	}
	if got := strings.Join(sortedKeys(documented["Firefox"]), ","); got != strings.Join(gotFirefox, ",") {
		t.Errorf("la ADR documenta los ID Firefox %q y el binario integra %q", got, strings.Join(gotFirefox, ","))
	}

	// Un manifiesto del propio host que lista ID antiguos no los autoriza en
	// producción: Firefox exige además que el ID esté en la política.
	manifestPath := filepath.Join(home, ".mozilla", "native-messaging-hosts", "io.github.aavidad.grxfirma.json")
	for _, rejected := range []string{"portafirmas@dipgra.es", "extension@dipgra.es"} {
		writeTestManifest(t, manifestPath, nativeHostManifest{
			Name:              "io.github.aavidad.grxfirma",
			Path:              executable,
			Type:              "stdio",
			AllowedExtensions: []string{rejected},
		})
		if _, err := nativeCallerFromArgs([]string{manifestPath, rejected}, policy); err == nil {
			t.Errorf("%s no debe estar autorizado", rejected)
		}
	}
}
