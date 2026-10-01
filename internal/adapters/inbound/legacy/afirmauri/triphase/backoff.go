// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package triphase

import (
	"context"
	"net/http"
	"time"
)

// backoffDuraciones son los tiempos de espera entre reintentos (1s, 2s, 4s).
var backoffDuraciones = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

// conReintentos ejecuta f hasta maxIntentos veces con backoff exponencial.
// Solo reintenta en errores de red (f devuelve error != nil).
// Errores HTTP (4xx/5xx) NO se reintentan: se devuelven tal cual al llamador.
// Respeta la cancelación del contexto entre reintentos.
func conReintentos(ctx context.Context, maxIntentos int, f func() (*http.Response, error)) (*http.Response, error) {
	if maxIntentos <= 0 {
		maxIntentos = 1
	}
	var lastErr error
	for intento := 0; intento < maxIntentos; intento++ {
		// Comprobar cancelación antes de cada intento.
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		resp, err := f()
		if err == nil {
			// Petición completada (independientemente del código HTTP).
			return resp, nil
		}

		// Error de red: registrar y esperar antes del siguiente intento.
		lastErr = err
		if intento < maxIntentos-1 {
			// Calcular tiempo de espera con índice seguro.
			idx := intento
			if idx >= len(backoffDuraciones) {
				idx = len(backoffDuraciones) - 1
			}
			espera := backoffDuraciones[idx]

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(espera):
			}
		}
	}
	return nil, lastErr
}
