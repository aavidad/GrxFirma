// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"grxfirma/internal/adapters/outbound/common/signer"
)

// KD-03 en escritorio (lote 17b). Antes de cofirmar o contrafirmar, Qt y
// WinUI preguntan al motor si la firma de entrada es una CAdES explícita (no
// lleva el documento dentro). Si lo es, ofrecen elegir el documento original
// (camino recomendado) o firmar de todos modos. Con el original, «sign» lo
// recibe en originalPath y el motor lo coteja antes de firmar; sin él firma
// como siempre, con el aviso de cofirma sin original.

const accionCofirmaOriginal = "cosign_original_check"

// paramsCofirmaOriginal mapea los parámetros de "cosign_original_check".
type paramsCofirmaOriginal struct {
	InputPath string `json:"inputPath"`
	Format    string `json:"format,omitempty"`
	Action    string `json:"action"`
}

// resultadoCofirmaOriginal dice si conviene pedir el documento original.
type resultadoCofirmaOriginal struct {
	NeedsOriginal bool `json:"needsOriginal"`
}

func (m *Manejador) handleCofirmaOriginal(_ context.Context, raw json.RawMessage) respuesta {
	var params paramsCofirmaOriginal
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: accionCofirmaOriginal, Error: m.t("error.formato_invalido")}
	}
	if !esCofirmaOContrafirmaIPC(params.Action) || !formatoPuedeSerCAdES(params.Format, params.InputPath) {
		return respuesta{OK: true, Action: accionCofirmaOriginal, Data: resultadoCofirmaOriginal{}}
	}
	if err := validarRutaLectura(params.InputPath); err != nil {
		return respuesta{OK: false, Action: accionCofirmaOriginal, Error: err.Error()}
	}
	if err := verificarTamano(params.InputPath); err != nil {
		return respuesta{OK: false, Action: accionCofirmaOriginal, Error: err.Error()}
	}
	contenido, err := leerFicheroSeguroIPC(params.InputPath)
	if err != nil {
		return respuesta{OK: false, Action: accionCofirmaOriginal, Error: m.t("error.archivo_no_encontrado", params.InputPath)}
	}
	_, implicita, err := signer.ContenidoCAdESImplicito(contenido)
	// Si no se puede leer como CAdES, la firma fallará después con su propio
	// error: no hay nada que preguntar antes.
	return respuesta{OK: true, Action: accionCofirmaOriginal, Data: resultadoCofirmaOriginal{
		NeedsOriginal: err == nil && !implicita,
	}}
}

func esCofirmaOContrafirmaIPC(accion string) bool {
	switch strings.ToLower(strings.TrimSpace(accion)) {
	case "cosign", "countersign":
		return true
	}
	return false
}

// formatoPuedeSerCAdES descarta los formatos que no son CAdES. Sin formato
// (o «auto») decide la extensión, como en «sign».
func formatoPuedeSerCAdES(formato, ruta string) bool {
	formato = strings.ToLower(strings.TrimSpace(formato))
	if formato == "" || formato == "auto" {
		formato = inferirFormatoRuta(ruta)
	}
	return formato == "cades"
}

// leerOriginalCofirmaIPC lee el documento original que acompaña a una
// cofirma o contrafirma. Fuera de esas acciones no se usa.
func (m *Manejador) leerOriginalCofirmaIPC(ruta, accion string) ([]byte, error) {
	ruta = strings.TrimSpace(ruta)
	if ruta == "" || !esCofirmaOContrafirmaIPC(accion) {
		return nil, nil
	}
	if err := validarRutaLectura(ruta); err != nil {
		return nil, err
	}
	if err := verificarTamano(ruta); err != nil {
		return nil, err
	}
	contenido, err := leerFicheroSeguroIPC(ruta)
	if err != nil {
		return nil, errors.New(m.t("error.archivo_no_encontrado", ruta))
	}
	return contenido, nil
}

// mensajeErrorCofirmaOriginal traduce los errores del cotejo con el original
// a sus claves de catálogo; los demás errores se devuelven como siempre.
func (m *Manejador) mensajeErrorCofirmaOriginal(err error) string {
	switch {
	case errors.Is(err, signer.ErrCotejoSHA1Prohibido):
		return m.t("firma.sha1_corresponde.prohibido")
	case errors.Is(err, signer.ErrFirmaNoCorrespondeOriginal):
		return m.t("firma.cofirma_sin_original.original_no_corresponde")
	}
	return err.Error()
}
