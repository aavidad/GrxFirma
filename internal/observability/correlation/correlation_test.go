// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package correlation_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"

	"grxfirma/internal/observability/correlation"
)

func TestWithPropagaSoloReferenciaOpaca(t *testing.T) {
	t.Parallel()

	const (
		requestID = "req-secreto-123"
		traceID   = "trace-secreto-456"
	)
	ctx, err := correlation.With(context.Background(), requestID, traceID)
	if err != nil {
		t.Fatalf("With() error = %v", err)
	}

	value, ok := correlation.From(ctx)
	if !ok {
		t.Fatal("From() no encontro la correlacion")
	}
	if !strings.HasPrefix(value.Reference(), "corr-sha256-") {
		t.Fatalf("Reference() = %q; falta el formato SHA-256 opaco", value.Reference())
	}
	for _, raw := range []string{requestID, traceID} {
		if strings.Contains(value.Reference(), raw) {
			t.Fatalf("la referencia contiene el identificador bruto %q", raw)
		}
		if strings.Contains(fmt.Sprintf("%#v", ctx), raw) {
			t.Fatalf("el contexto contiene el identificador bruto %q", raw)
		}
	}
}

func TestWithEsEstablePorTraceYValidaTodosLosIDs(t *testing.T) {
	t.Parallel()

	first, err := correlation.With(context.Background(), "req-1", "trace-common")
	if err != nil {
		t.Fatalf("With(first) error = %v", err)
	}
	second, err := correlation.With(context.Background(), "req-2", "trace-common")
	if err != nil {
		t.Fatalf("With(second) error = %v", err)
	}
	firstValue, _ := correlation.From(first)
	secondValue, _ := correlation.From(second)
	if firstValue.Reference() != secondValue.Reference() {
		t.Fatal("la misma traza debe producir la misma referencia entre peticiones")
	}

	if _, err := correlation.With(context.Background(), "req\ninyectado", "trace-common"); !errors.Is(err, correlation.ErrInvalidID) {
		t.Fatalf("un requestID invalido junto a traceID valido devolvio %v", err)
	}
}

func TestValidateIDRechazaValoresNoAcotados(t *testing.T) {
	t.Parallel()

	valid := []string{
		"a",
		"ipc-123_ABC.test:4",
		strings.Repeat("x", 128),
	}
	for _, value := range valid {
		if err := correlation.ValidateID(value); err != nil {
			t.Errorf("ValidateID(%q) error = %v", value, err)
		}
	}

	invalid := []string{
		"",
		" con-espacio",
		"con espacio",
		"linea\nnueva",
		"tab\tinterno",
		"control\x00nulo",
		"traza/segmento",
		"traza-ñ",
		strings.Repeat("x", 129),
	}
	for _, value := range invalid {
		if err := correlation.ValidateID(value); !errors.Is(err, correlation.ErrInvalidID) {
			t.Errorf("ValidateID(%q) error = %v; want ErrInvalidID", value, err)
		}
	}
}

func TestWithRequiereAlMenosUnID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	got, err := correlation.With(ctx, "", "")
	if !errors.Is(err, correlation.ErrMissingID) {
		t.Fatalf("With() error = %v; want ErrMissingID", err)
	}
	if got != ctx {
		t.Fatal("With() debe devolver el contexto original al rechazar la entrada")
	}
}

func TestReferenciaUsaSeparacionDeDominio(t *testing.T) {
	t.Parallel()

	const raw = "trace-123"
	ctx, err := correlation.With(context.Background(), "", raw)
	if err != nil {
		t.Fatalf("With() error = %v", err)
	}
	value, _ := correlation.From(ctx)

	plain := fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
	normalized := strings.ReplaceAll(strings.TrimPrefix(value.Reference(), "corr-sha256-"), "-", "")
	if normalized == plain {
		t.Fatal("la referencia coincide con SHA-256(raw): falta separación de dominio")
	}
}

func TestWithGeneratedCreaReferenciasInternasDistintas(t *testing.T) {
	t.Parallel()

	first, err := correlation.WithGenerated(context.Background())
	if err != nil {
		t.Fatalf("WithGenerated(first) error = %v", err)
	}
	second, err := correlation.WithGenerated(context.Background())
	if err != nil {
		t.Fatalf("WithGenerated(second) error = %v", err)
	}
	firstValue, firstOK := correlation.From(first)
	secondValue, secondOK := correlation.From(second)
	if !firstOK || !secondOK {
		t.Fatal("WithGenerated() no dejo una correlacion recuperable")
	}
	if firstValue.Reference() == secondValue.Reference() {
		t.Fatal("dos correlaciones internas aleatorias no deben coincidir")
	}
}

func TestFromSinCorrelacion(t *testing.T) {
	t.Parallel()

	if _, ok := correlation.From(context.Background()); ok {
		t.Fatal("From(context.Background()) = ok; want false")
	}
}
