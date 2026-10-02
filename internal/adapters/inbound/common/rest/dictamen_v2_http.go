// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

const contratoDictamenV2 = "autofirmav2.dictamen-verificacion.v2"

// La ruta v2 valida el contrato y nunca interpreta una solicitud v2 como v1.
func (a *Adaptador) handleVerifyV2(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "metodo no permitido")
		return
	}
	body, ok := a.readVerifyBody(w, r)
	if !ok {
		return
	}
	if err := rechazarClavesJSONDuplicadasConCampos(body, map[string]struct{}{
		"name": {}, "content_base64": {}, "original_content_base64": {}, "contrato_solicitado": {},
	}); err != nil {
		writeError(w, http.StatusBadRequest, "json de verificacion con claves duplicadas o invalido")
		return
	}
	var req struct {
		Name               string `json:"name"`
		ContentBase64      string `json:"content_base64"`
		OriginalBase64     string `json:"original_content_base64"`
		ContratoSolicitado string `json:"contrato_solicitado"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "json de verificacion invalido")
		return
	}
	if req.ContratoSolicitado != "" && req.ContratoSolicitado != contratoDictamenV2 {
		writeError(w, http.StatusBadRequest, "contrato_solicitado no reconocido")
		return
	}
	if req.ContentBase64 == "" {
		writeError(w, http.StatusBadRequest, "content_base64 obligatorio")
		return
	}
	if a.Verificar == nil {
		writeError(w, http.StatusServiceUnavailable, "verificacion no configurada")
		return
	}
	pdfBytes, err := base64.StdEncoding.Strict().DecodeString(req.ContentBase64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "content_base64 no es valido")
		return
	}
	var original []byte
	if req.OriginalBase64 != "" {
		original, err = base64.StdEncoding.Strict().DecodeString(req.OriginalBase64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "original_content_base64 no es valido")
			return
		}
	}
	writeJSON(w, http.StatusOK, a.evaluarDictamenV2(r.Context(), pdfBytes, original))
}

func (a *Adaptador) readVerifyBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	limit := a.MaxBodyBytes
	if limit <= 0 {
		limit = 100 << 20
	}
	if r.ContentLength > limit {
		writeError(w, http.StatusRequestEntityTooLarge, "cuerpo de verificacion demasiado grande")
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	var maxErr *http.MaxBytesError
	if int64(len(body)) > limit || errors.As(err, &maxErr) {
		writeError(w, http.StatusRequestEntityTooLarge, "cuerpo de verificacion demasiado grande")
		return nil, false
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "json de verificacion invalido")
		return nil, false
	}
	return body, true
}

// Recorre todos los objetos JSON, también los anidados, para detectar claves
// repetidas antes de decodificar el cuerpo en structs. Limita la profundidad
// antes de que un documento adversario consuma pila o memoria excesiva.
func rechazarClavesJSONDuplicadas(data []byte) error {
	return rechazarClavesJSONDuplicadasConCampos(data, nil)
}

func rechazarClavesJSONDuplicadasConCampos(data []byte, allowed map[string]struct{}) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var visit func(int) error
	visit = func(depth int) error {
		if depth > 8 {
			return errors.New("profundidad JSON excesiva")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok {
					return errors.New("clave JSON invalida")
				}
				if depth == 0 && allowed != nil {
					if _, exists := allowed[name]; !exists {
						return errors.New("campo JSON no permitido")
					}
				}
				name = strings.ToLower(name)
				if _, exists := seen[name]; exists {
					return errors.New("clave JSON duplicada")
				}
				seen[name] = struct{}{}
				if err := visit(depth + 1); err != nil {
					return err
				}
			}
			_, err := decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := visit(depth + 1); err != nil {
					return err
				}
			}
			_, err := decoder.Token()
			return err
		default:
			return errors.New("delimitador JSON invalido")
		}
	}
	if err := visit(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("JSON con datos posteriores")
	}
	return nil
}
