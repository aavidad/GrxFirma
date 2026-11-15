// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// SPDX-License-Identifier: EUPL-1.2
package nativehost

import (
	"bytes"
	"context"
	"encoding/json"
	"grxfirma/internal/testsupport/perfil"
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/appdirs"
)

func TestNativehostExcepcionesNoAceptaActivacionAunqueDeclareGUI(t *testing.T) {
	home := perfil.Nuevo(t)
	dir := appdirs.Config(home)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"excepciones_inseguras":{"sha1_crear":false}}`)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), original, 0600); err != nil {
		t.Fatal(err)
	}
	adapter := New(nil, nil, nil, nil)
	for _, action := range []string{"unsafe_exceptions_activate", "unsafe_exceptions_set", "unsafe_exceptions_retry", "save_settings"} {
		payload, err := json.Marshal(map[string]any{
			"requestId": "activation-test", "action": action,
			"id": "sha1_crear", "enabled": true, "accepted": true,
			"source": "gui", "channel": "local_ipc", "local": true,
			"params": map[string]any{"id": "sha1_crear", "enabled": true, "accepted": true, "excepciones_inseguras": map[string]bool{"sha1_crear": true}},
		})
		if err != nil {
			t.Fatal(err)
		}
		responses, err := adapter.Process(context.Background(), "", payload)
		if err != nil || len(responses) != 1 {
			t.Fatalf("%s: respuestas=%d err=%v", action, len(responses), err)
		}
		var response struct {
			Success bool   `json:"success"`
			Error   string `json:"error"`
		}
		if err := json.Unmarshal(responses[0], &response); err != nil {
			t.Fatal(err)
		}
		if response.Success || response.Error == "" {
			t.Fatalf("nativehost admite %s: %s", action, responses[0])
		}
	}
	got, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("nativehost cambió ajustes: %s (%v)", got, err)
	}
}
