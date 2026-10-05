// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pdfpreview

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Las dimensiones que sale de aqui viajan por IPC hasta el frontend, que las
// usa para situar el sello visible PAdES. Si el parseo falla en silencio y cae
// al A4 por defecto, el sello se coloca en coordenadas erroneas en cualquier
// documento que no sea A4.
func TestParsearSalidaPdfinfo(t *testing.T) {
	t.Parallel()

	// Formato real de poppler 26.01 al invocar con -f/-l, que es como lo llama
	// este renderer. Fue el caso que no se reconocia.
	const conPaginaAcotada = `Title:           Prueba
Pages:           3
Encrypted:       no
Page    1 size:  595.304 x 841.89 pts (A4)
Page    1 rot:   0
File size:       12345 bytes`

	// Formato de pdfinfo sin acotar paginas.
	const sinAcotar = `Title:           Prueba
Pages:           3
Page size:       841.89 x 595.304 pts (A4, landscape)
File size:       12345 bytes`

	casos := []struct {
		nombre       string
		salida       string
		ancho, alto  float64
		totalPaginas int
		ok           bool
	}{
		{"con -f/-l (formato del renderer)", conPaginaAcotada, 595.304, 841.89, 3, true},
		{"sin acotar paginas", sinAcotar, 841.89, 595.304, 3, true},
		{"solo total de paginas", "Pages:           7", 0, 0, 7, false},
		{"salida vacia", "", 0, 0, 0, false},
		{"sin dimensiones parseables", "Page    1 size:  ancho x alto pts", 0, 0, 0, false},
		{"dimensiones no positivas", "Page    1 size:  0 x 841.89 pts", 0, 0, 0, false},
		{"separador inesperado", "Page    1 size:  595.3 por 841.89 pts", 0, 0, 0, false},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			t.Parallel()
			ancho, alto, total, ok := parsearSalidaPdfinfo(caso.salida)
			if ok != caso.ok {
				t.Fatalf("ok = %v, se esperaba %v", ok, caso.ok)
			}
			if ancho != caso.ancho || alto != caso.alto {
				t.Errorf("dimensiones = %v x %v, se esperaba %v x %v", ancho, alto, caso.ancho, caso.alto)
			}
			if total != caso.totalPaginas {
				t.Errorf("totalPaginas = %d, se esperaba %d", total, caso.totalPaginas)
			}
		})
	}
}

// Un PDF apaisado no debe reportarse como A4 vertical.
func TestParsearSalidaPdfinfo_RespetaLaOrientacion(t *testing.T) {
	t.Parallel()

	ancho, alto, _, ok := parsearSalidaPdfinfo("Page    1 size:  841.89 x 595.304 pts (A4, landscape)")
	if !ok {
		t.Fatal("se esperaba parseo correcto")
	}
	if ancho <= alto {
		t.Fatalf("un apaisado debe tener ancho > alto, y fue %v x %v", ancho, alto)
	}
}

// pdftoppm dibuja la página ya girada; las dimensiones deben ser las de la
// página tal como se ve, que es como el motor interpreta el sello.
func TestParsearSalidaPdfinfo_AplicaRotate(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		rot         string
		ancho, alto float64
	}{{"0", 490, 780}, {"90", 780, 490}, {"180", 490, 780}, {"270", 780, 490}} {
		salida := "Pages:           1\nPage    1 size:  490 x 780 pts\nPage    1 rot:   " + c.rot + "\n"
		ancho, alto, _, ok := parsearSalidaPdfinfo(salida)
		if !ok || ancho != c.ancho || alto != c.alto {
			t.Errorf("rot %s: %v x %v (ok=%v), se esperaba %v x %v", c.rot, ancho, alto, ok, c.ancho, c.alto)
		}
	}
}

