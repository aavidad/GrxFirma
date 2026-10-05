// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	qrcode "github.com/skip2/go-qrcode"
)

// URL de ejemplo de la especificación de la AEAT (entorno de pruebas).
const vfQREjemploAEAT = "https://prewww2.aeat.es/wlpl/TIKE-CONT/ValidarQR?nif=89890001K&numserie=12345678-G33&fecha=01-09-2024&importe=241.4"

func vfQRImagenPrueba(t *testing.T, contenido string, lado, x, y, ancho int) *image.RGBA {
	t.Helper()
	q, e := qrcode.New(contenido, qrcode.Medium)
	if e != nil {
		t.Fatal(e)
	}
	codigo := q.Image(ancho)
	lienzo := image.NewRGBA(image.Rect(0, 0, lado, lado))
	draw.Draw(lienzo, lienzo.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(lienzo, image.Rect(x, y, x+ancho, y+ancho), codigo, image.Point{}, draw.Src)
	return lienzo
}

func vfQRPNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if e := png.Encode(&b, img); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}

func vfQRClave(e error) string {
	var p vfProblem
	if errors.As(e, &p) {
		return p.Key
	}
	return ""
}

func TestLeerQRVeriFactuImagenPNGYJPEG(t *testing.T) {
	img := vfQRImagenPrueba(t, vfQREjemploAEAT, 800, 420, 500, 256)
	qr, e := LeerQRVeriFactuImagen(context.Background(), vfQRPNG(t, img))
	if e != nil {
		t.Fatal(e)
	}
	if qr.NIF != "89890001K" || qr.Number != "12345678-G33" || qr.Date != "01-09-2024" || qr.Amount != "241.4" || !qr.Test || !qr.Verifiable {
		t.Fatalf("%+v", qr)
	}
	var j bytes.Buffer
	if e := jpeg.Encode(&j, img, &jpeg.Options{Quality: 80}); e != nil {
		t.Fatal(e)
	}
	if qr2, e := LeerQRVeriFactuImagen(context.Background(), j.Bytes()); e != nil || qr2 != qr {
		t.Fatalf("jpeg: %+v %v", qr2, e)
	}
}

func TestLeerQRVeriFactuImagenGrandeYTransparente(t *testing.T) {
	// Imagen mayor que el lado de escaneo: se prueba la versión reducida.
	grande := vfQRImagenPrueba(t, vfQREjemploAEAT, 3200, 2000, 2400, 600)
	if _, e := LeerQRVeriFactuImagen(context.Background(), vfQRPNG(t, grande)); e != nil {
		t.Fatal(e)
	}
	// Fondo transparente: debe interpretarse como papel blanco.
	q, _ := qrcode.New(vfQREjemploAEAT, qrcode.Medium)
	q.BackgroundColor = color.Transparent
	var b bytes.Buffer
	if e := q.Write(300, &b); e != nil {
		t.Fatal(e)
	}
	if _, e := LeerQRVeriFactuImagen(context.Background(), b.Bytes()); e != nil {
		t.Fatal(e)
	}
}

func TestLeerQRVeriFactuImagenEligeElQRTributario(t *testing.T) {
	img := vfQRImagenPrueba(t, "https://pagos.example.test/factura/1", 900, 40, 40, 300)
	otro := vfQRImagenPrueba(t, vfQREjemploAEAT, 300, 0, 0, 300)
	draw.Draw(img, image.Rect(560, 560, 860, 860), otro, image.Point{}, draw.Src)
	qr, e := LeerQRVeriFactuImagen(context.Background(), vfQRPNG(t, img))
	if e != nil || qr.NIF != "89890001K" {
		t.Fatalf("%+v %v", qr, e)
	}
}

