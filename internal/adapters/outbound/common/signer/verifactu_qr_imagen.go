// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"strings"
	"time"

	"github.com/makiuchi-d/gozxing"
	multiqr "github.com/makiuchi-d/gozxing/multi/qrcode"
	"github.com/makiuchi-d/gozxing/qrcode"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/ports"
)

// Límites de la lectura del QR desde imagen o PDF. Se comprueban antes de
// decodificar los píxeles para que una imagen pequeña en bytes pero enorme en
// dimensiones (bomba de descompresión) no llegue a reservar memoria.
const (
	// VeriFactuQRMaxImageBytes acota el fichero PNG o JPEG leído del disco
	// o recibido por IPC.
	VeriFactuQRMaxImageBytes = 20 * 1024 * 1024
	// VeriFactuQRMaxPDFPages es el número de páginas que se recorren buscando
	// el QR. El QR tributario va en la factura, normalmente en la primera.
	VeriFactuQRMaxPDFPages = 5

	// Una foto de móvil de 12 Mpx (4032x3024) y una página rasterizada por
	// el visor (lado máximo 4096) caben; un PNG RGBA de 16 bits con este
	// tope ocupa unos 128 MB decodificado.
	vfQRMaxSide     = 8192
	vfQRMaxPixels   = 16_000_000
	vfQRScanSide    = 2500
	vfQRTimeout     = 60 * time.Second
	vfQRPNGMaxBytes = 64 * 1024 * 1024
	// vfQRMaxPuntos acota los patrones candidatos (de localización y de
	// alineación) por intento. Una factura tiene uno o dos QR, con menos de
	// una docena; el tope deja margen para fotos con ruido.
	vfQRMaxPuntos = 120
)

// ErrorQRVeriFactuFuente es el error localizable de una fuente del QR que no
// es una URL, una imagen o un PDF admisibles.
func ErrorQRVeriFactuFuente() error { return vfError("qr_image") }

// LeerQRVeriFactuImagen localiza el QR tributario en una imagen PNG o JPEG y
// devuelve los mismos datos que LeerQRVeriFactu. No hace peticiones de red.
func LeerQRVeriFactuImagen(ctx context.Context, data []byte) (VeriFactuQR, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, vfQRTimeout)
	defer cancel()
	return vfQRDesdeImagen(ctx, data)
}

// LeerQRVeriFactuFichero lee el QR de un fichero PNG, JPEG o PDF. El PDF se
// rasteriza página a página con el mismo visor que la vista previa del sello,
// que aplica sus propios límites de tamaño, resolución y tiempo.
func LeerQRVeriFactuFichero(ctx context.Context, ruta string, visor ports.VisualizadorPDF) (VeriFactuQR, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(ruta) == "" || strings.IndexByte(ruta, 0) >= 0 {
		return VeriFactuQR{}, vfError("qr_image")
	}
	ctx, cancel := context.WithTimeout(ctx, vfQRTimeout)
	defer cancel()
	switch strings.ToLower(filepath.Ext(ruta)) {
	case ".png", ".jpg", ".jpeg":
		data, e := securefile.ReadFileLimit(ruta, VeriFactuQRMaxImageBytes)
		if e != nil {
			return VeriFactuQR{}, vfError("qr_image")
		}
		return vfQRDesdeImagen(ctx, data)
	case ".pdf":
		return vfQRDesdePDF(ctx, ruta, visor)
	default:
		return VeriFactuQR{}, vfError("qr_image")
	}
}

func vfQRDesdePDF(ctx context.Context, ruta string, visor ports.VisualizadorPDF) (VeriFactuQR, error) {
	if visor == nil {
		return VeriFactuQR{}, vfError("qr_pdf")
	}
	// Se comprueba la cabecera antes de lanzar el rasterizador externo.
	cabecera, e := vfQRCabecera(ruta)
	if e != nil || !bytes.HasPrefix(cabecera, []byte("%PDF-")) {
		return VeriFactuQR{}, vfError("qr_image")
	}
	var primerError error
	total := 1
	for pagina := 1; pagina <= total && pagina <= VeriFactuQRMaxPDFPages; pagina++ {
		if ctx.Err() != nil {
			return VeriFactuQR{}, vfError("qr_timeout")
		}
		b64, _, _, paginas, e := visor.RenderizarPagina(ctx, ruta, pagina)
		if e != nil {
			if ctx.Err() != nil {
				return VeriFactuQR{}, vfError("qr_timeout")
			}
			return VeriFactuQR{}, vfError("qr_pdf")
		}
		total = paginas
		if len(b64) > base64.StdEncoding.EncodedLen(vfQRPNGMaxBytes) {
			return VeriFactuQR{}, vfError("qr_pdf")
		}
		png, e := base64.StdEncoding.DecodeString(b64)
		if e != nil {
			return VeriFactuQR{}, vfError("qr_pdf")
		}
		qr, e := vfQRDesdeImagenSinLimiteBytes(ctx, png)
		if e == nil {
			return qr, nil
		}
		if p, ok := e.(vfProblem); ok && (p.Key == "verifactu.qr_timeout" || p.Key == "verifactu.qr_busy" || p.Key == "verifactu.qr_image") {
			return VeriFactuQR{}, e
		}
		if p, ok := e.(vfProblem); ok && p.Key != "verifactu.qr_not_found" && primerError == nil {
			primerError = e
		}
	}
	if primerError != nil {
		return VeriFactuQR{}, primerError
	}
	return VeriFactuQR{}, vfError("qr_not_found")
}

