package sign

// Prueba de regresión de E2-04: la imagen del sello se rechaza por sus
// dimensiones antes de decodificarla.

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"runtime"
	"testing"
)

// pngConCabecera devuelve un PNG en escala de grises cuya cabecera declara
// ancho x alto, con datos de imagen mínimos.
func pngConCabecera(t *testing.T, ancho, alto uint32) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	data := b.Bytes()
	// Firma (8) + longitud (4) + "IHDR" (4): ancho y alto empiezan en 16.
	binary.BigEndian.PutUint32(data[16:20], ancho)
	binary.BigEndian.PutUint32(data[20:24], alto)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	return data
}

func TestLimites_ImagenSelloConDimensionesEnormes(t *testing.T) {
	data := pngConCabecera(t, 30000, 30000)
	if cfg, err := png.DecodeConfig(bytes.NewReader(data)); err != nil || cfg.Width != 30000 {
		t.Fatalf("PNG de prueba mal construido: %v %+v", err, cfg)
	}
	runtime.GC()
	var antes, despues runtime.MemStats
	runtime.ReadMemStats(&antes)
	context := &SignContext{}
	_, _, err := context.createImageXObject(data)
	runtime.ReadMemStats(&despues)
	if err == nil {
		t.Fatal("se aceptó una imagen de 30000 x 30000 píxeles")
	}
	if reservado := despues.TotalAlloc - antes.TotalAlloc; reservado > 64<<20 {
		t.Fatalf("se reservaron %d MiB antes de rechazar la imagen", reservado>>20)
	}
}
