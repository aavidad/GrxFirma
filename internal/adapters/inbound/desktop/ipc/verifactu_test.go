// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/json"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/common/signer"
	"os"
	"path/filepath"
	"testing"
)

func TestVeriFactuIPCValidationAndDetection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "record.xml")
	if e := os.WriteFile(path, []byte(`<RegistroAlta xmlns="`+signer.VeriFactuNamespace+`"/>`), 0600); e != nil {
		t.Fatal(e)
	}
	m := &Manejador{Loc: localizador.Para("es")}
	raw, _ := json.Marshal(map[string]string{"inputPath": path})
	detected := m.despachar(context.Background(), peticion{Action: "detect_verifactu", Params: raw})
	if !detected.OK || !detected.Data.(map[string]any)["isVerifactu"].(bool) {
		t.Fatalf("%+v", detected)
	}
	raw, _ = json.Marshal(map[string]string{"inputPath": dir})
	checked := m.despachar(context.Background(), peticion{Action: "validate_verifactu", Params: raw})
	if !checked.OK {
		t.Fatal(checked.Error)
	}
	result := checked.Data.(signer.VeriFactuValidationResult)
	if result.Valid || result.Errors == 0 || len(result.Records) != 1 || result.Report == "" {
		t.Fatalf("%+v", result)
	}
	for _, bad := range []string{"/etc/passwd", "relative.xml", ""} {
		raw, _ = json.Marshal(map[string]string{"inputPath": bad})
		if m.despachar(context.Background(), peticion{Action: "validate_verifactu", Params: raw}).OK {
			t.Fatalf("accepted %s", bad)
		}
	}
}

func TestVeriFactuIPCQRCodeReadWithoutNetwork(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{"url": "https://prewww2.aeat.es/wlpl/TIKE-CONT/ValidarQRNoVerifactu?nif=89890001K&numserie=ABC%26D&fecha=01-01-2025&importe=-1.20"})
	response := (&Manejador{}).despachar(context.Background(), peticion{Action: "read_verifactu_qr", Params: raw})
	if !response.OK {
		t.Fatal(response.Error)
	}
	result := response.Data.(signer.VeriFactuQR)
	if result.Number != "ABC&D" || !result.Test || result.Verifiable {
		t.Fatalf("%+v", result)
	}
	raw, _ = json.Marshal(map[string]string{"url": "https://www2.agenciatributaria.gob.es.evil.test/wlpl/TIKE-CONT/ValidarQR?nif=89890001K"})
	if (&Manejador{}).despachar(context.Background(), peticion{Action: "query_verifactu_qr", Params: raw}).OK {
		t.Fatal("accepted foreign domain")
	}
}
