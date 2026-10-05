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
	"regexp"
	"strconv"
	"strings"
	"testing"

	"grxfirma/internal/testsupport/exttools"
)

// TestMotorFirmaGo_PAdESGiroLibreSeVeGirado firma de punta a punta con el sello
// girado un ángulo libre y comprueba lo que ve quien abre el PDF: firma válida,
// /Rect igual a la caja envolvente de la tarjeta girada y, si pdftoppm está
// disponible, la tinta del sello orientada al ángulo pedido (en sentido
// horario, como en las vistas previas de las interfaces).
//
// Con GRXFIRMA_SEAL_REVIEW_DIR guarda el PDF y la página renderizada de cada
// ángulo para revisarlos a ojo.
func TestMotorFirmaGo_PAdESGiroLibreSeVeGirado(t *testing.T) {
	const x, y, w, h = 200.0, 300.0, 220.0, 80.0
	for _, grados := range []int{30, 45, 200} {
		t.Run(strconv.Itoa(grados), func(t *testing.T) {
			firmado := firmarFormulario(t, pdfFormulario(false), map[string]string{
				"visibleSeal": "true", "page": "1",
				"visibleSealRectX": fmt.Sprint(x), "visibleSealRectY": fmt.Sprint(y),
				"visibleSealRectW": fmt.Sprint(w), "visibleSealRectH": fmt.Sprint(h),
				"rotation": strconv.Itoa(grados),
			})
			comprobarPDFFirmado(t, firmado)

			r := float64(grados) * math.Pi / 180
			bw := w*math.Abs(math.Cos(r)) + h*math.Abs(math.Sin(r))
			bh := w*math.Abs(math.Sin(r)) + h*math.Abs(math.Cos(r))
			want := [4]float64{x + (w-bw)/2, y + (h-bh)/2, x + (w+bw)/2, y + (h+bh)/2}
			rect, ap := widgetFirmaYApariencia(t, firmado)
			for i := range want {
				if math.Abs(rect[i]-want[i]) > 0.01 {
					t.Fatalf("/Rect=%v, esperado %v (caja envolvente de la tarjeta girada)", rect, want)
				}
			}
			// La apariencia ocupa la caja sin deformarse: el giro va dentro de la
			// imagen y el XObject se dibuja tal cual sobre /Rect.
			bbox := numerosPDF(t, ap, "BBox")
			if len(bbox) != 4 || math.Abs(bbox[2]-bbox[0]-bw) > 0.01 || math.Abs(bbox[3]-bbox[1]-bh) > 0.01 {
				t.Fatalf("/BBox=%v no mide %.2f×%.2f", bbox, bw, bh)
			}
			if m := numerosPDF(t, ap, "Matrix"); len(m) == 6 && (m[0] != 1 || m[1] != 0 || m[2] != 0 || m[3] != 1) {
				t.Fatalf("/Matrix=%v; la imagen ya viene girada y no debe girarse otra vez", m)
			}

			if !exttools.Available(t, "pdftoppm") {
				return
			}
			pagina := renderizarPagina(t, firmado, 144)
			const escala = 2.0 // 144 ppp / 72 pt
			recorte := image.Rect(int(rect[0]*escala), int((842-rect[3])*escala), int(rect[2]*escala), int((842-rect[1])*escala))
			angulo, alargamiento, cx, cy := orientacionTinta(pagina, recorte)
			t.Logf("tinta orientada a %.1f°, alargamiento %.2f", angulo, alargamiento)
			if alargamiento < 2 {
				t.Fatalf("la tinta del sello no tiene un eje claro (alargamiento %.2f)", alargamiento)
			}
			esperado := math.Mod(float64(grados), 180)
			if d := math.Abs(math.Remainder(angulo-esperado, 180)); d > 4 {
				t.Fatalf("la tinta del sello está orientada a %.1f°, se esperaba %.0f° (diferencia %.1f°)", angulo, esperado, d)
			}
			// La barra de acento marca el borde izquierdo de la tarjeta:
			// distingue 30° de 210°.
			vx, vy, n := centroAcento(pagina, recorte)
			if n == 0 {
				t.Fatal("no aparece la barra de acento del sello en la página renderizada")
			}
			if (vx-cx)*math.Cos(r)+(vy-cy)*math.Sin(r) >= 0 {
				t.Fatalf("la barra de acento no queda a la izquierda de la tarjeta girada %d°", grados)
			}
			guardarRevisionGiro(t, grados, firmado, pagina)
		})
	}
}

