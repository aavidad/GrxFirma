// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadProtocolDebugLogTailIsBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "debug.log")
	content := strings.Repeat("discarded-data\n", 64) + "last-event\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := readProtocolDebugLogTail(path, 32)
	if err != nil {
		t.Fatalf("readProtocolDebugLogTail: %v", err)
	}
	if len(got) > 32 {
		t.Fatalf("tail length = %d, want <= 32", len(got))
	}
	if !strings.Contains(got, "last-event") {
		t.Fatalf("tail does not contain final event: %q", got)
	}
	if strings.Contains(got, "discarded-data\ndiscarded-data") {
		t.Fatalf("tail contains data outside the requested window: %q", got)
	}
}

func TestReadProtocolDebugLogTailRejectsFinalSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.log")
	link := filepath.Join(dir, "debug.log")
	if err := os.WriteFile(target, []byte("secret\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := readProtocolDebugLogTail(link, 64); err == nil {
		t.Fatal("readProtocolDebugLogTail accepted a final-component symlink")
	}
}

func TestProtocolIncidentFileComponentRejectsSeparators(t *testing.T) {
	if got := protocolIncidentFileComponent("../../web/socket"); got != "websocket" {
		t.Fatalf("protocolIncidentFileComponent = %q", got)
	}
}

func TestProtocolRawURISummaryNoPersisteParametrosNiPayload(t *testing.T) {
	raw := "afirma://sign?op=sign&dat=PAYLOAD-SECRETO&token=TOKEN-SECRETO&filename=nomina-secreta.pdf&origin=https%3A%2F%2Fportal.example%2Fruta%3Fsecret%3D1"
	got := protocolRawURISummary(raw)

	if got != "scheme=afirma;operation=sign;origin=external_portal" {
		t.Fatalf("protocolRawURISummary = %q", got)
	}
	for _, secret := range []string{"PAYLOAD-SECRETO", "TOKEN-SECRETO", "nomina-secreta.pdf", "/ruta", "secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("el resumen contiene %q: %q", secret, got)
		}
	}
}

func TestRedactProtocolLogTextEliminaSolicitudRutasYDocumento(t *testing.T) {
	t.Setenv("HOME", "/home/usuario-prueba")
	raw := strings.Join([]string{
		`event session_id=SESSION-SECRETA message_prefix="afirma://sign?dat=PAYLOAD-SECRETO&token=TOKEN-SECRETO&filename=nomina-secreta.pdf"`,
		`error="open /home/usuario-prueba/Documentos/nomina-secreta.pdf and /mnt/confidential/expediente.xml"`,
		`endpoint=https://portal.example/private/upload?token=TOKEN-SECRETO`,
		`json={"request_id":"REQUEST-SECRETO","token":"TOKEN-JSON-SECRETO"}`,
		"-----BEGIN PRIVATE KEY-----\n" + strings.Repeat("A", 96) + "\n-----END PRIVATE KEY-----",
	}, "\n")

	got := redactProtocolLogText(raw)
	for _, secret := range []string{
		"PAYLOAD-SECRETO",
		"TOKEN-SECRETO",
		"nomina-secreta.pdf",
		"expediente.xml",
		"/home/usuario-prueba",
		"/mnt/confidential",
		"/private/upload",
		"TOKEN-JSON-SECRETO",
		"SESSION-SECRETA",
		"REQUEST-SECRETO",
		strings.Repeat("A", 32),
	} {
		if strings.Contains(got, secret) {
			t.Fatalf("el log saneado contiene %q: %q", secret, got)
		}
	}
	if !strings.Contains(got, "[REDACTED_PREFIX]") {
		t.Fatalf("no se conserva el marcador seguro del evento: %q", got)
	}
}

func TestProtocolDiagnosticPersistFailureNoEscribeSecretos(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GRXFIRMA_LOG_FILE", "")
	t.Setenv("GRXFIRMA_DEBUG_LOG_FILE", "")

	rawURI := "afirma://sign?dat=PAYLOAD-SECRETO&token=TOKEN-SECRETO&filename=nomina-secreta.pdf"
	protocolDiagnosticReset(rawURI)
	protocolDiagnosticSetOrigin("https://portal.example/private/route?token=TOKEN-SECRETO")
	protocolDiagnosticSetOperation("sign")
	protocolDiagnosticMarkPhase(
		"handle_request",
		"Falló nomina-secreta.pdf para alice@example.test",
		"open "+filepath.Join(home, "Documentos", "nomina-secreta.pdf")+" token=TOKEN-DETALLE",
	)
	protocolDiagnosticPersistFailure(
		"handle_request",
		errors.New("SAF_25: open "+filepath.Join(home, "Documentos", "nomina-secreta.pdf")+": token=TOKEN-SECRETO"),
	)

	entries, err := os.ReadDir(protocolIncidentDir())
	if err != nil {
		t.Fatalf("ReadDir(protocolIncidentDir): %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no se ha persistido la incidencia")
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(protocolIncidentDir(), entry.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", entry.Name(), err)
		}
		got := string(data)
		for _, secret := range []string{
			"PAYLOAD-SECRETO",
			"TOKEN-SECRETO",
			"nomina-secreta.pdf",
			home,
			"/private/route",
			"alice@example.test",
			"TOKEN-DETALLE",
			"Error original:",
		} {
			if strings.Contains(got, secret) {
				t.Fatalf("%s contiene %q: %s", entry.Name(), secret, got)
			}
		}
		if !strings.Contains(got, "SAF_25") {
			t.Fatalf("%s no conserva el código estable: %s", entry.Name(), got)
		}
	}
}

