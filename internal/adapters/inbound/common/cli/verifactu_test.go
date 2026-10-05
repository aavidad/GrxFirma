// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"os"
	"path/filepath"
	"testing"
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
