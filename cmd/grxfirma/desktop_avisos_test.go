// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"os"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

// Lote 17b (2): la interfaz Fyne aplica al verificar lo que el usuario
// aceptó en este equipo, como la CLI, y lo enseña. La gestión de la lista
// sigue en la CLI, Qt y WinUI.
func TestFyneVerificaConLaConfianzaAceptadaEnEsteEquipo(t *testing.T) {
	fuente, err := os.ReadFile("desktop_fyne.go")
	if err != nil {
		t.Fatal(err)
	}
	// Con core.autocrlf (CI de Windows) los fuentes llegan con CRLF.
	texto := strings.ReplaceAll(string(fuente), "\r\n", "\n")
	if strings.Count(texto, "construirServiciosEscritorio(") != 2 || strings.Contains(texto, "construirServicios(") {
		t.Fatal("el arranque y la recarga de Fyne deben componer los servicios con construirServiciosEscritorio")
	}
	if !strings.Contains(texto, "avisosVerificacionDesktop(localizador.Detectar(), resultado, firma, rutaOriginal)") {
		t.Fatal("la verificación de Fyne debe enseñar los avisos y lo aplicado")
	}
	arranque, err := os.ReadFile("bootstrap.go")
	if err != nil {
		t.Fatal(err)
	}
	cuerpo := strings.ReplaceAll(string(arranque), "\r\n", "\n")
	inicio := strings.Index(cuerpo, "func construirServiciosEscritorio(")
	if inicio < 0 {
		t.Fatal("bootstrap.go no define construirServiciosEscritorio")
	}
	cuerpo = cuerpo[inicio:]
	if fin := strings.Index(cuerpo, "\n}\n"); fin >= 0 {
		cuerpo = cuerpo[:fin]
	}
	if !strings.Contains(cuerpo, "servicios.verificar.ConConfianzaForzada(config.NuevoAlmacenConfianzaForzada(servicios.configDir))") {
		t.Fatal("construirServiciosEscritorio debe aplicar el almacén de confianza aceptada")
	}
}

func TestAvisosVerificacionDesktop(t *testing.T) {
	loc := localizador.Para("es")
	aplicada := domain.ConfianzaCADesconocida.ClaveAplicada()
	resultado := application.VerifyResult{
		Verification: domain.VerificationResult{Valid: true, Warnings: []string{aplicada}},
	}
	texto := avisosVerificacionDesktop(loc, resultado, "/tmp/firma.csig", "")
	if !strings.Contains(texto, loc.T("Advertencias")) || !strings.Contains(texto, escaparMarkdownDesktop(loc.T(aplicada))) {
		t.Fatalf("faltan los avisos traducidos de lo aplicado: %q", texto)
	}
	if strings.Contains(texto, aplicada) || strings.Contains(texto, "-aceptar-confianza") {
		t.Fatalf("sin nada que aceptar no sale la clave cruda ni la orden: %q", texto)
	}

	resultado = application.VerifyResult{
		Verification: domain.VerificationResult{Warnings: []string{domain.ClavePistaConfianzaForzable}},
		ConfianzaForzada: application.ResultadoConfianzaForzada{
			Forzable: []domain.ObstaculoConfianza{{Tipo: domain.ConfianzaCADesconocida, Huella: "ab"}},
		},
	}
	texto = avisosVerificacionDesktop(loc, resultado, "/tmp/mi firma.csig", "/tmp/acta.pdf")
	for _, want := range []string{
		escaparMarkdownDesktop(loc.T("verify.forced_trust.cli.hint_command")),
		"grxfirma -modo-cli -aceptar-confianza -entrada '/tmp/mi firma.csig' -documento-original /tmp/acta.pdf",
	} {
		if !strings.Contains(texto, want) {
			t.Fatalf("falta %q en %q", want, texto)
		}
	}
	// Una ruta con salto de línea no se copia en la orden.
	texto = avisosVerificacionDesktop(loc, resultado, "/tmp/a\n```\nb.csig", "")
	if strings.Contains(texto, "-aceptar-confianza") {
		t.Fatalf("una ruta con saltos de línea no debe salir en la orden: %q", texto)
	}
	if got := escaparMarkdownDesktop("a*b_[c]`d`"); got != `a\*b\_\[c\]\`+"`"+`d\`+"`" {
		t.Fatalf("escape markdown = %q", got)
	}
}
