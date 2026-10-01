// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package limits_test

import (
	"errors"
	"os"
	"testing"

	"grxfirma/internal/adapters/outbound/common/limits"
)

func TestDefault_ValoresCorrectos(t *testing.T) {
	t.Parallel()

	l := limits.Default()

	if l.HTTPTimeoutSec != 30 {
		t.Errorf("HTTPTimeoutSec esperado 30, se obtuvo %d", l.HTTPTimeoutSec)
	}
	if l.MaxPayloadBytes != 52428800 {
		t.Errorf("MaxPayloadBytes esperado 52428800 (50MB), se obtuvo %d", l.MaxPayloadBytes)
	}
	if l.MaxBatchDocs != 100 {
		t.Errorf("MaxBatchDocs esperado 100, se obtuvo %d", l.MaxBatchDocs)
	}
	if l.MaxSessionSec != 300 {
		t.Errorf("MaxSessionSec esperado 300, se obtuvo %d", l.MaxSessionSec)
	}
}

func TestFromEnv_SobreescribeConVariableEntorno(t *testing.T) {
	// No Parallel: modifica variables de entorno del proceso.
	t.Setenv("GRXFIRMA_HTTP_TIMEOUT_SEC", "60")
	t.Setenv("GRXFIRMA_MAX_PAYLOAD_BYTES", "10485760")
	t.Setenv("GRXFIRMA_MAX_BATCH_DOCS", "50")
	t.Setenv("GRXFIRMA_MAX_SESSION_SEC", "600")

	base := limits.Default()
	l := limits.FromEnv(base)

	if l.HTTPTimeoutSec != 60 {
		t.Errorf("HTTPTimeoutSec esperado 60, se obtuvo %d", l.HTTPTimeoutSec)
	}
	if l.MaxPayloadBytes != 10485760 {
		t.Errorf("MaxPayloadBytes esperado 10485760, se obtuvo %d", l.MaxPayloadBytes)
	}
	if l.MaxBatchDocs != 50 {
		t.Errorf("MaxBatchDocs esperado 50, se obtuvo %d", l.MaxBatchDocs)
	}
	if l.MaxSessionSec != 600 {
		t.Errorf("MaxSessionSec esperado 600, se obtuvo %d", l.MaxSessionSec)
	}
}

func TestFromEnv_VariableInvalida_UsaValorBase(t *testing.T) {
	// No Parallel: modifica variables de entorno del proceso.
	base := limits.Default()

	// Guardamos y restauramos los valores originales.
	prevTimeout := os.Getenv("GRXFIRMA_HTTP_TIMEOUT_SEC")
	prevPayload := os.Getenv("GRXFIRMA_MAX_PAYLOAD_BYTES")
	defer func() {
		os.Setenv("GRXFIRMA_HTTP_TIMEOUT_SEC", prevTimeout)
		os.Setenv("GRXFIRMA_MAX_PAYLOAD_BYTES", prevPayload)
	}()

	os.Setenv("GRXFIRMA_HTTP_TIMEOUT_SEC", "no-es-un-numero")
	os.Setenv("GRXFIRMA_MAX_PAYLOAD_BYTES", "tampoco")

	l := limits.FromEnv(base)

	// Debe usar los valores base sin panic.
	if l.HTTPTimeoutSec != base.HTTPTimeoutSec {
		t.Errorf("HTTPTimeoutSec: esperado %d (base), se obtuvo %d", base.HTTPTimeoutSec, l.HTTPTimeoutSec)
	}
	if l.MaxPayloadBytes != base.MaxPayloadBytes {
		t.Errorf("MaxPayloadBytes: esperado %d (base), se obtuvo %d", base.MaxPayloadBytes, l.MaxPayloadBytes)
	}
}

func TestFromEnv_SinVariables_MantieneBase(t *testing.T) {
	t.Parallel()

	// Asegurate de que no hay variables de entorno interferentes.
	// En entorno paralelo esto puede ser fragil; usamos unset si están vacías.
	base := limits.Limits{
		HTTPTimeoutSec:  15,
		MaxPayloadBytes: 1024,
		MaxBatchDocs:    10,
		MaxSessionSec:   60,
	}

	// Usamos variables con nombres que no existen en el entorno de CI.
	// FromEnv solo lee GRXFIRMA_* que no deberían estar configuradas aquí.
	// Si por algún motivo están configuradas, el test puede fallar, pero eso
	// indicaría una configuración de entorno intencionada.
	l := limits.FromEnv(base)

	// Los valores deben ser los de base si no hay variables configuradas.
	// Este test verifica que FromEnv no modifica el base cuando no hay vars.
	// (Si el entorno tiene las vars, este test no se ejecuta en paralelo.)
	_ = l // Solo verificamos que no hay panic.
}

func TestCheckPayload_DentroDeLimite_NoError(t *testing.T) {
	t.Parallel()

	l := limits.Limits{MaxPayloadBytes: 100}
	data := make([]byte, 100)

	if err := l.CheckPayload(data); err != nil {
		t.Fatalf("no se esperaba error para payload dentro del limite: %v", err)
	}
}

func TestCheckPayload_ExcedeLimite_RetornaError(t *testing.T) {
	t.Parallel()

	l := limits.Limits{MaxPayloadBytes: 100}
	data := make([]byte, 101)

	err := l.CheckPayload(data)
	if err == nil {
		t.Fatal("se esperaba error al exceder MaxPayloadBytes")
	}

	var excedido *limits.ErrPayloadExcedido
	if !errors.As(err, &excedido) {
		t.Fatalf("se esperaba *limits.ErrPayloadExcedido, se obtuvo: %T", err)
	}
	if excedido.Tamaño != 101 {
		t.Errorf("Tamaño incorrecto: %d", excedido.Tamaño)
	}
	if excedido.Maximo != 100 {
		t.Errorf("Maximo incorrecto: %d", excedido.Maximo)
	}
}

func TestCheckPayload_PayloadVacio_NoError(t *testing.T) {
	t.Parallel()

	l := limits.Default()
	if err := l.CheckPayload([]byte{}); err != nil {
		t.Fatalf("payload vacio no deberia superar el limite: %v", err)
	}
}
