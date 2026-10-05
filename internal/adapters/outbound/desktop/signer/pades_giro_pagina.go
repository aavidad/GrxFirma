// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/digitorus/pdf"
	pdfsign "github.com/digitorus/pdfsign/sign"
)

// Convención única del sello visible: la posición, el tamaño y el giro que
// llegan de las interfaces y de las sedes se refieren a la página tal como se
// ve, es decir, ya girada según su /Rotate. Es lo que muestran las vistas
// previas (QtPdf, Poppler, Windows.Data.Pdf y PdfRenderer aplican /Rotate) y
// lo que hace AutoFirma Java con iText. El motor traduce aquí esa caja al
// espacio de usuario de la página sin girar y compensa el giro de la página
// en la imagen del sello, para que con giro 0 se lea derecho en pantalla.

// marcoPagina describe la caja visible de una página (CropBox o, si falta,
// MediaBox) en el espacio de usuario sin girar, y su /Rotate.
type marcoPagina struct {
	x0, y0, ancho, alto float64
	giro                int
}

// marcoVisiblePagina lee la caja visible y el /Rotate (heredado) de la página.
func marcoVisiblePagina(page pdf.Page) (marcoPagina, error) {
	x0, y0, x1, y1, err := cajaVisiblePagina(page)
	if err != nil {
		return marcoPagina{}, err
	}
	m := marcoPagina{x0: x0, y0: y0, ancho: x1 - x0, alto: y1 - y0, giro: giroPagina(page)}
	if !numeroFinitoSello(m.ancho) || !numeroFinitoSello(m.alto) || m.ancho <= 0 || m.alto <= 0 || m.ancho > 14400 || m.alto > 14400 {
		return marcoPagina{}, fmt.Errorf("dimensiones inválidas de la página PDF")
	}
	return m, nil
}

// giroPagina devuelve el /Rotate de la página, heredado de los nodos Pages
// si la página no lo define, normalizado a 0, 90, 180 o 270. Un valor que no
// sea un múltiplo entero de 90 se trata como 0, igual que pdf.js; el PDF de
// entrada no es de confianza y un valor raro no debe impedir firmar.
func giroPagina(page pdf.Page) int {
	v := page.V.Key("Rotate")
	for parent, depth := page.V.Key("Parent"), 0; v.IsNull() && !parent.IsNull() && depth < 64; parent, depth = parent.Key("Parent"), depth+1 {
		v = parent.Key("Rotate")
	}
	var n int64
	switch v.Kind() {
	case pdf.Integer:
		n = v.Int64()
	case pdf.Real:
		f := v.Float64()
		if !numeroFinitoSello(f) || f != math.Trunc(f) || math.Abs(f) > 1e9 {
			return 0
		}
		n = int64(f)
	default:
		return 0
	}
	if n%90 != 0 {
		return 0
	}
	n %= 360
	if n < 0 {
		n += 360
	}
	return int(n)
}

// dimensionesVisibles devuelve el ancho y el alto de la página tal como se ve.
func (m marcoPagina) dimensionesVisibles() (float64, float64) {
	if m.giro == 90 || m.giro == 270 {
		return m.alto, m.ancho
	}
	return m.ancho, m.alto
}

// origenVisible es la esquina inferior izquierda de la página vista en
// coordenadas absolutas giradas: como Rectangle.rotate de iText, con 90 y 270
// se intercambian x e y.
func (m marcoPagina) origenVisible() (float64, float64) {
	if m.giro == 90 || m.giro == 270 {
		return m.y0, m.x0
	}
	return m.x0, m.y0
}

// colocarSello recibe la tarjeta del sello sin girar (w x h puntos) con su
// esquina inferior izquierda en (u, v), medida desde la esquina inferior
// izquierda de la página vista, y girada «grados» en sentido horario tal como
// se ve. Devuelve el /Rect del widget en el espacio de usuario de la página y
// el giro horario que debe llevar la imagen dentro de ese /Rect.
//
// Sin /Rotate hace exactamente lo mismo que antes: cajaSelloGirado sobre la
// caja visible y el mismo giro.
func (m marcoPagina) colocarSello(u, v, w, h, grados float64) ([4]float64, float64, error) {
	if m.giro == 0 {
		rect, err := cajaSelloGirado(m.x0+u, m.y0+v, w, h, grados, m.x0, m.y0, m.ancho, m.alto)
		return rect, grados, err
	}
	if !numeroFinitoSello(u) || !numeroFinitoSello(v) || !numeroFinitoSello(w) || !numeroFinitoSello(h) || !numeroFinitoSello(grados) {
		return [4]float64{}, 0, fmt.Errorf("geometría o giro del sello no válido")
	}
	// /Rotate gira la página en sentido horario al mostrarla. Se lleva el
	// centro de la tarjeta de la página vista a la página sin girar.
	cu, cv := u+w/2, v+h/2
	var cx, cy float64
	switch m.giro {
	case 90:
		cx, cy = m.x0+m.ancho-cv, m.y0+cu
	case 180:
		cx, cy = m.x0+m.ancho-cu, m.y0+m.alto-cv
	default: // 270
		cx, cy = m.x0+cv, m.y0+m.alto-cu
	}
	// La página suma su giro al mostrarse; la imagen lo descuenta.
	giroImagen := math.Mod(grados-float64(m.giro), 360)
	if giroImagen < 0 {
		giroImagen += 360
	}
	// La tarjeta conserva su ancho y su alto: es la imagen la que gira. La
	// caja envolvente se mantiene dentro de la página igual que sin /Rotate,
	// porque el giro de la página lleva cajas alineadas a cajas alineadas.
	rect, err := cajaSelloGirado(cx-w/2, cy-h/2, w, h, giroImagen, m.x0, m.y0, m.ancho, m.alto)
	return rect, giroImagen, err
}

