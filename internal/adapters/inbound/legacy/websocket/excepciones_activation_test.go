// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package websocket

import (
	"bytes"
	"context"
	"encoding/base64"
	"grxfirma/internal/testsupport/perfil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/appdirs"
)

// Recorre el despachador de comandos sin abrir puertos ni hacer peticiones.
func TestSocketExcepcionesNoAceptaActivacionDesdePortal(t *testing.T) {
	home := perfil.Nuevo(t)
	dir := appdirs.Config(home)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"excepciones_inseguras":{"sha1_crear":false}}`)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), original, 0600); err != nil {
		t.Fatal(err)
	}
	server := &LegacySocketServer{adapter: New(afirmauri.New(nil), nil), session: "activation-test", protocolVersion: 3}
	for _, action := range []string{"unsafe_exceptions_activate", "unsafe_exceptions_set", "unsafe_exceptions_retry", "save_settings"} {
		uri := "afirma://" + action + "?id=sha1_crear&enabled=true&accepted=true&channel=local_ipc&source=gui"
		encoded := base64.StdEncoding.EncodeToString([]byte(uri))
		result := server.processCommand(context.Background(), "cmd="+encoded+"&idsession=activation-test@EOF")
		if result == "1" {
			result = server.processCommand(context.Background(), "send=@1@1&idsession=activation-test@EOF")
		}
		if !strings.Contains(result, "SAF_") {
			t.Fatalf("comando externo %s no rechazado: %q", action, result)
		}
		result = server.processCommand(context.Background(), action+"=true&channel=local_ipc&source=gui&idsession=activation-test@EOF")
		if !strings.Contains(result, "SAF_") {
			t.Fatalf("comando directo %s no rechazado: %q", action, result)
		}
	}
	got, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("socket cambió ajustes: %s (%v)", got, err)
	}
}
