// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	pdf "github.com/digitorus/pdf"
)

// Utilidades para fabricar actualizaciones incrementales como las haría un
// editor real: objetos, tabla xref con sus desplazamientos y trailer con /Prev.

type objetoPDFPrueba struct {
	numero  int
	cuerpo  string // diccionario o flujo, sin cabecera ni endobj
	prefijo string // bytes que preceden a la cabecera "N 0 obj"
}

var (
	reCabeceraObjetoPrueba = regexp.MustCompile(`(\d+) 0 obj\b`)
	reStartXRefPrueba      = regexp.MustCompile(`startxref\s+(\d+)\s+%%EOF`)
	reTrailerRootPrueba    = regexp.MustCompile(`/Root\s+(\d+)\s+0\s+R`)
	reTrailerInfoPrueba    = regexp.MustCompile(`/Info\s+\d+\s+\d+\s+R`)
	reTrailerIDPrueba      = regexp.MustCompile(`(?s)/ID\s*\[[^\]]*\]`)
	reTrailerSizePrueba    = regexp.MustCompile(`/Size\s+(\d+)`)
)

// ultimoTrailerPrueba devuelve el último diccionario trailer y su startxref.
func ultimoTrailerPrueba(t *testing.T, pdf []byte) (string, int) {
	t.Helper()
	i := bytes.LastIndex(pdf, []byte("trailer"))
	if i < 0 {
		t.Fatal("PDF sin trailer clásico")
	}
	sx := reStartXRefPrueba.FindAllSubmatch(pdf, -1)
	if len(sx) == 0 {
		t.Fatal("PDF sin startxref")
	}
	prev, _ := strconv.Atoi(string(sx[len(sx)-1][1]))
	fin := bytes.Index(pdf[i:], []byte("startxref"))
	return string(pdf[i : i+fin]), prev
}

// objetosPDFPrueba indexa la última definición textual de cada objeto.
func objetosPDFPrueba(pdf []byte) map[int]string {
	out := make(map[int]string)
	for _, loc := range reCabeceraObjetoPrueba.FindAllSubmatchIndex(pdf, -1) {
		n, _ := strconv.Atoi(string(pdf[loc[2]:loc[3]]))
		fin := bytes.Index(pdf[loc[1]:], []byte("endobj"))
		if fin < 0 {
			continue
		}
		out[n] = strings.TrimSpace(string(pdf[loc[1] : loc[1]+fin]))
	}
	return out
}

// paginaPDFPrueba localiza la (única) página del PDF de prueba.
func paginaPDFPrueba(t *testing.T, pdf []byte) (int, string) {
	t.Helper()
	objetos := objetosPDFPrueba(pdf)
	numeros := make([]int, 0, len(objetos))
	for n := range objetos {
		numeros = append(numeros, n)
	}
	sort.Ints(numeros)
	re := regexp.MustCompile(`/Type\s*/Page\b`)
	for _, n := range numeros {
		if re.MatchString(objetos[n]) && !strings.Contains(objetos[n], "stream") {
			return n, objetos[n]
		}
	}
	t.Fatal("no se localizó la página")
	return 0, ""
}

// catalogoPDFPrueba devuelve el número y el cuerpo del catálogo vigente.
func catalogoPDFPrueba(t *testing.T, pdf []byte) (int, string) {
	t.Helper()
	trailer, _ := ultimoTrailerPrueba(t, pdf)
	m := reTrailerRootPrueba.FindStringSubmatch(trailer)
	if m == nil {
		t.Fatal("trailer sin /Root")
	}
	n, _ := strconv.Atoi(m[1])
	return n, objetosPDFPrueba(pdf)[n]
}

// anexarActualizacionPrueba añade una revisión incremental completa: los
// objetos, una tabla xref con desplazamientos reales y un trailer que
// conserva /Root, /Info e /ID y enlaza con /Prev. Con desplazamientos
// explícitos (extra) se pueden reasignar objetos sin definir ninguno nuevo.
func anexarActualizacionPrueba(t *testing.T, pdf []byte, objetos []objetoPDFPrueba, extra map[int]int) []byte {
	t.Helper()
	trailer, prev := ultimoTrailerPrueba(t, pdf)
	// Línea en blanco tras %%EOF, como escriben los editores: el primer
	// objeto nuevo empieza claramente fuera de la revisión firmada.
	out := append(append([]byte(nil), pdf...), '\n')
	offsets := make(map[int]int, len(objetos)+len(extra))
	for n, off := range extra {
		offsets[n] = off
	}
	for _, obj := range objetos {
		out = append(out, obj.prefijo...)
		offsets[obj.numero] = len(out)
		out = append(out, fmt.Sprintf("%d 0 obj\n%s\nendobj\n", obj.numero, obj.cuerpo)...)
	}
	size := 0
	if m := reTrailerSizePrueba.FindStringSubmatch(trailer); m != nil {
		size, _ = strconv.Atoi(m[1])
	}
	numeros := make([]int, 0, len(offsets))
	for n := range offsets {
		numeros = append(numeros, n)
		if n+1 > size {
			size = n + 1
		}
	}
	sort.Ints(numeros)
	xref := len(out)
	var b strings.Builder
	b.WriteString("xref\n")
	for _, n := range numeros {
		fmt.Fprintf(&b, "%d 1\n%010d 00000 n\r\n", n, offsets[n])
	}
	root := reTrailerRootPrueba.FindString(trailer)
	fmt.Fprintf(&b, "trailer\n<</Size %d %s /Prev %d %s %s>>\nstartxref\n%d\n%%%%EOF\n",
		size, root, prev, reTrailerInfoPrueba.FindString(trailer), reTrailerIDPrueba.FindString(trailer), xref)
	return append(out, b.String()...)
}

// contenidoVisiblePrueba lee con el lector PDF (siguiendo xref y /Prev) el
// flujo de contenido de la primera página: lo que pintaría un visor.
func contenidoVisiblePrueba(t *testing.T, data []byte) string {
	t.Helper()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("el lector PDF no abre el documento: %v", err)
	}
	rc := r.Page(1).V.Key("Contents").Reader()
	defer rc.Close()
	contenido, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("no se pudo leer el contenido de la página: %v", err)
	}
	return string(contenido)
}

// anotacionVisiblePrueba devuelve /Contents de la primera anotación de la
// primera página, resuelta por el lector PDF.
func anotacionVisiblePrueba(t *testing.T, data []byte) string {
	t.Helper()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("el lector PDF no abre el documento: %v", err)
	}
	return r.Page(1).V.Key("Annots").Index(0).Key("Contents").Text()
}
