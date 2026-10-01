// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	pdfsign "github.com/digitorus/pdfsign/sign"
)

func TestAparienciaSelloRespetaCajaGiradayTarjetaRigida(t *testing.T) {
	t.Parallel()
	for _, region := range []struct {
		name string
		w, h float64
	}{
		{"horizontal", 220, 70},
		{"alto_minimo", 80, 10},
		{"vertical_estrecho", 35, 180},
	} {
		for _, rotation := range []int{0, 15, 30, 80, 90, 270} {
			t.Run(region.name+"_"+strconv.Itoa(rotation), func(t *testing.T) {
				data := pdfsign.SignData{Signature: pdfsign.SignDataSignature{Info: testInfoPAdES()}}
				data.Signature.Info.Name = "Identidad sintética con nombre y apellidos especialmente largos para comprobar la composición"
				err := aplicarOpcionesAparienciaPdfsign(&data, map[string]string{
					"visibleSeal": "true", "visibleSealRectX": "12", "visibleSealRectY": "0",
					"visibleSealRectW": strconv.FormatFloat(region.w, 'f', -1, 64),
					"visibleSealRectH": strconv.FormatFloat(region.h, 'f', -1, 64),
					"rotation":         strconv.Itoa(rotation),
				})
				if err != nil {
					t.Fatal(err)
				}
				a := data.Appearance
				expected, err := cajaSelloGirado(12, 0, region.w, region.h, float64(rotation), 0, 0, 0, 0)
				if err != nil {
					t.Fatal(err)
				}
				for i, got := range []float64{a.LowerLeftX, a.LowerLeftY, a.UpperRightX, a.UpperRightY} {
					if math.Abs(got-expected[i]) > 0.001 {
						t.Fatalf("caja PDF[%d]=%g, esperada %g", i, got, expected[i])
					}
				}
				img, err := png.Decode(bytes.NewReader(a.Image))
				if err != nil {
					t.Fatal(err)
				}
				got := float64(img.Bounds().Dx()) / float64(img.Bounds().Dy())
				boxW, boxH := expected[2]-expected[0], expected[3]-expected[1]
				if math.Abs(got-boxW/boxH) > 0.03 {
					t.Fatalf("proporción imagen=%g, caja PDF=%g", got, boxW/boxH)
				}
				// Desgirar todos los píxeles visibles debe recuperar la tarjeta
				// elegida, con la misma proporción y sin reducción de escala.
				minX, minY := math.Inf(1), math.Inf(1)
				maxX, maxY := math.Inf(-1), math.Inf(-1)
				r := float64(rotation) * math.Pi / 180
				for py := 0; py < img.Bounds().Dy(); py++ {
					for px := 0; px < img.Bounds().Dx(); px++ {
						_, _, _, alpha := img.At(px, py).RGBA()
						if alpha < 32000 {
							continue
						}
						dx := (float64(px)+0.5)/float64(img.Bounds().Dx())*boxW - boxW/2
						dy := (float64(py)+0.5)/float64(img.Bounds().Dy())*boxH - boxH/2
						ux := dx*math.Cos(r) + dy*math.Sin(r)
						uy := -dx*math.Sin(r) + dy*math.Cos(r)
						minX, maxX = math.Min(minX, ux), math.Max(maxX, ux)
						minY, maxY = math.Min(minY, uy), math.Max(maxY, uy)
					}
				}
				if math.Abs((maxX-minX)/region.w-1) > 0.1 || math.Abs((maxY-minY)/region.h-1) > 0.1 {
					t.Fatalf("tarjeta desgirada mide %.1f×%.1f, esperada %.1f×%.1f", maxX-minX, maxY-minY, region.w, region.h)
				}
			})
		}
	}
}

