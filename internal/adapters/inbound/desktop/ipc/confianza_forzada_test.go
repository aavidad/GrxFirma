// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

var huellaIPCPrueba = strings.Repeat("d", 64)

type gestionConfianzaIPCMock struct {
	confianza domain.ConfianzaForzada
	aceptadas []domain.ObstaculoConfianza
	quitadas  []string
}

func (m *gestionConfianzaIPCMock) Listar(context.Context) (domain.ConfianzaForzada, error) {
	return m.confianza, nil
}

func (m *gestionConfianzaIPCMock) Aceptar(_ context.Context, o domain.ObstaculoConfianza) (domain.AceptacionConfianza, error) {
	if m.confianza.Prohibida {
		return domain.AceptacionConfianza{}, domain.ErrConfianzaProhibida
	}
	m.aceptadas = append(m.aceptadas, o)
	a := domain.AceptacionConfianza{Tipo: o.Tipo, Huella: o.Huella, Sujeto: o.Sujeto, AceptadaEn: time.Now()}
	m.confianza.Aceptaciones = append(m.confianza.Aceptaciones, a)
	return a, nil
}

func (m *gestionConfianzaIPCMock) Quitar(_ context.Context, tipo domain.TipoConfianzaForzada, huella string) (bool, error) {
	m.quitadas = append(m.quitadas, string(tipo)+"|"+huella)
	return true, nil
}