// opcionesConGiroImagen copia las opciones con el giro de la imagen que
// corresponde a la página.
func opcionesConGiroImagen(options map[string]string, giroImagen float64) map[string]string {
	out := make(map[string]string, len(options)+1)
	for k, v := range options {
		if strings.EqualFold(strings.TrimSpace(k), "rotation") {
			continue
		}
		out[k] = v
	}
	out["rotation"] = strconv.FormatFloat(giroImagen, 'f', -1, 64)
	return out
}

// ajustarSelloAPaginasGiradas corrige el sello de una sola caja
// (visibleSealRectX/Y/W/H, también el traducido desde los parámetros de
// AutoFirma Java) cuando alguna de las páginas elegidas tiene /Rotate. Si
// ninguna lo tiene no cambia nada, de modo que el resultado es idéntico al
// de antes. Si alguna lo tiene, cada página recibe su propio rectángulo e
// imagen, porque el mismo sello ocupa sitios distintos según el giro.
func ajustarSelloAPaginasGiradas(signData *pdfsign.SignData, options map[string]string, pdfData []byte) error {
	if signData == nil || !signData.Appearance.Visible || len(signData.Appearance.PerPage) > 0 ||
		signData.Appearance.FieldName != "" || !solicitaSelloVisiblePAdES(options) ||
		strings.TrimSpace(valorOpcion(options, "visibleSealPlacements")) != "" ||
		strings.TrimSpace(valorOpcion(options, "signatureField")) != "" {
		return nil
	}
	r, err := abrirPDF(pdfData, options)
	if err != nil {
		return err
	}
	total := r.NumPage()
	paginas := paginasSelloElegidas(signData.Appearance, total)
	if len(paginas) == 0 {
		return nil // pdfsign informa de la página inexistente
	}
	girada := false
	for _, p := range paginas {
		if giroPagina(r.Page(int(p))) != 0 {
			girada = true
			break
		}
	}
	if !girada {
		return nil
	}

	x := valorFloatOpcionPAdES(options, "visibleSealRectX", 0.62*595.28)
	y := valorFloatOpcionPAdES(options, "visibleSealRectY", 0.04*841.89)
	w := valorFloatOpcionPAdES(options, "visibleSealRectW", 0.34*595.28)
	h := valorFloatOpcionPAdES(options, "visibleSealRectH", 0.12*841.89)
	if err := validarGeometriaSello(options, x, y, w, h); err != nil {
		return fmt.Errorf("PAdES pdfsign: %w", err)
	}
	grados := valorFloatOpcionPAdES(options, "rotation", 0)
	relativa := valorBoolOpcionPAdES(options, "visibleSealRectRelativeToCrop", false)

	imagenes := make(map[float64][]byte, 4)
	placements := make([]pdfsign.PageAppearance, 0, len(paginas))
	for _, p := range paginas {
		m, err := marcoVisiblePagina(r.Page(int(p)))
		if err != nil {
			return fmt.Errorf("PAdES: página %d: %w", p, err)
		}
		u, v := x, y
		if !relativa {
			ox, oy := m.origenVisible()
			u, v = x-ox, y-oy
		}
		rect, giroImagen, err := m.colocarSello(u, v, w, h, grados)
		if err != nil {
			return fmt.Errorf("PAdES pdfsign: página %d: %w", p, err)
		}
		img, ok := imagenes[giroImagen]
		if !ok {
			if img, err = componerImagenSelloParaFirma(signData.Signature.Info, opcionesConGiroImagen(options, giroImagen), w, h); err != nil {
				return err
			}
			imagenes[giroImagen] = img
		}
		placements = append(placements, pdfsign.PageAppearance{Page: p, Rect: rect, Image: img})
	}
	signData.Appearance.PerPage = placements
	return nil
}

// paginasSelloElegidas resuelve las páginas del sello como pdfsign: todas, la
// lista sin repetidas o la página única. Devuelve nil si alguna no existe.
//
// El total sale del /Count del PDF, que no es de confianza: con «todas» y un
// total mayor que el límite de páginas expandidas no se reserva nada y el
// sello sigue el camino de siempre.
func paginasSelloElegidas(a pdfsign.Appearance, total int) []uint32 {
	if total <= 0 {
		return nil
	}
	if a.AllPages && total > maxVisibleSealExpandedPages {
		return nil
	}
	if a.AllPages {
		out := make([]uint32, total)
		for i := range out {
			out[i] = uint32(i + 1) // #nosec G115 -- i < total, que es un int positivo del lector.
		}
		return out
	}
	if len(a.Pages) > 0 {
		vistas := make(map[uint32]bool, len(a.Pages))
		out := make([]uint32, 0, len(a.Pages))
		for _, p := range a.Pages {
			if p == 0 || uint64(p) > uint64(total) {
				return nil
			}
			if !vistas[p] {
				vistas[p] = true
				out = append(out, p)
			}
		}
		return out
	}
	p := a.Page
	if p == 0 {
		p = 1
	}
	if uint64(p) > uint64(total) {
		return nil
	}
	return []uint32{p}
}
