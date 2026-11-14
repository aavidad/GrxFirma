// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest_test

import (
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// B-05: la API REST ya no tiene modo de rutas de fichero. Ninguna ruta puede
// escribir el resultado en un outputPath elegido por el cliente.
func TestRutasDeFichero_NingunEndpointEscribeEnOutputPath(t *testing.T) {
	destino := filepath.Join(t.TempDir(), ".ssh", "authorized_keys")
	contenido := base64.StdEncoding.EncodeToString([]byte("abc"))
	adaptador := nuevoAdaptadorPruebas(signUseCaseMock{}, verifyUseCaseMock{}, nil).
		WithBatchSigner(&batchSignUseCaseMock{}).
		WithProtection(&protectUseCaseMock{}, &unprotectUseCaseMock{}, nil).
		WithSignedProtection(&protectAndSignUseCaseMock{}).
		WithHashes(hashCreateUseCaseMock{}, hashCheckUseCaseMock{})
	h := adaptador.Routes()

	casos := map[string]map[string]any{
		"/sign": {"name": "doc.txt", "content_base64": contenido, "format": "CAdES", "certificate_id": "cert-1",
			"inputPath": destino, "outputPath": destino, "overwrite": "true"},
		"/sign-batch": {"certificate_id": "cert-1", "format": "CAdES", "saveToDisk": true, "overwrite": "true",
			"items": []map[string]any{{"name": "doc.txt", "content_base64": contenido, "outputPath": destino}}},
		"/protect":      {"name": "s.txt", "content_base64": contenido, "inputPath": destino, "outputPath": destino},
		"/protect-sign": {"name": "s.txt", "content_base64": contenido, "inputPath": destino, "outputPath": destino},
		"/unprotect":    {"name": "s.p7m", "content_base64": contenido, "inputPath": destino, "outputPath": destino},
		"/hash":         {"content_base64": contenido, "format": "hex", "outputPath": destino},
		"/verify":       {"inputPath": destino},
	}
	for ruta, cuerpo := range casos {
		rr := doJSON(t, h, http.MethodPost, ruta, cuerpo)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("%s con ruta de fichero = %d; se esperaba 403: %s", ruta, rr.Code, rr.Body.String())
		}
		if _, err := os.Stat(destino); !os.IsNotExist(err) {
			t.Fatalf("%s escribió en %s", ruta, destino)
		}
	}
}
