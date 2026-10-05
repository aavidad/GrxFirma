// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/url"
	"strings"

	"github.com/digitorus/pdf"
	pdfsign "github.com/digitorus/pdfsign/sign"
)

// Leyenda de Código Seguro de Verificación (CSV) en el margen inferior de
// cada página, como la que imprimen las sedes electrónicas: texto con el
// código y la dirección de cotejo y un QR que abre esa dirección. El CSV lo
// emite el sistema de la Administración; la aplicación solo lo estampa, y lo
// hace en la misma actualización incremental que la firma, de modo que la
// leyenda queda protegida por ella.
//
// Opciones: csv (código), csvUrl (dirección HTTPS de cotejo; "{csv}" se
// sustituye por el código) y csvText (texto alternativo, con {csv} y {url}).

const (
	maxLongitudCSV       = 128
	maxLongitudCSVURL    = 2048
	maxLongitudCSVTexto  = 512
	altoLeyendaCSVPt     = 34.0
	margenLeyendaCSVPt   = 18.0
	escalaLeyendaCSV     = 3
	textoLeyendaCSVDefec = "Código Seguro de Verificación (CSV): {csv}. Puede comprobar la autenticidad de este documento en {url}"
)

type opcionesLeyendaCSV struct {
	codigo, direccion, texto string
	qr                       bool
}

func leerOpcionesLeyendaCSV(options map[string]string) (opcionesLeyendaCSV, bool, error) {
	codigo := strings.TrimSpace(valorOpcion(options, "csv"))
	if codigo == "" {
		return opcionesLeyendaCSV{}, false, nil
	}
	if len(codigo) > maxLongitudCSV || contieneControlOFormato(codigo) {
		return opcionesLeyendaCSV{}, false, fmt.Errorf("el código CSV no es válido")
	}
	direccion := strings.ReplaceAll(strings.TrimSpace(valorOpcion(options, "csvUrl")), "{csv}", url.QueryEscape(codigo))
	direccion, err := normalizarURLQRSello(direccion)
	if err != nil || direccion == "" || len(direccion) > maxLongitudCSVURL {
		return opcionesLeyendaCSV{}, false, fmt.Errorf("la dirección de cotejo del CSV (csvUrl) debe ser una URL HTTPS válida")
	}
	texto := strings.TrimSpace(valorOpcion(options, "csvText"))
	if texto == "" {
		texto = textoLeyendaCSVDefec
	}
	if len(texto) > maxLongitudCSVTexto || contieneControlOFormato(texto) {
		return opcionesLeyendaCSV{}, false, fmt.Errorf("el texto de la leyenda CSV supera el límite permitido")
	}
	texto = strings.NewReplacer("{csv}", codigo, "{url}", direccion).Replace(texto)
	return opcionesLeyendaCSV{codigo: codigo, direccion: direccion, texto: texto, qr: !strings.EqualFold(valorOpcion(options, "csvQR"), "false")}, true, nil
}

// ResolverLeyendaCSV valida csv, csvUrl y csvText con las reglas de la firma
// y devuelve la dirección de cotejo normalizada (dominio IDN en ASCII) y el
// texto final, para que una interfaz avise antes de firmar.
func ResolverLeyendaCSV(options map[string]string) (direccion, texto string, activa bool, err error) {
	opciones, activa, err := leerOpcionesLeyendaCSV(options)
	if err != nil || !activa {
		return "", "", activa, err
	}
	return opciones.direccion, opciones.texto, true, nil
}

