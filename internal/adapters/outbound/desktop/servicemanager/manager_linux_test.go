// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package servicemanager

import (
	"fmt"
	"strings"
	"testing"
)

// validarSocketPath es un control de seguridad: la ruta se interpola en la
// plantilla de la unidad systemd, y llega por JSON desde los adaptadores IPC y
// REST. Sin estas comprobaciones se pueden anadir directivas o argumentos a un
// servicio que arranca en cada sesion del usuario.
func TestValidarSocketPath(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre string
		ruta   string
		valido bool
	}{
		{"ruta absoluta normal", "/run/user/1000/grxfirma.sock", true},
		{"con guiones y puntos", "/tmp/grxfirma.ipc.sock", true},
		{"acentos permitidos", "/home/josé/grxfirma.sock", true},

		{"vacia", "", false},
		{"relativa", "grxfirma.sock", false},
		{"relativa con punto", "./grxfirma.sock", false},

		// Inyeccion de directivas en el fichero .service
		{"salto de linea", "/tmp/s.sock\nExecStartPre=/bin/sh", false},
		{"retorno de carro", "/tmp/s.sock\rExecStartPre=/bin/sh", false},
		{"nulo", "/tmp/s.sock\x00", false},
		{"tabulador", "/tmp/s.sock\tmas", false},
		{"del", "/tmp/s.sock\x7f", false},

		// Metacaracteres de fichero ini
		{"corchete abre", "/tmp/[Service]", false},
		{"corchete cierra", "/tmp/s]ock", false},
		{"igual", "/tmp/s=ock", false},
		{"barra invertida", "/tmp/s\\ock", false},
		{"comilla doble", "/tmp/s\"ock", false},
		{"comilla simple", "/tmp/s'ock", false},

		// Inyeccion de argumentos: systemd separa ExecStart por espacios.
		{"espacio simple", "/tmp/s.sock --server-modo rest", false},
		{"espacio al final", "/tmp/s.sock ", false},
		{"espacio unicode", "/tmp/s.sock --flag", false},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			t.Parallel()
			err := validarSocketPath(caso.ruta)
			if caso.valido && err != nil {
				t.Fatalf("validarSocketPath(%q) = %v, se esperaba valida", caso.ruta, err)
			}
			if !caso.valido && err == nil {
				t.Fatalf("validarSocketPath(%q) debio rechazarse", caso.ruta)
			}
		})
	}
}

// La plantilla solo debe crecer por donde esperamos. Si una ruta valida
// pudiera anadir lineas o argumentos, la unidad generada lo delataria.
func TestPlantillaUnidadNoCreceConRutasValidas(t *testing.T) {
	t.Parallel()

	const ruta = "/run/user/1000/grxfirma.sock"
	if err := validarSocketPath(ruta); err != nil {
		t.Fatalf("la ruta de referencia debe ser valida: %v", err)
	}

	unidad := fmt.Sprintf(plantillaUnidad, "/usr/bin/grxfirma-gui", ruta)

	esperadas := strings.Count(plantillaUnidad, "\n")
	if got := strings.Count(unidad, "\n"); got != esperadas {
		t.Fatalf("la unidad generada tiene %d lineas y la plantilla %d", got, esperadas)
	}

	var execStart string
	for _, linea := range strings.Split(unidad, "\n") {
		if strings.HasPrefix(linea, "ExecStart=") {
			execStart = linea
		}
	}
	if execStart == "" {
		t.Fatal("la unidad debe declarar ExecStart")
	}
	// ExecStart=<binario> --server --server-modo ipc --ipc-socket <ruta>
	if campos := strings.Fields(execStart); len(campos) != 6 {
		t.Fatalf("ExecStart tiene %d campos, se esperaban 6: %q", len(campos), execStart)
	}
	if !strings.HasSuffix(execStart, ruta) {
		t.Fatalf("ExecStart debe terminar en la ruta del socket: %q", execStart)
	}
}
