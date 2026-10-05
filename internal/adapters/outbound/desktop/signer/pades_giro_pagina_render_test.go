// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/testsupport/exttools"
)

// casoPaginaGirada es una página sintética y la caja en la que el usuario
// coloca el sello sobre la vista previa (fracciones de la página tal como se
// ve, con la y desde abajo).
type casoPaginaGirada struct {
	nombre      string
	rotatePages string
	pagina      signer.PaginaPrueba
}

func (c casoPaginaGirada) cajaVisible() (x0, y0, ancho, alto float64, giro int) {
	caja := c.pagina.MediaBox
	if c.pagina.CropBox != nil {
		caja = *c.pagina.CropBox
	}
	rot := c.pagina.Rotate
	if rot == "" {
		rot = c.rotatePages
	}
	giro, _ = strconv.Atoi(rot)
	giro = ((giro % 360) + 360) % 360
	ancho, alto = caja[2]-caja[0], caja[3]-caja[1]
	if giro == 90 || giro == 270 {
		return caja[1], caja[0], alto, ancho, giro
	}
	return caja[0], caja[1], ancho, alto, giro
}

const fx, fy, fw, fh = 0.25, 0.4, 0.5, 0.09

// TestMotorFirmaGo_PAdESSelloEnPaginaGirada firma páginas con /Rotate 0, 90,
// 180 y 270 (también heredado del nodo Pages y con CropBox de origen distinto
// de cero), con el sello a 0° y a 30°, por la lista de sellos de las
// interfaces y por la caja única en puntos. Comprueba la firma y, con
// pdftoppm (que aplica /Rotate y aquí dibuja la CropBox como los lectores),
// que el sello aparece donde se pidió sobre la página vista y con el giro
// pedido, leyéndose derecho a 0°.
//
// Con GRXFIRMA_SEAL_REVIEW_DIR guarda cada PDF y su página renderizada.
func TestMotorFirmaGo_PAdESSelloEnPaginaGirada(t *testing.T) {
	a4 := [4]float64{0, 0, 595, 842}
	crop := [4]float64{120, 80, 680, 860}
	casos := []casoPaginaGirada{
		{"rotate0", "", signer.PaginaPrueba{MediaBox: a4}},
		{"rotate90", "", signer.PaginaPrueba{Rotate: "90", MediaBox: a4}},
		{"rotate180", "", signer.PaginaPrueba{Rotate: "180", MediaBox: a4}},
		{"rotate270", "", signer.PaginaPrueba{Rotate: "270", MediaBox: a4}},
		{"rotate90-heredado", "90", signer.PaginaPrueba{MediaBox: a4}},
		{"rotate270-cropbox", "", signer.PaginaPrueba{Rotate: "270", MediaBox: [4]float64{100, 50, 700, 900}, CropBox: &crop}},
		{"rotate90-cropbox", "", signer.PaginaPrueba{Rotate: "90", MediaBox: [4]float64{100, 50, 700, 900}, CropBox: &crop}},
	}
	for _, c := range casos {
		for _, grados := range []int{0, 30} {
			for _, modo := range []string{"lista", "caja", "caja-absoluta"} {
				nombre := fmt.Sprintf("%s-giro%d-%s", c.nombre, grados, modo)
				t.Run(nombre, func(t *testing.T) {
					x0, y0, ancho, alto, _ := c.cajaVisible()
					opciones := map[string]string{"visibleSeal": "true", "rotation": strconv.Itoa(grados)}
					switch modo {
					case "lista":
						opciones["visibleSealPlacements"] = fmt.Sprintf(`[{"page":1,"rect":{"x":%g,"y":%g,"w":%g,"h":%g},"rotation":%d}]`, fx, fy, fw, fh, grados)
					case "caja":
						opciones["page"] = "1"
						opciones["visibleSealRectRelativeToCrop"] = "true"
						opciones["visibleSealRectX"] = fmt.Sprint(fx * ancho)
						opciones["visibleSealRectY"] = fmt.Sprint(fy * alto)
						opciones["visibleSealRectW"] = fmt.Sprint(fw * ancho)
						opciones["visibleSealRectH"] = fmt.Sprint(fh * alto)
					default:
						// Coordenadas absolutas de la página vista, como las de
						// AutoFirma Java: el origen es el de la caja girada.
						opciones["page"] = "1"
						opciones["visibleSealRectX"] = fmt.Sprint(x0 + fx*ancho)
						opciones["visibleSealRectY"] = fmt.Sprint(y0 + fy*alto)
						opciones["visibleSealRectW"] = fmt.Sprint(fw * ancho)
						opciones["visibleSealRectH"] = fmt.Sprint(fh * alto)
					}
					firmado := firmarFormulario(t, signer.PDFPaginasPrueba(c.rotatePages, c.pagina), opciones)
					comprobarPDFFirmado(t, firmado)
					if !exttools.Available(t, "pdftoppm") {
						return
					}
					pagina := renderizarPaginaVista(t, firmado, 1, 144)
					guardarRevisionRotate(t, nombre, firmado, pagina)
					comprobarSelloVisto(t, pagina, ancho, alto, grados)
				})
			}
		}
	}
}

