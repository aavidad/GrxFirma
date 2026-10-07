// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"testing"

	pdfsign "github.com/digitorus/pdfsign/sign"
)

// PaginaPrueba describe una página sintética: /Rotate tal cual se escribe
// ("" si no lo lleva), MediaBox y CropBox opcional.
type PaginaPrueba struct {
	Rotate   string
	MediaBox [4]float64
	CropBox  *[4]float64
}

// PDFPaginasPrueba genera un PDF sintético con las páginas indicadas. Si
// rotatePages no está vacío, el nodo Pages lleva ese /Rotate (heredable).
// Cada página escribe «Solicitud» cerca de su esquina superior izquierda sin
// girar, para ver en las capturas hacia dónde gira la página.
func PDFPaginasPrueba(rotatePages string, paginas ...PaginaPrueba) []byte {
	n := len(paginas)
	fuente := 3 + 2*n
	objs := []string{"<< /Type /Catalog /Pages 2 0 R >>"}
	kids := ""
	for i := range paginas {
		kids += fmt.Sprintf(" %d 0 R", 3+2*i)
	}
	pages := fmt.Sprintf("<< /Type /Pages /Kids [%s ] /Count %d", kids, n)
	if rotatePages != "" {
		pages += " /Rotate " + rotatePages
	}
	objs = append(objs, pages+" >>")
	caja := func(c [4]float64) string {
		return fmt.Sprintf("[%g %g %g %g]", c[0], c[1], c[2], c[3])
	}
	for i, p := range paginas {
		page := fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox %s", caja(p.MediaBox))
		if p.CropBox != nil {
			page += " /CropBox " + caja(*p.CropBox)
		}
		if p.Rotate != "" {
			page += " /Rotate " + p.Rotate
		}
		page += fmt.Sprintf(" /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", fuente, 4+2*i)
		ref := p.MediaBox
		if p.CropBox != nil {
			ref = *p.CropBox
		}
		texto := fmt.Sprintf("BT /F1 14 Tf %g %g Td (Solicitud) Tj ET", ref[0]+30, ref[3]-40)
		objs = append(objs, page, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(texto), texto))
	}
	objs = append(objs, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
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

var a4Prueba = [4]float64{0, 0, 595, 842}

func TestGiroPaginaLeeYNormalizaRotate(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		pages, page string
		want        int
	}{
		{"", "", 0},
		{"", "90", 90},
		{"", "180", 180},
		{"", "270", 270},
		{"", "-90", 270},
		{"", "450", 90},
		{"", "360", 0},
		{"", "90.0", 90},
		{"", "45", 0},
		{"", "90.5", 0},
		{"", "/Noventa", 0},
		{"", "999999999999999999", 0},
		{"90", "", 90},  // heredado del nodo Pages
		{"270", "0", 0}, // la página manda sobre el nodo Pages
		{"180", "90", 90},
	} {
		pdfData := PDFPaginasPrueba(c.pages, PaginaPrueba{Rotate: c.page, MediaBox: a4Prueba})
		r, err := abrirPDF(pdfData, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := giroPagina(r.Page(1)); got != c.want {
			t.Errorf("Pages /Rotate %q, página /Rotate %q: giro %d, esperado %d", c.pages, c.page, got, c.want)
		}
	}
}

// Sin /Rotate la colocación es literalmente la de antes.
func TestColocarSelloSinGiroIgualQueAntes(t *testing.T) {
	t.Parallel()
	for _, caja := range [][4]float64{{0, 0, 595, 842}, {100, 50, 640, 860}, {-20, -30, 400, 300}} {
		m := marcoPagina{x0: caja[0], y0: caja[1], ancho: caja[2] - caja[0], alto: caja[3] - caja[1]}
		for _, g := range []float64{0, 30, 90, 180, 200, 270, 359} {
			for _, f := range [][4]float64{{0.1, 0.2, 0.3, 0.1}, {0, 0, 0.5, 0.2}, {0.7, 0.85, 0.3, 0.15}} {
				u, v, w, h := f[0]*m.ancho, f[1]*m.alto, f[2]*m.ancho, f[3]*m.alto
				got, giro, err := m.colocarSello(u, v, w, h, g)
				want, wantErr := cajaSelloGirado(caja[0]+f[0]*m.ancho, caja[1]+f[1]*m.alto, w, h, g, caja[0], caja[1], m.ancho, m.alto)
				if (err == nil) != (wantErr == nil) || !cajasCasiIguales(got, want) || giro != g {
					t.Fatalf("caja %v giro %g fracción %v: %v %g %v, antes %v %v", caja, g, f, got, giro, err, want, wantErr)
				}
			}
		}
	}
}

// En arm64 el compilador funde multiplicaciones y sumas (FMA) y el último bit
// puede variar; la colocación es la misma aunque el redondeo no lo sea.
func cajasCasiIguales(a, b [4]float64) bool {
	for i := range a {
		if math.Abs(a[i]-b[i]) > 1e-9 {
			return false
		}
	}
	return true
}

func TestColocarSelloCompensaRotateDeLaPagina(t *testing.T) {
	t.Parallel()
	// A4 vertical; tarjeta de 200 x 80 a (100, 50) de la esquina inferior
	// izquierda de la página tal como se ve, giro 0.
	m := marcoPagina{ancho: 595, alto: 842}
	for _, c := range []struct {
		giro      int
		rect      [4]float64
		giroImg   float64
		anchoVis  float64
		altoVis   float64
		origenX   float64
		origenY   float64
		marcoOrig [2]float64
	}{
		// Vista apaisada de 842 x 595: el centro visible (200, 90) cae en
		// x = 595 - 90, y = 200 de la página sin girar.
		{90, [4]float64{465, 100, 545, 300}, 270, 842, 595, 0, 0, [2]float64{0, 0}},
		{180, [4]float64{595 - 300, 842 - 130, 595 - 100, 842 - 50}, 180, 595, 842, 0, 0, [2]float64{0, 0}},
		{270, [4]float64{50, 842 - 300, 130, 842 - 100}, 90, 842, 595, 0, 0, [2]float64{0, 0}},
	} {
		m.giro = c.giro
		rect, giro, err := m.colocarSello(100, 50, 200, 80, 0)
		if err != nil {
			t.Fatal(err)
		}
		for i := range rect {
			if math.Abs(rect[i]-c.rect[i]) > 1e-9 {
				t.Fatalf("/Rotate %d: rect %v, esperado %v", c.giro, rect, c.rect)
			}
		}
		if giro != c.giroImg {
			t.Fatalf("/Rotate %d: giro de la imagen %g, esperado %g", c.giro, giro, c.giroImg)
		}
		if w, h := m.dimensionesVisibles(); w != c.anchoVis || h != c.altoVis {
			t.Fatalf("/Rotate %d: página vista %gx%g", c.giro, w, h)
		}
	}
	// Con giro libre la imagen suma el giro elegido y descuenta el de la página.
	m.giro = 90
	if _, giro, err := m.colocarSello(100, 50, 200, 80, 30); err != nil || giro != 300 {
		t.Fatalf("giro 30 en /Rotate 90: %g %v", giro, err)
	}
	// La caja envolvente se mantiene dentro de la página vista.
	rect, _, err := m.colocarSello(-40, 560, 200, 80, 0)
	if err != nil || rect[0] < 0 || rect[1] < 0 || rect[2] > 595 || rect[3] > 842 {
		t.Fatalf("caja fuera de la página: %v %v", rect, err)
	}
	if _, _, err := m.colocarSello(0, 0, 900, 80, 0); err == nil {
		t.Fatal("se aceptó un sello más ancho que la página vista")
	}
	// CropBox con origen distinto de cero: la esquina visible (0, 0) de una
	// página con /Rotate 90 es la esquina inferior derecha sin girar.
	m = marcoPagina{x0: 50, y0: 30, ancho: 600, alto: 840, giro: 90}
	if ox, oy := m.origenVisible(); ox != 30 || oy != 50 {
		t.Fatalf("origen visible %g,%g", ox, oy)
	}
	rect, _, err = m.colocarSello(0, 0, 100, 40, 0)
	if err != nil || rect != [4]float64{650 - 40, 30, 650, 130} {
		t.Fatalf("CropBox desplazada: %v %v", rect, err)
	}
}

// En un PDF sin /Rotate el ajuste no toca nada y la lista de sellos produce
// el mismo rectángulo y la misma imagen que la fórmula anterior.
func TestSelloSinRotateSinCambios(t *testing.T) {
	t.Parallel()
	for _, rotate := range []string{"", "0", "360", "45"} {
		pdfData := PDFPaginasPrueba("", PaginaPrueba{Rotate: rotate, MediaBox: [4]float64{20, 10, 615, 852}})
		opciones := map[string]string{
			"visibleSeal": "true", "page": "1", "visibleSealRectX": "120", "visibleSealRectY": "200",
			"visibleSealRectW": "220", "visibleSealRectH": "80", "rotation": "30",
			"visibleSealPageX": "20", "visibleSealPageY": "10", "visibleSealPageWidth": "595", "visibleSealPageHeight": "842",
		}
		data := pdfsign.SignData{Signature: pdfsign.SignDataSignature{Info: testInfoPAdES()}}
		if err := aplicarOpcionesAparienciaPdfsign(&data, opciones); err != nil {
			t.Fatal(err)
		}
		antes := data
		if err := ajustarSelloAPaginasGiradas(&data, opciones, pdfData); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(antes, data) {
			t.Fatalf("/Rotate %q: el ajuste cambió la apariencia de un PDF sin giro", rotate)
		}

		for _, g := range []int{0, 30, 90, 270} {
			raw := fmt.Sprintf(`[{"page":1,"rect":{"x":0.2,"y":0.3,"w":0.4,"h":0.1},"rotation":%d}]`, g)
			data := pdfsign.SignData{Signature: pdfsign.SignDataSignature{Info: testInfoPAdES()}}
			if err := aplicarPosicionesSello(&data, map[string]string{"visibleSealPlacements": raw}, pdfData); err != nil {
				t.Fatal(err)
			}
			w, h := 0.4*595, 0.1*842
			wantRect, err := cajaSelloGirado(20+0.2*595, 10+0.3*842, w, h, float64(g), 20, 10, 595, 842)
			if err != nil {
				t.Fatal(err)
			}
			wantImg, err := componerImagenSelloParaFirma(data.Signature.Info, map[string]string{
				"visibleSealPlacements": raw, "rotation": strconv.Itoa(g),
			}, w, h)
			if err != nil {
				t.Fatal(err)
			}
			got := data.Appearance.PerPage[0]
			if got.Rect != wantRect || !bytes.Equal(got.Image, wantImg) {
				t.Fatalf("/Rotate %q giro %d: rect %v (antes %v) o imagen distinta", rotate, g, got.Rect, wantRect)
			}
		}
	}
}

// Con alguna página girada, cada página elegida recibe su propio rectángulo.
func TestSelloUnaCajaEnPaginasMixtas(t *testing.T) {
	t.Parallel()
	pdfData := PDFPaginasPrueba("",
		PaginaPrueba{MediaBox: a4Prueba},
		PaginaPrueba{Rotate: "90", MediaBox: a4Prueba},
		PaginaPrueba{Rotate: "180", MediaBox: a4Prueba},
	)
	opciones := map[string]string{
		"visibleSeal": "true", "page": "all", "visibleSealRectX": "100", "visibleSealRectY": "50",
		"visibleSealRectW": "200", "visibleSealRectH": "80",
	}
	data := pdfsign.SignData{Signature: pdfsign.SignDataSignature{Info: testInfoPAdES()}}
	if err := aplicarOpcionesAparienciaPdfsign(&data, opciones); err != nil {
		t.Fatal(err)
	}
	if err := ajustarSelloAPaginasGiradas(&data, opciones, pdfData); err != nil {
		t.Fatal(err)
	}
	want := [][4]float64{{100, 50, 300, 130}, {465, 100, 545, 300}, {295, 712, 495, 792}}
	if len(data.Appearance.PerPage) != 3 {
		t.Fatalf("%d sellos por página, esperados 3", len(data.Appearance.PerPage))
	}
	for i, p := range data.Appearance.PerPage {
		if p.Page != uint32(i+1) {
			t.Fatalf("sello %d en la página %d", i, p.Page)
		}
		for j := range p.Rect {
			if math.Abs(p.Rect[j]-want[i][j]) > 1e-9 {
				t.Fatalf("página %d: %+v, esperado %v", i+1, p.Rect, want[i])
			}
		}
	}
}

// Las posiciones con nombre («inferior-derecha»...) se refieren a la página
// tal como se ve.
func TestPosicionElegidaEnPaginaGirada(t *testing.T) {
	t.Parallel()
	pdfData := PDFPaginasPrueba("", PaginaPrueba{Rotate: "90", MediaBox: a4Prueba})
	out, err := posicionSelloElegida(map[string]string{}, "inferior-derecha", pdfData)
	if err != nil {
		t.Fatal(err)
	}
	urx, _ := strconv.ParseFloat(out["signaturePositionOnPageUpperRightX"], 64)
	if math.Abs(urx-(842-margenSelloElegido)) > 0.01 {
		t.Fatalf("la esquina derecha visible debería estar en x=%g, está en %g", 842-margenSelloElegido, urx)
	}
}

// Un /Count desmesurado no debe hacer reservar memoria al resolver «todas».
func TestPaginasSelloElegidasAcotaCountNoFiable(t *testing.T) {
	t.Parallel()
	if got := paginasSelloElegidas(pdfsign.Appearance{AllPages: true}, 2_000_000_000); got != nil {
		t.Fatalf("se resolvieron %d páginas con un /Count no fiable", len(got))
	}
	if got := paginasSelloElegidas(pdfsign.Appearance{AllPages: true}, 3); len(got) != 3 {
		t.Fatalf("todas en un PDF de 3 páginas: %v", got)
	}
	if got := paginasSelloElegidas(pdfsign.Appearance{Pages: []uint32{2, 2, 9}}, 3); got != nil {
		t.Fatalf("se aceptó una página inexistente: %v", got)
	}
}