// leyendaCSV devuelve un sello por página con la leyenda CSV, en el margen
// inferior de la página tal como se ve (con su CropBox y su /Rotate).
func leyendaCSV(options map[string]string, pdfData []byte) ([]pdfsign.StampImage, error) {
	opciones, activa, err := leerOpcionesLeyendaCSV(options)
	if err != nil || !activa {
		return nil, err
	}
	r, err := abrirPDF(pdfData, options)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer el PDF: %w", err)
	}
	// El /Count no es de confianza: no se recorre ni se reserva memoria por
	// encima de las estampas que admite una firma.
	total := r.NumPage()
	if total > pdfsign.MaxSellosImagen {
		return nil, fmt.Errorf("la leyenda CSV admite documentos de hasta %d páginas", pdfsign.MaxSellosImagen)
	}
	type claveImagen struct{ ancho, giro int }
	imagenes := map[claveImagen][]byte{} // para no repetir el renderizado
	out := make([]pdfsign.StampImage, 0, max(total, 0))
	for p := 1; p <= total; p++ {
		m := marcoLeyendaCSV(r.Page(p))
		anchoVisto, altoVisto := m.dimensionesVisibles()
		ancho := anchoVisto - 2*margenLeyendaCSVPt
		if ancho < 120 || altoVisto < altoLeyendaCSVPt+6 {
			return nil, fmt.Errorf("la página %d es demasiado estrecha para la leyenda CSV", p)
		}
		rect, giroImagen, err := m.colocarSello(margenLeyendaCSVPt, 6, ancho, altoLeyendaCSVPt, 0)
		if err != nil {
			return nil, fmt.Errorf("página %d: %w", p, err)
		}
		clave := claveImagen{int(ancho), int(giroImagen)}
		if _, ok := imagenes[clave]; !ok {
			img, err := imagenLeyendaCSV(opciones, ancho)
			if err != nil {
				return nil, err
			}
			// En una página girada la imagen descuenta el giro para leerse
			// derecha en pantalla.
			if imagenes[clave], err = rotarImagenPNGSiProcede(img, int(giroImagen)); err != nil {
				return nil, err
			}
		}
		out = append(out, pdfsign.StampImage{Page: uint32(p), Rect: rect, Image: imagenes[clave]}) // #nosec G115 -- 1 <= p <= NumPage.
	}
	return out, nil
}

// marcoLeyendaCSV devuelve la caja visible de la página o, en documentos
// antiguos sin MediaBox válida, un A4 con el /Rotate de la página.
func marcoLeyendaCSV(page pdf.Page) marcoPagina {
	if m, err := marcoVisiblePagina(page); err == nil {
		return m
	}
	x0, y0, x1, y1 := cajaPagina(page)
	return marcoPagina{x0: x0, y0: y0, ancho: x1 - x0, alto: y1 - y0, giro: giroPagina(page)}
}

// cajaPagina devuelve la MediaBox de la página (heredable) o A4 por defecto.
func cajaPagina(p pdf.Page) (x0, y0, x1, y1 float64) {
	// Heredado corta las cadenas /Parent circulares o demasiado largas.
	caja := p.Heredado("MediaBox")
	if caja.Len() != 4 {
		return 0, 0, 595.28, 841.89
	}
	return caja.Index(0).Float64(), caja.Index(1).Float64(), caja.Index(2).Float64(), caja.Index(3).Float64()
}

func imagenLeyendaCSV(o opcionesLeyendaCSV, anchoPt float64) ([]byte, error) {
	w := int(anchoPt * escalaLeyendaCSV)
	h := int(altoLeyendaCSVPt * escalaLeyendaCSV)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
	x := 6
	if qr := generarQRSeccionSello(o.direccion, h); o.qr && qr != nil {
		b := qr.Bounds()
		y := (h - b.Dy()) / 2
		draw.Draw(img, image.Rect(x, y, x+b.Dx(), y+b.Dy()), qr, b.Min, draw.Src)
		x += b.Dx() + 10
	}
	face, err := nuevaFuenteSelloPAdES(7.5 * escalaLeyendaCSV)
	if err != nil {
		return nil, err
	}
	defer face.Close()
	lineas := partirLineaSello(o.texto, face, w-x-8)
	alto := face.Metrics().Height.Ceil()
	y := (h-alto*len(lineas))/2 + face.Metrics().Ascent.Ceil()
	for _, l := range lineas {
		dibujarTexto(img, x, y, l, color.Black, face)
		y += alto
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
