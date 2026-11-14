// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmahandler

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/ports"
)

func politicaFija(valor string) func() ports.PoliticaProgramaAfirma {
	return func() ports.PoliticaProgramaAfirma { return ports.PoliticaProgramaAfirma{Valor: valor} }
}

func TestPreguntarDejaAGrxFirmaYGuardaLaPreferencia(t *testing.T) {
	fx := newFixture(t, true)
	fx.installLikeNSIS(t)
	if _, err := fx.sel.Elegir(context.Background(), ports.ProgramaAfirmaAutoFirma); err != nil {
		t.Fatal(err)
	}
	estado, err := ConPolitica(fx.sel, nil).Elegir(context.Background(), " Preguntar ")
	if err != nil {
		t.Fatalf("elegir preguntar: %v", err)
	}
	if estado.Actual != ports.ProgramaAfirmaGrxFirma || estado.Preferencia != ports.ProgramaAfirmaPreguntar ||
		estado.Efectiva != ports.ProgramaAfirmaPreguntar {
		t.Fatalf("con preguntar los enlaces llegan a GrxFirma: %+v", estado)
	}
	v, _ := fx.reg.Read(CurrentUser, PreferenceKey, PreferenceValue)
	if v.Data != ports.ProgramaAfirmaPreguntar {
		t.Fatalf("preferencia guardada: %+v", v)
	}
}

func TestPreguntarSinAutoFirmaNoCambiaNada(t *testing.T) {
	fx := newFixture(t, false)
	fx.installLikeNSIS(t)
	if _, err := fx.sel.Elegir(context.Background(), ports.ProgramaAfirmaPreguntar); !errors.Is(err, ports.ErrAutoFirmaNoInstalada) {
		t.Fatalf("sin AutoFirma no se puede preguntar: %v", err)
	}
	if fx.reg.writes != 0 {
		t.Fatalf("no debe escribir nada: %d escrituras", fx.reg.writes)
	}
}

// La preferencia «autofirma» de la 0.0.125 se sigue leyendo igual.
func TestPreferenciaDeLaVersionAnteriorSeConserva(t *testing.T) {
	fx := newFixture(t, true)
	fx.installLikeNSIS(t)
	fx.reg.set(CurrentUser, PreferenceKey, PreferenceValue, KindString, "autofirma")
	estado, err := ConPolitica(fx.sel, nil).Estado(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if estado.Preferencia != ports.ProgramaAfirmaAutoFirma || estado.Politica != "" {
		t.Fatalf("preferencia anterior: %+v", estado)
	}
}

func TestLaPoliticaPrevaleceYNoDejaElegirOtraCosa(t *testing.T) {
	fx := newFixture(t, true)
	fx.installLikeNSIS(t)
	sel := ConPolitica(fx.sel, politicaFija(ports.ProgramaAfirmaAutoFirma))
	estado, err := sel.Estado(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if estado.Politica != ports.ProgramaAfirmaAutoFirma || estado.Efectiva != ports.ProgramaAfirmaAutoFirma {
		t.Fatalf("la política manda: %+v", estado)
	}
	for _, otro := range []string{ports.ProgramaAfirmaGrxFirma, ports.ProgramaAfirmaPreguntar, "x"} {
		if _, err := sel.Elegir(context.Background(), otro); !errors.Is(err, ports.ErrProtocoloAfirmaPolitica) {
			t.Fatalf("%s con política autofirma: %v", otro, err)
		}
	}
	if fx.reg.writes != 0 {
		t.Fatalf("con la política no se escribe nada: %d", fx.reg.writes)
	}
	// Elegir lo mismo que la política aplica lo que manda la organización.
	estado, err = sel.Elegir(context.Background(), ports.ProgramaAfirmaAutoFirma)
	if err != nil || estado.Actual != ports.ProgramaAfirmaAutoFirma {
		t.Fatalf("aplicar la política: %v %+v", err, estado)
	}
}

// Una política ilegible deja lo de hoy: la elección del usuario, editable.
func TestPoliticaIlegibleMantieneLaEleccionDelUsuario(t *testing.T) {
	fx := newFixture(t, true)
	fx.installLikeNSIS(t)
	sel := ConPolitica(fx.sel, func() ports.PoliticaProgramaAfirma {
		return ports.PoliticaProgramaAfirma{Valor: ports.ProgramaAfirmaGrxFirma, Ilegible: true}
	})
	estado, err := sel.Elegir(context.Background(), ports.ProgramaAfirmaAutoFirma)
	if err != nil {
		t.Fatalf("con la política ilegible el usuario puede elegir: %v", err)
	}
	if estado.Politica != "" || !estado.PoliticaIlegible || estado.Efectiva != ports.ProgramaAfirmaAutoFirma {
		t.Fatalf("estado con política ilegible: %+v", estado)
	}
}

func TestCompletarEstadoEfectiva(t *testing.T) {
	casos := []struct {
		nombre   string
		estado   ports.EstadoProtocoloAfirma
		politica ports.PoliticaProgramaAfirma
		efectiva string
	}{
		{"por defecto", ports.EstadoProtocoloAfirma{Soportado: true, Actual: "grxfirma", AutoFirmaInstalada: true}, ports.PoliticaProgramaAfirma{}, "grxfirma"},
		{"preguntar", ports.EstadoProtocoloAfirma{Soportado: true, Actual: "grxfirma", AutoFirmaInstalada: true, Preferencia: "preguntar"}, ports.PoliticaProgramaAfirma{}, "preguntar"},
		{"preguntar sin AutoFirma", ports.EstadoProtocoloAfirma{Soportado: true, Actual: "grxfirma", Preferencia: "preguntar"}, ports.PoliticaProgramaAfirma{}, "grxfirma"},
		{"AutoFirma atiende", ports.EstadoProtocoloAfirma{Soportado: true, Actual: "autofirma", AutoFirmaInstalada: true}, ports.PoliticaProgramaAfirma{}, "autofirma"},
		{"política preguntar", ports.EstadoProtocoloAfirma{Soportado: true, Actual: "grxfirma", AutoFirmaInstalada: true, Preferencia: "grxfirma"}, ports.PoliticaProgramaAfirma{Valor: "preguntar"}, "preguntar"},
		{"política autofirma sin AutoFirma", ports.EstadoProtocoloAfirma{Soportado: true, Actual: "grxfirma"}, ports.PoliticaProgramaAfirma{Valor: "autofirma"}, "grxfirma"},
		{"política grxfirma con AutoFirma registrado", ports.EstadoProtocoloAfirma{Soportado: true, Actual: "autofirma", AutoFirmaInstalada: true}, ports.PoliticaProgramaAfirma{Valor: "grxfirma"}, "autofirma"},
		{"otro programa", ports.EstadoProtocoloAfirma{Soportado: true, Actual: "other"}, ports.PoliticaProgramaAfirma{}, "other"},
		{"sin selector", ports.EstadoProtocoloAfirma{Actual: "none"}, ports.PoliticaProgramaAfirma{Valor: "autofirma"}, "none"},
	}
	for _, c := range casos {
		e := c.estado
		CompletarEstado(&e, c.politica)
		if e.Efectiva != c.efectiva {
			t.Errorf("%s: efectiva %q, se esperaba %q", c.nombre, e.Efectiva, c.efectiva)
		}
	}
}
