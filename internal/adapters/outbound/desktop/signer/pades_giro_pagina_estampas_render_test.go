// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/testsupport/exttools"
)

// Casos de página girada comunes a la leyenda CSV, la imagen de AutoFirma
// Java y el campo de firma existente: A4 con cada /Rotate y una CropBox de
// origen distinto de cero.
func casosEstampaGirada() []casoPaginaGirada {
	a4 := [4]float64{0, 0, 595, 842}
	crop := [4]float64{120, 80, 680, 860}
	return []casoPaginaGirada{
		{"rotate0", "", signer.PaginaPrueba{MediaBox: a4}},
		{"rotate90", "", signer.PaginaPrueba{Rotate: "90", MediaBox: a4}},
		{"rotate180", "", signer.PaginaPrueba{Rotate: "180", MediaBox: a4}},
		{"rotate270", "", signer.PaginaPrueba{Rotate: "270", MediaBox: a4}},
		{"rotate90-heredado", "90", signer.PaginaPrueba{MediaBox: a4}},
		{"rotate270-cropbox", "", signer.PaginaPrueba{Rotate: "270", MediaBox: [4]float64{100, 50, 700, 900}, CropBox: &crop}},
	}
}

// La leyenda CSV va en el margen inferior de la página tal como se ve y se
// lee en horizontal, con el QR a la izquierda.
func TestMotorFirmaGo_PAdESLeyendaCSVEnPaginaGirada(t *testing.T) {
	for _, c := range casosEstampaGirada() {
		t.Run(c.nombre, func(t *testing.T) {
			firmado := firmarFormulario(t, signer.PDFPaginasPrueba(c.rotatePages, c.pagina), map[string]string{
				"csv": "ABCD-1234-EFGH", "csvUrl": "https://sede.example.es/cotejo?csv={csv}",
			})
			comprobarPDFFirmado(t, firmado)
			if !exttools.Available(t, "pdftoppm") {
				return
			}
			_, _, ancho, alto, _ := c.cajaVisible()
			pagina := renderizarPaginaVista(t, firmado, 1, 144)
			guardarRevisionRotate(t, "csv-"+c.nombre, firmado, pagina)
			const escala = 2.0
			// Mitad izquierda de la franja inferior de 41 pt, la de la
			// leyenda (de 6 a 40 pt). Con /Rotate 270 el texto de prueba de
			// la página queda justo encima.
			zona := image.Rect(0, int((alto-41)*escala), int(ancho/2*escala), int(alto*escala))
			caja := cajaNoBlanca(pagina, zona)
			if caja.Empty() {
				t.Fatal("no aparece la leyenda CSV en el margen inferior de la página vista")
			}
			izquierda, derecha := float64(caja.Min.X)/escala, float64(caja.Max.X)/escala
			abajo := alto - float64(caja.Max.Y)/escala
			if izquierda < 16 || izquierda > 32 || derecha < 200 || abajo < 4 || abajo > 12 {
				t.Fatalf("la leyenda ocupa x de %.1f a %.1f pt y empieza a %.1f pt del borde inferior de la página vista; se esperaba una franja desde x≈20 y a unos 6 pt del borde", izquierda, derecha, abajo)
			}
			angulo, alargamiento, _, _ := orientacionTinta(pagina, zona)
			if alargamiento < 2 || math.Abs(math.Remainder(angulo, 180)) > 8 {
				t.Fatalf("la leyenda no se lee en horizontal: eje a %.1f°, alargamiento %.2f", angulo, alargamiento)
			}
		})
	}
}