// Un mismo sello de caja única en todas las páginas cae en el mismo sitio
// visible de cada una aunque tengan /Rotate distintos.
func TestMotorFirmaGo_PAdESSelloTodasLasPaginasConRotateDistinto(t *testing.T) {
	a4 := [4]float64{0, 0, 595, 842}
	paginas := []signer.PaginaPrueba{{MediaBox: a4}, {Rotate: "90", MediaBox: a4}, {Rotate: "180", MediaBox: a4}, {Rotate: "270", MediaBox: a4}}
	const x, y, w, h = 200.0, 150.0, 260.0, 60.0
	firmado := firmarFormulario(t, signer.PDFPaginasPrueba("", paginas...), map[string]string{
		"visibleSeal": "true", "page": "all", "visibleSealRectRelativeToCrop": "true",
		"visibleSealRectX": fmt.Sprint(x), "visibleSealRectY": fmt.Sprint(y),
		"visibleSealRectW": fmt.Sprint(w), "visibleSealRectH": fmt.Sprint(h),
	})
	comprobarPDFFirmado(t, firmado)
	if !exttools.Available(t, "pdftoppm") {
		return
	}
	for i, p := range paginas {
		alto := 842.0
		if p.Rotate == "90" || p.Rotate == "270" {
			alto = 595.0
		}
		pagina := renderizarPaginaVista(t, firmado, i+1, 144)
		guardarRevisionRotate(t, fmt.Sprintf("todas-pagina%d-rotate%s", i+1, p.Rotate), firmado, pagina)
		comprobarSelloEnCaja(t, pagina, alto, x, y, w, h, 0)
	}
}

func comprobarSelloVisto(t *testing.T, pagina image.Image, ancho, alto float64, grados int) {
	t.Helper()
	comprobarSelloEnCaja(t, pagina, alto, fx*ancho, fy*alto, fw*ancho, fh*alto, grados)
}

