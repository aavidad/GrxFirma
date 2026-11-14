// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/domain"
)

type gestionConfianzaAuditadaMock struct {
	gestionConfianzaMock
	errListar   error
	operaciones []string
	aplicadas   []domain.ObstaculoConfianza
}

func (m *gestionConfianzaAuditadaMock) Listar(ctx context.Context) (domain.ConfianzaForzada, error) {
	if m.errListar != nil {
		return domain.ConfianzaForzada{}, m.errListar
	}
	return m.gestionConfianzaMock.Listar(ctx)
}

func (m *gestionConfianzaAuditadaMock) RegistrarAplicadas(_ context.Context, operacion string, aplicadas []domain.ObstaculoConfianza) {
	m.operaciones = append(m.operaciones, operacion)
	m.aplicadas = append(m.aplicadas, aplicadas...)
}

// Veri*Factu en la CLI usa lo aceptado (SHA-1) con el mismo registro que la
// verificación y lo deja en la auditoría.
func TestConfianzaForzadaVeriFactu_CLIUsaLoAceptadoYAudita(t *testing.T) {
	sha1 := domain.ObstaculoConfianza{Tipo: domain.ConfianzaAlgoritmoSHA1}
	gestion := &gestionConfianzaAuditadaMock{gestionConfianzaMock: gestionConfianzaMock{
		confianza: domain.ConfianzaForzada{Aceptaciones: []domain.AceptacionConfianza{{Tipo: domain.ConfianzaAlgoritmoSHA1}}},
	}}
	a := &Adaptador{ConfianzaForzada: gestion}
	ctx, registro := a.contextoConfianzaForzada(context.Background())
	if registro == nil || domain.RegistroConfianzaForzadaDe(ctx) != registro {
		t.Fatal("la CLI debe adjuntar el registro de lo aceptado al contexto")
	}
	if !registro.Decidir(sha1) {
		t.Fatal("SHA-1 aceptado debe aplicarse")
	}
	a.auditarConfianzaForzadaAplicada(ctx, "verifactu_validar", registro)
	if len(gestion.operaciones) != 1 || gestion.operaciones[0] != "verifactu_validar" || len(gestion.aplicadas) != 1 || gestion.aplicadas[0].Tipo != domain.ConfianzaAlgoritmoSHA1 {
		t.Fatalf("auditoría: %v %v", gestion.operaciones, gestion.aplicadas)
	}

	// Si no se puede leer lo aceptado, cuenta como prohibido.
	gestion.errListar = errors.New("config.json ilegible")
	_, registro = a.contextoConfianzaForzada(context.Background())
	if registro == nil || !registro.Confianza().Prohibida || registro.Decidir(sha1) {
		t.Fatal("lo aceptado ilegible debe tratarse como prohibido")
	}

	// Sin gestión (no debería ocurrir en la CLI) no se adjunta nada.
	sin := &Adaptador{}
	ctx, registro = sin.contextoConfianzaForzada(context.Background())
	if registro != nil || domain.RegistroConfianzaForzadaDe(ctx) != nil {
		t.Fatal("sin gestión no hay registro")
	}
	sin.auditarConfianzaForzadaAplicada(ctx, "verifactu_validar", registro)
}
