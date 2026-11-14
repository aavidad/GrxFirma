// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"grxfirma/internal/adapters/outbound/common/protector"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

func TestProtectIPC_AvisosOpcionalesConYSinGuardado(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "acta.txt")
	if err := os.WriteFile(input, []byte("contenido"), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := domain.NewDocument("acta.txt.afp", []byte("protegido"), domain.MIMETypeProtectedEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	for _, guardar := range []bool{false, true} {
		for _, conAvisos := range []bool{false, true} {
			var avisos []string
			if conAvisos {
				avisos = []string{protector.AvisoProteccionAltoSinAlmacen}
			}
			m := &Manejador{Proteger: &stubProteger{result: application.ProtectResult{Protected: domain.ProtectedPayload{Document: doc, Profile: domain.ProtectionProfileStrong, RecipientCount: 1}, Warnings: avisos}}}
			params := paramsProtection{InputPath: input, OutputPath: filepath.Join(dir, "salida.afp"), Profile: "alto", RecipientIDs: []string{"propio"}, SaveToDisk: guardar, ReturnProtectedB64: true, Overwrite: "rename"}
			res := m.despachar(context.Background(), peticionJSON(t, "protect", params))
			if !res.OK {
				t.Fatalf("protect: %s", res.Error)
			}
			data := res.Data.(resultadoProteccion)
			if !slices.Equal(data.Warnings, avisos) {
				t.Fatalf("avisos=%q", data.Warnings)
			}
			raw, err := json.Marshal(res.Data)
			if err != nil {
				t.Fatal(err)
			}
			var campos map[string]json.RawMessage
			if err := json.Unmarshal(raw, &campos); err != nil {
				t.Fatal(err)
			}
			if _, present := campos["warnings"]; present != conAvisos {
				t.Fatalf("warnings presente=%v: %s", present, raw)
			}
			if data.ProtectedContentB64 == "" || guardar && data.OutputPath == "" {
				t.Fatalf("resultado incompleto: %+v", data)
			}
		}
	}
}
