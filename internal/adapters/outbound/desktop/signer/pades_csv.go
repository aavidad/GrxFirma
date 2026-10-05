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
	if len(codigo) > maxLongitudCSV || strings.ContainsAny(codigo, "\r\n\t") {
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
	if len(texto) > maxLongitudCSVTexto || strings.ContainsAny(texto, "\r\n\t") {
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

// leyendaCSV devuelve un sello por página con la leyenda CSV.
func leyendaCSV(options map[string]string, pdfData []byte) ([]pdfsign.StampImage, error) {
	opciones, activa, err := leerOpcionesLeyendaCSV(options)
	if err != nil || !activa {
		return nil, err
	}
	r, err := abrirPDF(pdfData, options)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer el PDF: %w", err)
	}
	total := r.NumPage()
	imagenes := map[int][]byte{} // por ancho en puntos, para no repetir el renderizado
	out := make([]pdfsign.StampImage, 0, total)
	for p := 1; p <= total; p++ {
		x0, y0, x1, _ := cajaPagina(r.Page(p))
		ancho := x1 - x0 - 2*margenLeyendaCSVPt
		if ancho < 120 {
			return nil, fmt.Errorf("la página %d es demasiado estrecha para la leyenda CSV", p)
		}
		clave := int(ancho)
		if _, ok := imagenes[clave]; !ok {
			if imagenes[clave], err = imagenLeyendaCSV(opciones, ancho); err != nil {
				return nil, err
			}
		}
		rect := [4]float64{x0 + margenLeyendaCSVPt, y0 + 6, x0 + margenLeyendaCSVPt + ancho, y0 + 6 + altoLeyendaCSVPt}
		out = append(out, pdfsign.StampImage{Page: uint32(p), Rect: rect, Image: imagenes[clave]}) // #nosec G115 -- 1 <= p <= NumPage.
	}
	return out, nil
}

// cajaPagina devuelve la MediaBox de la página (heredable) o A4 por defecto.
func cajaPagina(p pdf.Page) (x0, y0, x1, y1 float64) {
	caja := p.V.Key("MediaBox")
	for n := p.V.Key("Parent"); caja.Len() != 4 && !n.IsNull(); n = n.Key("Parent") {
		caja = n.Key("MediaBox")
	}
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
