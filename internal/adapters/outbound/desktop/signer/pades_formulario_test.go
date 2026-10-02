// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/testsupport/pdfsigtest"
)

// pdfFormulario genera un PDF con AcroForm: un campo de texto relleno y, si se
// pide, un campo de firma vacío, como los formularios de las sedes.
func pdfFormulario(conCampoFirma bool) []byte {
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R%s] /DA (/Helv 0 Tf 0 g) /DR << /Font << /Helv 6 0 R >> >> >> >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /Helv 6 0 R >> >> /Annots [4 0 R%s] /Contents 7 0 R >>",
		"<< /Type /Annot /Subtype /Widget /FT /Tx /T (nombre) /V (Ana) /Rect [50 700 300 720] /P 3 0 R /F 4 >>",
		"<< /Type /Annot /Subtype /Widget /FT /Sig /T (FirmaSolicitante) /Rect [300 100 550 160] /P 3 0 R /F 4 >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		"<< /Length 43 >>\nstream\nBT /Helv 12 Tf 50 750 Td (Solicitud) Tj ET\nendstream",
	}
	extra := ""
	if conCampoFirma {
		extra = " 5 0 R"
	}
	objs[0] = fmt.Sprintf(objs[0], extra)
	objs[2] = fmt.Sprintf(objs[2], extra)
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

func firmarFormulario(t *testing.T, pdfData []byte, opciones map[string]string) []byte {
	t.Helper()
	priv, cert := generarCertRSA(t)
	doc, _ := domain.NewDocument("formulario.pdf", pdfData, "application/pdf")
	res, err := signer.NuevoMotorFirmaGo(nil).Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatPAdES, Action: domain.ActionSign, Options: opciones,
	}, signer.NuevaClaveLocal(priv, cert))
	if err != nil {
		t.Fatalf("firma del formulario: %v", err)
	}
	return res.Data
}

func comprobarPDFFirmado(t *testing.T, firmado []byte) {
	t.Helper()
	dir := t.TempDir()
	f := filepath.Join(dir, "f.pdf")
	_ = os.WriteFile(f, firmado, 0o600)
	if _, err := exec.LookPath("qpdf"); err == nil {
		if out, err := exec.Command("qpdf", "--check", f).CombinedOutput(); err != nil {
			t.Fatalf("qpdf rechaza el PDF firmado: %v\n%s", err, out)
		}
	}
	if _, err := exec.LookPath("pdfsig"); err == nil {
		t.Run("pdfsig", func(t *testing.T) {
			out, _ := pdfsigtest.Command(t, f).CombinedOutput()
			if !strings.Contains(string(out), "Signature is Valid") {
				// Poppler anterior a 25 no recorre los hijos de un campo de firma con
				// varios widgets y los da por no firmados; la estructura se comprueba
				// con qpdf en TestMotorFirmaGo_EstructuraCampoFirmaPorPagina.
				if bytes.Count(firmado, []byte("/Subtype /Widget")) > 1 && pdfsigAnterior25(t) &&
					strings.Contains(string(out), "The signature form field is not signed") {
					t.Skip("pdfsig < 25 no admite campos de firma con varios widgets")
				}
				t.Fatalf("pdfsig no valida la firma:\n%s", out)
			}
		})
	}
	doc, _ := domain.NewDocument("f.pdf", firmado, "application/pdf")
	vr, _, err := commonsigner.NewPAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
	if err != nil || vr.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("el verificador propio no acepta la firma: %v %+v", err, vr.Integrity)
	}
}

func TestMotorFirmaGo_PAdESGiroLibre30Valido(t *testing.T) {
	firmado := firmarFormulario(t, pdfFormulario(false), map[string]string{
		"visibleSeal": "true", "visibleSealRectX": "50", "visibleSealRectY": "100",
		"visibleSealRectW": "220", "visibleSealRectH": "80", "page": "1",
		"rotation": "30", "visibleSealLogoOpacityPercent": "65",
		"visibleSealLogo": "institucional",
	})
	comprobarPDFFirmado(t, firmado)
}