// pngDosColores es una imagen apaisada con la mitad izquierda roja y la
// derecha azul, para ver dónde cae y hacia dónde mira.
func pngDosColores(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 80, 40))
	for x := 0; x < 80; x++ {
		c := color.RGBA{220, 0, 0, 255}
		if x >= 40 {
			c = color.RGBA{0, 0, 220, 255}
		}
		for y := 0; y < 40; y++ {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// cajaColor devuelve la caja de los píxeles que cumplen el predicado.
func cajaColor(img image.Image, es func(r, g, b uint32) bool) image.Rectangle {
	bnd := img.Bounds()
	minX, minY, maxX, maxY := bnd.Max.X, bnd.Max.Y, bnd.Min.X-1, bnd.Min.Y-1
	for py := bnd.Min.Y; py < bnd.Max.Y; py++ {
		for px := bnd.Min.X; px < bnd.Max.X; px++ {
			r, g, b, _ := img.At(px, py).RGBA()
			if !es(r>>8, g>>8, b>>8) {
				continue
			}
			minX, minY = min(minX, px), min(minY, py)
			maxX, maxY = max(maxX, px), max(maxY, py)
		}
	}
	if maxX < minX {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX+1, maxY+1)
}

// La imagen de AutoFirma Java (image, imagePositionOnPage*) se coloca en
// coordenadas de la página tal como se ve, con el mismo origen que el sello,
// y se ve derecha: rojo a la izquierda y azul a la derecha.
func TestMotorFirmaGo_PAdESImagenJavaEnPaginaGirada(t *testing.T) {
	img := base64.StdEncoding.EncodeToString(pngDosColores(t))
	const u, v, w, h = 100.0, 150.0, 160.0, 80.0
	for _, c := range casosEstampaGirada() {
		t.Run(c.nombre, func(t *testing.T) {
			x0, y0, _, alto, _ := c.cajaVisible()
			f := func(n float64) string { return fmt.Sprint(n) }
			firmado := firmarFormulario(t, signer.PDFPaginasPrueba(c.rotatePages, c.pagina), map[string]string{
				"image": img, "imagePage": "1",
				"imagePositionOnPageLowerLeftX": f(x0 + u), "imagePositionOnPageLowerLeftY": f(y0 + v),
				"imagePositionOnPageUpperRightX": f(x0 + u + w), "imagePositionOnPageUpperRightY": f(y0 + v + h),
			})
			comprobarPDFFirmado(t, firmado)
			if !exttools.Available(t, "pdftoppm") {
				return
			}
			pagina := renderizarPaginaVista(t, firmado, 1, 144)
			guardarRevisionRotate(t, "imagen-"+c.nombre, firmado, pagina)
			const escala = 2.0
			rojo := cajaColor(pagina, func(r, g, b uint32) bool { return r > 180 && g < 80 && b < 80 })
			azul := cajaColor(pagina, func(r, g, b uint32) bool { return b > 180 && r < 80 && g < 80 })
			if rojo.Empty() || azul.Empty() {
				t.Fatal("no aparece la imagen estampada en la página vista")
			}
			todo := rojo.Union(azul)
			want := [4]float64{u * escala, (alto - v - h) * escala, (u + w) * escala, (alto - v) * escala}
			got := [4]float64{float64(todo.Min.X), float64(todo.Min.Y), float64(todo.Max.X), float64(todo.Max.Y)}
			for i := range want {
				if math.Abs(got[i]-want[i]) > 4 {
					t.Fatalf("la imagen ocupa %v px en la página vista; se esperaba %v", todo, want)
				}
			}
			if rojo.Max.X > azul.Min.X+2 || math.Abs(float64(rojo.Min.Y-azul.Min.Y)) > 2 {
				t.Fatalf("la imagen no se ve derecha: rojo %v, azul %v", rojo, azul)
			}
		})
	}
}

// pdfCampoFirmaGirado genera una página con /Rotate y un campo de firma
// vacío cuyo /Rect, en el espacio de usuario sin girar, se ve como la caja
// (u, v, w, h) de la página vista.
func pdfCampoFirmaGirado(rotate int, u, v, w, h float64) []byte {
	const ancho, alto = 595.0, 842.0
	var rect [4]float64
	switch rotate {
	case 90:
		rect = [4]float64{ancho - v - h, u, ancho - v, u + w}
	case 180:
		rect = [4]float64{ancho - u - w, alto - v - h, ancho - u, alto - v}
	case 270:
		rect = [4]float64{v, alto - u - w, v + h, alto - u}
	default:
		rect = [4]float64{u, v, u + w, v + h}
	}
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] >> >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Rotate %d /Resources << >> /Annots [4 0 R] /Contents 5 0 R >>", rotate),
		fmt.Sprintf("<< /Type /Annot /Subtype /Widget /FT /Sig /T (FirmaSolicitante) /Rect [%g %g %g %g] /F 4 >>", rect[0], rect[1], rect[2], rect[3]),
		"<< /Length 0 >>\nstream\n\nendstream",
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
	return b.Bytes()
}

// El sello que se pone en un campo de firma existente (signatureField) se
// lee derecho en la página vista aunque la página tenga /Rotate: el widget
// conserva su /Rect y la imagen descuenta el giro.
func TestMotorFirmaGo_PAdESCampoFirmaEnPaginaGirada(t *testing.T) {
	// La caja vista tiene la proporción de la imagen del campo (250 x 64).
	const u, v, w, h = 150.0, 300.0, 250.0, 64.0
	for _, rotate := range []int{0, 90, 180, 270} {
		t.Run(fmt.Sprintf("rotate%d", rotate), func(t *testing.T) {
			firmado := firmarFormulario(t, pdfCampoFirmaGirado(rotate, u, v, w, h), map[string]string{
				"signatureField": "FirmaSolicitante",
			})
			comprobarPDFFirmado(t, firmado)
			if !exttools.Available(t, "pdftoppm") {
				return
			}
			alto := 842.0
			if rotate == 90 || rotate == 270 {
				alto = 595
			}
			pagina := renderizarPaginaVista(t, firmado, 1, 144)
			guardarRevisionRotate(t, fmt.Sprintf("campo-rotate%d", rotate), firmado, pagina)
			comprobarSelloEnCaja(t, pagina, alto, u, v, w, h, 0)
		})
	}
}

// La ruta REST de la interfaz QML (backendBuildSignOptions) envía las
// fracciones de la vista previa multiplicadas por el tamaño de la página
// vista y marca visibleSealRectRelativeToCrop, como la ruta IPC. Con una
// CropBox de origen distinto de cero el sello debe caer donde se pidió.
func TestMotorFirmaGo_PAdESSelloRutaRESTQMLConCropBox(t *testing.T) {
	crop := [4]float64{120, 80, 680, 860}
	media := [4]float64{100, 50, 700, 900}
	for _, c := range []casoPaginaGirada{
		{"cropbox", "", signer.PaginaPrueba{MediaBox: media, CropBox: &crop}},
		{"cropbox-rotate90", "", signer.PaginaPrueba{Rotate: "90", MediaBox: media, CropBox: &crop}},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			_, _, ancho, alto, _ := c.cajaVisible()
			f := func(n float64) string { return fmt.Sprintf("%.2f", n) }
			// Mismas claves y formato que backendBuildSignOptions.
			firmado := firmarFormulario(t, signer.PDFPaginasPrueba(c.rotatePages, c.pagina), map[string]string{
				"visibleSeal": "true", "page": "1", "visibleSealRectRelativeToCrop": "true",
				"visibleSealRectX": f(fx * ancho), "visibleSealRectY": f(fy * alto),
				"visibleSealRectW": f(fw * ancho), "visibleSealRectH": f(fh * alto),
				"rotation": "0", "visibleSealKeepText": "true",
			})
			comprobarPDFFirmado(t, firmado)
			if !exttools.Available(t, "pdftoppm") {
				return
			}
			pagina := renderizarPaginaVista(t, firmado, 1, 144)
			guardarRevisionRotate(t, "rest-qml-"+c.nombre, firmado, pagina)
			comprobarSelloVisto(t, pagina, ancho, alto, 0)
		})
	}
}
