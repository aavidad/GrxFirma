// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"sync"

	pdfsign "github.com/digitorus/pdfsign/sign"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/f64"

	"grxfirma/internal/adapters/outbound/common/localizador"
)

// Sello visible de GrxFirma: tarjeta blanca con barra de acento en los
// colores corporativos de la Diputación de Granada, emblema opcional,
// identidad del firmante destacada, fecha y motivo, y un QR de verificación
// con su rótulo. El texto se ajusta al tamaño del sello y el contenido se
// apila en los sellos estrechos o verticales.

//go:embed recursos/emblema-firma-dipgra.png
var emblemaFirmaPNG []byte

const motivoPorDefectoPAdES = "Firma electrónica avanzada"

var (
	colorSelloFondo    = color.NRGBA{255, 255, 255, 250}
	colorSelloBorde    = color.NRGBA{190, 202, 211, 255}
	colorSelloAcento   = color.NRGBA{172, 203, 73, 255} // #accb49
	colorSelloNombre   = color.NRGBA{23, 58, 78, 255}   // #173a4e
	colorSelloEtiqueta = color.NRGBA{91, 107, 120, 255}
	colorSelloDetalle  = color.NRGBA{51, 67, 79, 255}
	fuentesSello       struct {
		una              sync.Once
		regular, negrita *opentype.Font
		err              error
	}
)

func caraSello(negrita bool, tam float64) (font.Face, error) {
	fuentesSello.una.Do(func() {
		if fuentesSello.regular, fuentesSello.err = opentype.Parse(goregular.TTF); fuentesSello.err == nil {
			fuentesSello.negrita, fuentesSello.err = opentype.Parse(gobold.TTF)
		}
	})
	if fuentesSello.err != nil {
		return nil, fuentesSello.err
	}
	f := fuentesSello.regular
	if negrita {
		f = fuentesSello.negrita
	}
	return opentype.NewFace(f, &opentype.FaceOptions{Size: tam, DPI: 72, Hinting: font.HintingFull})
}

// lineaSello es una línea lógica del sello antes de partirla.
type lineaSello struct {
	texto    string
	negrita  bool
	relativo float64 // tamaño relativo al de referencia
	color    color.Color
	maxLin   int
}