func TestMotorFirmaGo_PAdESOpacidadSelloCompleto(t *testing.T) {
	logo := image.NewRGBA(image.Rect(0, 0, 12, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 12; x++ {
			logo.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, logo); err != nil {
		t.Fatal(err)
	}
	for _, withLogo := range []bool{false, true} {
		name := "sin_logo"
		if withLogo {
			name = "con_logo"
		}
		t.Run(name, func(t *testing.T) {
			options := map[string]string{
				"visibleSeal": "true", "visibleSealRectX": "50", "visibleSealRectY": "100",
				"visibleSealRectW": "220", "visibleSealRectH": "70", "page": "1",
				"visibleSealLogoOpacityPercent": "40",
			}
			if withLogo {
				options["visibleSealImageBase64"] = base64.StdEncoding.EncodeToString(encoded.Bytes())
			}
			firmado := firmarFormulario(t, pdfFormulario(false), options)
			for _, marker := range [][]byte{
				[]byte("/ExtGState << /GSSeal << /Type /ExtGState /ca 0.40 /CA 0.40 >> >>"),
				[]byte("stream\nq\n/GSSeal gs\n"), []byte("/Im1 Do\n"),
			} {
				if !bytes.Contains(firmado, marker) {
					t.Fatalf("falta %q en la apariencia del sello", marker)
				}
			}
			// El logo se compone dentro de Im1 para que el alfa sea uniforme.
			if bytes.Contains(firmado, []byte("/ImLogo Do\n")) {
				t.Fatal("el logo no debe recibir una segunda capa de transparencia")
			}
			if got := bytes.Count(firmado, []byte("/GSSeal gs\n")); got != 1 {
				t.Fatalf("el estado debe rodear la apariencia completa: %d usos", got)
			}
			comprobarPDFFirmado(t, firmado)
		})
	}
}

func TestMotorFirmaGo_PAdESOpacidadSelloPorPagina(t *testing.T) {
	firmado := firmarFormulario(t, pdfFormulario(false), map[string]string{
		"visibleSeal":                   "true",
		"visibleSealPlacements":         `[{"page":1,"rect":{"x":0.1,"y":0.1,"w":0.3,"h":0.1},"rotation":0}]`,
		"visibleSealLogoOpacityPercent": "40",
	})
	if !bytes.Contains(firmado, []byte("/GSSeal << /Type /ExtGState /ca 0.40 /CA 0.40")) ||
		!bytes.Contains(firmado, []byte("stream\nq\n/GSSeal gs\n")) {
		t.Fatal("el sello por página no aplica la opacidad al conjunto")
	}
	comprobarPDFFirmado(t, firmado)
}

// Firmar un formulario no puede borrar sus campos: antes /AcroForm /Fields
// se reescribía solo con las firmas.
func TestMotorFirmaGo_PAdESConservaCamposDelFormulario(t *testing.T) {
	firmado := firmarFormulario(t, pdfFormulario(false), nil)
	ultimo := firmado[bytes.LastIndex(firmado, []byte("/AcroForm")):]
	if !bytes.Contains(ultimo[:bytes.Index(ultimo, []byte("]"))], []byte("4 0 R")) {
		t.Fatalf("el campo de texto ha desaparecido del formulario:\n%s", ultimo[:200])
	}
	if !bytes.Contains(ultimo, []byte("/DR")) || !bytes.Contains(ultimo, []byte("/DA")) {
		t.Fatal("se han perdido los recursos del formulario (/DR, /DA)")
	}
	comprobarPDFFirmado(t, firmado)
}

// signatureField de AutoFirma Java: firmar en el campo vacío del formulario.
func TestMotorFirmaGo_PAdESFirmaEnCampoExistente(t *testing.T) {
	firmado := firmarFormulario(t, pdfFormulario(true), map[string]string{
		"signatureField": "FirmaSolicitante", "layer2Text": "Firmado por $$SUBJECTCN$$",
	})
	incremental := firmado[len(pdfFormulario(true)):]
	if !bytes.Contains(incremental, []byte("5 0 obj")) || !bytes.Contains(incremental, []byte("/T (FirmaSolicitante)")) {
		t.Fatalf("no se ha rellenado el campo existente:\n%s", incremental[:min(600, len(incremental))])
	}
	if bytes.Contains(incremental, []byte("/T (Signature 1)")) {
		t.Fatal("no debe crearse un campo de firma nuevo")
	}
	comprobarPDFFirmado(t, firmado)

	priv, cert := generarCertRSA(t)
	doc, _ := domain.NewDocument("formulario.pdf", pdfFormulario(true), "application/pdf")
	if _, err := signer.NuevoMotorFirmaGo(nil).Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatPAdES, Action: domain.ActionSign,
		Options: map[string]string{"signatureField": "nombre"},
	}, signer.NuevaClaveLocal(priv, cert)); err == nil || !strings.Contains(err.Error(), "no es un campo de firma") {
		t.Fatalf("firmar sobre un campo de texto debe rechazarse: %v", err)
	}
}

