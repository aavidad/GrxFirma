// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package limits centraliza los valores configurables de limites operacionales:
// timeout HTTP, tamaño maximo de payload, maximo documentos en batch y tiempo
// maximo de sesion. Los valores se pueden sobreescribir mediante variables de
// entorno GRXFIRMA_* sin necesidad de recompilar.
package limits

import (
	"fmt"
	"os"
	"strconv"
)

// Limits agrupa todos los limites operacionales del sistema.
type Limits struct {
	// HTTPTimeoutSec es el timeout en segundos para peticiones HTTP salientes.
	// Defecto: 30.
	HTTPTimeoutSec int

	// MaxPayloadBytes es el tamaño maximo en bytes de un documento a procesar.
	// Defecto: 52428800 (50 MB).
	MaxPayloadBytes int64

	// MaxBatchDocs es el numero maximo de documentos en una operacion por lotes.
	// Defecto: 100.
	MaxBatchDocs int

	// MaxSessionSec es la duracion maxima en segundos de una sesion de firma.
	// Defecto: 300 (5 min).
	MaxSessionSec int
}

// ErrPayloadExcedido se retorna cuando el tamaño del payload supera MaxPayloadBytes.
type ErrPayloadExcedido struct {
	Tamaño int64
	Maximo int64
}

func (e *ErrPayloadExcedido) Error() string {
	return fmt.Sprintf("payload de %d bytes supera el limite de %d bytes", e.Tamaño, e.Maximo)
}

// Default devuelve los limites con los valores por defecto del sistema.
func Default() Limits {
	return Limits{
		HTTPTimeoutSec:  30,
		MaxPayloadBytes: 52428800, // 50 MB
		MaxBatchDocs:    100,
		MaxSessionSec:   300,
	}
}

// FromEnv devuelve una copia de base sobreescribiendo los campos cuyos valores se
// encuentren en las variables de entorno GRXFIRMA_*. Si una variable esta presente
// pero contiene un valor no numerico, se usa el valor de base sin panic.
func FromEnv(base Limits) Limits {
	result := base

	if v, ok := envInt("GRXFIRMA_HTTP_TIMEOUT_SEC"); ok {
		result.HTTPTimeoutSec = v
	}
	if v, ok := envInt64("GRXFIRMA_MAX_PAYLOAD_BYTES"); ok {
		result.MaxPayloadBytes = v
	}
	if v, ok := envInt("GRXFIRMA_MAX_BATCH_DOCS"); ok {
		result.MaxBatchDocs = v
	}
	if v, ok := envInt("GRXFIRMA_MAX_SESSION_SEC"); ok {
		result.MaxSessionSec = v
	}

	return result
}

// CheckPayload verifica que data no supera MaxPayloadBytes. Retorna
// *ErrPayloadExcedido si el tamaño supera el limite.
func (l Limits) CheckPayload(data []byte) error {
	size := int64(len(data))
	if size > l.MaxPayloadBytes {
		return &ErrPayloadExcedido{Tamaño: size, Maximo: l.MaxPayloadBytes}
	}
	return nil
}

// envInt lee una variable de entorno y la convierte a int.
// Devuelve (valor, true) si la variable existe y es valida, o (0, false) si no existe o es invalida.
func envInt(key string) (int, bool) {
	s := os.Getenv(key)
	if s == "" {
		return 0, false
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return v, true
}

// envInt64 lee una variable de entorno y la convierte a int64.
func envInt64(key string) (int64, bool) {
	s := os.Getenv(key)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
