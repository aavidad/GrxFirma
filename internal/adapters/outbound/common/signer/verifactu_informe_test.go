// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
)

// El informe no muestra nombres de elementos XML, separa avisos de errores
// y el resumen no dice «sin errores» a secas cuando hay avisos.
func TestVeriFactuInformeSinNombresXMLYConAvisos(t *testing.T) {
	previo := strings.Repeat("A", 64)
	data := vfTestXML("RegistroAlta", "12345679/G33", previo, "2024-01-01T19:20:30+01:00")
	result := ValidarRegistrosVeriFactu(context.Background(), map[string][]byte{"registro.xml": data})
	if result.Errors != 0 || result.Warnings != 2 {
		t.Fatalf("se esperaban 0 errores y 2 avisos: %+v", result)
	}
	casos := map[string][]string{
		"es": {"Aviso · Firma: ", "Aviso · Encadenamiento: ", "Sin errores, con 2 avisos. Consulte"},
		"en": {"Warning · Signature: ", "Warning · Chaining: ", "No errors, with 2 warnings. See"},
	}
	for idioma, esperados := range casos {
		loc := localizador.Para(idioma)
		result.Localize(func(k string) string { return loc.T(k) })
		texto := result.Report + "\n" + result.Summary
		for _, esperado := range esperados {
			if !strings.Contains(texto, esperado) {
				t.Errorf("%s: falta %q en %q", idioma, esperado, texto)
			}
		}
		for _, prohibido := range []string{"\nSignature:", "\nEncadenamiento:", loc.T("verifactu.scope"), loc.T("verifactu.valid")} {
			if strings.Contains("\n"+texto, prohibido) {
				t.Errorf("%s: el informe contiene %q: %q", idioma, prohibido, texto)
			}
		}
	}
}

func TestVeriFactuFicheroQueNoEsXML(t *testing.T) {
	result := ValidarRegistrosVeriFactu(context.Background(), map[string][]byte{"factura.pdf": []byte("%PDF-1.7\n...")})
	loc := localizador.Para("es")
	result.Localize(func(k string) string { return loc.T(k) })
	if !strings.Contains(result.Report, loc.T("verifactu.root")) || strings.Contains(result.Report, loc.T("verifactu.xml")) {
		t.Fatalf("un PDF debe decir que no es un registro Veri*Factu: %q", result.Report)
	}
	if result.Summary != "El registro presenta 1 error. Consulte el detalle en el informe." {
		t.Fatalf("resumen: %q", result.Summary)
	}
	// Recorrido Windows 0.0.118 (B6): sin «[]», sin huella vacía y sin la
	// explicación de la raíz XML, que no ayuda ante un PDF.
	for _, prohibido := range []string{"[]", loc.T("verifactu.hash_label") + ":", loc.T("verifactu.root_detail")} {
		if strings.Contains(result.Report, prohibido) {
			t.Errorf("el informe de un PDF no debe contener %q: %q", prohibido, result.Report)
		}
	}
	if !strings.HasPrefix(result.Report, "factura.pdf\n") {
		t.Errorf("el informe debe empezar por el nombre del fichero: %q", result.Report)
	}
}
