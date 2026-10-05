// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/ports"
	"os"
	"path/filepath"
	"testing"

	qrcode "github.com/skip2/go-qrcode"
)

func TestVeriFactuCLIValidationAndExport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "record.xml")
	output := filepath.Join(t.TempDir(), "report.txt")
	if e := os.WriteFile(path, []byte(`<RegistroAlta xmlns="urn:fake"/>`), 0600); e != nil {
		t.Fatal(e)
	}
	var stdout, stderr bytes.Buffer
	a := &Adaptador{Stdout: &stdout, Stderr: &stderr, Localizador: localizador.Para("es")}
	rc := a.Run(context.Background(), []string{"-operacion", "validar-verifactu", "-entrada", path, "-salida", output, "-json"})
	if rc != 1 || !json.Valid(stdout.Bytes()) {
		t.Fatalf("rc=%d stdout=%s stderr=%s", rc, stdout.String(), stderr.String())
	}
	report, e := os.ReadFile(output)
	if e != nil || !bytes.Contains(report, []byte("RegistroAlta")) {
		t.Fatalf("%s %v", report, e)
	}
}

func TestVeriFactuCLIReadsQROffline(t *testing.T) {
	var stdout, stderr bytes.Buffer
	a := &Adaptador{Stdout: &stdout, Stderr: &stderr}
	rc := a.Run(context.Background(), []string{"-operacion", "leer-qr-verifactu", "-entrada", "https://www2.agenciatributaria.gob.es/wlpl/TIKE-CONT/ValidarQR?nif=89890001K&numserie=A&fecha=01-01-2025&importe=1"})
	if rc != 0 || !json.Valid(stdout.Bytes()) {
		t.Fatalf("%d %s %s", rc, stdout.String(), stderr.String())
	}
}

const qrEjemploAEAT = "https://prewww2.aeat.es/wlpl/TIKE-CONT/ValidarQR?nif=89890001K&numserie=12345678-G33&fecha=01-09-2024&importe=241.4"

type visorPDFPrueba struct{ png []byte }

func (v visorPDFPrueba) RenderizarPagina(context.Context, string, int) (string, float64, float64, int, error) {
	return base64.StdEncoding.EncodeToString(v.png), 595, 842, 1, nil
}

func TestVeriFactuCLIReadsQRFromImageAndPDF(t *testing.T) {
	dir := t.TempDir()
	png, e := qrcode.Encode(qrEjemploAEAT, qrcode.Medium, 320)
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
	for _, c := range []struct {
		entrada string
		visor   ports.VisualizadorPDF
	}{{imagen, nil}, {pdf, visorPDFPrueba{png}}} {
		var stdout, stderr bytes.Buffer
		a := &Adaptador{Stdout: &stdout, Stderr: &stderr, VisorPDF: c.visor}
		rc := a.Run(context.Background(), []string{"-operacion", "leer-qr-verifactu", "-entrada", c.entrada})
		var qr map[string]any
		if rc != 0 || json.Unmarshal(stdout.Bytes(), &qr) != nil || qr["numserie"] != "12345678-G33" {
			t.Fatalf("%s: rc=%d %s %s", c.entrada, rc, stdout.String(), stderr.String())
		}
	}
	// Sin rasterizador el PDF se rechaza con un mensaje propio, y el cotejo
	// de un fichero sin QR falla antes de cualquier petición de red.
	sinQR := filepath.Join(dir, "blanco.png")
	if e := os.WriteFile(sinQR, []byte("\x89PNG\r\n\x1a\nroto"), 0o600); e != nil {
		t.Fatal(e)
	}
	loc := localizador.Para("es")
	for _, c := range []struct{ operacion, entrada, clave string }{
		{"leer-qr-verifactu", pdf, "verifactu.qr_pdf"},
		{"cotejar-qr-verifactu", sinQR, "verifactu.qr_image"},
		{"leer-qr-verifactu", "www2.agenciatributaria.gob.es/wlpl/TIKE-CONT/ValidarQR", "verifactu.qr_url"},
	} {
		var stdout, stderr bytes.Buffer
		a := &Adaptador{Stdout: &stdout, Stderr: &stderr, Localizador: loc}
		if rc := a.Run(context.Background(), []string{"-operacion", c.operacion, "-entrada", c.entrada}); rc != 1 || stdout.Len() != 0 || !bytes.Contains(stderr.Bytes(), []byte(loc.T(c.clave))) {
			t.Fatalf("%s %s: rc=%d %q", c.operacion, c.entrada, rc, stderr.String())
		}
	}
}