// Estampado de imagen de AutoFirma Java (image, imagePage,
// imagePositionOnPage*) junto a una firma visible en la misma página.
func TestMotorFirmaGo_PAdESEstampaImagenComoJava(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for x := 0; x < 40; x++ {
		for y := 0; y < 20; y++ {
			img.Set(x, y, color.RGBA{200, 0, 0, 255})
		}
	}
	var png1 bytes.Buffer
	if err := png.Encode(&png1, img); err != nil {
		t.Fatal(err)
	}
	firmado := firmarFormulario(t, pdfFormulario(false), map[string]string{
		"image": base64.StdEncoding.EncodeToString(png1.Bytes()), "imagePage": "-1",
		"imagePositionOnPageLowerLeftX": "400", "imagePositionOnPageLowerLeftY": "750",
		"imagePositionOnPageUpperRightX": "560", "imagePositionOnPageUpperRightY": "820",
		"signaturePositionOnPageLowerLeftX": "50", "signaturePositionOnPageLowerLeftY": "60",
		"signaturePositionOnPageUpperRightX": "250", "signaturePositionOnPageUpperRightY": "140",
	})
	incremental := firmado[len(pdfFormulario(false)):]
	if !bytes.Contains(incremental, []byte("/Subtype /Stamp")) {
		t.Fatal("no se ha estampado la imagen")
	}
	if n := bytes.Count(incremental, []byte("\n3 0 obj")); n != 1 {
		t.Fatalf("la página debe actualizarse una sola vez con la firma y la imagen: %d", n)
	}
	comprobarPDFFirmado(t, firmado)
}

func firmarCifrado(t *testing.T, data []byte, opciones map[string]string) ([]byte, error) {
	t.Helper()
	priv, cert := generarCertRSA(t)
	doc, _ := domain.NewDocument("cifrado.pdf", data, "application/pdf")
	res, err := signer.NuevoMotorFirmaGo(nil).Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatPAdES, Action: domain.ActionSign, Options: opciones,
	}, signer.NuevaClaveLocal(priv, cert))
	return res.Data, err
}

// Un PDF cifrado se firma conservando su cifrado: qpdf lo descifra y lo da
// por correcto y pdfsig valida la firma.
func comprobarFirmaCifrada(t *testing.T, nombre string, firmado []byte, password string) {
	t.Helper()
	dir := t.TempDir()
	f := filepath.Join(dir, "f.pdf")
	_ = os.WriteFile(f, firmado, 0o600)
	if _, err := exec.LookPath("qpdf"); err == nil {
		out, err := exec.Command("qpdf", "--password="+password, "--show-encryption", f).CombinedOutput()
		if err != nil || !strings.Contains(string(out), "stream encryption method") && !strings.Contains(string(out), "R = ") {
			t.Fatalf("%s: el PDF firmado debe seguir cifrado: %v\n%s", nombre, err, out)
		}
		if out, err := exec.Command("qpdf", "--password="+password, "--check", f).CombinedOutput(); err != nil {
			t.Fatalf("%s: qpdf --check: %v\n%s", nombre, err, out)
		}
	}
	if _, err := exec.LookPath("pdfsig"); err == nil {
		t.Run("pdfsig", func(t *testing.T) {
			args := []string{f}
			switch password {
			case "":
			case "propietario":
				args = []string{"-opw", password, f}
			default:
				args = []string{"-upw", password, f}
			}
			out, _ := pdfsigtest.Command(t, args...).CombinedOutput()
			if !strings.Contains(string(out), "Signature is Valid") {
				// Poppler anterior a 25 no recorre los hijos de un campo de firma con
				// varios widgets y los da por no firmados; la estructura se comprueba
				// con qpdf en TestMotorFirmaGo_EstructuraCampoFirmaPorPagina.
				if bytes.Count(firmado, []byte("/Subtype /Widget")) > 1 && pdfsigAnterior25(t) &&
					strings.Contains(string(out), "The signature form field is not signed") {
					t.Skip("pdfsig < 25 no admite campos de firma con varios widgets")
				}
				t.Fatalf("%s: pdfsig no valida la firma:\n%s", nombre, out)
			}
		})
	}
}

