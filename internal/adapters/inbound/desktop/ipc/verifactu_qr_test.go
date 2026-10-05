// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	qrcode "github.com/skip2/go-qrcode"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/desktop/pdfpreview"
	"grxfirma/internal/application"
)

const qrEjemploAEATIPC = "https://prewww2.aeat.es/wlpl/TIKE-CONT/ValidarQR?nif=89890001K&numserie=12345678-G33&fecha=01-09-2024&importe=241.4"

type previewQRPrueba struct{ png []byte }

func (p previewQRPrueba) Ejecutar(context.Context, application.PdfPreviewCommand) (application.PdfPreviewResult, error) {
	return application.PdfPreviewResult{DataB64: base64.StdEncoding.EncodeToString(p.png), Ancho: 595, Alto: 842, PaginaActual: 1, TotalPaginas: 1}, nil
}

func leerQRPorIPC(t *testing.T, m *Manejador, params map[string]any) respuesta {
	t.Helper()
	raw, e := json.Marshal(params)
	if e != nil {
		t.Fatal(e)
	}
	return m.despachar(context.Background(), peticion{Action: "read_verifactu_qr", Params: raw})
}

func TestVeriFactuIPCReadsQRFromFilesAndImage(t *testing.T) {
	dir := t.TempDir()
	png, e := qrcode.Encode(qrEjemploAEATIPC, qrcode.Medium, 320)
	if e != nil {
		t.Fatal(e)
	}
	imagen := filepath.Join(dir, "factura.png")
	pdf := filepath.Join(dir, "factura.pdf")
	if e := os.WriteFile(imagen, png, 0o600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(pdf, []byte("%PDF-1.4\n"), 0o600); e != nil {
		t.Fatal(e)
	}
	m := &Manejador{Loc: localizador.Para("es"), Preview: previewQRPrueba{png}}
	for nombre, params := range map[string]map[string]any{
		"ruta png": {"inputPath": imagen},
		"ruta pdf": {"inputPath": pdf},
		"imagen":   {"imageB64": png},
	} {
		r := leerQRPorIPC(t, m, params)
		if !r.OK || r.Data.(signer.VeriFactuQR).Number != "12345678-G33" {
			t.Fatalf("%s: %+v", nombre, r)
		}
	}
	mensaje := m.t("verifactu.qr_image")
	for nombre, params := range map[string]map[string]any{
		"dos fuentes":   {"inputPath": imagen, "url": qrEjemploAEATIPC},
		"ninguna":       {},
		"relativa":      {"inputPath": "factura.png"},
		"zona sistema":  {"inputPath": "/etc/passwd"},
		"inexistente":   {"inputPath": filepath.Join(dir, "no.png")},
		"espacios":      {"inputPath": imagen + " "},
		"imagen basura": {"imageB64": []byte("no es una imagen")},
	} {
		if r := leerQRPorIPC(t, m, params); r.OK || r.Error != mensaje {
			t.Fatalf("%s: %+v", nombre, r)
		}
	}
	// El código estable deja a WinUI distinguir «no encontrado» (probar la
	// página siguiente) de cualquier otro fallo (parar) sin leer el texto.
	if r := leerQRPorIPC(t, m, map[string]any{"imageB64": []byte("no es una imagen")}); r.ErrorCode != "verifactu_qr_image" {
		t.Fatalf("código imagen: %+v", r)
	}
	blanca, e := qrcode.Encode("x", qrcode.Low, 64)
	if e != nil {
		t.Fatal(e)
	}
	if r := leerQRPorIPC(t, m, map[string]any{"imageB64": blanca}); r.OK || r.ErrorCode == "operation_failed" || r.ErrorCode == "" {
		t.Fatalf("código QR ajeno: %+v", r)
	}
	// Sin visor de PDF configurado, el PDF se rechaza con su propio mensaje.
	sinVisor := &Manejador{Loc: localizador.Para("es")}
	if r := leerQRPorIPC(t, sinVisor, map[string]any{"inputPath": pdf}); r.OK || r.Error != m.t("verifactu.qr_pdf") {
		t.Fatalf("sin visor: %+v", r)
	}
	// La imagen solo se admite en la lectura, nunca en el cotejo ni en otras acciones.
	raw, _ := json.Marshal(map[string]any{"imageB64": png, "url": qrEjemploAEATIPC})
	if r := m.despachar(context.Background(), peticion{Action: "query_verifactu_qr", Params: raw}); r.OK {
		t.Fatalf("cotejo con imagen: %+v", r)
	}
}

// TestVeriFactuIPCReadsQRFromRealPDF genera un PDF con el QR dibujado como
// vectores y lo rasteriza con pdftoppm, igual que la vista previa del sello.
func TestVeriFactuIPCReadsQRFromRealPDF(t *testing.T) {
	if _, e := exec.LookPath("pdftoppm"); e != nil {
		t.Skip("pdftoppm no disponible")
	}
	if _, e := exec.LookPath("pdfinfo"); e != nil {
		t.Skip("pdfinfo no disponible")
	}
	q, e := qrcode.New(qrEjemploAEATIPC, qrcode.Medium)
	if e != nil {
		t.Fatal(e)
	}
	var contenido bytes.Buffer
	contenido.WriteString("0 g\n")
	bitmap := q.Bitmap()
	modulo := 2.5 // puntos: un QR de unos 3 cm, como en una factura real
	for y, fila := range bitmap {
		for x, negro := range fila {
			if negro {
				fmt.Fprintf(&contenido, "%.2f %.2f %.2f %.2f re\n", 400+float64(x)*modulo, 700-float64(y)*modulo, modulo, modulo)
			}
		}
	}
	contenido.WriteString("f\n")
	ruta := filepath.Join(t.TempDir(), "factura.pdf")
	if e := os.WriteFile(ruta, pdfMinimoPrueba(contenido.Bytes()), 0o600); e != nil {
		t.Fatal(e)
	}
	m := &Manejador{Loc: localizador.Para("es"), Preview: application.NuevoPdfPreviewUseCase(pdfpreview.New())}
	r := leerQRPorIPC(t, m, map[string]any{"inputPath": ruta})
	if !r.OK || r.Data.(signer.VeriFactuQR).NIF != "89890001K" {
		t.Fatalf("%+v", r)
	}
}

func pdfMinimoPrueba(contenido []byte) []byte {
	objetos := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(contenido), contenido),
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objetos))
	for i, o := range objetos {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objetos)+1)
	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objetos)+1, xref)
	return b.Bytes()
}
