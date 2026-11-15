// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/afirmahandler"
	"grxfirma/internal/ports"
)

// Las pruebas no leen el registro, mimeapps.list ni la política reales.
// Tampoco abren ventanas nativas: el aviso de AutoFirma es un TaskDialog en
// Windows que, por SSH o en CI, nadie cierra y deja la prueba colgada.
func TestMain(m *testing.M) {
	nuevoSelectorFirmaWeb = func() ports.ProtocoloAfirma { return &selectorFirmaWebFalso{} }
	presentarAvisoFirmaWeb = func(context.Context, string) {}
	os.Exit(m.Run())
}

const exeAutoFirmaPrueba = "/usr/bin/autofirma"

type selectorFirmaWebFalso struct {
	estado    ports.EstadoProtocoloAfirma
	elegidos  []string
	consultas int
}

func (s *selectorFirmaWebFalso) Estado(context.Context) (ports.EstadoProtocoloAfirma, error) {
	s.consultas++
	return s.estado, nil
}

func (s *selectorFirmaWebFalso) Elegir(_ context.Context, programa string) (ports.EstadoProtocoloAfirma, error) {
	s.elegidos = append(s.elegidos, programa)
	return s.estado, nil
}

func (s *selectorFirmaWebFalso) EjecutableAutoFirma(context.Context) (string, error) {
	if !s.estado.AutoFirmaInstalada {
		return "", ports.ErrAutoFirmaNoInstalada
	}
	return exeAutoFirmaPrueba, nil
}

type escenarioFirmaWeb struct {
	sel        *selectorFirmaWebFalso
	preguntas  []bool
	respuesta  eleccionProgramaFirmaWeb
	errDialogo error
	reenvios   [][2]string
	avisos     []string
	errReenvio error
	fallos     int
	firmarAqui bool
}

func nuevoEscenarioFirmaWeb(t *testing.T, estado ports.EstadoProtocoloAfirma) *escenarioFirmaWeb {
	t.Helper()
	e := &escenarioFirmaWeb{sel: &selectorFirmaWebFalso{estado: estado}}
	prevSel, prevPreg, prevFallo, prevReenv, prevAviso := nuevoSelectorFirmaWeb, preguntarProgramaFirmaWeb, preguntarFalloReenvio, reenviarAutoFirma, presentarAvisoFirmaWeb
	t.Cleanup(func() {
		nuevoSelectorFirmaWeb, preguntarProgramaFirmaWeb, preguntarFalloReenvio, reenviarAutoFirma, presentarAvisoFirmaWeb = prevSel, prevPreg, prevFallo, prevReenv, prevAviso
	})
	presentarAvisoFirmaWeb = func(_ context.Context, mensaje string) {
		e.avisos = append(e.avisos, mensaje)
	}
	nuevoSelectorFirmaWeb = func() ports.ProtocoloAfirma { return e.sel }
	preguntarProgramaFirmaWeb = func(_ context.Context, puedeRecordar bool) (eleccionProgramaFirmaWeb, error) {
		e.preguntas = append(e.preguntas, puedeRecordar)
		return e.respuesta, e.errDialogo
	}
	preguntarFalloReenvio = func(context.Context) (bool, error) {
		e.fallos++
		return e.firmarAqui, nil
	}
	reenviarAutoFirma = func(_ context.Context, exe, uri string) error {
		e.reenvios = append(e.reenvios, [2]string{exe, uri})
		return e.errReenvio
	}
	return e
}

const uriPrueba = "afirma://sign?op=sign&id=1"

func estadoPreguntar() ports.EstadoProtocoloAfirma {
	return ports.EstadoProtocoloAfirma{Soportado: true, Actual: "grxfirma", AutoFirmaInstalada: true,
		Preferencia: "preguntar", Efectiva: "preguntar"}
}

func TestFirmaWebPorDefectoNoPreguntaNiReenvia(t *testing.T) {
	e := nuevoEscenarioFirmaWeb(t, ports.EstadoProtocoloAfirma{Soportado: true, Actual: "grxfirma", AutoFirmaInstalada: true, Efectiva: "grxfirma"})
	if handled, _ := decidirProgramaFirmaWeb(context.Background(), uriPrueba); handled {
		t.Fatal("por defecto firma GrxFirma")
	}
	if len(e.preguntas) != 0 || len(e.reenvios) != 0 {
		t.Fatalf("no debía preguntar ni reenviar: %v %v", e.preguntas, e.reenvios)
	}
}