func TestProtocolDiagnosticEtiquetasYURIQuedanAcotadas(t *testing.T) {
	if got := protocolRawURISummary("afirma://sign?" + strings.Repeat("x", 20*1024)); got != "scheme=unknown;operation=unknown" {
		t.Fatalf("resumen de URI sobredimensionada = %q", got)
	}
	if got := sanitizeProtocolEventLabel("../../SECRET phase"); got != "secretphase" {
		t.Fatalf("etiqueta saneada = %q", got)
	}
	if got := sanitizeProtocolOperation("cosign"); got != "cosign" {
		t.Fatalf("operación cofirma = %q", got)
	}
}

func TestPruneProtocolIncidentFilesSoloRetiraArtefactosPropios(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "incidents")
	now := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)
	groups := []struct {
		base     string
		modified time.Time
	}{
		{"protocol-incident-20260501-120000-websocket-10001", now.AddDate(0, 0, -60)},
		{"protocol-incident-20260728-120000-websocket-10002", now.Add(-2 * time.Hour)},
		{"protocol-incident-20260729-110000-websocket-10003", now.Add(-time.Hour)},
	}
	if err := ensurePrivateProtocolIncidentDir(dir); err != nil {
		t.Fatalf("ensurePrivateProtocolIncidentDir: %v", err)
	}
	for _, group := range groups {
		for _, suffix := range []string{".json", ".txt", ".logtail.txt"} {
			path := filepath.Join(dir, group.base+suffix)
			if err := os.WriteFile(path, []byte("safe"), 0o600); err != nil {
				t.Fatalf("WriteFile(%s): %v", path, err)
			}
			if err := os.Chtimes(path, group.modified, group.modified); err != nil {
				t.Fatalf("Chtimes(%s): %v", path, err)
			}
		}
	}
	foreign := filepath.Join(dir, "notas-del-usuario.txt")
	if err := os.WriteFile(foreign, []byte("no borrar"), 0o600); err != nil {
		t.Fatalf("WriteFile(foreign): %v", err)
	}
	external := filepath.Join(t.TempDir(), "external.json")
	if err := os.WriteFile(external, []byte("external"), 0o600); err != nil {
		t.Fatalf("WriteFile(external): %v", err)
	}
	link := filepath.Join(dir, "protocol-incident-20250101-000000-websocket-99999.json")
	linkCreated := os.Symlink(external, link) == nil

	removed, err := pruneProtocolIncidentFiles(dir, now, 30, 1)
	if err != nil {
		t.Fatalf("pruneProtocolIncidentFiles: %v", err)
	}
	if removed != 6 {
		t.Fatalf("removed = %d, want 6", removed)
	}
	for _, index := range []int{0, 1} {
		if _, err := os.Stat(filepath.Join(dir, groups[index].base+".json")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("el grupo %d no fue retirado: %v", index, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, groups[2].base+".json")); err != nil {
		t.Fatalf("el grupo reciente no debe retirarse: %v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("el fichero ajeno no debe retirarse: %v", err)
	}
	if data, err := os.ReadFile(external); err != nil || string(data) != "external" {
		t.Fatalf("el destino externo fue alterado: data=%q err=%v", data, err)
	}
	if linkCreated {
		if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("el enlace reconocido no debe seguirse ni retirarse: %v", err)
		}
	}
}

func TestPruneProtocolIncidentFilesRechazaPoliticaInvalida(t *testing.T) {
	if _, err := pruneProtocolIncidentFiles(t.TempDir(), time.Time{}, 30, 50); err == nil {
		t.Fatal("se aceptó una fecha de retención vacía")
	}
	if _, err := pruneProtocolIncidentFiles(t.TempDir(), time.Now(), 0, 50); err == nil {
		t.Fatal("se aceptó una antigüedad nula")
	}
	if _, err := pruneProtocolIncidentFiles(t.TempDir(), time.Now(), 30, 0); err == nil {
		t.Fatal("se aceptó un máximo de grupos nulo")
	}
}