func TestCajaSelloGiradoDentroDePagina(t *testing.T) {
	rect, err := cajaSelloGirado(500, 760, 220, 80, 30, 0, 0, 595, 842)
	if err != nil {
		t.Fatal(err)
	}
	if rect[0] < 0 || rect[1] < 0 || rect[2] > 595 || rect[3] > 842 {
		t.Fatalf("caja fuera de página: %v", rect)
	}
	if math.Abs(rect[2]-rect[0]-(220*math.Cos(math.Pi/6)+80*math.Sin(math.Pi/6))) > 0.001 {
		t.Fatalf("la caja cambió de tamaño: %v", rect)
	}
	if _, err := cajaSelloGirado(0, 0, 900, 80, 90, 0, 0, 595, 842); err == nil {
		t.Fatal("debe avisar si la tarjeta girada no cabe")
	}
}

func TestSelloEnCropBoxConOrigenNoCero(t *testing.T) {
	data := pdfsign.SignData{Signature: pdfsign.SignDataSignature{Info: testInfoPAdES()}}
	err := aplicarOpcionesAparienciaPdfsign(&data, map[string]string{
		"visibleSeal": "true", "visibleSealRectX": "0", "visibleSealRectY": "0",
		"visibleSealRectW": "220", "visibleSealRectH": "80", "rotation": "30",
		"visibleSealPageX": "20", "visibleSealPageY": "40",
		"visibleSealPageWidth": "400", "visibleSealPageHeight": "600",
		"visibleSealRectRelativeToCrop": "true",
	})
	if err != nil {
		t.Fatal(err)
	}
	if data.Appearance.LowerLeftX < 20 || data.Appearance.LowerLeftY < 40 {
		t.Fatalf("la caja sale del CropBox: %+v", data.Appearance)
	}
}

func TestGiroLibreConLogoYOpacidad(t *testing.T) {
	options := map[string]string{
		"rotation": "80", "visibleSealLogo": "institucional",
		"visibleSealLogoOpacityPercent": "40",
	}
	data, err := componerImagenSello(testInfoPAdES(), options, 220, 80)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	box, err := cajaSelloGirado(0, 0, 220, 80, 80, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(float64(img.Bounds().Dx())/float64(img.Bounds().Dy())-(box[2]-box[0])/(box[3]-box[1])) > 0.02 {
		t.Fatalf("el logo y la tarjeta no comparten caja: %v", img.Bounds())
	}
	var maxAlpha uint32
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			_, _, _, alpha := img.At(x, y).RGBA()
			maxAlpha = max(maxAlpha, alpha)
		}
	}
	if maxAlpha < 25000 || maxAlpha > 27000 {
		t.Fatalf("opacidad del conjunto girado=%d; se esperaba el 40 %%", maxAlpha)
	}
}

func TestImagenesCompuestasParaRevision(t *testing.T) {
	dir := os.Getenv("GRXFIRMA_SEAL_REVIEW_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, grados := range []int{0, 15, 30, 80, 90} {
		options := map[string]string{
			"rotation": strconv.Itoa(grados), "visibleSealLogo": "institucional",
			"visibleSealLogoOpacityPercent": "65",
		}
		img, err := componerImagenSello(testInfoPAdES(), options, 220, 80)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("despues-%d.png", grados)), img, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOpacidadSelloAfectaTodaLaVistaPrevia(t *testing.T) {
	logo := image.NewRGBA(image.Rect(0, 0, 24, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			logo.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, logo); err != nil {
		t.Fatal(err)
	}
	var imagenSinLogo []byte
	for _, withLogo := range []bool{false, true} {
		options := map[string]string{
			"visibleSeal": "true", "visibleSealRectW": "220", "visibleSealRectH": "70",
			"visibleSealLogoOpacityPercent": "40",
		}
		if withLogo {
			options["visibleSealImageBase64"] = base64.StdEncoding.EncodeToString(encoded.Bytes())
		}
		data := pdfsign.SignData{Signature: pdfsign.SignDataSignature{Info: testInfoPAdES()}}
		if err := aplicarOpcionesAparienciaPdfsign(&data, options); err != nil {
			t.Fatal(err)
		}
		if data.Appearance.SealOpacityPercent == nil || *data.Appearance.SealOpacityPercent != 40 || len(data.Appearance.Image) == 0 {
			t.Fatalf("opacidad no propagada al motor: logo=%t", withLogo)
		}
		if withLogo {
			if bytes.Equal(imagenSinLogo, data.Appearance.Image) {
				t.Fatal("el logo no se compuso en la imagen del sello")
			}
		} else {
			imagenSinLogo = data.Appearance.Image
		}
		preview, err := componerImagenSello(testInfoPAdES(), options, 220, 70)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(preview))
		if err != nil {
			t.Fatal(err)
		}
		var maxAlpha uint32
		for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
			for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
				_, _, _, alpha := img.At(x, y).RGBA()
				if alpha > maxAlpha {
					maxAlpha = alpha
				}
			}
		}
		if maxAlpha < 25000 || maxAlpha > 27000 {
			t.Fatalf("el fondo, texto o logo de la vista previa superan el 40 %%, logo=%t, alfa=%d", withLogo, maxAlpha)
		}
	}
	for _, raw := range []string{"-1", "101", "nan", "30.5"} {
		if _, err := componerImagenSello(testInfoPAdES(), map[string]string{"visibleSealLogoOpacityPercent": raw}, 220, 70); err == nil {
			t.Errorf("no se rechazó la opacidad %q", raw)
		}
	}
}