func TestMotorFirmaGo_PAdESCifradoConservaElCifrado(t *testing.T) {
	// Formulario sintético cifrado con qpdf (third_party/pdf/testdata).
	testdata := filepath.Join("..", "..", "..", "..", "..", "third_party", "pdf", "testdata")
	leer := func(n string) []byte {
		data, err := os.ReadFile(filepath.Join(testdata, n))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	aes128 := leer("form-aes128.pdf")
	casos := []struct {
		nombre   string
		data     []byte
		opciones map[string]string
		password string
	}{
		{"AES-128", aes128, nil, ""},
		{"RC4-128", leer("form-rc4.pdf"), nil, ""},
		{"AES-256", leer("form-aes256.pdf"), nil, ""},
		{"AES-256 con contraseña de usuario", leer("form-aes256-usuario.pdf"), map[string]string{"userPassword": "usuario"}, "usuario"},
		{"AES-128 con sello visible", aes128, map[string]string{
			"signaturePositionOnPageLowerLeftX": "50", "signaturePositionOnPageLowerLeftY": "60",
			"signaturePositionOnPageUpperRightX": "250", "signaturePositionOnPageUpperRightY": "140",
			"layer2Text": "Firmado por $$SUBJECTCN$$",
		}, ""},
	}
	for _, c := range casos {
		firmado, err := firmarCifrado(t, c.data, c.opciones)
		if err != nil {
			t.Fatalf("%s: %v", c.nombre, err)
		}
		if bytes.Contains(firmado[len(c.data):], []byte("Firma electr")) {
			t.Errorf("%s: el motivo de la firma no se ha cifrado", c.nombre)
		}
		comprobarFirmaCifrada(t, c.nombre, firmado, c.password)
	}
}

func TestMotorFirmaGo_PAdESCifradoContrasenaYPermisos(t *testing.T) {
	testdata := filepath.Join("..", "..", "..", "..", "..", "third_party", "pdf", "testdata")
	usuario, _ := os.ReadFile(filepath.Join(testdata, "form-aes256-usuario.pdf"))
	if _, err := firmarCifrado(t, usuario, nil); !errors.Is(err, signer.ErrPDFContrasena) {
		t.Errorf("sin contraseña de apertura se esperaba ErrPDFContrasena: %v", err)
	}
	restringido, _ := os.ReadFile(filepath.Join(testdata, "form-aes256-restringido.pdf"))
	if _, err := firmarCifrado(t, restringido, nil); !errors.Is(err, signer.ErrPDFPermisos) {
		t.Errorf("sin permiso de anotar se esperaba ErrPDFPermisos: %v", err)
	}
	firmado, err := firmarCifrado(t, restringido, map[string]string{"ownerPassword": "propietario"})
	if err != nil {
		t.Fatalf("con la contraseña de propietario debe firmarse: %v", err)
	}
	comprobarFirmaCifrada(t, "restringido con propietario", firmado, "propietario")
}

func TestMotorFirmaGo_PAdESLeyendaCSVEnCadaPagina(t *testing.T) {
	opciones := map[string]string{"csv": "ABCD-1234-EFGH", "csvUrl": "https://sede.example.es/cotejo?csv={csv}"}
	// Sin firma visible: la leyenda no depende del sello.
	firmado := firmarFormulario(t, pdfFormulario(false), opciones)
	incremental := firmado[len(pdfFormulario(false)):]
	if bytes.Count(incremental, []byte("/Subtype /Stamp")) != 1 {
		t.Fatal("se esperaba una leyenda CSV en la única página")
	}
	comprobarPDFFirmado(t, firmado)
	if dir := os.Getenv("GRXFIRMA_GUARDAR_PDF_CSV"); dir != "" {
		_ = os.WriteFile(filepath.Join(dir, "leyenda-csv.pdf"), firmado, 0o600)
	}

	priv, cert := generarCertRSA(t)
	doc, _ := domain.NewDocument("f.pdf", pdfFormulario(false), "application/pdf")
	_, err := signer.NuevoMotorFirmaGo(nil).Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatPAdES, Action: domain.ActionSign,
		Options: map[string]string{"csv": "X", "csvUrl": "http://sede.example.es/"},
	}, signer.NuevaClaveLocal(priv, cert))
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("una dirección de cotejo sin HTTPS debe rechazarse: %v", err)
	}
}

func TestMotorFirmaGo_PAdESSelloEnPosicionElegida(t *testing.T) {
	firmado := firmarFormulario(t, pdfFormulario(false), map[string]string{
		"visibleSignature":         "want",
		domain.OpcionPosicionSello: "inferior-derecha",
		domain.OpcionPaginaSello:   "-1",
	})
	incremental := string(firmado[len(pdfFormulario(false)):])
	// Página A4 de 595 x 842: sello de 200 x 70 a 36 puntos de los bordes.
	if !strings.Contains(incremental, "/Rect [359") || !strings.Contains(incremental, "/Subtype /Widget") {
		t.Fatalf("el sello no está abajo a la derecha:\n%s", incremental)
	}
	comprobarPDFFirmado(t, firmado)
}

func pdfsigAnterior25(t *testing.T) bool {
	t.Helper()
	out, _ := exec.Command("pdfsig", "-v").CombinedOutput()
	var mayor int
	for _, campo := range strings.Fields(string(out)) {
		if n, err := fmt.Sscanf(campo, "%d.", &mayor); err == nil && n == 1 {
			return mayor < 25
		}
	}
	return false
}
