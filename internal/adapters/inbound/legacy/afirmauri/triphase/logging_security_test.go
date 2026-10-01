// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package triphase

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/logging"
)

func TestTraceLegacyBatchSoloPersisteVocabularioCerrado(t *testing.T) {
	t.Setenv("GRXFIRMA_ENV", "production")
	t.Setenv("GRXFIRMA_DEBUG", "1")
	logPath := filepath.Join(t.TempDir(), "legacy-batch.log")
	t.Setenv("GRXFIRMA_LOG_FILE", logPath)

	var console bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(logging.New("legacy/batch", &console))
	defer slog.SetDefault(previous)

	traceLegacyBatch(
		"upload legacy error endpoint=%s document=%s err=%v",
		"https://portal.example/private/upload?token=TOKEN-SECRETO",
		"nomina-secreta.pdf",
		errors.New("open /home/usuario-prueba/Documentos/nomina-secreta.pdf: TOKEN-SECRETO"),
	)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(logPath): %v", err)
	}
	got := string(data)
	for _, secret := range []string{
		"TOKEN-SECRETO",
		"portal.example",
		"/private/upload",
		"nomina-secreta.pdf",
		"/home/usuario-prueba",
	} {
		if strings.Contains(got, secret) {
			t.Fatalf("el trace batch contiene %q: %s", secret, got)
		}
	}
	if !logging.DebugAllowed() {
		if got != "" {
			t.Fatalf("el build production no debe persistir trazas DEBUG: %s", got)
		}
		return
	}
	for _, field := range []string{
		`"msg":"legacy_batch_event"`,
		`"phase":"upload"`,
		`"outcome":"failed"`,
	} {
		if !strings.Contains(got, field) {
			t.Fatalf("el trace batch no conserva %q: %s", field, got)
		}
	}
}