func TestFuenteSelloIncluyeCaracteresDeNombresEnCastellano(t *testing.T) {
	face, err := nuevaFuenteSelloPAdES(24)
	if err != nil {
		t.Fatal(err)
	}
	defer face.Close()
	for _, r := range "ÁÉÍÓÚÜÑáéíóúüñ" {
		if _, ok := face.GlyphAdvance(r); !ok {
			t.Errorf("la fuente no puede representar %q", r)
		}
	}
}

func TestMarcaDeAguaSelloConservaCanalesPremultiplicados(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			source.SetRGBA(x, y, color.RGBA{R: 255, G: 128, A: 255})
		}
	}
	dest := image.NewRGBA(source.Bounds())
	dibujarImagenAjustada(dest, source, dest.Bounds(), 78)
	pixel := dest.RGBAAt(1, 1)
	if pixel.A != 78 || pixel.R > pixel.A || pixel.G > pixel.A || pixel.B > pixel.A {
		t.Fatalf("la transparencia cambió el color del logo: %#v", pixel)
	}
}

func TestImagenSelloDesdeOpciones(t *testing.T) {
	t.Parallel()

	raw := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	got, ok, err := imagenSelloDesdeOpciones(map[string]string{
		"visibleSealImageBase64": "data:image/png;base64," + raw,
	})
	if err != nil {
		t.Fatalf("imagenSelloDesdeOpciones() error = %v", err)
	}
	if !ok {
		t.Fatal("se esperaba imagen presente")
	}
	if string(got) != "png-bytes" {
		t.Fatalf("imagen inesperada: %q", string(got))
	}
}

func TestImagenSelloDesdeOpciones_Invalida(t *testing.T) {
	t.Parallel()

	_, _, err := imagenSelloDesdeOpciones(map[string]string{
		"visibleSealImageBase64": "%%%not-base64%%%",
	})
	if err == nil {
		t.Fatal("se esperaba error por base64 inválido")
	}
}

func TestGenerarImagenSelloPAdES_SinTextoPersonal(t *testing.T) {
	t.Parallel()

	imgConTexto, err := generarImagenSelloPAdES(testInfoPAdES(), 220, 70, true, "", "", nil, estiloTextoSello{})
	if err != nil {
		t.Fatalf("generarImagenSelloPAdES(con texto) error = %v", err)
	}
	imgSinTexto, err := generarImagenSelloPAdES(testInfoPAdES(), 220, 70, false, "", "", nil, estiloTextoSello{})
	if err != nil {
		t.Fatalf("generarImagenSelloPAdES(sin texto) error = %v", err)
	}
	if len(imgSinTexto) == 0 {
		t.Fatal("se esperaba imagen de sello")
	}
	if len(imgSinTexto) >= len(imgConTexto) {
		t.Fatalf("se esperaba una imagen más compacta sin texto personal: con=%d sin=%d", len(imgConTexto), len(imgSinTexto))
	}
}