func TestFirmaWebPreguntarYRecordarAutoFirma(t *testing.T) {
	e := nuevoEscenarioFirmaWeb(t, estadoPreguntar())
	e.respuesta = eleccionProgramaFirmaWeb{programa: "autofirma", recordar: true}
	handled, code := decidirProgramaFirmaWeb(context.Background(), uriPrueba)
	if !handled || code != 0 {
		t.Fatalf("handled=%v code=%d", handled, code)
	}
	if len(e.preguntas) != 1 || !e.preguntas[0] {
		t.Fatalf("debía preguntar ofreciendo recordar: %v", e.preguntas)
	}
	if len(e.sel.elegidos) != 1 || e.sel.elegidos[0] != "autofirma" {
		t.Fatalf("recordar debe guardar la elección: %v", e.sel.elegidos)
	}
	if len(e.reenvios) != 1 || e.reenvios[0] != [2]string{exeAutoFirmaPrueba, uriPrueba} {
		t.Fatalf("reenvío: %v", e.reenvios)
	}
	if len(e.avisos) != 1 || e.avisos[0] != tl("excepciones.autofirma.aviso") {
		t.Fatalf("la persona debe ver en una ventana que firma AutoFirma: %q", e.avisos)
	}
}

func TestFirmaWebPreguntarYElegirGrxFirma(t *testing.T) {
	e := nuevoEscenarioFirmaWeb(t, estadoPreguntar())
	e.respuesta = eleccionProgramaFirmaWeb{programa: "grxfirma"}
	if handled, _ := decidirProgramaFirmaWeb(context.Background(), uriPrueba); handled {
		t.Fatal("con GrxFirma elegido sigue GrxFirma")
	}
	if len(e.sel.elegidos) != 0 || len(e.reenvios) != 0 {
		t.Fatalf("sin recordar no se guarda nada: %v %v", e.sel.elegidos, e.reenvios)
	}
}

func TestFirmaWebConPoliticaNoSeRecuerda(t *testing.T) {
	estado := estadoPreguntar()
	estado.Politica = "preguntar"
	e := nuevoEscenarioFirmaWeb(t, estado)
	e.respuesta = eleccionProgramaFirmaWeb{programa: "autofirma", recordar: true}
	decidirProgramaFirmaWeb(context.Background(), uriPrueba)
	if len(e.preguntas) != 1 || e.preguntas[0] {
		t.Fatalf("con la política no se ofrece recordar: %v", e.preguntas)
	}
	if len(e.sel.elegidos) != 0 {
		t.Fatalf("con la política no se cambia la preferencia: %v", e.sel.elegidos)
	}
	if len(e.avisos) != 1 {
		t.Fatalf("el reenvío a AutoFirma debe avisar una vez: %q", e.avisos)
	}
}

func TestFirmaWebEnlaceNoReenviableNoPregunta(t *testing.T) {
	e := nuevoEscenarioFirmaWeb(t, estadoPreguntar())
	if handled, _ := decidirProgramaFirmaWeb(context.Background(), `afirma://sign?x="1"`); handled {
		t.Fatal("un enlace que no se puede reenviar lo atiende GrxFirma")
	}
	if len(e.preguntas) != 0 {
		t.Fatal("no debía preguntar")
	}
}

func TestFirmaWebDialogoQueFallaFirmaConGrxFirma(t *testing.T) {
	e := nuevoEscenarioFirmaWeb(t, estadoPreguntar())
	e.errDialogo = errDialogoNoDisponible
	if handled, _ := decidirProgramaFirmaWeb(context.Background(), uriPrueba); handled {
		t.Fatal("sin diálogo firma GrxFirma")
	}
	if len(e.reenvios) != 0 {
		t.Fatal("nunca se reenvía en silencio")
	}
}

func TestFirmaWebPreferenciaAutoFirmaReenviaYSiFallaOfreceGrxFirma(t *testing.T) {
	estado := ports.EstadoProtocoloAfirma{Soportado: true, Actual: "grxfirma", AutoFirmaInstalada: true, Preferencia: "autofirma", Efectiva: "autofirma"}
	e := nuevoEscenarioFirmaWeb(t, estado)
	if handled, code := decidirProgramaFirmaWeb(context.Background(), uriPrueba); !handled || code != 0 || len(e.reenvios) != 1 {
		t.Fatalf("debía reenviar: %v %d %v", handled, code, e.reenvios)
	}
	if len(e.preguntas) != 0 {
		t.Fatal("con la preferencia no se pregunta")
	}

	e = nuevoEscenarioFirmaWeb(t, estado)
	e.errReenvio = errors.New("no arranca")
	e.firmarAqui = true
	if handled, _ := decidirProgramaFirmaWeb(context.Background(), uriPrueba); handled || e.fallos != 1 {
		t.Fatalf("si falla y elige GrxFirma, sigue GrxFirma: %v %d", handled, e.fallos)
	}
	e = nuevoEscenarioFirmaWeb(t, estado)
	e.errReenvio = errors.New("no arranca")
	if handled, code := decidirProgramaFirmaWeb(context.Background(), uriPrueba); !handled || code != 1 {
		t.Fatalf("si falla y cancela, termina: %v %d", handled, code)
	}
}