// comprobarSelloEnCaja busca el sello en la página renderizada a 144 ppp: su
// caja envolvente debe ser la de la tarjeta (x, y, w, h en puntos de la página
// vista, y desde abajo) girada «grados» en sentido horario, y la barra de
// acento y la tinta deben seguir ese giro.
func comprobarSelloEnCaja(t *testing.T, pagina image.Image, alto, x, y, w, h float64, grados int) {
	t.Helper()
	const escala = 2.0
	r := float64(grados) * math.Pi / 180
	bw := (w*math.Abs(math.Cos(r)) + h*math.Abs(math.Sin(r))) * escala
	bh := (w*math.Abs(math.Sin(r)) + h*math.Abs(math.Cos(r))) * escala
	cx, cy := (x+w/2)*escala, (alto-(y+h/2))*escala
	// Se busca con un margen de 20 pt alrededor de la posición pedida.
	const margen = 20 * escala
	caja := cajaNoBlanca(pagina, image.Rect(int(cx-bw/2-margen), int(cy-bh/2-margen), int(cx+bw/2+margen), int(cy+bh/2+margen)))
	want := [4]float64{cx - bw/2, cy - bh/2, cx + bw/2, cy + bh/2}
	if caja.Empty() {
		t.Fatalf("no aparece el sello cerca de %v px en la página vista", want)
	}
	got := [4]float64{float64(caja.Min.X), float64(caja.Min.Y), float64(caja.Max.X), float64(caja.Max.Y)}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 6 { // esquinas redondeadas y antialias
			t.Fatalf("el sello ocupa %v px en la página vista; se esperaba %v", caja, want)
		}
	}
	// La barra de acento está en el borde izquierdo de la tarjeta: su centro
	// fija el sentido del giro (distingue 30° de 330° y de 210°).
	vx, vy, n := centroAcento(pagina, caja)
	if n == 0 {
		t.Fatal("no aparece la barra de acento del sello")
	}
	ex, ey := cx-w/2*escala*math.Cos(r), cy-w/2*escala*math.Sin(r)
	if d := math.Hypot(vx-ex, vy-ey); d > 0.05*w*escala+6 {
		t.Fatalf("la barra de acento está en (%.0f, %.0f) px y debería estar en (%.0f, %.0f): el sello no se ve con el giro pedido", vx, vy, ex, ey)
	}
	// La tinta del texto sigue el giro (comprobación aproximada: el texto
	// alineado a la izquierda inclina algo su eje principal).
	angulo, alargamiento, _, _ := orientacionTinta(pagina, caja)
	if alargamiento < 2 {
		t.Fatalf("la tinta del sello no tiene un eje claro (alargamiento %.2f)", alargamiento)
	}
	if d := math.Abs(math.Remainder(angulo-math.Mod(float64(grados), 180), 180)); d > 8 {
		t.Fatalf("la tinta del sello está orientada a %.1f°, se esperaba %d° (diferencia %.1f°)", angulo, grados, d)
	}
}

// cajaNoBlanca devuelve la caja de los píxeles que no son blancos dentro de
// la zona.
func cajaNoBlanca(img image.Image, zona image.Rectangle) image.Rectangle {
	zona = zona.Intersect(img.Bounds())
	minX, minY, maxX, maxY := zona.Max.X, zona.Max.Y, zona.Min.X-1, zona.Min.Y-1
	for py := zona.Min.Y; py < zona.Max.Y; py++ {
		for px := zona.Min.X; px < zona.Max.X; px++ {
			r, g, b, _ := img.At(px, py).RGBA()
			if r>>8 > 247 && g>>8 > 247 && b>>8 > 247 {
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

// renderizarPaginaVista renderiza la CropBox ya girada, como la ven los lectores.
func renderizarPaginaVista(t *testing.T, pdf []byte, numero, ppp int) image.Image {
	t.Helper()
	dir := t.TempDir()
	entrada := filepath.Join(dir, "firmado.pdf")
	if err := os.WriteFile(entrada, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	salida := filepath.Join(dir, "pagina")
	n := strconv.Itoa(numero)
	// #nosec G204 -- herramienta fija y rutas temporales propias del test.
	if out, err := exec.Command("pdftoppm", "-cropbox", "-r", strconv.Itoa(ppp), "-png", "-f", n, "-l", n, "-singlefile", entrada, salida).CombinedOutput(); err != nil {
		t.Fatalf("pdftoppm: %v\n%s", err, out)
	}
	datos, err := os.ReadFile(salida + ".png") // #nosec G304 -- fichero temporal propio.
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(datos))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func guardarRevisionRotate(t *testing.T, nombre string, pdf []byte, pagina image.Image) {
	t.Helper()
	dir := os.Getenv("GRXFIRMA_SEAL_REVIEW_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	var imagen bytes.Buffer
	if err := png.Encode(&imagen, pagina); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, nombre)
	if err := os.WriteFile(base+".pdf", pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base+".png", imagen.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}
