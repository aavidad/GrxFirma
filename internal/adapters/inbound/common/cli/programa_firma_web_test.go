// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"strings"
	"testing"

	"grxfirma/internal/ports"
)

type protocoloAfirmaCLIFalso struct {
	estado  ports.EstadoProtocoloAfirma
	err     error
	elegido []string
}

func (p *protocoloAfirmaCLIFalso) Estado(context.Context) (ports.EstadoProtocoloAfirma, error) {
	return p.estado, p.err
}

func (p *protocoloAfirmaCLIFalso) Elegir(_ context.Context, programa string) (ports.EstadoProtocoloAfirma, error) {
	p.elegido = append(p.elegido, programa)
	if p.err != nil {
		return ports.EstadoProtocoloAfirma{}, p.err
	}
	p.estado.Preferencia, p.estado.Efectiva = programa, programa
	return p.estado, nil
}

func adaptadorProgramaFirmaWeb(p ports.ProtocoloAfirma) (*Adaptador, *strings.Builder, *strings.Builder) {
	var stdout, stderr strings.Builder
	a := New(nil, nil).WithProtocoloAfirma(p)
	a.Stdout, a.Stderr = &stdout, &stderr
	a.esInteractivo = func() bool { return false }
	return a, &stdout, &stderr
}

func TestCLIProgramaFirmaWebVer(t *testing.T) {
	falso := &protocoloAfirmaCLIFalso{estado: ports.EstadoProtocoloAfirma{
		Soportado: true, Actual: "grxfirma", Efectiva: "preguntar", Preferencia: "preguntar",
		AutoFirmaInstalada: true, RutaAutoFirma: `C:\Program Files\Autofirma\Autofirma\Autofirma.exe`,
	}}
	a, stdout, _ := adaptadorProgramaFirmaWeb(falso)
	if rc := a.Run(context.Background(), []string{"-ver-programa-firma-web"}); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if !strings.Contains(stdout.String(), "protocolo.estado.preguntar") || !strings.Contains(stdout.String(), "Autofirma.exe") {
		t.Fatalf("salida: %s", stdout.String())
	}
	a, stdout, _ = adaptadorProgramaFirmaWeb(falso)
	if rc := a.Run(context.Background(), []string{"-web-signing-program", "-salida-json"}); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if !strings.Contains(strings.ReplaceAll(stdout.String(), " ", ""), `"effective":"preguntar"`) {
		t.Fatalf("JSON: %s", stdout.String())
	}
	if len(falso.elegido) != 0 {
		t.Fatalf("ver no debe cambiar nada: %v", falso.elegido)
	}
}

func TestCLIProgramaFirmaWebElegir(t *testing.T) {
	falso := &protocoloAfirmaCLIFalso{estado: ports.EstadoProtocoloAfirma{Soportado: true, Actual: "grxfirma", AutoFirmaInstalada: true}}
	a, _, _ := adaptadorProgramaFirmaWeb(falso)
	if rc := a.Run(context.Background(), []string{"-elegir-programa-firma-web", " AutoFirma "}); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if len(falso.elegido) != 1 || falso.elegido[0] != ports.ProgramaAfirmaAutoFirma {
		t.Fatalf("elegido: %v", falso.elegido)
	}
	a, _, stderr := adaptadorProgramaFirmaWeb(falso)
	if rc := a.Run(context.Background(), []string{"-set-web-signing-program", "AutoFirma.exe"}); rc != 1 {
		t.Fatalf("un valor no válido debe fallar: rc=%d", rc)
	}
	if !strings.Contains(stderr.String(), "cli.web_signing.invalid") || len(falso.elegido) != 1 {
		t.Fatalf("valor no válido: %s %v", stderr.String(), falso.elegido)
	}
}

func TestCLIProgramaFirmaWebConPolitica(t *testing.T) {
	falso := &protocoloAfirmaCLIFalso{err: ports.ErrProtocoloAfirmaPolitica}
	a, _, stderr := adaptadorProgramaFirmaWeb(falso)
	if rc := a.Run(context.Background(), []string{"-elegir-programa-firma-web", "grxfirma"}); rc != 1 {
		t.Fatalf("rc=%d", rc)
	}
	if !strings.Contains(stderr.String(), "protocolo.error.politica") {
		t.Fatalf("debe decir que lo fija la organización: %s", stderr.String())
	}
}

func TestCLIProgramaFirmaWebSinSelector(t *testing.T) {
	var stdout, stderr strings.Builder
	a := New(nil, nil)
	a.Stdout, a.Stderr = &stdout, &stderr
	if rc := a.Run(context.Background(), []string{"-ver-programa-firma-web"}); rc != 1 {
		t.Fatalf("rc=%d", rc)
	}
	if !strings.Contains(stderr.String(), "protocolo.error.no_disponible") {
		t.Fatalf("salida: %s", stderr.String())
	}
}
