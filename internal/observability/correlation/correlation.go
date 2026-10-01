// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package correlation propaga una referencia de operación opaca mediante
// context.Context. Los identificadores recibidos de protocolos externos se
// validan, pero nunca se guardan en el contexto.
package correlation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
)

const (
	maxIDBytes = 128

	domainSeparator = "grxfirma:operation-correlation:v1\x00"
)

var (
	// ErrInvalidID indica que un identificador presente no cumple el formato
	// ASCII acotado admitido en las fronteras de GrxFirma.
	ErrInvalidID = errors.New("identificador de correlacion no valido")

	// ErrMissingID indica que no se ha proporcionado ningún identificador del
	// que derivar una referencia. Las fronteras que mantengan clientes legacy
	// pueden usar WithGenerated de forma explícita.
	ErrMissingID = errors.New("falta identificador de correlacion")
)

// Correlation contiene exclusivamente una referencia derivada. El
// identificador protocolario bruto no forma parte de este valor.
type Correlation struct {
	reference string
}

// Reference devuelve la referencia opaca, apta para correlacionar trazas sin
// revelar el identificador recibido.
func (c Correlation) Reference() string {
	return c.reference
}

type contextKey struct{}

// ValidateID acepta identificadores ASCII de 1 a 128 bytes. El alfabeto
// limitado evita controles, espacios, Unicode y separadores que podrían
// alterar logs o formatos de exportación.
func ValidateID(value string) error {
	if len(value) == 0 || len(value) > maxIDBytes {
		return ErrInvalidID
	}
	for i := 0; i < len(value); i++ {
		switch b := value[i]; {
		case b >= 'a' && b <= 'z':
		case b >= 'A' && b <= 'Z':
		case b >= '0' && b <= '9':
		case b == '.', b == '_', b == ':', b == '-':
		default:
			return ErrInvalidID
		}
	}
	return nil
}

// With valida los identificadores presentes y añade al contexto únicamente
// una referencia SHA-256 con separación de dominio. traceID es la identidad
// de operación preferida; requestID actúa como respaldo cuando no hay traza.
func With(parent context.Context, requestID, traceID string) (context.Context, error) {
	if requestID != "" {
		if err := ValidateID(requestID); err != nil {
			return parent, err
		}
	}
	if traceID != "" {
		if err := ValidateID(traceID); err != nil {
			return parent, err
		}
	}
	if requestID == "" && traceID == "" {
		return parent, ErrMissingID
	}

	kind, selectedID := "request", requestID
	if traceID != "" {
		kind, selectedID = "trace", traceID
	}
	return withReference(parent, deriveReference(kind, []byte(selectedID))), nil
}

// WithGenerated añade una referencia interna aleatoria. Permite mantener
// compatibilidad con clientes legacy que no enviaban identificadores sin
// introducir un identificador bruto inventado en el contexto.
func WithGenerated(parent context.Context) (context.Context, error) {
	var seed [32]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return parent, errors.Join(errors.New("generando correlacion interna"), err)
	}
	return withReference(parent, deriveReference("generated", seed[:])), nil
}

// From recupera la correlación opaca del contexto.
func From(ctx context.Context) (Correlation, bool) {
	if ctx == nil {
		return Correlation{}, false
	}
	value, ok := ctx.Value(contextKey{}).(Correlation)
	if !ok || value.reference == "" {
		return Correlation{}, false
	}
	return value, true
}

func withReference(parent context.Context, reference string) context.Context {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithValue(parent, contextKey{}, Correlation{reference: reference})
}

func deriveReference(kind string, value []byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte(domainSeparator))
	_, _ = h.Write([]byte(kind))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(value)
	sum := h.Sum(nil)

	return fmt.Sprintf(
		"corr-sha256-%x-%x-%x-%x-%x-%x-%x-%x",
		sum[0:4], sum[4:8], sum[8:12], sum[12:16],
		sum[16:20], sum[20:24], sum[24:28], sum[28:32],
	)
}