func TestFirmaWebProcesoReenviadoNoVuelveAReenviar(t *testing.T) {
	e := nuevoEscenarioFirmaWeb(t, ports.EstadoProtocoloAfirma{Soportado: true, Actual: "grxfirma", AutoFirmaInstalada: true, Efectiva: "autofirma"})
	t.Setenv(afirmahandler.EnvReenvioAfirma, "1")
	if handled, _ := decidirProgramaFirmaWeb(context.Background(), uriPrueba); handled {
		t.Fatal("un enlace devuelto por AutoFirma lo firma GrxFirma")
	}
	if e.sel.consultas != 0 || len(e.reenvios) != 0 {
		t.Fatalf("no debía consultar ni reenviar: %d %v", e.sel.consultas, e.reenvios)
	}
}

// El reenvío ocurre antes de preparar la confianza TLS y sin llamar al
// manejador de GrxFirma.
func TestFirmaWebReenvioAntesDeGrxFirma(t *testing.T) {
	e := nuevoEscenarioFirmaWeb(t, estadoPreguntar())
	e.respuesta = eleccionProgramaFirmaWeb{programa: "autofirma"}
	previousEnsure := ensureStartupLocalTLSTrust
	t.Cleanup(func() { ensureStartupLocalTLSTrust = previousEnsure })
	setTestUserHome(t, t.TempDir())
	ensureStartupLocalTLSTrust = func(context.Context, string) (bool, error) {
		t.Fatal("no debía preparar la confianza TLS para un enlace de AutoFirma")
		return false, nil
	}
	var stderr bytes.Buffer
	code := runConDependencias(context.Background(), []string{uriPrueba}, &stderr,
		func(context.Context, io.Writer, string) int {
			t.Fatal("GrxFirma no debía atender el enlace")
			return 1
		})
	if code != 0 || len(e.reenvios) != 1 {
		t.Fatalf("code=%d reenvíos=%v stderr=%q", code, e.reenvios, stderr.String())
	}
}

func TestAnalizarEleccionFirmaWeb(t *testing.T) {
	casos := map[string]eleccionProgramaFirmaWeb{
		"grxfirma":            {programa: "grxfirma"},
		"autofirma":           {programa: "autofirma"},
		"autofirma recordar":  {programa: "autofirma", recordar: true},
		"grxfirma recordar\n": {programa: "grxfirma", recordar: true},
	}
	for salida, want := range casos {
		if got, err := analizarEleccionFirmaWeb(salida); err != nil || got != want {
			t.Errorf("%q: %+v %v", salida, got, err)
		}
	}
	for _, salida := range []string{"", "preguntar", "autofirma recordar x", "autofirma sí", "/usr/bin/sh"} {
		if _, err := analizarEleccionFirmaWeb(salida); err == nil {
			t.Errorf("%q no es una respuesta válida", salida)
		}
	}
	if got := formatearEleccionFirmaWeb(eleccionProgramaFirmaWeb{programa: "autofirma", recordar: true}); got != "autofirma recordar" {
		t.Fatal(got)
	}
}

func TestModoDialogoFirmaWebSoloConArgumentosFijos(t *testing.T) {
	var out bytes.Buffer
	if handled, code := ejecutarModoDialogoFirmaWeb(context.Background(), []string{flagElegirProgramaFirmaWeb, uriPrueba}, &out); !handled || code != 2 {
		t.Fatalf("un argumento extra se rechaza: %v %d", handled, code)
	}
	if handled, _ := ejecutarModoDialogoFirmaWeb(context.Background(), []string{uriPrueba}, &out); handled {
		t.Fatal("un enlace no es un modo de diálogo")
	}
	if out.Len() != 0 {
		t.Fatalf("salida inesperada: %q", out.String())
	}
}