// Con /Rotate y una CropBox distinta de la MediaBox, la imagen y las
// dimensiones deben describir la misma caja: la CropBox girada.
func TestRenderizarPagina_RotateYCropBoxCoinciden(t *testing.T) {
	for _, tool := range []string{"pdftoppm", "pdfinfo"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s no disponible", tool)
		}
	}
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 /Rotate 90 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /CropBox [10 20 500 800] >>",
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	ruta := filepath.Join(t.TempDir(), "girado.pdf")
	if err := os.WriteFile(ruta, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	b64, width, height, _, err := New().WithDPI(72).RenderizarPagina(context.Background(), ruta, 1)
	if err != nil {
		t.Fatal(err)
	}
	if width != 780 || height != 490 {
		t.Fatalf("dimensiones %v x %v, se esperaba la CropBox girada 780 x 490", width, height)
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	// -scale-to puede cambiar la escala; la proporción debe ser la misma.
	bw, bh := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	if d := bw/bh - width/height; d > 0.01 || d < -0.01 {
		t.Fatalf("la imagen mide %vx%v y no tiene la proporción de %vx%v", bw, bh, width, height)
	}
}

func TestWithDPI(t *testing.T) {
	t.Parallel()

	if got := New().DPI; got != 150 {
		t.Errorf("DPI por defecto = %d, se esperaba 150", got)
	}
	if got := New().WithDPI(300).DPI; got != 300 {
		t.Errorf("WithDPI(300) = %d", got)
	}
	// Un DPI no positivo no debe dejar el renderer inservible.
	if got := New().WithDPI(0).DPI; got != 150 {
		t.Errorf("WithDPI(0) debe ignorarse y dejar 150, y dejo %d", got)
	}
	if got := New().WithDPI(-10).DPI; got != 150 {
		t.Errorf("WithDPI(-10) debe ignorarse y dejar 150, y dejo %d", got)
	}
	if got := New().WithDPI(1200).DPI; got != maxPreviewDPI {
		t.Errorf("WithDPI debe acotar resoluciones excesivas a %d, y dejo %d", maxPreviewDPI, got)
	}
	var nilRenderer *RendererPdftoppm
	if nilRenderer.WithDPI(300) != nil {
		t.Error("WithDPI sobre un renderer nil debe devolver nil sin panic")
	}
}

func TestRenderizarPagina_RealAcotaSalidaYConservaGeometria(t *testing.T) {
	for _, tool := range []string{"pdftoppm", "pdfinfo"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s no disponible", tool)
		}
	}
	fixture, err := filepath.Abs("../../../../../test/regression/fixtures/v1/samples/multiple_pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	b64, width, height, total, err := New().WithDPI(36).RenderizarPagina(context.Background(), fixture, 2)
	if err != nil {
		t.Fatalf("RenderizarPagina() error = %v", err)
	}
	png, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("PNG base64 invalido: %v", err)
	}
	if !strings.HasPrefix(string(png), "\x89PNG\r\n\x1a\n") {
		t.Fatalf("cabecera PNG inesperada: %x", png[:min(len(png), 8)])
	}
	if width <= 0 || height <= 0 || total != 6 {
		t.Fatalf("geometria/paginacion inesperada: %.2fx%.2f total=%d", width, height, total)
	}
}

func TestRenderizarPagina_RechazaPDFInvalidoSinAsumirA4(t *testing.T) {
	for _, tool := range []string{"pdftoppm", "pdfinfo"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s no disponible", tool)
		}
	}
	path := filepath.Join(t.TempDir(), "invalido.pdf")
	if err := os.WriteFile(path, []byte("%PDF-no-es-un-pdf"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := New().RenderizarPagina(context.Background(), path, 1); err == nil {
		t.Fatal("un PDF invalido no debe renderizarse con dimensiones A4 de fallback")
	}
}

func TestBoundedCommandBuffer_DescartaExcesoSinShortWrite(t *testing.T) {
	var buffer boundedCommandBuffer
	buffer.max = 4
	n, err := buffer.Write([]byte("abcdefgh"))
	if err != nil || n != 8 {
		t.Fatalf("Write() = (%d, %v), want (8, nil)", n, err)
	}
	if got := buffer.String(); got != "abcd" {
		t.Fatalf("contenido acotado = %q", got)
	}
}

func TestNormalizarRutaPDF_DevuelveFicheroRegularAbsoluto(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "-documento.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.7"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	got, err := normalizarRutaPDF("-documento.pdf")
	if err != nil {
		t.Fatalf("normalizarRutaPDF() error = %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("ruta normalizada no absoluta: %q", got)
	}
	if filepath.Base(got) != "-documento.pdf" {
		t.Fatalf("ruta normalizada = %q", got)
	}
}

func TestNormalizarRutaPDF_RechazaDirectorios(t *testing.T) {
	if _, err := normalizarRutaPDF(t.TempDir()); err == nil {
		t.Fatal("normalizarRutaPDF() debe rechazar directorios")
	}
}

func TestNormalizarRutaPDF_ConservaEspaciosDelNombre(t *testing.T) {
	const nombre = " documento con espacios .pdf"
	path := filepath.Join(t.TempDir(), nombre)
	if err := os.WriteFile(path, []byte("%PDF-1.7"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := normalizarRutaPDF(path)
	if err != nil {
		t.Fatalf("normalizarRutaPDF() error = %v", err)
	}
	if filepath.Base(got) != nombre {
		t.Fatalf("normalizarRutaPDF() = %q", got)
	}
}

func TestNormalizarRutaPDF_RechazaSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.pdf")
	link := filepath.Join(dir, "link.pdf")
	if err := os.WriteFile(target, []byte("%PDF-1.7"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := normalizarRutaPDF(link); err == nil {
		t.Fatal("normalizarRutaPDF accepted a final-component symlink")
	}
}