func TestLeerQRVeriFactuImagenRechazos(t *testing.T) {
	blanca := image.NewGray(image.Rect(0, 0, 400, 400))
	draw.Draw(blanca, blanca.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	casos := map[string]struct {
		data  []byte
		clave string
	}{
		"sin qr":        {vfQRPNG(t, blanca), "verifactu.qr_not_found"},
		"dominio ajeno": {vfQRPNG(t, vfQRImagenPrueba(t, "https://evil.test/wlpl/TIKE-CONT/ValidarQR?nif=89890001K&numserie=A&fecha=01-09-2024&importe=1", 500, 50, 50, 300)), "verifactu.qr_url"},
		"vacio":         {nil, "verifactu.qr_image"},
		"gif":           {[]byte("GIF89a\x01\x00\x01\x00"), "verifactu.qr_image"},
		"png roto":      {[]byte("\x89PNG\r\n\x1a\nbasura"), "verifactu.qr_image"},
		"bomba":         {vfQRPNGConDimensiones(t, 50000, 50000), "verifactu.qr_image"},
		"lado excesivo": {vfQRPNGConDimensiones(t, vfQRMaxSide+1, 10), "verifactu.qr_image"},
		"demasiado":     {append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, VeriFactuQRMaxImageBytes)...), "verifactu.qr_image"},
	}
	for nombre, c := range casos {
		if _, e := LeerQRVeriFactuImagen(context.Background(), c.data); vfQRClave(e) != c.clave {
			t.Errorf("%s: %v (%q)", nombre, e, vfQRClave(e))
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := LeerQRVeriFactuImagen(ctx, vfQRPNG(t, blanca)); vfQRClave(e) != "verifactu.qr_timeout" {
		t.Fatalf("cancelado: %v", e)
	}
}

// vfQRPNGConDimensiones crea un PNG cuya cabecera declara unas dimensiones
// enormes sin datos que las respalden.
func vfQRPNGConDimensiones(t *testing.T, w, h uint32) []byte {
	t.Helper()
	data := vfQRPNG(t, image.NewGray(image.Rect(0, 0, 1, 1)))
	ihdr := data[8+8 : 8+8+13]
	binary.BigEndian.PutUint32(ihdr[0:4], w)
	binary.BigEndian.PutUint32(ihdr[4:8], h)
	binary.BigEndian.PutUint32(data[8+8+13:], crc32.ChecksumIEEE(data[8+4:8+8+13]))
	return data
}

type vfQRVisorFalso struct {
	paginas  map[int][]byte
	total    int
	llamadas []int
	err      error
}

func (v *vfQRVisorFalso) RenderizarPagina(_ context.Context, _ string, pagina int) (string, float64, float64, int, error) {
	v.llamadas = append(v.llamadas, pagina)
	if v.err != nil {
		return "", 0, 0, 0, v.err
	}
	return base64.StdEncoding.EncodeToString(v.paginas[pagina]), 595, 842, v.total, nil
}

func TestLeerQRVeriFactuFichero(t *testing.T) {
	dir := t.TempDir()
	conQR := vfQRPNG(t, vfQRImagenPrueba(t, vfQREjemploAEAT, 600, 300, 300, 256))
	blanca := image.NewGray(image.Rect(0, 0, 300, 300))
	draw.Draw(blanca, blanca.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	sinQR := vfQRPNG(t, blanca)
	escribir := func(nombre string, data []byte) string {
		ruta := filepath.Join(dir, nombre)
		if e := os.WriteFile(ruta, data, 0o600); e != nil {
			t.Fatal(e)
		}
		return ruta
	}
	if qr, e := LeerQRVeriFactuFichero(context.Background(), escribir("factura.png", conQR), nil); e != nil || qr.NIF != "89890001K" {
		t.Fatalf("png: %+v %v", qr, e)
	}
	if _, e := LeerQRVeriFactuFichero(context.Background(), escribir("factura.gif", conQR), nil); vfQRClave(e) != "verifactu.qr_image" {
		t.Fatalf("extensión: %v", e)
	}
	if _, e := LeerQRVeriFactuFichero(context.Background(), escribir("falsa.jpg", []byte("no es imagen")), nil); vfQRClave(e) != "verifactu.qr_image" {
		t.Fatalf("contenido: %v", e)
	}
	if _, e := LeerQRVeriFactuFichero(context.Background(), filepath.Join(dir, "no-existe.png"), nil); vfQRClave(e) != "verifactu.qr_image" {
		t.Fatalf("inexistente: %v", e)
	}

	pdf := escribir("factura.pdf", []byte("%PDF-1.4\n%prueba\n"))
	visor := &vfQRVisorFalso{paginas: map[int][]byte{1: sinQR, 2: conQR}, total: 3}
	if qr, e := LeerQRVeriFactuFichero(context.Background(), pdf, visor); e != nil || qr.Number != "12345678-G33" || len(visor.llamadas) != 2 {
		t.Fatalf("pdf: %+v %v %v", qr, e, visor.llamadas)
	}
	visor = &vfQRVisorFalso{paginas: map[int][]byte{}, total: 40}
	for i := 1; i <= 40; i++ {
		visor.paginas[i] = sinQR
	}
	if _, e := LeerQRVeriFactuFichero(context.Background(), pdf, visor); vfQRClave(e) != "verifactu.qr_not_found" || len(visor.llamadas) != VeriFactuQRMaxPDFPages {
		t.Fatalf("límite de páginas: %v %v", e, visor.llamadas)
	}
	if _, e := LeerQRVeriFactuFichero(context.Background(), pdf, nil); vfQRClave(e) != "verifactu.qr_pdf" {
		t.Fatalf("sin visor: %v", e)
	}
	if _, e := LeerQRVeriFactuFichero(context.Background(), pdf, &vfQRVisorFalso{err: errors.New("pdftoppm")}); vfQRClave(e) != "verifactu.qr_pdf" {
		t.Fatalf("visor con error: %v", e)
	}
	noPDF := escribir("disfrazado.pdf", conQR)
	visor = &vfQRVisorFalso{paginas: map[int][]byte{1: conQR}, total: 1}
	if _, e := LeerQRVeriFactuFichero(context.Background(), noPDF, visor); vfQRClave(e) != "verifactu.qr_image" || len(visor.llamadas) != 0 {
		t.Fatalf("cabecera: %v", e)
	}
}
