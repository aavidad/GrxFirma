// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package afirmahandler

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/ports"
)

// Linux: AutoFirma fijó afirma:// con un lanzador que no se llama
// afirma.desktop ni autofirma.desktop (el nombre depende del paquete). El
// instalador de GrxFirma guardó ese manejador anterior para devolverlo al
// desinstalar, pero el selector solo buscaba los dos nombres fijos y decía
// «AutoFirma no está instalado».
func TestQuejaXDGAutoFirmaConOtroLanzadorGuardadoAlInstalar(t *testing.T) {
	f := newXDGFixture(t, nil)
	f.installGrxFirma()
	bin := filepath.Join(filepath.Dir(f.system), "bin", "autofirma")
	f.write(bin, "#!/bin/sh\n")
	f.write(filepath.Join(f.system, "applications", "AutoFirma.desktop"), "[Desktop Entry]\n"+
		"Name=AutoFirma\nType=Application\nExec="+bin+" %u\nMimeType=x-scheme-handler/afirma;\n")
	// Lo que dejó configure-browsers.sh: antes de GrxFirma abría AutoFirma.desktop.
	f.write(f.mimeapps(), "[Default Applications]\nx-scheme-handler/afirma=grxfirma.desktop\n")
	f.write(filepath.Join(f.home, ".local", "lib", "grxfirma", "config", "afirma-handler-anterior.json"),
		`{"`+f.mimeapps()+`": {"x-scheme-handler/afirma": "AutoFirma.desktop", "x-scheme-handler/afirmav2": null}}`)
	sel := NewXDGSelector(f.env)

	estado, err := sel.Estado(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !estado.AutoFirmaInstalada || estado.RutaAutoFirma != bin || estado.Actual != ports.ProgramaAfirmaGrxFirma {
		t.Fatalf("AutoFirma está instalado y no se ofrece: %+v", estado)
	}
	estado, err = sel.Elegir(context.Background(), ports.ProgramaAfirmaAutoFirma)
	if err != nil {
		t.Fatalf("elegir AutoFirma: %v", err)
	}
	if estado.Actual != ports.ProgramaAfirmaAutoFirma ||
		!strings.Contains(f.read(f.mimeapps()), "x-scheme-handler/afirma=AutoFirma.desktop") {
		t.Fatalf("no se devolvió afirma:// a AutoFirma: %+v\n%s", estado, f.read(f.mimeapps()))
	}
}

// El manejador guardado no cuenta como AutoFirma si es otro programa o el
// propio GrxFirma.
func TestXDGManejadorGuardadoQueNoEsAutoFirmaNoCuenta(t *testing.T) {
	f := newXDGFixture(t, nil)
	f.installGrxFirma()
	otro := filepath.Join(filepath.Dir(f.system), "bin", "firmador")
	f.write(otro, "#!/bin/sh\n")
	f.write(filepath.Join(f.system, "applications", "firmador.desktop"), "[Desktop Entry]\n"+
		"Name=Firmador\nType=Application\nExec="+otro+" %u\nMimeType=x-scheme-handler/afirma;\n")
	f.write(filepath.Join(f.home, ".local", "lib", "grxfirma", "config", "afirma-handler-anterior.json"),
		`{"x": {"x-scheme-handler/afirma": "firmador.desktop;grxfirma.desktop;../../etc/x.desktop"}}`)
	estado, err := NewXDGSelector(f.env).Estado(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if estado.AutoFirmaInstalada {
		t.Fatalf("un manejador que no es AutoFirma no debe ofrecerse como AutoFirma: %+v", estado)
	}
}
