// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/domain"
)

type gestionConfianzaIPCAuditadaMock struct {
	gestionConfianzaIPCMock
	errListar   error
	operaciones []string
	aplicadas   []domain.ObstaculoConfianza
}

func (m *gestionConfianzaIPCAuditadaMock) Listar(ctx context.Context) (domain.ConfianzaForzada, error) {
	if m.errListar != nil {
		return domain.ConfianzaForzada{}, m.errListar
	}
	return m.gestionConfianzaIPCMock.Listar(ctx)
}

func (m *gestionConfianzaIPCAuditadaMock) RegistrarAplicadas(_ context.Context, operacion string, aplicadas []domain.ObstaculoConfianza) {
	m.operaciones = append(m.operaciones, operacion)
	m.aplicadas = append(m.aplicadas, aplicadas...)
}

// Veri*Factu en el escritorio usa lo aceptado (SHA-1) con el mismo registro
// que la verificación y lo deja en la auditoría.
func TestConfianzaForzadaVeriFactu_EscritorioUsaLoAceptadoYAudita(t *testing.T) {
	sha1 := domain.ObstaculoConfianza{Tipo: domain.ConfianzaAlgoritmoSHA1}
	gestion := &gestionConfianzaIPCAuditadaMock{gestionConfianzaIPCMock: gestionConfianzaIPCMock{
		confianza: domain.ConfianzaForzada{Aceptaciones: []domain.AceptacionConfianza{{Tipo: domain.ConfianzaAlgoritmoSHA1}}},
	}}
	m := &Manejador{ConfianzaForzada: gestion}
	ctx, registro := m.contextoConfianzaForzada(context.Background())
	if registro == nil || domain.RegistroConfianzaForzadaDe(ctx) != registro || !registro.Decidir(sha1) {
		t.Fatal("el escritorio debe aplicar SHA-1 aceptado en Veri*Factu")
	}
	m.auditarConfianzaForzadaAplicada(ctx, "verifactu_validar", registro)
	if len(gestion.operaciones) != 1 || gestion.operaciones[0] != "verifactu_validar" || len(gestion.aplicadas) != 1 {
		t.Fatalf("auditoría: %v %v", gestion.operaciones, gestion.aplicadas)
	}

	gestion.errListar = errors.New("config.json ilegible")
	_, registro = m.contextoConfianzaForzada(context.Background())
	if registro == nil || !registro.Confianza().Prohibida || registro.Decidir(sha1) {
		t.Fatal("lo aceptado ilegible debe tratarse como prohibido")
	}

	sin := &Manejador{}
	ctx, registro = sin.contextoConfianzaForzada(context.Background())
	if registro != nil || domain.RegistroConfianzaForzadaDe(ctx) != nil {
		t.Fatal("sin gestión no hay registro")
	}
}