func TestRecortarBordesTransparentes(t *testing.T) {
	t.Parallel()

	src := image.NewRGBA(image.Rect(0, 0, 12, 10))
	for y := 3; y < 8; y++ {
		for x := 4; x < 9; x++ {
			src.SetRGBA(x, y, color.RGBA{R: 220, A: 255})
		}
	}

	cropped := recortarBordesTransparentes(src)
	if got, want := cropped.Bounds().Dx(), 5; got != want {
		t.Fatalf("ancho recortado = %d, want %d", got, want)
	}
	if got, want := cropped.Bounds().Dy(), 5; got != want {
		t.Fatalf("alto recortado = %d, want %d", got, want)
	}
}

func TestGenerarImagenSelloPAdES_ConLogoPersonalizado(t *testing.T) {
	t.Parallel()

	logo := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 30; y < 70; y++ {
		for x := 20; x < 80; x++ {
			logo.SetRGBA(x, y, color.RGBA{R: 200, G: 16, B: 16, A: 255})
		}
	}
	var logoRaw bytes.Buffer
	if err := png.Encode(&logoRaw, logo); err != nil {
		t.Fatalf("png.Encode() error = %v", err)
	}

	out, err := generarImagenSelloPAdES(testInfoPAdES(), 220, 70, true, "", "", logoRaw.Bytes(), estiloTextoSello{})
	if err != nil {
		t.Fatalf("generarImagenSelloPAdES(con logo) error = %v", err)
	}
	if len(out) == 0 {
		t.Fatal("se esperaba sello compuesto con logo")
	}
	decoded, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("image.Decode() error = %v", err)
	}
	if got := decoded.Bounds().Dx(); got < 200 {
		t.Fatalf("ancho inesperadamente pequeño = %d", got)
	}
	// El logo ocupa su propia columna a la izquierda y no se superpone al
	// texto: debe haber rojo en el primer tercio y no en el centro.
	b := decoded.Bounds()
	rojo := func(x0, x1 int) bool {
		for y := b.Min.Y; y < b.Max.Y; y += 2 {
			for x := x0; x < x1; x += 2 {
				r, g, bb, a := decoded.At(x, y).RGBA()
				if a > 0x8000 && r > 0x9000 && g < 0x5000 && bb < 0x5000 {
					return true
				}
			}
		}
		return false
	}
	if !rojo(b.Min.X, b.Min.X+b.Dx()/3) {
		t.Fatal("se esperaba el logo en la columna izquierda del sello")
	}
	if rojo(b.Min.X+b.Dx()/2, b.Max.X) {
		t.Fatal("el logo no debe superponerse al texto")
	}
}

func TestGenerarImagenSelloPAdES_ConLogoYQR(t *testing.T) {
	t.Parallel()

	logo := image.NewRGBA(image.Rect(0, 0, 120, 100))
	for y := 20; y < 80; y++ {
		for x := 25; x < 95; x++ {
			logo.SetRGBA(x, y, color.RGBA{R: 220, G: 30, B: 30, A: 255})
		}
	}
	var logoRaw bytes.Buffer
	if err := png.Encode(&logoRaw, logo); err != nil {
		t.Fatalf("png.Encode() error = %v", err)
	}

	out, err := generarImagenSelloPAdES(testInfoPAdES(), 240, 72, true, "https://verifica.ejemplo/", "", logoRaw.Bytes(), estiloTextoSello{})
	if err != nil {
		t.Fatalf("generarImagenSelloPAdES(con logo y QR) error = %v", err)
	}
	if len(out) == 0 {
		t.Fatal("se esperaba sello compuesto con logo y QR")
	}
}

func TestGenerarImagenSelloPAdES_ConResumenFirmantes(t *testing.T) {
	t.Parallel()

	out, err := generarImagenSelloPAdES(testInfoPAdES(), 260, 76, true, "", "Firmantes: Principal | Cofirmantes: Otro, Tercero", nil, estiloTextoSello{})
	if err != nil {
		t.Fatalf("generarImagenSelloPAdES(con resumen de firmantes) error = %v", err)
	}
	if len(out) == 0 {
		t.Fatal("se esperaba sello compuesto con resumen de firmantes")
	}
}

