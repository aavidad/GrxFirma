// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

// vfQRMosaico repite el mismo QR por toda la imagen: cada copia aporta tres
// patrones de localización y la selección de la biblioteca crece con su cubo.
func vfQRMosaico(t *testing.T, lado int) []byte {
	t.Helper()
	q, e := qrcode.New("https://example.com/no-aeat?x=1", qrcode.Low)
	if e != nil {
		t.Fatal(e)
	}
	q.DisableBorder = false
	bits := q.Bitmap()
	modulo := 3
	paso := len(bits) * modulo
	img := image.NewGray(image.Rect(0, 0, lado, lado))
	for y := 0; y < lado; y++ {
		fila := bits[(y%paso)/modulo]
		for x := 0; x < lado; x++ {
			v := uint8(255)
			if fila[(x%paso)/modulo] {
				v = 0
			}
			img.Pix[y*img.Stride+x] = v
		}
	}
	var b bytes.Buffer
	if e := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&b, img); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}

func vfQRSemaforoLibre(t *testing.T) {
	t.Helper()
	limite := time.Now().Add(5 * time.Second)
	for len(vfQRSemaforo) != 0 {
		if time.Now().After(limite) {
			t.Fatal("el escaneo no ha liberado el semáforo")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Con el lector de varios códigos y TRY_HARDER sin tope, esta imagen tenía
// varios núcleos ocupados durante minutos después de devolver qr_timeout.
func TestLeerQRVeriFactuImagenMosaicoAcotado(t *testing.T) {
	data := vfQRMosaico(t, 4000)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	inicio := time.Now()
	_, e := LeerQRVeriFactuImagen(ctx, data)
	if clave := vfQRClave(e); clave != "verifactu.qr_not_found" && clave != "verifactu.qr_url" {
		t.Fatalf("mosaico: %v (%q) en %s", e, clave, time.Since(inicio))
	}
	// El trabajo ha terminado de verdad: el semáforo queda libre enseguida.
	vfQRSemaforoLibre(t)
}

func TestLeerQRVeriFactuImagenDimensiones(t *testing.T) {
	admitidas := [][2]int{{4032, 3024}, {2896, 4096}, {4000, 4000}, {8192, 1500}}
	for _, d := range admitidas {
		if !vfQRDimensionesAdmitidas(d[0], d[1]) {
			t.Errorf("%dx%d debería admitirse", d[0], d[1])
		}
	}
	// 6000x6000 en RGBA de 16 bits superaba los 600 MB al decodificarse.
	rechazadas := [][2]int{{6000, 6000}, {4001, 4001}, {8193, 100}, {0, 10}}
	for _, d := range rechazadas {
		if vfQRDimensionesAdmitidas(d[0], d[1]) {
			t.Errorf("%dx%d debería rechazarse", d[0], d[1])
		}
	}
	inicio := time.Now()
	if _, e := LeerQRVeriFactuImagen(context.Background(), vfQRMosaico(t, 6000)); vfQRClave(e) != "verifactu.qr_image" || time.Since(inicio) > 10*time.Second {
		t.Fatalf("6000x6000: %v en %s", e, time.Since(inicio))
	}
}

// Mientras un escaneo vencido sigue en marcha, las peticiones nuevas se
// rechazan en lugar de sumar otro núcleo y otra imagen en memoria.
func TestLeerQRVeriFactuImagenUnEscaneoALaVez(t *testing.T) {
	soltar := make(chan struct{})
	original := vfQRTrabajo
	vfQRTrabajo = func(context.Context, []byte) (VeriFactuQR, error) {
		<-soltar
		return VeriFactuQR{}, vfError("qr_not_found")
	}
	defer func() { vfQRTrabajo = original }()
	cerrado := false
	defer func() {
		if !cerrado {
			close(soltar)
		}
	}()

	png := vfQRPNG(t, vfQRImagenPrueba(t, vfQREjemploAEAT, 400, 50, 50, 256))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, e := LeerQRVeriFactuImagen(ctx, png); vfQRClave(e) != "verifactu.qr_timeout" || len(vfQRSemaforo) != 1 {
		t.Fatalf("primera: %v", e)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	if _, e := LeerQRVeriFactuImagen(ctx2, png); vfQRClave(e) != "verifactu.qr_busy" {
		t.Fatalf("segunda: %v", e)
	}
	// El recorrido de un PDF tampoco sigue con más páginas.
	dir := t.TempDir()
	pdf := filepath.Join(dir, "factura.pdf")
	if e := os.WriteFile(pdf, []byte("%PDF-1.7\n"), 0o600); e != nil {
		t.Fatal(e)
	}
	visor := &vfQRVisorFalso{paginas: map[int][]byte{1: png, 2: png}, total: 2}
	if _, e := LeerQRVeriFactuFichero(context.Background(), pdf, visor); vfQRClave(e) != "verifactu.qr_busy" || len(visor.llamadas) != 1 {
		t.Fatalf("pdf: %v %v", e, visor.llamadas)
	}

	close(soltar)
	cerrado = true
	vfQRSemaforoLibre(t)
	vfQRTrabajo = original
	if qr, e := LeerQRVeriFactuImagen(context.Background(), png); e != nil || qr.NIF != "89890001K" {
		t.Fatalf("tras liberar: %+v %v", qr, e)
	}
}

func TestLeerQRVeriFactuImagenPanicoInterno(t *testing.T) {
	original := vfQRTrabajo
	vfQRTrabajo = func(context.Context, []byte) (VeriFactuQR, error) { panic("decodificador") }
	defer func() { vfQRTrabajo = original }()
	if _, e := LeerQRVeriFactuImagen(context.Background(), []byte("\x89PNG\r\n\x1a\nx")); vfQRClave(e) != "verifactu.qr_image" {
		t.Fatalf("pánico: %v", e)
	}
	vfQRSemaforoLibre(t)
}
