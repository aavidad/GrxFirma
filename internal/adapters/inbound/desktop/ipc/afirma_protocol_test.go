// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/json"
	"runtime"
	"slices"
	"strings"
	"testing"

	"grxfirma/internal/ports"
)

type protocoloAfirmaFalso struct {
	estado  ports.EstadoProtocoloAfirma
	err     error
	elegido string
}

func (p *protocoloAfirmaFalso) Estado(context.Context) (ports.EstadoProtocoloAfirma, error) {
	return p.estado, p.err
}

func (p *protocoloAfirmaFalso) Elegir(_ context.Context, programa string) (ports.EstadoProtocoloAfirma, error) {
	p.elegido = programa
	if p.err != nil {
		return ports.EstadoProtocoloAfirma{}, p.err
	}
	p.estado.Actual = programa
	return p.estado, nil
}

func TestProtocoloAfirmaEstadoSinSelectorNoEsSoportado(t *testing.T) {
	m := &Manejador{}
	resp := m.despachar(context.Background(), peticion{Action: "afirma_handler_status"})
	data, ok := resp.Data.(resultadoProtocoloAfirma)
	if !resp.OK || !ok || data.Supported {
		t.Fatalf("respuesta inesperada: %+v", resp)
	}
}

func TestProtocoloAfirmaEstadoYEleccion(t *testing.T) {
	falso := &protocoloAfirmaFalso{estado: ports.EstadoProtocoloAfirma{
		Soportado: true, Actual: ports.ProgramaAfirmaGrxFirma, GrxFirmaInstalada: true,
		AutoFirmaInstalada: true, RutaAutoFirma: `C:\Program Files\Autofirma\Autofirma\Autofirma.exe`,
	}}
	m := &Manejador{ProtocoloAfirma: falso}
	resp := m.despachar(context.Background(), peticion{Action: "afirma_handler_status"})
	raw, _ := json.Marshal(resp.Data)
	for _, want := range []string{`"supported":true`, `"current":"grxfirma"`, `"autofirmaInstalled":true`, `"grxfirmaInstalled":true`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("falta %s en %s", want, raw)
		}
	}
	resp = m.despachar(context.Background(), peticion{
		Action: "afirma_handler_select",
		Params: json.RawMessage(`{"handler":"autofirma"}`),
	})
	data, _ := resp.Data.(resultadoProtocoloAfirma)
	if !resp.OK || falso.elegido != "autofirma" || data.Current != "autofirma" {
		t.Fatalf("elección no aplicada: %+v elegido=%q", resp, falso.elegido)
	}
}

func TestProtocoloAfirmaErroresConCodigoEstable(t *testing.T) {
	casos := map[error]string{
		ports.ErrProtocoloAfirmaAjeno:       "afirma_handler_foreign",
		ports.ErrAutoFirmaNoInstalada:       "autofirma_not_installed",
		ports.ErrGrxFirmaAfirmaNoInstalada:  "grxfirma_afirma_missing",
		ports.ErrProtocoloAfirmaNoSoportado: "afirma_handler_unsupported",
		ports.ErrProgramaAfirmaDesconocido:  "afirma_handler_invalid",
	}
	for err, code := range casos {
		m := &Manejador{ProtocoloAfirma: &protocoloAfirmaFalso{err: err}}
		resp := m.despachar(context.Background(), peticion{
			Action: "afirma_handler_select",
			Params: json.RawMessage(`{"handler":"grxfirma"}`),
		})
		if resp.OK || resp.ErrorCode != code || resp.Error == "" {
			t.Fatalf("%v: respuesta %+v", err, resp)
		}
		if strings.Contains(resp.Error, "Software\\") {
			t.Fatalf("el mensaje expone detalles del registro: %q", resp.Error)
		}
	}
}

func TestProtocoloAfirmaSoloSeAnunciaEnWindows(t *testing.T) {
	hello := desktopIPCHello()
	tiene := slices.Contains(hello.Actions, "afirma_handler_status") &&
		slices.Contains(hello.Actions, "afirma_handler_select")
	if tiene != (runtime.GOOS == "windows") {
		t.Fatalf("anuncio del selector en %s: %v", runtime.GOOS, tiene)
	}
}