func TestCodigoQRSelloDesdeOpciones(t *testing.T) {
	t.Parallel()

	got := codigoQRSelloDesdeOpciones(map[string]string{
		"qrContent": "https://verifica.ejemplo/",
	})
	if got != "https://verifica.ejemplo/" {
		t.Fatalf("qrContent = %q", got)
	}
}

func TestNormalizarURLQRSello(t *testing.T) {
	for _, tc := range []struct {
		input, want string
		valid       bool
	}{
		{"verifica.ejemplo/ruta", "https://verifica.ejemplo/ruta", true},
		{"https://verifica.ejemplo/ruta", "https://verifica.ejemplo/ruta", true},
		{"http://verifica.ejemplo", "", false},
		{"ftp://verifica.ejemplo", "", false},
		{"https://usuario:clave@verifica.ejemplo", "", false},
		{"verifica.ejemplo/espacio malo", "", false},
	} {
		got, err := normalizarURLQRSello(tc.input)
		if (err == nil) != tc.valid || got != tc.want {
			t.Errorf("entrada %q: URL=%q error=%v", tc.input, got, err)
		}
	}
}

func TestNormalizarQROpciones_RechazaAliasContradictorios(t *testing.T) {
	options := map[string]string{"qrContent": "verifica.ejemplo/a", "QRCONTENT": "http://verifica.ejemplo/b"}
	if err := normalizarQROpciones(options); err == nil {
		t.Fatal("se aceptó un segundo QR inseguro")
	}
	options = map[string]string{"visibleSealQRContent": "verifica.ejemplo/a"}
	if err := normalizarQROpciones(options); err != nil || options["qrContent"] != "https://verifica.ejemplo/a" {
		t.Fatalf("alias QR no normalizado: opciones=%v error=%v", options, err)
	}
	data := pdfsign.SignData{Signature: pdfsign.SignDataSignature{Info: testInfoPAdES()}}
	if err := aplicarOpcionesAparienciaPdfsign(&data, map[string]string{
		"visibleSeal": "true", "qrContent": "http://verifica.ejemplo/",
	}); err == nil {
		t.Fatal("la firma aceptó un QR HTTP")
	}
}

func TestPosicionesSelloRechazaEntradasFueraDePagina(t *testing.T) {
	fixture := filepath.Join("..", "..", "..", "..", "..", "test", "regression", "fixtures", "v1", "samples", "2.pdf")
	pdf, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		``, `[]`, `[{"page":16,"rect":{"x":0,"y":0,"w":0.2,"h":0.1},"rotation":0}]`,
		`[{"page":1,"rect":{"x":0.9,"y":0,"w":0.2,"h":0.1},"rotation":0}]`,
		`[{"page":1,"rect":{"x":0,"y":0,"w":0.2,"h":0.1},"rotation":360}]`,
		`[{"page":1,"rect":{"x":0,"y":0,"w":0.2,"h":0.1},"rotation":0},{"page":1,"rect":{"x":0,"y":0,"w":0.2,"h":0.1},"rotation":0}]`,
	} {
		data := pdfsign.SignData{Signature: pdfsign.SignDataSignature{Info: testInfoPAdES()}}
		if err := aplicarPosicionesSello(&data, map[string]string{"visibleSealPlacements": raw}, pdf); err == nil {
			t.Errorf("lista inválida aceptada: %s", raw)
		}
	}
	data := pdfsign.SignData{Signature: pdfsign.SignDataSignature{Info: testInfoPAdES()}}
	if err := aplicarPosicionesSello(&data, map[string]string{
		"visibleSealPlacements": `[{"page":1,"rect":{"x":0,"y":0,"w":0.2,"h":0.1},"rotation":0}]`,
		"signatureField":        "FirmaExistente",
	}, pdf); err == nil {
		t.Fatal("se aceptó combinar lista de sellos y campo existente")
	}
}

