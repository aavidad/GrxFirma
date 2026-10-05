// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/json"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/protector"
)

func TestIPCImportarYQuitarDestinatarioPublico(t *testing.T) {
	ctx := context.Background()
	book := protector.NuevoLocalPublicRecipientBook(t.TempDir())
	m := &Manejador{Loc: localizador.Para("en"), Destinatarios: &protector.LocalCombinedKeyring{Public: book}}
	cert := generarDestinatarioAuthEnvelopedIPC(t, "sintetico")
	path := filepath.Join(t.TempDir(), "sintetico.cer")
	if err := os.WriteFile(path, cert.CertificateDER, 0o600); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]string{"path": path})
	result := m.despachar(ctx, peticion{Action: "protection_recipient_import", Params: params})
	if !result.OK {
		t.Fatalf("alta IPC: %s", result.Error)
	}
	listed := m.despachar(ctx, peticion{Action: "protection_recipients", Params: json.RawMessage(`{}`)})
	if !listed.OK {
		t.Fatalf("listado IPC: %s", listed.Error)
	}
	data := listed.Data.(resultadoDestinatariosProteccion)
	if len(data.Recipients) != 1 || data.Recipients[0].Origin != "importado" {
		t.Fatalf("origen o listado: %+v", data.Recipients)
	}
	id := data.Recipients[0].ID
	remove, _ := json.Marshal(map[string]string{"id": id})
	if result := m.despachar(ctx, peticion{Action: "protection_recipient_remove", Params: remove}); !result.OK {
		t.Fatalf("baja IPC: %s", result.Error)
	}
	if result := m.despachar(ctx, peticion{Action: "protection_recipient_remove", Params: remove}); result.OK {
		t.Fatal("segunda baja aceptada")
	}
	if result := m.handleProtectionRecipientImport(ctx, json.RawMessage(`{"path":"/etc/passwd"}`)); result.OK {
		t.Fatal("ruta prohibida aceptada")
	}
	if err := os.WriteFile(path, []byte("-----BEGIN PRIVATE KEY-----"), 0o600); err != nil {
		t.Fatal(err)
	}
	if result := m.handleProtectionRecipientImport(ctx, params); result.OK || result.Diagnostic == nil ||
		!strings.Contains(result.Diagnostic.UserMessage, "private key") {
		t.Fatalf("rechazo sin explicación traducida: %+v", result)
	}
}
