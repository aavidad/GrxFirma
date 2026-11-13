// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

// Pruebas de regresión de E2-03 en la firma PAdES: un PDF malicioso debe
// acabar en error, sin colgar ni tumbar el proceso. Los casos que antes no
// terminaban se ejecutan con plazo.

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
)

// pdfLimites escribe un PDF con xref clásica; objetos[i] es el cuerpo del
// objeto i+1 (el 1 es el catálogo). desvio[n] = m hace que la xref del
// objeto n apunte al objeto m, un PDF roto que el lector rechaza con pánico.
func pdfLimites(desvio map[int]int, objetos ...string) []byte {
	var file bytes.Buffer
	file.WriteString("%PDF-1.4\n%" + strings.Repeat("x", 230) + "\n")
	offsets := make([]int, len(objetos))
	for i, cuerpo := range objetos {
		offsets[i] = file.Len()
		fmt.Fprintf(&file, "%d 0 obj\n%s\nendobj\n", i+1, cuerpo)
	}
	xref := file.Len()
	fmt.Fprintf(&file, "xref\n0 %d\n0000000000 65535 f \n", len(objetos)+1)
	for i, off := range offsets {
		if destino, ok := desvio[i+1]; ok {
			off = offsets[destino-1]
		}
		fmt.Fprintf(&file, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&file, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objetos)+1, xref)
	return file.Bytes()
}

// pdfConCount devuelve un PDF de una página real cuyo /Count declara total
// páginas, con relleno de objetos para que total no supere el número de
// objetos cuando se pide.
func pdfConCount(total, relleno int) []byte {
	objetos := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		fmt.Sprintf("<< /Type /Pages /Kids [3 0 R] /Count %d >>", total),
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] >>",
	}
	for i := 0; i < relleno; i++ {
		objetos = append(objetos, "<< >>")
	}
	return pdfLimites(nil, objetos...)
}

func firmarConPlazo(t *testing.T, pdfData []byte, opciones map[string]string) error {
	t.Helper()
	priv, cert := generarCertRSA(t)
	doc, _ := domain.NewDocument("malicioso.pdf", pdfData, "application/pdf")
	hecho := make(chan error, 1)
	go func() {
		_, err := signer.NuevoMotorFirmaGo(nil).Sign(context.Background(), domain.SignatureJob{
			Document: doc, Format: domain.FormatPAdES, Action: domain.ActionSign, Options: opciones,
		}, signer.NuevaClaveLocal(priv, cert))
		hecho <- err
	}()
	select {
	case err := <-hecho:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("la firma no terminó en 10 s: bucle sin límite")
		return nil
	}
}

func opcionesCSV() map[string]string {
	return map[string]string{"csv": "ABCD-1234", "csvUrl": "https://sede.example.es/cotejo?csv={csv}", "csvQR": "false"}
}

func opcionesImagen(img []byte, pagina string) map[string]string {
	return map[string]string{
		"image":                          base64.StdEncoding.EncodeToString(img),
		"imagePage":                      pagina,
		"imagePositionOnPageLowerLeftX":  "10",
		"imagePositionOnPageLowerLeftY":  "10",
		"imagePositionOnPageUpperRightX": "60",
		"imagePositionOnPageUpperRightY": "60",
	}
}

func pngPequeno(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestPAdESLimites_LeyendaCSVConCountEnorme(t *testing.T) {
	// /Count de 2^31-1: antes se reservaban 128 GiB de golpe.
	if err := firmarConPlazo(t, pdfConCount(2147483647, 0), opcionesCSV()); err == nil {
		t.Fatal("se firmó un PDF con /Count imposible")
	}
}

func TestPAdESLimites_LeyendaCSVConMasPaginasQueSellos(t *testing.T) {
	// 33 páginas declaradas y objetos de sobra: se rechaza antes de reservar
	// un sello por página, igual que haría el motor al pasar de 32.
	if err := firmarConPlazo(t, pdfConCount(33, 40), opcionesCSV()); err == nil {
		t.Fatal("se aceptó una leyenda CSV para más páginas que sellos")
	}
}

func TestPAdESLimites_ImagenEnTodasLasPaginasConCountEnorme(t *testing.T) {
	if err := firmarConPlazo(t, pdfConCount(2147483647, 0), opcionesImagen(pngPequeno(t), "0")); err == nil {
		t.Fatal("se firmó un PDF con /Count imposible")
	}
}

func TestPAdESLimites_ParentEnCicloSinMediaBox(t *testing.T) {
	data := pdfLimites(nil,
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 3 0 R >>",
	)
	// Sin MediaBox se usa A4; lo que importa es que termine.
	_ = firmarConPlazo(t, data, opcionesCSV())
}

func TestPAdESLimites_PanicoDelLectorEsError(t *testing.T) {
	// La xref del objeto 3 apunta al objeto 2: el lector aborta con pánico al
	// pedir la página, ya fuera de abrirPDF.
	data := pdfLimites(map[int]int{3: 2},
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] >>",
	)
	if err := firmarConPlazo(t, data, opcionesCSV()); err == nil {
		t.Fatal("se firmó un PDF que el lector no puede leer")
	}
}