func TestValorBoolOpcionPAdES(t *testing.T) {
	t.Parallel()

	if !valorBoolOpcionPAdES(map[string]string{"k": "sí"}, "k", false) {
		t.Fatal("se esperaba true para 'sí'")
	}
	if valorBoolOpcionPAdES(map[string]string{"k": "off"}, "k", true) {
		t.Fatal("se esperaba false para 'off'")
	}
	if !valorBoolOpcionPAdES(map[string]string{"k": "desconocido"}, "k", true) {
		t.Fatal("se esperaba fallback para valor desconocido")
	}
}

func TestParseVisibleSealPageSelection_All(t *testing.T) {
	t.Parallel()

	allPages, pages, err := parseVisibleSealPageSelection("all")
	if err != nil {
		t.Fatalf("parseVisibleSealPageSelection(all) error = %v", err)
	}
	if !allPages {
		t.Fatal("se esperaba allPages=true")
	}
	if len(pages) != 0 {
		t.Fatalf("no se esperaban páginas explícitas, obtenido %v", pages)
	}
}

func TestParseVisibleSealPageSelection_RangosYDuplicados(t *testing.T) {
	t.Parallel()

	allPages, pages, err := parseVisibleSealPageSelection("1,3-5,4,2")
	if err != nil {
		t.Fatalf("parseVisibleSealPageSelection() error = %v", err)
	}
	if allPages {
		t.Fatal("no se esperaba selección completa")
	}
	want := []uint32{1, 3, 4, 5, 2}
	if len(pages) != len(want) {
		t.Fatalf("longitud inesperada: got=%v want=%v", pages, want)
	}
	for i := range want {
		if pages[i] != want[i] {
			t.Fatalf("páginas inesperadas: got=%v want=%v", pages, want)
		}
	}
}

func TestParseVisibleSealPageSelection_RechazaRangoDescomunal(t *testing.T) {
	t.Parallel()

	allPages, pages, err := parseVisibleSealPageSelection("1-4294967295")
	if err == nil {
		t.Fatal("se esperaba error para un rango que agotaría recursos")
	}
	if allPages || pages != nil {
		t.Fatalf("resultado parcial inesperado: allPages=%v pages=%v", allPages, pages)
	}
}

func TestParseVisibleSealPageSelection_RechazaOverflow(t *testing.T) {
	t.Parallel()

	for _, selection := range []string{
		"4294967296",
		"1-4294967296",
		"4294967296-4294967296",
	} {
		selection := selection
		t.Run(selection, func(t *testing.T) {
			t.Parallel()

			if _, _, err := parseVisibleSealPageSelection(selection); err == nil {
				t.Fatalf("se esperaba error para %q", selection)
			}
		})
	}
}

func TestParseVisibleSealPageSelection_RechazaDemasiadasPartes(t *testing.T) {
	t.Parallel()

	selection := strings.Repeat("1,", maxVisibleSealPageSelectionParts) + "1"
	if _, _, err := parseVisibleSealPageSelection(selection); err == nil {
		t.Fatalf("se esperaba error para más de %d partes", maxVisibleSealPageSelectionParts)
	}

	selection = strings.Repeat("1", maxVisibleSealPageSelectionLength+1)
	if _, _, err := parseVisibleSealPageSelection(selection); err == nil {
		t.Fatalf("se esperaba error para más de %d bytes", maxVisibleSealPageSelectionLength)
	}
}

func TestParseVisibleSealPageSelection_LimitaTrabajoConRangosDuplicados(t *testing.T) {
	t.Parallel()

	selection := strings.Repeat("1-10000,", 2) + "1-10000"
	if _, _, err := parseVisibleSealPageSelection(selection); err == nil {
		t.Fatalf("se esperaba error al superar %d páginas expandidas", maxVisibleSealExpandedPages)
	}
}