func vfQRCabecera(ruta string) ([]byte, error) {
	f, e := securefile.OpenRead(ruta)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() {
		return nil, errors.New("no regular")
	}
	buf := make([]byte, 1024)
	n, _ := f.Read(buf)
	return buf[:n], nil
}

func vfQRDesdeImagen(ctx context.Context, data []byte) (VeriFactuQR, error) {
	if len(data) == 0 || len(data) > VeriFactuQRMaxImageBytes {
		return VeriFactuQR{}, vfError("qr_image")
	}
	return vfQRDesdeImagenSinLimiteBytes(ctx, data)
}

// vfQRSemaforo deja un único escaneo en curso en todo el proceso. La
// biblioteca de QR no admite cancelación: si vence el plazo, el trabajo
// sigue hasta terminar, y sin este límite cada reintento sumaría otro núcleo
// ocupado y otra imagen en memoria. Lo libera la goroutine al acabar, no
// quien espera, así que una petición que llega mientras sigue en marcha uno
// vencido se rechaza con qr_busy.
var vfQRSemaforo = make(chan struct{}, 1)

// vfQRTrabajo es el trabajo protegido por el semáforo; las pruebas lo
// sustituyen para simular bloqueos y fallos internos.
var vfQRTrabajo = vfQRProcesar

// vfQRDesdeImagenSinLimiteBytes se usa con el PNG que genera el rasterizador,
// cuyo tamaño ya acota el visor; las dimensiones se siguen comprobando.
func vfQRDesdeImagenSinLimiteBytes(ctx context.Context, data []byte) (VeriFactuQR, error) {
	if ctx.Err() != nil {
		return VeriFactuQR{}, vfError("qr_timeout")
	}
	select {
	case vfQRSemaforo <- struct{}{}:
	default:
		return VeriFactuQR{}, vfError("qr_busy")
	}
	type salida struct {
		qr  VeriFactuQR
		err error
	}
	ch := make(chan salida, 1)
	trabajo := vfQRTrabajo
	go func() {
		var s salida
		defer func() {
			// Cubre la decodificación de la imagen, la conversión a gris, la
			// reducción y la propia biblioteca de QR.
			if recover() != nil {
				s = salida{err: vfError("qr_image")}
			}
			<-vfQRSemaforo
			ch <- s
		}()
		s.qr, s.err = trabajo(ctx, data)
	}()
	select {
	case <-ctx.Done():
		return VeriFactuQR{}, vfError("qr_timeout")
	case s := <-ch:
		return s.qr, s.err
	}
}

func vfQRProcesar(ctx context.Context, data []byte) (VeriFactuQR, error) {
	img, e := vfQRDecodificar(data)
	if e != nil {
		return VeriFactuQR{}, e
	}
	return vfQREscanear(ctx, img)
}

// vfQRDecodificar solo admite PNG y JPEG, reconocidos por su firma y no por
// los decodificadores registrados globalmente en image.
func vfQRDecodificar(data []byte) (image.Image, error) {
	var config func([]byte) (image.Config, error)
	var decode func([]byte) (image.Image, error)
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		config = func(b []byte) (image.Config, error) { return png.DecodeConfig(bytes.NewReader(b)) }
		decode = func(b []byte) (image.Image, error) { return png.Decode(bytes.NewReader(b)) }
	case bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}):
		config = func(b []byte) (image.Config, error) { return jpeg.DecodeConfig(bytes.NewReader(b)) }
		decode = func(b []byte) (image.Image, error) { return jpeg.Decode(bytes.NewReader(b)) }
	default:
		return nil, vfError("qr_image")
	}
	cfg, e := config(data)
	if e != nil || !vfQRDimensionesAdmitidas(cfg.Width, cfg.Height) {
		return nil, vfError("qr_image")
	}
	img, e := decode(data)
	if e != nil {
		return nil, vfError("qr_image")
	}
	b := img.Bounds()
	if !vfQRDimensionesAdmitidas(b.Dx(), b.Dy()) {
		return nil, vfError("qr_image")
	}
	return img, nil
}

func vfQRDimensionesAdmitidas(w, h int) bool {
	return w > 0 && h > 0 && w <= vfQRMaxSide && h <= vfQRMaxSide && int64(w)*int64(h) <= vfQRMaxPixels
}