func widgetFirmaYApariencia(t *testing.T, pdf []byte) ([4]float64, []byte) {
	t.Helper()
	objetos := regexp.MustCompile(`(?s)(\d+) 0 obj\s*(.*?)endobj`).FindAllSubmatch(pdf, -1)
	porID := map[string][]byte{}
	for _, o := range objetos {
		porID[string(o[1])] = o[2]
	}
	// El formulario de prueba trae además un campo de firma vacío sin
	// apariencia; el widget que interesa es el que añade la firma.
	for _, o := range objetos {
		cuerpo := o[2]
		if !bytes.Contains(cuerpo, []byte("/Subtype /Widget")) || !bytes.Contains(cuerpo, []byte("/FT /Sig")) {
			continue
		}
		ap := regexp.MustCompile(`/AP\s*<<\s*/N\s+(\d+) 0 R`).FindSubmatch(cuerpo)
		if ap == nil {
			continue
		}
		if porID[string(ap[1])] == nil {
			t.Fatalf("falta el objeto %s de la apariencia /AP /N", ap[1])
		}
		n := numerosPDF(t, cuerpo, "Rect")
		if len(n) != 4 {
			t.Fatalf("/Rect inesperado: %v", n)
		}
		return [4]float64{n[0], n[1], n[2], n[3]}, porID[string(ap[1])]
	}
	t.Fatal("no se encontró el widget de la firma")
	return [4]float64{}, nil
}

func numerosPDF(t *testing.T, dic []byte, clave string) []float64 {
	t.Helper()
	m := regexp.MustCompile(`/` + clave + `\s*\[([^\]]*)\]`).FindSubmatch(dic)
	if m == nil {
		return nil
	}
	var out []float64
	for _, campo := range strings.Fields(string(m[1])) {
		v, err := strconv.ParseFloat(campo, 64)
		if err != nil {
			t.Fatalf("/%s con valor no numérico %q", clave, campo)
		}
		out = append(out, v)
	}
	return out
}

func renderizarPagina(t *testing.T, pdf []byte, ppp int) image.Image {
	t.Helper()
	dir := t.TempDir()
	entrada := filepath.Join(dir, "firmado.pdf")
	if err := os.WriteFile(entrada, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	salida := filepath.Join(dir, "pagina")
	// #nosec G204 -- herramienta fija y rutas temporales propias del test.
	if out, err := exec.Command("pdftoppm", "-r", strconv.Itoa(ppp), "-png", "-f", "1", "-l", "1", "-singlefile", entrada, salida).CombinedOutput(); err != nil {
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

// orientacionTinta devuelve el ángulo del eje principal de los píxeles oscuros
// (en grados, con y hacia abajo: positivo es horario), cuánto más largo es ese
// eje que el perpendicular y su centro.
func orientacionTinta(img image.Image, zona image.Rectangle) (angulo, alargamiento, cx, cy float64) {
	zona = zona.Intersect(img.Bounds())
	var n, sx, sy, sxx, syy, sxy float64
	for py := zona.Min.Y; py < zona.Max.Y; py++ {
		for px := zona.Min.X; px < zona.Max.X; px++ {
			r, g, b, _ := img.At(px, py).RGBA()
			if (299*r+587*g+114*b)/1000 > 200*257 {
				continue
			}
			fx, fy := float64(px), float64(py)
			n++
			sx += fx
			sy += fy
			sxx += fx * fx
			syy += fy * fy
			sxy += fx * fy
		}
	}
	if n == 0 {
		return 0, 0, 0, 0
	}
	cx, cy = sx/n, sy/n
	vxx, vyy, vxy := sxx/n-cx*cx, syy/n-cy*cy, sxy/n-cx*cy
	angulo = 0.5 * math.Atan2(2*vxy, vxx-vyy) * 180 / math.Pi
	if angulo < 0 {
		angulo += 180
	}
	traza, raiz := vxx+vyy, math.Sqrt((vxx-vyy)*(vxx-vyy)+4*vxy*vxy)
	menor := (traza - raiz) / 2
	if menor <= 0 {
		return angulo, math.Inf(1), cx, cy
	}
	return angulo, math.Sqrt((traza + raiz) / 2 / menor), cx, cy
}

// centroAcento devuelve el centro de la barra de acento del sello (azul
// #206bc4, en el borde izquierdo de una tarjeta apaisada). El texto azul
// marino es más oscuro y no cuenta.
func centroAcento(img image.Image, zona image.Rectangle) (cx, cy float64, n int) {
	zona = zona.Intersect(img.Bounds())
	for py := zona.Min.Y; py < zona.Max.Y; py++ {
		for px := zona.Min.X; px < zona.Max.X; px++ {
			r, g, b, _ := img.At(px, py).RGBA()
			r, g, b = r>>8, g>>8, b>>8
			if b > 150 && b > r+80 && b > g+40 {
				cx += float64(px)
				cy += float64(py)
				n++
			}
		}
	}
	if n > 0 {
		cx, cy = cx/float64(n), cy/float64(n)
	}
	return cx, cy, n
}

func guardarRevisionGiro(t *testing.T, grados int, pdf []byte, pagina image.Image) {
	t.Helper()
	dir := os.Getenv("GRXFIRMA_SEAL_REVIEW_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, fmt.Sprintf("sello-girado-%d", grados))
	var imagen bytes.Buffer
	if err := png.Encode(&imagen, pagina); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base+".pdf", pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base+".png", imagen.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}
