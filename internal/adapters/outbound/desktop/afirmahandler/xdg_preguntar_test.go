// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package afirmahandler

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"grxfirma/internal/ports"
)

func TestXDGPreguntarDejaAGrxFirmaYGuardaLaPreferencia(t *testing.T) {
	f := newXDGFixture(t, nil)
	f.installGrxFirma()
	f.installAutoFirma()
	f.write(f.mimeapps(), "[Default Applications]\nx-scheme-handler/afirma=afirma.desktop\n")
	sel := NewXDGSelector(f.env)

	estado, err := sel.Elegir(context.Background(), ports.ProgramaAfirmaPreguntar)
	if err != nil {
		t.Fatal(err)
	}
	if estado.Actual != ports.ProgramaAfirmaGrxFirma || estado.Preferencia != ports.ProgramaAfirmaPreguntar {
		t.Fatalf("con preguntar los enlaces llegan a GrxFirma: %+v", estado)
	}
	if !strings.Contains(f.read(f.mimeapps()), "x-scheme-handler/afirma=grxfirma.desktop") {
		t.Fatalf("mimeapps.list: %s", f.read(f.mimeapps()))
	}
	if got := strings.TrimSpace(f.read(sel.preferencePath())); got != ports.ProgramaAfirmaPreguntar {
		t.Fatalf("preferencia: %q", got)
	}
}

func TestXDGPreguntarSinAutoFirmaNoCambiaNada(t *testing.T) {
	f := newXDGFixture(t, nil)
	f.installGrxFirma()
	f.write(f.mimeapps(), "[Default Applications]\nx-scheme-handler/afirma=grxfirma.desktop\n")
	sel := NewXDGSelector(f.env)
	if _, err := sel.Elegir(context.Background(), ports.ProgramaAfirmaPreguntar); !errors.Is(err, ports.ErrAutoFirmaNoInstalada) {
		t.Fatalf("sin AutoFirma no se puede preguntar: %v", err)
	}
	if _, err := os.Stat(sel.preferencePath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("se guardó la preferencia: %v", err)
	}
}

func TestXDGLaPoliticaNoDejaElegirOtraCosa(t *testing.T) {
	f := newXDGFixture(t, nil)
	f.installGrxFirma()
	f.installAutoFirma()
	f.write(f.mimeapps(), "[Default Applications]\nx-scheme-handler/afirma=grxfirma.desktop\n")
	before := f.read(f.mimeapps())
	sel := ConPolitica(NewXDGSelector(f.env), politicaFija(ports.ProgramaAfirmaGrxFirma))
	if _, err := sel.Elegir(context.Background(), ports.ProgramaAfirmaAutoFirma); !errors.Is(err, ports.ErrProtocoloAfirmaPolitica) {
		t.Fatalf("con política grxfirma no se puede elegir AutoFirma: %v", err)
	}
	if f.read(f.mimeapps()) != before {
		t.Fatal("se modificó mimeapps.list con la política")
	}
}
