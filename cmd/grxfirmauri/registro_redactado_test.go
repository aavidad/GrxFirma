// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"log"
	"log/slog"
	"strings"
	"testing"
)

// A-10: lo que se escribe con slog por defecto o con log.Printf (como el
// orquestador de sesión) pasa por la misma redacción que logging.New.
func TestRegistroPorDefectoRedacta(t *testing.T) {
	prevSlog := slog.Default()
	prevOut, prevFlags := log.Writer(), log.Flags()
	t.Cleanup(func() {
		slog.SetDefault(prevSlog)
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})

	var buf bytes.Buffer
	configurarRegistroPorDefecto(&buf)
	log.Printf("[Session] identificacion start upload=%s request_id=%s key=%t origenes=%v",
		"https://srv.example/afirma-signature-retriever/StorageService?op=put", "REQSECRETO123", true, []string{"https://sede.example"})
	slog.Info("legacy_document_pick_selected", "name", "nomina_marta.pdf")

	salida := buf.String()
	if salida == "" {
		t.Fatal("no se registró nada")
	}
	for _, secreto := range []string{"REQSECRETO123", "srv.example", "StorageService", "nomina_marta"} {
		if strings.Contains(salida, secreto) {
			t.Fatalf("la traza contiene %q sin redactar:\n%s", secreto, salida)
		}
	}
}