func manejadorConfianza(t *testing.T, forzable []domain.ObstaculoConfianza, gestion *gestionConfianzaIPCMock) (*Manejador, string) {
	t.Helper()
	inputPath := filepath.Join(t.TempDir(), "firma.csig")
	if err := os.WriteFile(inputPath, []byte("firma"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &Manejador{
		Verificar: &stubVerificar{result: application.VerifyResult{
			Verification:     domain.NewVerificationFailure("CAdES", "autoridad desconocida", nil),
			ConfianzaForzada: application.ResultadoConfianzaForzada{Forzable: forzable, Prohibida: gestion.confianza.Prohibida},
		}},
		ConfianzaForzada: gestion,
	}
	return m, inputPath
}

type itemAceptarIPC struct {
	Type        string `json:"type"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// Lote 14: el frontend solo puede aceptar lo que una verificación de esta
// sesión ofreció; no puede inventar huellas.
func TestConfianzaForzadaIPC_SoloAceptaLoOfrecido(t *testing.T) {
	gestion := &gestionConfianzaIPCMock{}
	forzable := []domain.ObstaculoConfianza{{Tipo: domain.ConfianzaCADesconocida, Huella: huellaIPCPrueba, Sujeto: "CN=Raíz"}}
	m, inputPath := manejadorConfianza(t, forzable, gestion)

	inventado := map[string]any{"items": []itemAceptarIPC{{Type: "ca_desconocida", Fingerprint: strings.Repeat("e", 64)}}}
	if resp := m.despachar(context.Background(), peticionJSON(t, "forced_trust_accept", inventado)); resp.OK {
		t.Fatal("se aceptó una huella que ninguna verificación ofreció")
	}
	ofrecido := map[string]any{"items": []itemAceptarIPC{{Type: "ca_desconocida", Fingerprint: huellaIPCPrueba}}}
	if resp := m.despachar(context.Background(), peticionJSON(t, "forced_trust_accept", ofrecido)); resp.OK {
		t.Fatal("antes de verificar no hay nada que aceptar")
	}

	resp := m.despachar(context.Background(), peticionJSON(t, "verify", paramsVerify{InputPath: inputPath}))
	if !resp.OK {
		t.Fatalf("verify: %#v", resp)
	}
	data := resp.Data.(resultadoVerificacion)
	if data.ForcedTrust == nil || !data.ForcedTrust.Available || len(data.ForcedTrust.Acceptable) != 1 {
		t.Fatalf("la verificación debe ofrecer aceptar: %#v", data.ForcedTrust)
	}
	if data.ForcedTrust.Acceptable[0].Notice != "verify.forced_trust.notice.ca_desconocida" {
		t.Fatalf("el aviso viaja como clave de catálogo: %#v", data.ForcedTrust.Acceptable[0])
	}

	if resp := m.despachar(context.Background(), peticionJSON(t, "forced_trust_accept", inventado)); resp.OK {
		t.Fatal("tras verificar, una huella distinta sigue sin poder aceptarse")
	}
	resp = m.despachar(context.Background(), peticionJSON(t, "forced_trust_accept", ofrecido))
	if !resp.OK {
		t.Fatalf("aceptar lo ofrecido: %#v", resp)
	}
	if len(gestion.aceptadas) != 1 || gestion.aceptadas[0].Sujeto != "CN=Raíz" {
		t.Fatalf("aceptadas: %v", gestion.aceptadas)
	}
	lista := resp.Data.(resultadoListaConfianzaForzada)
	if len(lista.Entries) != 1 || lista.Entries[0].Fingerprint != huellaIPCPrueba {
		t.Fatalf("la respuesta devuelve la lista actualizada: %#v", lista)
	}

	resp = m.despachar(context.Background(), peticionJSON(t, "forced_trust_remove", itemAceptarIPC{Type: "ca_desconocida", Fingerprint: huellaIPCPrueba}))
	if !resp.OK || len(gestion.quitadas) != 1 {
		t.Fatalf("quitar: %#v %v", resp, gestion.quitadas)
	}
}

func TestConfianzaForzadaIPC_PoliticaProhibe(t *testing.T) {
	gestion := &gestionConfianzaIPCMock{confianza: domain.ConfianzaForzada{Prohibida: true}}
	forzable := []domain.ObstaculoConfianza{{Tipo: domain.ConfianzaAlgoritmoSHA1}}
	m, inputPath := manejadorConfianza(t, forzable, gestion)
	resp := m.despachar(context.Background(), peticionJSON(t, "verify", paramsVerify{InputPath: inputPath}))
	data := resp.Data.(resultadoVerificacion)
	if data.ForcedTrust == nil || data.ForcedTrust.Available || !data.ForcedTrust.Forbidden {
		t.Fatalf("con la política prohibiéndolo no se ofrece el botón: %#v", data.ForcedTrust)
	}
	resp = m.despachar(context.Background(), peticionJSON(t, "forced_trust_accept", map[string]any{"items": []itemAceptarIPC{{Type: "algoritmo_sha1"}}}))
	if resp.OK || resp.ErrorCode != "forced_trust_forbidden" {
		t.Fatalf("aceptar con la política prohibiéndolo: %#v", resp)
	}
}

// Sin gestión configurada (otros canales) verify no lleva forcedTrust y las
// acciones fallan.
func TestConfianzaForzadaIPC_SinGestion(t *testing.T) {
	gestion := &gestionConfianzaIPCMock{}
	m, inputPath := manejadorConfianza(t, []domain.ObstaculoConfianza{{Tipo: domain.ConfianzaAlgoritmoSHA1}}, gestion)
	m.ConfianzaForzada = nil
	resp := m.despachar(context.Background(), peticionJSON(t, "verify", paramsVerify{InputPath: inputPath}))
	if data := resp.Data.(resultadoVerificacion); data.ForcedTrust != nil {
		t.Fatalf("sin gestión no se ofrece nada: %#v", data.ForcedTrust)
	}
	for _, accion := range []string{"forced_trust_list", "forced_trust_accept", "forced_trust_remove"} {
		if resp := m.despachar(context.Background(), peticionJSON(t, accion, map[string]any{"items": []itemAceptarIPC{{Type: "algoritmo_sha1"}}})); resp.OK {
			t.Fatalf("%s sin gestión debe fallar", accion)
		}
	}
}
