// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package pdfpreview implementa ports.VisualizadorPDF mediante el subproceso
// pdftoppm de la suite Poppler. No usa CGO: invoca el binario externo.
//
// Requisito del sistema: poppler-utils (pdftoppm + pdfinfo) instalado.
// En Debian/Ubuntu: apt install poppler-utils
package pdfpreview

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"grxfirma/internal/adapters/outbound/common/components"
	"grxfirma/internal/adapters/outbound/common/securefile"
)

const (
	defaultPreviewDPI              = 150
	maxPreviewDPI                  = 300
	maxPreviewPixelDimension       = 4096
	maxPreviewInputBytes     int64 = 100 * 1024 * 1024
	maxPreviewPNGBytes       int64 = 50 * 1024 * 1024
	maxToolDiagnosticBytes         = 64 * 1024
	previewRenderTimeout           = 30 * time.Second
)

// RendererPdftoppm implementa ports.VisualizadorPDF via subprocess pdftoppm.
type RendererPdftoppm struct {
	// DPI es la resolucion de render. Por defecto 150.
	DPI int
}

// New construye el renderer con DPI 150.
func New() *RendererPdftoppm {
	return &RendererPdftoppm{DPI: defaultPreviewDPI}
}

// WithDPI establece la resolucion de render.
func (r *RendererPdftoppm) WithDPI(dpi int) *RendererPdftoppm {
	if r != nil && dpi > 0 {
		if dpi > maxPreviewDPI {
			dpi = maxPreviewDPI
		}
		r.DPI = dpi
	}
	return r
}

// RenderizarPagina implementa ports.VisualizadorPDF.
// Renderiza la pagina indicada del PDF en ruta y devuelve la imagen PNG en base64.
// Las dimensiones devueltas son las de la pagina en puntos PDF (72 ppi).
func (r *RendererPdftoppm) RenderizarPagina(ctx context.Context, ruta string, pagina int) (string, float64, float64, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	renderCtx, cancel := context.WithTimeout(ctx, previewRenderTimeout)
	defer cancel()

	rutaPDF, err := normalizarRutaPDF(ruta)
	if err != nil {
		return "", 0, 0, 0, err
	}
	if pagina < 1 {
		pagina = 1
	}

	dpi := r.DPI
	if dpi <= 0 {
		dpi = defaultPreviewDPI
	}
	if dpi > maxPreviewDPI {
		dpi = maxPreviewDPI
	}

	pdfData, err := securefile.ReadFileLimit(rutaPDF, maxPreviewInputBytes)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("leyendo PDF para previsualización: %w", err)
	}
	tmpDir, err := os.MkdirTemp("", "grxfirma-pdf-preview-")
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("creando directorio temporal: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	rutaPDF = filepath.Join(tmpDir, "input.pdf")
	if err := securefile.WriteFileAtomic(rutaPDF, pdfData, 0o600); err != nil {
		return "", 0, 0, 0, fmt.Errorf("preparando PDF temporal: %w", err)
	}

	pdfinfoPath, err := exec.LookPath("pdfinfo")
	if err != nil {
		return "", 0, 0, 0, components.ErrorFalta("pdfinfo")
	}
	totalPaginas, err := totalPaginasPDF(renderCtx, pdfinfoPath, rutaPDF)
	if err != nil {
		return "", 0, 0, 0, err
	}
	if pagina > totalPaginas {
		pagina = totalPaginas
	}
	ancho, alto, err := metadataPagina(renderCtx, pdfinfoPath, rutaPDF, pagina)
	if err != nil {
		return "", 0, 0, 0, err
	}

	prefix := filepath.Join(tmpDir, "pag")
	paginaStr := strconv.Itoa(pagina)
	pdftoppmPath, err := exec.LookPath("pdftoppm")
	if err != nil {
		return "", 0, 0, 0, components.ErrorFalta("pdftoppm")
	}

	// #nosec G204 -- pdftoppm is a fixed system tool and rutaPDF is an
	// absolute regular-file path, so it cannot be interpreted as an option.
	// -cropbox: se dibuja la CropBox, que es lo que muestran los lectores y la
	// caja que mide pdfinfo y sobre la que el motor coloca el sello.
	cmd := exec.CommandContext(renderCtx, pdftoppmPath,
		"-cropbox",
		"-r", strconv.Itoa(dpi),
		"-scale-to", strconv.Itoa(maxPreviewPixelDimension),
		"-f", paginaStr,
		"-l", paginaStr,
		"-singlefile",
		"-png",
		rutaPDF, prefix,
	)

	var toolOutput boundedCommandBuffer
	toolOutput.max = maxToolDiagnosticBytes
	cmd.Stdout = &toolOutput
	cmd.Stderr = &toolOutput
	if err := cmd.Run(); err != nil {
		if wrapped := components.EnvolverErrorEjecucion("pdftoppm", err); wrapped != err {
			return "", 0, 0, 0, wrapped
		}
		return "", 0, 0, 0, fmt.Errorf("pdftoppm: %w — %s", err, toolOutput.String())
	}

	pngPath := prefix + ".png"
	if _, err := os.Stat(pngPath); err != nil {
		return "", 0, 0, 0, fmt.Errorf("pdftoppm no genero salida PNG para la pagina %d", pagina)
	}

	data, err := securefile.ReadFileLimit(pngPath, maxPreviewPNGBytes)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("leyendo imagen PNG generada: %w", err)
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	return b64, ancho, alto, totalPaginas, nil
}

