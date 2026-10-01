// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"encoding/json"
	"net/http"
	"strings"

	"grxfirma/internal/observability/correlation"
)

// restCorrelationMiddleware da a cada petición una referencia interna antes
// de autorización y dispatch. No crea cabeceras ni modifica la respuesta: la
// correlación es exclusivamente contextual.
func restCorrelationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, err := correlation.WithGenerated(r.Context())
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "no se pudo iniciar la operacion")
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// withRESTRequestID sustituye la referencia generada por una derivada del
// request_id protocolario cuando el cliente ya lo envía. El valor bruto se
// valida, pero no se guarda en context.Context.
func withRESTRequestID(w http.ResponseWriter, r *http.Request, requestID string, present bool) (*http.Request, bool) {
	if present && requestID == "" {
		writeError(w, http.StatusBadRequest, "request_id no es válido")
		return r, false
	}
	if !present {
		// Routes ya instala una referencia. Este respaldo conserva seguros los
		// handlers invocados directamente en pruebas o integraciones internas.
		if _, ok := correlation.From(r.Context()); ok {
			return r, true
		}
		ctx, err := correlation.WithGenerated(r.Context())
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "no se pudo iniciar la operacion")
			return r, false
		}
		return r.WithContext(ctx), true
	}

	ctx, err := correlation.With(r.Context(), requestID, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, "request_id no es válido")
		return r, false
	}
	return r.WithContext(ctx), true
}

func (req *signRequest) UnmarshalJSON(data []byte) error {
	type wireSignRequest signRequest

	var decoded wireSignRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*req = signRequest(decoded)
	req.requestIDPresent = jsonFieldPresent(data, "request_id")
	return nil
}

func (req *signBatchRequest) UnmarshalJSON(data []byte) error {
	type wireSignBatchRequest signBatchRequest

	var decoded wireSignBatchRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*req = signBatchRequest(decoded)
	req.requestIDPresent = jsonFieldPresent(data, "request_id")
	return nil
}

func jsonFieldPresent(data []byte, expected string) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return false
	}
	for name := range fields {
		if strings.EqualFold(name, expected) {
			return true
		}
	}
	return false
}
