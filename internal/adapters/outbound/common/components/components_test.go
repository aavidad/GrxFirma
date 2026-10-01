// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package components

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestComprobarDevuelveSoloRelevantes(t *testing.T) {
	for _, e := range Comprobar() {
		if !e.Relevante {
			t.Fatalf("Comprobar() no debería incluir componentes no relevantes: %s", e.Nombre)
		}
	}
}

func TestCertutilSoloEsRelevanteEnLinux(t *testing.T) {
	certutil, ok := buscarComponente("certutil")
	if !ok {
		t.Fatal("certutil no esta en el catalogo")
	}
	for _, tc := range []struct {
		goos string
		want bool
	}{
		{goos: "linux", want: true},
		{goos: "windows", want: false},
		{goos: "darwin", want: false},
	} {
		if got := esRelevanteEn(certutil, tc.goos); got != tc.want {
			t.Errorf("esRelevanteEn(certutil, %q) = %t, want %t", tc.goos, got, tc.want)
		}
	}
}

func TestErrorFaltaMencionaComponenteYFinalidad(t *testing.T) {
	err := ErrorFalta("openssl")
	if err == nil {
		t.Fatal("ErrorFalta(openssl) no debería ser nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "openssl") {
		t.Fatalf("el mensaje debería mencionar 'openssl': %q", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "pkcs#12") {
		t.Fatalf("el mensaje debería explicar para qué sirve: %q", msg)
	}
}

func TestErrorFaltaComponenteDesconocido(t *testing.T) {
	err := ErrorFalta("herramienta-inexistente")
	if err == nil || !strings.Contains(err.Error(), "herramienta-inexistente") {
		t.Fatalf("ErrorFalta con nombre desconocido debería mencionarlo: %v", err)
	}
}

func TestEnvolverErrorEjecucionDetectaEjecutableAusente(t *testing.T) {
	// Simula el error que produce exec.Command cuando el binario no existe.
	_, execErr := exec.Command("componente-que-no-existe-xyz").Output()
	if execErr == nil {
		t.Skip("no se pudo generar un error de ejecución en esta plataforma")
	}
	wrapped := EnvolverErrorEjecucion("openssl", execErr)
	if wrapped == execErr {
		t.Fatal("EnvolverErrorEjecucion debería reemplazar un error de ejecutable ausente")
	}
	if !strings.Contains(wrapped.Error(), "openssl") {
		t.Fatalf("el error envuelto debería mencionar el componente: %q", wrapped.Error())
	}
}

func TestEnvolverErrorEjecucionRespetaOtrosErrores(t *testing.T) {
	otro := errors.New("el comando devolvió código 3")
	if got := EnvolverErrorEjecucion("openssl", otro); got != otro {
		t.Fatalf("un error no relacionado con ausencia debería pasar sin cambios, got %v", got)
	}
}