// vfQREscanear prueba primero una versión reducida (más rápida y tolerante al
// ruido de las fotos) y después la resolución original. En cada una usa antes
// el lector de un solo código, que es barato, y solo si no da un QR
// tributario el lector de varios códigos. La biblioteca no admite
// cancelación: el trabajo se acota con el tamaño máximo de la imagen y con
// un tope de patrones candidatos (vfQRMaxPuntos), y se consulta el contexto
// entre intentos.
func vfQREscanear(ctx context.Context, img image.Image) (VeriFactuQR, error) {
	gris := vfQRGris(img)
	candidatas := []*image.Gray{}
	b := gris.Bounds()
	if lado := max(b.Dx(), b.Dy()); lado > vfQRScanSide {
		candidatas = append(candidatas, vfQRReducir(gris, vfQRScanSide))
	}
	candidatas = append(candidatas, gris)
	var primerError error
	for _, c := range candidatas {
		for _, varios := range []bool{false, true} {
			if ctx.Err() != nil {
				return VeriFactuQR{}, vfError("qr_timeout")
			}
			for _, texto := range vfQRDecodificarCodigos(c, varios) {
				qr, e := LeerQRVeriFactu(strings.TrimRight(texto, "\r\n"))
				if e == nil {
					return qr, nil
				}
				if primerError == nil {
					primerError = e
				}
			}
		}
	}
	if primerError != nil {
		return VeriFactuQR{}, primerError
	}
	return VeriFactuQR{}, vfError("qr_not_found")
}

// vfQRDemasiadosPuntos interrumpe la búsqueda cuando la imagen tiene más
// patrones candidatos de los que cabe esperar en una factura.
type vfQRDemasiadosPuntos struct{}

func vfQRDecodificarCodigos(img *image.Gray, varios bool) (textos []string) {
	// Un QR dañado o malicioso no debe tumbar el motor si la biblioteca
	// encuentra un caso no previsto; el tope de puntos también sale por aquí.
	defer func() {
		if recover() != nil {
			textos = nil
		}
	}()
	bmp, e := gozxing.NewBinaryBitmap(gozxing.NewHybridBinarizer(gozxing.NewLuminanceSourceFromImage(img)))
	if e != nil {
		return nil
	}
	// La selección de patrones de la biblioteca crece con el cubo de los
	// candidatos: una imagen con el mismo QR repetido en mosaico la deja
	// minutos al 100 %. Se cuentan con la llamada de retorno y se aborta.
	puntos := 0
	hints := map[gozxing.DecodeHintType]interface{}{
		gozxing.DecodeHintType_TRY_HARDER:    true,
		gozxing.DecodeHintType_CHARACTER_SET: "UTF-8",
		gozxing.DecodeHintType_NEED_RESULT_POINT_CALLBACK: gozxing.ResultPointCallback(func(gozxing.ResultPoint) {
			puntos++
			if puntos > vfQRMaxPuntos {
				panic(vfQRDemasiadosPuntos{})
			}
		}),
	}
	if !varios {
		r, e := qrcode.NewQRCodeReader().Decode(bmp, hints)
		if e != nil || r == nil || len(r.GetText()) > 4096 {
			return nil
		}
		return []string{r.GetText()}
	}
	results, e := multiqr.NewQRCodeMultiReader().DecodeMultiple(bmp, hints)
	if e != nil {
		return nil
	}
	for _, r := range results {
		if r != nil && len(r.GetText()) <= 4096 {
			textos = append(textos, r.GetText())
		}
	}
	return textos
}

func vfQRGris(img image.Image) *image.Gray {
	b := img.Bounds()
	if g, ok := img.(*image.Gray); ok && b.Min == (image.Point{}) {
		return g
	}
	g := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
	if y, ok := img.(*image.YCbCr); ok {
		// JPEG: la luminancia ya está en el plano Y.
		for fila := 0; fila < b.Dy(); fila++ {
			origen := y.YOffset(b.Min.X, b.Min.Y+fila)
			copy(g.Pix[fila*g.Stride:fila*g.Stride+b.Dx()], y.Y[origen:origen+b.Dx()])
		}
		return g
	}
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, gg, bb, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			// RGBA() devuelve valores premultiplicados por alfa: componer
			// sobre blanco es sumar lo que falta de opacidad, así que las
			// zonas transparentes se leen como papel blanco.
			l := (299*r+587*gg+114*bb)/1000 + (0xffff - a)
			g.Pix[y*g.Stride+x] = uint8(min(l, 0xffff) >> 8) // #nosec G115 -- l <= 0xffff, luego l>>8 <= 0xff.
		}
	}
	return g
}

// vfQRReducir promedia bloques para que el lado mayor no supere lado.
func vfQRReducir(src *image.Gray, lado int) *image.Gray {
	b := src.Bounds()
	factor := (max(b.Dx(), b.Dy()) + lado - 1) / lado
	if factor < 2 {
		return src
	}
	w, h := b.Dx()/factor, b.Dy()/factor
	dst := image.NewGray(image.Rect(0, 0, w, h))
	n := uint32(factor * factor) // #nosec G115 -- 2 <= factor <= vfQRMaxSide/vfQRScanSide+1.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var suma uint32
			for dy := 0; dy < factor; dy++ {
				fila := src.Pix[(y*factor+dy)*src.Stride+x*factor:]
				for dx := 0; dx < factor; dx++ {
					suma += uint32(fila[dx])
				}
			}
			dst.Pix[y*dst.Stride+x] = uint8(suma / n) // #nosec G115 -- media de n bytes, <= 0xff.
		}
	}
	return dst
}
