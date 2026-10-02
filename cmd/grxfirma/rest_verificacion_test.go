// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestExtraerFlagsREST_SoloVerificacion(t *testing.T) {
	cfg, resto := extraerFlagsREST([]string{
		"-rest-solo-verificacion",
		"-verificacion-anclas", "anclas-prueba",
		"-verificacion-crl", "crl-prueba",
		"-direccion-rest", "127.0.0.1:63118",
	})
	if len(resto) != 0 {
		t.Fatalf("argumentos no consumidos: %v", resto)
	}
	if !cfg.habilitado || !cfg.soloVerificacion {
		t.Fatal("el modo solo verificación debe habilitar REST en modo restringido")
	}
	if cfg.anclasVerificacion != "anclas-prueba" || cfg.crlVerificacion != "crl-prueba" {
		t.Fatalf("rutas inesperadas: %q %q", cfg.anclasVerificacion, cfg.crlVerificacion)
	}

	alias, _ := extraerFlagsREST([]string{"--rest-verify-only", "--verify-anchors", "a.pem", "--verify-crl-dir", "crl"})
	if !alias.soloVerificacion || alias.anclasVerificacion != "a.pem" || alias.crlVerificacion != "crl" {
		t.Fatalf("alias en inglés no reconocidos: %+v", alias)
	}
}

func TestValidarPoliticaSoloVerificacion(t *testing.T) {
	base := restFlags{addr: "127.0.0.1:63118", soloVerificacion: true, habilitado: true, anclasVerificacion: "anclas.pem"}
	if err := validarPoliticaSoloVerificacion(base); err != nil {
		t.Fatalf("configuración mínima válida rechazada: %v", err)
	}
	sinAnclas := base
	sinAnclas.anclasVerificacion = ""
	if err := validarPoliticaSoloVerificacion(sinAnclas); err == nil || !strings.Contains(err.Error(), "-verificacion-anclas") {
		t.Fatalf("se esperaba exigir anclas locales: %v", err)
	}
	conHuellas := base
	conHuellas.certFingerprintsCSV = strings.Repeat("a", 64)
	if err := validarPoliticaSoloVerificacion(conHuellas); err == nil {
		t.Fatal("la autenticación por certificado no se publica en este modo")
	}
	remoto := base
	remoto.addr = "0.0.0.0:63118"
	if err := validarPoliticaSoloVerificacion(remoto); err == nil {
		t.Fatal("una dirección no loopback exige -rest-publico")
	}
	remoto.permitirRemoto = true
	if err := validarPoliticaSoloVerificacion(remoto); err == nil {
		t.Fatal("una dirección no loopback exige token")
	}
	remoto.token = "token-sintetico"
	if err := validarPoliticaSoloVerificacion(remoto); err != nil {
		t.Fatalf("exposición deliberada con token rechazada: %v", err)
	}
}

func TestConstruirFuentesVerificacion_RutasInvalidas(t *testing.T) {
	dir := t.TempDir()
	if _, err := construirFuentesVerificacion(restFlags{anclasVerificacion: filepath.Join(dir, "no-existe.pem")}); err == nil {
		t.Fatal("una ruta de anclas inexistente debe impedir el arranque")
	}
	if _, err := construirFuentesVerificacion(restFlags{crlVerificacion: filepath.Join(dir, "no-existe")}); err == nil {
		t.Fatal("un directorio de CRL inexistente debe impedir el arranque")
	}
	fuentes, err := construirFuentesVerificacion(restFlags{crlVerificacion: dir})
	if err != nil || fuentes.evaluador == nil || !fuentes.conCRL {
		t.Fatalf("directorio de CRL vacío pero válido: %+v %v", fuentes, err)
	}
}
