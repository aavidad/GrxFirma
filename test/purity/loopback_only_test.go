// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// TestListenersInboundSoloLoopback (T085, criterio 4) impide que los adaptadores
// de entrada abran superficies de red fuera de loopback. Falla si alguna fuente
// de internal/adapters/inbound contiene un literal de bind a todas las
// interfaces ("0.0.0.0" o un puerto sin host tipo ":%d" / ":"+puerto pasados a
// Listen), y verifica que los servidores legacy TLS sí anclan a 127.0.0.1.
package purity_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListenersInboundSoloLoopback(t *testing.T) {
	raiz := raizModulo(t)
	inbound := filepath.Join(raiz, "internal", "adapters", "inbound")

	// Patrones prohibidos: bind a todas las interfaces.
	prohibidos := []string{
		`"0.0.0.0`,             // dirección explícita de todas las interfaces
		`tls.Listen("tcp", ":`, // puerto sin host => todas las interfaces
		`net.Listen("tcp", ":`,
	}

	err := filepath.WalkDir(inbound, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			t.Fatalf("no se pudo leer %s: %v", path, rerr)
		}
		texto := string(data)
		for _, patron := range prohibidos {
			if strings.Contains(texto, patron) {
				t.Errorf("%s: bind a todas las interfaces detectado (%q); los adaptadores de entrada deben anclar a 127.0.0.1", relParaTest(raiz, path), patron)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("error recorriendo %s: %v", inbound, err)
	}

	// Verificación positiva: los servidores legacy TLS anclan a loopback.
	for _, rel := range []string{
		filepath.Join("internal", "adapters", "inbound", "legacy", "websocket", "server.go"),
		filepath.Join("internal", "adapters", "inbound", "legacy", "websocket", "service_socket.go"),
	} {
		data, rerr := os.ReadFile(filepath.Join(raiz, rel))
		if rerr != nil {
			t.Fatalf("no se pudo leer %s: %v", rel, rerr)
		}
		if !strings.Contains(string(data), "127.0.0.1:") {
			t.Errorf("%s: se esperaba un bind explícito a 127.0.0.1", rel)
		}
	}
}
