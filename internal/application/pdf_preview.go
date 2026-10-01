// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"fmt"

	"grxfirma/internal/ports"
)

// PdfPreviewUseCase renderiza una pagina de un PDF como imagen PNG en base64.
// Es el caso de uso que sirve la previa de documentos PDF en la GUI Qt/QML.
type PdfPreviewUseCase struct {
	visualizador ports.VisualizadorPDF
}

// NuevoPdfPreviewUseCase construye el caso de uso con el adaptador de render.
func NuevoPdfPreviewUseCase(visualizador ports.VisualizadorPDF) *PdfPreviewUseCase {
	return &PdfPreviewUseCase{visualizador: visualizador}
}

// PdfPreviewCommand describe la pagina que se quiere renderizar.
type PdfPreviewCommand struct {
	// Ruta es la ruta absoluta al fichero PDF en el sistema de ficheros local.
	Ruta string

	// Pagina es el numero de pagina (1-based). Si es 0 o negativo se usa la 1.
	Pagina int
}

// PdfPreviewResult contiene la imagen renderizada y las dimensiones de la pagina.
type PdfPreviewResult struct {
	// DataB64 es la imagen PNG de la pagina codificada en base64 estandar.
	DataB64 string

	// Ancho y Alto son las dimensiones de la pagina en puntos PDF (72 dpi).
	Ancho float64
	Alto  float64

	// PaginaActual y TotalPaginas permiten a la GUI navegar por el documento
	// sin tener que renderizarlo completo de una sola vez.
	PaginaActual int
	TotalPaginas int
}

// Ejecutar renderiza la pagina solicitada del PDF indicado.
func (uc *PdfPreviewUseCase) Ejecutar(ctx context.Context, cmd PdfPreviewCommand) (PdfPreviewResult, error) {
	if uc == nil || uc.visualizador == nil {
		return PdfPreviewResult{}, fmt.Errorf("visor PDF no configurado")
	}
	pagina := cmd.Pagina
	if pagina < 1 {
		pagina = 1
	}
	b64, ancho, alto, totalPaginas, err := uc.visualizador.RenderizarPagina(ctx, cmd.Ruta, pagina)
	if err != nil {
		return PdfPreviewResult{}, fmt.Errorf("error al renderizar pagina %d de %s: %w", pagina, cmd.Ruta, err)
	}
	if totalPaginas < 1 {
		totalPaginas = 1
	}
	if pagina > totalPaginas {
		pagina = totalPaginas
	}
	return PdfPreviewResult{
		DataB64:      b64,
		Ancho:        ancho,
		Alto:         alto,
		PaginaActual: pagina,
		TotalPaginas: totalPaginas,
	}, nil
}
