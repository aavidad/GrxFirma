// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package nativehost

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// El plazo declarado aquí debe ser el que usan de verdad los fondos de la extensión.
func TestPlazoExtensionCoincideConBackgroundJS(t *testing.T) {
	t.Parallel()

	patron := regexp.MustCompile(`\}, (\d+)\);\s+pendingRequests\.set\(requestId`)
	for _, variante := range []string{"chromium", "firefox"} {
		ruta := filepath.Join("..", "..", "..", "..", "..", "packaging", "browser-extensions", "src", variante, "background.js")
		fuente, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatalf("leer %s: %v", ruta, err)
		}
		m := patron.FindSubmatch(fuente)
		if m == nil {
			t.Fatalf("%s: no se encuentra el plazo de sendToNativeHost", ruta)
		}
		ms, _ := strconv.Atoi(string(m[1]))
		if time.Duration(ms)*time.Millisecond != PlazoExtension {
			t.Fatalf("%s: plazo %d ms, PlazoExtension = %v", ruta, ms, PlazoExtension)
		}
	}
}

// C-04: un JSON que no decodifica debe devolver el requestId que traía, para
// que la extensión cierre la petición pendiente sin esperar a su plazo.
func TestProcessJSONInvalidoConservaRequestID(t *testing.T) {
	t.Parallel()

	adapter := New(nil, nil, nil, nil)
	payload := []byte(`{"requestId":"r1","action":"sign","signatureOptions":{"x":1}}`)
	responses, err := adapter.Process(context.Background(), "", payload)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	var resp response
	if err := json.Unmarshal(responses[0], &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.Success || resp.RequestID != "r1" {
		t.Fatalf("resp = %+v, se esperaba error con requestId r1", resp)
	}
}

func TestPlazoOperacionQuedaPorDebajoDelDeLaExtension(t *testing.T) {
	t.Parallel()

	if PlazoMaximoOperacion >= PlazoExtension {
		t.Fatalf("PlazoMaximoOperacion = %v; debe ser menor que el de la extensión (%v)", PlazoMaximoOperacion, PlazoExtension)
	}
	casos := []struct {
		configurado time.Duration
		want        time.Duration
	}{
		{300 * time.Second, PlazoMaximoOperacion},
		{60 * time.Second, 60 * time.Second},
		{0, PlazoMaximoOperacion},
		{-1, PlazoMaximoOperacion},
	}
	for _, c := range casos {
		if got := PlazoOperacion(c.configurado); got != c.want {
			t.Errorf("PlazoOperacion(%v) = %v, want %v", c.configurado, got, c.want)
		}
	}
}