func TestParseVisibleSealPageSelection_LimitesNormales(t *testing.T) {
	t.Parallel()

	selection := fmt.Sprintf("1-%d", maxVisibleSealSelectedPages)
	allPages, pages, err := parseVisibleSealPageSelection(selection)
	if err != nil {
		t.Fatalf("parseVisibleSealPageSelection(%q) error = %v", selection, err)
	}
	if allPages {
		t.Fatal("no se esperaba selección completa")
	}
	if len(pages) != maxVisibleSealSelectedPages {
		t.Fatalf("longitud inesperada: got=%d want=%d", len(pages), maxVisibleSealSelectedPages)
	}
	if pages[0] != 1 || pages[len(pages)-1] != maxVisibleSealSelectedPages {
		t.Fatalf("límites inesperados: first=%d last=%d", pages[0], pages[len(pages)-1])
	}

	selection = fmt.Sprintf("1-%d,%d", maxVisibleSealSelectedPages, maxVisibleSealSelectedPages+1)
	if _, _, err := parseVisibleSealPageSelection(selection); err == nil {
		t.Fatalf("se esperaba error al superar %d páginas", maxVisibleSealSelectedPages)
	}

	const maxUint32Page = "4294967295"
	_, pages, err = parseVisibleSealPageSelection(maxUint32Page)
	if err != nil {
		t.Fatalf("parseVisibleSealPageSelection(%s) error = %v", maxUint32Page, err)
	}
	if len(pages) != 1 || pages[0] != ^uint32(0) {
		t.Fatalf("página uint32 límite inesperada: %v", pages)
	}

	_, pages, err = parseVisibleSealPageSelection("4294967294-4294967295")
	if err != nil {
		t.Fatalf("rango junto al límite uint32: %v", err)
	}
	if len(pages) != 2 || pages[0] != ^uint32(1) || pages[1] != ^uint32(0) {
		t.Fatalf("rango uint32 límite inesperado: %v", pages)
	}
}

func TestAplicarMetadatosFirmaPdfsign_Overrides(t *testing.T) {
	t.Parallel()

	info := testInfoPAdES()
	aplicarMetadatosFirmaPdfsign(&info, map[string]string{
		"reason":      "Aprobación interna",
		"location":    "Granada",
		"contactInfo": "contacto@example.invalid",
	})
	if info.Reason != "Aprobación interna" {
		t.Fatalf("Reason inesperado: %q", info.Reason)
	}
	if info.Location != "Granada" {
		t.Fatalf("Location inesperado: %q", info.Location)
	}
	if info.ContactInfo != "contacto@example.invalid" {
		t.Fatalf("ContactInfo inesperado: %q", info.ContactInfo)
	}
}

func TestNormalizarRotacionPAdES(t *testing.T) {
	t.Parallel()

	casos := map[int]int{
		0:   0,
		90:  90,
		180: 180,
		270: 270,
		360: 0,
		-90: 270,
		45:  0,
	}
	for input, want := range casos {
		if got := normalizarRotacionPAdES(input); got != want {
			t.Fatalf("normalizarRotacionPAdES(%d) = %d, want %d", input, got, want)
		}
	}
}

func TestRotarImagenPNGSiProcede_90Grados_IntercambiaDimensiones(t *testing.T) {
	t.Parallel()

	src := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			src.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 200, A: 255})
		}
	}
	var raw bytes.Buffer
	if err := png.Encode(&raw, src); err != nil {
		t.Fatalf("png.Encode() error = %v", err)
	}

	rotated, err := rotarImagenPNGSiProcede(raw.Bytes(), 90)
	if err != nil {
		t.Fatalf("rotarImagenPNGSiProcede() error = %v", err)
	}
	decoded, _, err := image.Decode(bytes.NewReader(rotated))
	if err != nil {
		t.Fatalf("image.Decode() error = %v", err)
	}
	if got, want := decoded.Bounds().Dx(), 20; got != want {
		t.Fatalf("ancho rotado = %d, want %d", got, want)
	}
	if got, want := decoded.Bounds().Dy(), 40; got != want {
		t.Fatalf("alto rotado = %d, want %d", got, want)
	}
}

func testInfoPAdES() pdfsign.SignDataSignatureInfo {
	return pdfsign.SignDataSignatureInfo{
		Name:     "Ana Pérez Fernández",
		Location: "Certificado digital FNMT",
	}
}