func totalPaginasPDF(ctx context.Context, pdfinfoPath, ruta string) (int, error) {
	// #nosec G204 -- pdfinfoPath is the resolved fixed tool and ruta is a
	// private regular-file copy created by RenderizarPagina.
	out, err := ejecutarHerramientaAcotada(ctx, pdfinfoPath, ruta)
	if err != nil {
		return 0, fmt.Errorf("pdfinfo no pudo leer el total de páginas: %w", err)
	}
	_, _, total, _ := parsearSalidaPdfinfo(string(out))
	if total < 1 {
		return 0, fmt.Errorf("pdfinfo no devolvió un total de páginas válido")
	}
	return total, nil
}

func metadataPagina(ctx context.Context, pdfinfoPath, ruta string, pagina int) (ancho, alto float64, err error) {
	// #nosec G204 -- pdfinfoPath is the resolved fixed tool and ruta is a
	// private regular-file copy created by RenderizarPagina.
	out, err := ejecutarHerramientaAcotada(
		ctx,
		pdfinfoPath,
		"-f", strconv.Itoa(pagina),
		"-l", strconv.Itoa(pagina),
		ruta,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("pdfinfo no pudo leer la página %d: %w", pagina, err)
	}
	ancho, alto, _, ok := parsearSalidaPdfinfo(string(out))
	if !ok {
		return 0, 0, fmt.Errorf("pdfinfo no devolvió dimensiones válidas para la página %d", pagina)
	}
	return ancho, alto, nil
}

func ejecutarHerramientaAcotada(ctx context.Context, path string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...) // #nosec G204 -- path se resuelve con exec.LookPath y los argumentos no pasan por shell.
	var stdout boundedCommandBuffer
	var stderr boundedCommandBuffer
	stdout.max = maxToolDiagnosticBytes
	stderr.max = maxToolDiagnosticBytes
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("%w: %s", err, stderr.String())
	}
	return append([]byte(nil), stdout.Bytes()...), nil
}

type boundedCommandBuffer struct {
	bytes.Buffer
	max int
}

func (b *boundedCommandBuffer) Write(p []byte) (int, error) {
	originalLen := len(p)
	if b.max <= 0 || b.Len() >= b.max {
		return originalLen, nil
	}
	remaining := b.max - b.Len()
	if len(p) > remaining {
		p = p[:remaining]
	}
	_, _ = b.Buffer.Write(p)
	return originalLen, nil
}

func normalizarRutaPDF(ruta string) (string, error) {
	if strings.TrimSpace(ruta) == "" {
		return "", fmt.Errorf("ruta del PDF no puede estar vacia")
	}
	absoluta, err := filepath.Abs(ruta)
	if err != nil {
		return "", fmt.Errorf("resolviendo ruta del PDF: %w", err)
	}
	file, err := securefile.OpenRead(absoluta)
	if err != nil {
		return "", fmt.Errorf("inspeccionando PDF: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("inspeccionando PDF: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("la ruta del PDF no es un fichero regular")
	}
	return absoluta, nil
}

// parsearSalidaPdfinfo extrae dimensiones de pagina y numero total de paginas
// de la salida de pdfinfo. Devuelve ok=false si no encuentra dimensiones, para
// que el llamante decida el valor por defecto.
//
// Hay que reconocer dos formatos, y ese fue el fallo original: sin acotar
// paginas, pdfinfo emite "Page size: 595.28 x 841.89 pts"; pero al invocarlo
// con -f/-l, como hace este renderer, emite "Page    1 size: 595.28 x 841.89
// pts". Solo se reconocia el primero, asi que la rama de dimensiones no se
// ejecutaba nunca y todo PDF se reportaba como A4. Las dimensiones viajan por
// IPC hasta el frontend, que las usa para situar el sello visible PAdES: en
// cualquier documento que no fuera A4 el sello caia en coordenadas erroneas.
//
// pdfinfo da el tamaño de la CropBox sin girar y el /Rotate aparte ("Page
// 1 rot: 90"). pdftoppm dibuja la página ya girada, así que con 90 o 270 se
// devuelven ancho y alto intercambiados: las interfaces miden el sello sobre
// la página tal como se ve, que es la convención del motor de firma.
func parsearSalidaPdfinfo(salida string) (ancho, alto float64, totalPaginas int, ok bool) {
	giro, giroLeido := 0, false
	defer func() {
		if ok && (giro == 90 || giro == 270) {
			ancho, alto = alto, ancho
		}
	}()
	for _, linea := range strings.Split(salida, "\n") {
		linea = strings.TrimSpace(linea)

		if !giroLeido && strings.HasPrefix(linea, "Page") {
			if idx := strings.Index(linea, "rot:"); idx >= 0 {
				if n, err := strconv.Atoi(strings.TrimSpace(linea[idx+len("rot:"):])); err == nil {
					giro, giroLeido = ((n%360)+360)%360, true
				}
				continue
			}
		}

		if strings.HasPrefix(linea, "Pages:") {
			raw := strings.TrimSpace(strings.TrimPrefix(linea, "Pages:"))
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				totalPaginas = n
			}
			continue
		}

		if ok || !strings.HasPrefix(linea, "Page") {
			continue
		}
		idx := strings.Index(linea, "size:")
		if idx < 0 {
			continue
		}
		// "595.28 x 841.89 pts (A4)" -> partes[0] x partes[2]
		partes := strings.Fields(strings.TrimSpace(linea[idx+len("size:"):]))
		if len(partes) < 3 || partes[1] != "x" {
			continue
		}
		a, errA := strconv.ParseFloat(partes[0], 64)
		b, errB := strconv.ParseFloat(partes[2], 64)
		if errA != nil || errB != nil || a <= 0 || b <= 0 {
			continue
		}
		// Se conserva la primera pagina encontrada, pero se sigue recorriendo
		// por si "Pages:" apareciera despues.
		ancho, alto, ok = a, b, true
	}
	return ancho, alto, totalPaginas, ok
}
