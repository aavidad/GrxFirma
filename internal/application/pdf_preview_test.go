// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/application"
)

// ---------------------------------------------------------------------------
// Doble de prueba del puerto VisualizadorPDF
// ---------------------------------------------------------------------------

type stubVisualizador struct {
	b64   string
	ancho float64
	alto  float64
	total int
	err   error
	// para verificar que se paso la pagina correcta
	paginaRecibida int
}

func (s *stubVisualizador) RenderizarPagina(_ context.Context, _ string, pagina int) (string, float64, float64, int, error) {
	s.paginaRecibida = pagina
	return s.b64, s.ancho, s.alto, s.total, s.err
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestPdfPreviewUseCase_Ejecutar_OK(t *testing.T) {
	t.Parallel()
	stub := &stubVisualizador{b64: "aGVsbG8=", ancho: 595.28, alto: 841.89, total: 12}
	uc := application.NuevoPdfPreviewUseCase(stub)

	res, err := uc.Ejecutar(context.Background(), application.PdfPreviewCommand{
		Ruta:   "/tmp/doc.pdf",
		Pagina: 2,
	})
	if err != nil {
		t.Fatalf("Ejecutar: %v", err)
	}
	if res.DataB64 != "aGVsbG8=" {
		t.Errorf("DataB64 = %q", res.DataB64)
	}
	if res.Ancho != 595.28 || res.Alto != 841.89 {
		t.Errorf("dimensiones: %v x %v", res.Ancho, res.Alto)
	}
	if res.PaginaActual != 2 || res.TotalPaginas != 12 {
		t.Errorf("paginacion: got actual=%d total=%d", res.PaginaActual, res.TotalPaginas)
	}
	if stub.paginaRecibida != 2 {
		t.Errorf("pagina enviada al visualizador: %d, queria 2", stub.paginaRecibida)
	}
}

func TestPdfPreviewUseCase_Pagina0_NormalizaA1(t *testing.T) {
	t.Parallel()
	stub := &stubVisualizador{b64: "x"}
	uc := application.NuevoPdfPreviewUseCase(stub)

	_, err := uc.Ejecutar(context.Background(), application.PdfPreviewCommand{
		Ruta:   "/tmp/doc.pdf",
		Pagina: 0,
	})
	if err != nil {
		t.Fatalf("Ejecutar: %v", err)
	}
	if stub.paginaRecibida != 1 {
		t.Errorf("pagina 0 debia normalizarse a 1, got %d", stub.paginaRecibida)
	}
}

func TestPdfPreviewUseCase_PaginaNegativa_NormalizaA1(t *testing.T) {
	t.Parallel()
	stub := &stubVisualizador{b64: "x"}
	uc := application.NuevoPdfPreviewUseCase(stub)

	_, err := uc.Ejecutar(context.Background(), application.PdfPreviewCommand{
		Ruta:   "/tmp/doc.pdf",
		Pagina: -5,
	})
	if err != nil {
		t.Fatalf("Ejecutar: %v", err)
	}
	if stub.paginaRecibida != 1 {
		t.Errorf("pagina negativa debia normalizarse a 1, got %d", stub.paginaRecibida)
	}
}

func TestPdfPreviewUseCase_ErrorVisualizador(t *testing.T) {
	t.Parallel()
	stub := &stubVisualizador{err: errors.New("pdftoppm no disponible")}
	uc := application.NuevoPdfPreviewUseCase(stub)

	_, err := uc.Ejecutar(context.Background(), application.PdfPreviewCommand{
		Ruta:   "/tmp/doc.pdf",
		Pagina: 1,
	})
	if err == nil {
		t.Fatal("esperaba error, got nil")
	}
}

func TestPdfPreviewUseCase_VisualizadorNil(t *testing.T) {
	t.Parallel()
	uc := application.NuevoPdfPreviewUseCase(nil)

	_, err := uc.Ejecutar(context.Background(), application.PdfPreviewCommand{
		Ruta:   "/tmp/doc.pdf",
		Pagina: 1,
	})
	if err == nil {
		t.Fatal("esperaba error con visualizador nil")
	}
}