func lineasSelloModerno(info pdfsign.SignDataSignatureInfo, keepText bool, signerSummary string, estilo estiloTextoSello) []lineaSello {
	nombre, detalle, etiqueta := color.Color(colorSelloNombre), color.Color(colorSelloDetalle), color.Color(colorSelloEtiqueta)
	if estilo.color != nil {
		nombre, detalle, etiqueta = *estilo.color, *estilo.color, *estilo.color
	}
	if texto := strings.TrimSpace(estilo.texto); texto != "" {
		// layer2Text de AutoFirma Java: la web decide el texto; la primera
		// línea se destaca.
		var out []lineaSello
		for i, l := range strings.Split(strings.ReplaceAll(texto, "\r\n", "\n"), "\n") {
			if l = normalizarLineaSello(l); l == "" {
				continue
			}
			if i == 0 {
				out = append(out, lineaSello{texto: l, negrita: true, relativo: 1, color: nombre, maxLin: 2})
			} else {
				out = append(out, lineaSello{texto: l, relativo: 0.78, color: detalle, maxLin: 2})
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	textos := estilo.textos
	if textos == nil {
		textos = localizador.Para("es")
	}
	firmado := textos.T("seal.digitally_signed")
	out := []lineaSello{{texto: firmado, relativo: 0.62, color: etiqueta, maxLin: 1}}
	if !keepText {
		out[0] = lineaSello{texto: firmado, negrita: true, relativo: 1, color: nombre, maxLin: 2}
		return out
	}
	if n := normalizarLineaSello(info.Name); n != "" {
		out = append(out, lineaSello{texto: n, negrita: true, relativo: 1, color: nombre, maxLin: 2})
	}
	if !info.Date.IsZero() {
		out = append(out, lineaSello{texto: textos.T("seal.date", textos.FechaHora(info.Date, estilo.zona, false)), relativo: 0.74, color: detalle, maxLin: 1})
	}
	if r := normalizarLineaSello(info.Reason); r != "" && r != motivoPorDefectoPAdES {
		out = append(out, lineaSello{texto: textos.T("seal.reason", r), relativo: 0.74, color: detalle, maxLin: 2})
	}
	if l := normalizarLineaSello(info.Location); l != "" {
		// «Certificado: <emisor>» y «Certificado digital» son marcas internas
		// del valor /Location que pone el firmador, no texto del sello.
		if emisor, ok := strings.CutPrefix(l, "Certificado: "); ok {
			out = append(out, lineaSello{texto: textos.T("seal.issued_by", emisor), relativo: 0.66, color: etiqueta, maxLin: 1})
		} else if l != "Certificado digital" {
			out = append(out, lineaSello{texto: textos.T("seal.location", l), relativo: 0.74, color: detalle, maxLin: 1})
		}
	}
	if s := normalizarLineaSello(signerSummary); s != "" {
		out = append(out, lineaSello{texto: s, relativo: 0.66, color: etiqueta, maxLin: 2})
	}
	return out
}

type trozoSello struct {
	texto   string
	cara    font.Face
	color   color.Color
	alto    int
	ascenso int
}

// componerTexto parte las líneas para el ancho dado con el tamaño de
// referencia y devuelve los trozos y la altura total.
func componerTexto(lineas []lineaSello, ref float64, ancho int) ([]trozoSello, int, bool, func()) {
	var (
		trozos []trozoSello
		caras  []font.Face
		alto   int
		cabe   = true
	)
	cerrar := func() {
		for _, c := range caras {
			_ = c.Close()
		}
	}
	for _, l := range lineas {
		cara, err := caraSello(l.negrita, math.Max(5, ref*l.relativo))
		if err != nil {
			cabe = false
			continue
		}
		caras = append(caras, cara)
		m := cara.Metrics()
		altoLinea := int(math.Ceil(float64(m.Height.Ceil()) * 1.08))
		partes := partirEnLineas(l.texto, cara, ancho)
		if len(partes) > l.maxLin {
			cabe = false
			partes = partes[:l.maxLin]
			partes[len(partes)-1] = recortarTexto(cara, partes[len(partes)-1]+"…", ancho)
		}
		for _, p := range partes {
			if medirTexto(cara, p) > ancho {
				cabe = false
			}
			trozos = append(trozos, trozoSello{texto: p, cara: cara, color: l.color, alto: altoLinea, ascenso: m.Ascent.Ceil()})
			alto += altoLinea
		}
	}
	return trozos, alto, cabe, cerrar
}

// partirEnLineas reparte las palabras en líneas que caben en el ancho.
func partirEnLineas(s string, cara font.Face, ancho int) []string {
	palabras := strings.Fields(s)
	if len(palabras) == 0 {
		return nil
	}
	var out []string
	actual := palabras[0]
	for _, p := range palabras[1:] {
		if medirTexto(cara, actual+" "+p) <= ancho {
			actual += " " + p
			continue
		}
		out = append(out, actual)
		actual = p
	}
	return append(out, actual)
}

type capaSello uint8

const (
	capaSelloCompleto capaSello = iota
	capaSelloSinLogo
	capaSelloSoloLogo
)

func renderizarSelloModerno(info pdfsign.SignDataSignatureInfo, ancho, alto int, keepText bool, qrContent, signerSummary string, logo image.Image, estilo estiloTextoSello, capa capaSello, opacidadLogo uint8) (*image.RGBA, error) {
	if err := validarPixelesSello(ancho, alto, maxSealRasterEdge, maxSealRasterPixels); err != nil {
		return nil, err
	}
	img := image.NewRGBA(image.Rect(0, 0, ancho, alto))
	menor := float64(minInt(ancho, alto))
	unidad := math.Max(1, menor/100)
	radio := int(math.Max(3, menor*0.07))
	borde := int(math.Max(1, math.Round(unidad*1.2)))
	acento := int(math.Max(3, math.Round(menor*0.045)))
	horizontal := float64(ancho)/float64(alto) >= 1.35

	if capa != capaSelloSoloLogo {
		rectanguloRedondeado(img, img.Bounds(), radio, colorSelloBorde)
		rectanguloRedondeado(img, img.Bounds().Inset(borde), maxInt(1, radio-borde), colorSelloFondo)
	}
	var contenido image.Rectangle
	margen := int(math.Max(4, menor*0.09))
	if horizontal {
		barra := image.Rect(borde, borde+radio/2, borde+acento, alto-borde-radio/2)
		if capa != capaSelloSoloLogo {
			draw.Draw(img, barra, image.NewUniform(colorSelloAcento), image.Point{}, draw.Over)
		}
		contenido = image.Rect(borde+acento+margen, margen, ancho-margen, alto-margen)
	} else {
		barra := image.Rect(borde+radio/2, borde, ancho-borde-radio/2, borde+acento)
		if capa != capaSelloSoloLogo {
			draw.Draw(img, barra, image.NewUniform(colorSelloAcento), image.Point{}, draw.Over)
		}
		contenido = image.Rect(margen, borde+acento+margen, ancho-margen, alto-margen)
	}
	if contenido.Dx() < 10 || contenido.Dy() < 10 {
		return img, nil
	}

	var qr image.Image
	rotuloQR := ""
	if strings.TrimSpace(qrContent) != "" {
		textos := estilo.textos
		if textos == nil {
			textos = localizador.Para("es")
		}
		rotuloQR = textos.T("seal.verify")
	}
	espacio := int(math.Max(3, menor*0.06))
	texto := contenido
	var zonaLogo, zonaQR image.Rectangle
	if horizontal {
		lado := contenido.Dy()
		if logo != nil {
			l := minInt(lado, int(float64(contenido.Dx())*0.26))
			zonaLogo = image.Rect(texto.Min.X, contenido.Min.Y+(lado-l)/2, texto.Min.X+l, contenido.Min.Y+(lado-l)/2+l)
			texto.Min.X += l + espacio
		}
		if rotuloQR != "" {
			l := minInt(lado, int(float64(contenido.Dx())*0.3))
			zonaQR = image.Rect(texto.Max.X-l, contenido.Min.Y, texto.Max.X, contenido.Max.Y)
			texto.Max.X -= l + espacio
		}
	} else {
		lado := contenido.Dx()
		centro := contenido.Min.X + lado/2
		if logo != nil {
			l := minInt(int(float64(lado)*0.6), int(float64(contenido.Dy())*0.26))
			zonaLogo = image.Rect(centro-l/2, texto.Min.Y, centro-l/2+l, texto.Min.Y+l)
			texto.Min.Y += l + espacio
		}
		if rotuloQR != "" {
			l := minInt(int(float64(lado)*0.7), int(float64(contenido.Dy())*0.34))
			zonaQR = image.Rect(centro-l/2, texto.Max.Y-l, centro-l/2+l, texto.Max.Y)
			texto.Max.Y -= l + espacio
		}
	}
	if texto.Dx() < 20 || texto.Dy() < 8 {
		// Sin sitio para el texto: se da prioridad a la identidad.
		texto, zonaLogo, zonaQR = contenido, image.Rectangle{}, image.Rectangle{}
		rotuloQR = ""
	}
	if !zonaLogo.Empty() && capa != capaSelloSinLogo {
		dibujarImagenAjustada(img, logo, zonaLogo, opacidadLogo)
	}
	if capa == capaSelloSoloLogo {
		return img, nil
	}
	if !zonaQR.Empty() {
		cara, err := caraSello(true, math.Max(6, float64(zonaQR.Dx())*0.13))
		if err == nil {
			altoRotulo := cara.Metrics().Height.Ceil()
			ladoQR := minInt(zonaQR.Dx(), zonaQR.Dy()-altoRotulo)
			if ladoQR > 20 {
				qr = generarQRSeccionSello(qrContent, ladoQR+12)
			}
			if qr != nil {
				b := qr.Bounds()
				x := zonaQR.Min.X + (zonaQR.Dx()-ladoQR)/2
				y := zonaQR.Min.Y + (zonaQR.Dy()-ladoQR-altoRotulo)/2
				xdraw.CatmullRom.Scale(img, image.Rect(x, y, x+ladoQR, y+ladoQR), qr, b, draw.Over, nil)
				ancho := medirTexto(cara, rotuloQR)
				dibujarTexto(img, zonaQR.Min.X+(zonaQR.Dx()-ancho)/2, y+ladoQR+cara.Metrics().Ascent.Ceil(), rotuloQR, colorSelloNombre, cara)
			}
			_ = cara.Close()
		}
	}

	// Ajuste del tamaño de letra al espacio disponible.
	lineas := lineasSelloModerno(info, keepText, signerSummary, estilo)
	if !horizontal {
		// En vertical hay poco ancho: más renglones por línea.
		for i := range lineas {
			lineas[i].maxLin += 2
		}
	}
	ref := float64(texto.Dy()) / 3.4
	if !horizontal {
		ref = math.Min(ref, float64(texto.Dx())/6)
	}
	if len(lineas) <= 1 {
		ref = float64(texto.Dy()) / 2.2
	}
	if estilo.tamPuntos > 0 && estilo.escala > 0 {
		ref = estilo.tamPuntos * estilo.escala // layer2FontSize de Java
	}
	var (
		trozos []trozoSello
		total  int
		cerrar = func() {}
	)
	for i := 0; i < 40; i++ {
		cerrar()
		var cabe bool
		trozos, total, cabe, cerrar = componerTexto(lineas, ref, texto.Dx())
		if (cabe && total <= texto.Dy()) || ref <= 5 {
			break
		}
		ref *= 0.92
	}
	defer cerrar()
	y := texto.Min.Y + maxInt(0, (texto.Dy()-total)/2)
	for _, t := range trozos {
		if y+t.alto > texto.Max.Y+t.alto/3 {
			break
		}
		x := texto.Min.X
		if !horizontal {
			x += maxInt(0, (texto.Dx()-medirTexto(t.cara, t.texto))/2)
		}
		dibujarTexto(img, x, y+t.ascenso, t.texto, t.color, t.cara)
		y += t.alto
	}
	return img, nil
}

// rectanguloRedondeado rellena un rectángulo con esquinas redondeadas y
// bordes suavizados.
func rectanguloRedondeado(img *image.RGBA, r image.Rectangle, radio int, c color.NRGBA) {
	if r.Empty() {
		return
	}
	rad := float64(radio)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cx, cy := float64(x)+0.5, float64(y)+0.5
			dx := math.Max(math.Max(float64(r.Min.X)+rad-cx, cx-(float64(r.Max.X)-rad)), 0)
			dy := math.Max(math.Max(float64(r.Min.Y)+rad-cy, cy-(float64(r.Max.Y)-rad)), 0)
			cobertura := 1.0
			if dx > 0 && dy > 0 {
				cobertura = math.Min(1, math.Max(0, rad-math.Hypot(dx, dy)+0.5))
			}
			if cobertura <= 0 {
				continue
			}
			a := uint8(float64(c.A) * cobertura)
			img.Set(x, y, mezclar(img.RGBAAt(x, y), color.NRGBA{c.R, c.G, c.B, a}))
		}
	}
}

func mezclar(fondo color.RGBA, c color.NRGBA) color.RGBA {
	a := float64(c.A) / 255
	fa := float64(fondo.A) / 255
	oa := a + fa*(1-a)
	if oa == 0 {
		return color.RGBA{}
	}
	mix := func(cc, fc uint8) uint8 {
		// fondo en RGBA premultiplicado
		return uint8(math.Round(float64(cc)*a + float64(fc)*(1-a)))
	}
	return color.RGBA{mix(c.R, fondo.R), mix(c.G, fondo.G), mix(c.B, fondo.B), uint8(math.Round(oa * 255))}
}

// rotarImagenLibre gira la imagen el ángulo indicado (grados en sentido
// horario, igual que los giros de 90, 180 y 270 de rotarImagen) sin modificar
// su escala; el lienzo recibido ya es la caja envolvente.
func rotarImagenLibre(src image.Image, grados float64, ancho, alto int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, ancho, alto))
	b := src.Bounds()
	rad := grados * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	w, h := float64(b.Dx()), float64(b.Dy())
	cx, cy := float64(ancho)/2, float64(alto)/2
	// En coordenadas de imagen (y hacia abajo) esta matriz gira en sentido
	// horario alrededor del centro.
	m := f64.Aff3{
		cos, -sin, cx - (cos*w/2 - sin*h/2),
		sin, cos, cy - (sin*w/2 + cos*h/2),
	}
	xdraw.CatmullRom.Transform(dst, m, src, b, draw.Over, nil)
	return dst
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
