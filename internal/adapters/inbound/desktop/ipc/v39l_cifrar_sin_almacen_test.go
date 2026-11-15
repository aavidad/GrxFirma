// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"path/filepath"
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/common/protector"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"grxfirma/internal/testsupport/perfil"
)

// Lote 39l: en Linux sin almacén de secretos, entrar en Cifrar carga los
// destinatarios y eso intenta crear la identidad propia. Sin directiva de
// máquina el bloqueo debe llegar activable y con su clave normal, nunca
// como «la organización no permite».
func TestV39lDestinatariosSinAlmacenBloqueoActivableSinDirectiva(t *testing.T) {
	home := perfil.Nuevo(t)
	cfg := perfil.DirConfig(home)
	e, ok := excepciones.Find("alto_clave_fichero_crear")
	if !ok {
		t.Fatal("falta alto_clave_fichero_crear en el catálogo")
	}
	machinepolicy.UnsetForTest(t, e.PolicyKey)
	machinepolicy.UnsetForTest(t, "permitir_alto_clave_fichero_leer")
	m := &Manejador{
		Loc:           localizador.Para("es"),
		ConfigDir:     cfg,
		Destinatarios: protector.NuevoLocalStrongKeyring(cfg, filepath.Join(home, "datos"), nil),
	}
	calculado := 0
	ctx := conEstadoInterfazPerezoso(withUnsafeRetryOwner(context.Background()), func() string {
		calculado++
		return interfazVerificada
	})
	resp := m.despachar(ctx, peticion{Action: "protection_recipients"})
	if resp.OK || resp.ExceptionID != e.ID || resp.ErrorCode != "unsafe_exception_blocked" {
		t.Fatalf("sin almacén se esperaba el bloqueo de %s: %+v", e.ID, resp)
	}
	if resp.Activable == nil || !*resp.Activable || resp.Exception == nil || !resp.Exception.Activable {
		t.Fatalf("sin directiva el bloqueo debe ser activable: %+v", resp)
	}
	loc := localizador.Para("es")
	if resp.Error != loc.T(e.BlockKey) || resp.Error == loc.T(e.OrganizationKey) {
		t.Fatalf("el texto debe ser el del bloqueo normal, no el de la organización: %q", resp.Error)
	}
	if calculado != 1 {
		t.Fatalf("el estado de la interfaz se calcula una sola vez y solo al bloquear: %d", calculado)
	}

	// La directiva de máquina sigue mandando: si la prohíbe, no es activable.
	machinepolicy.SetForTest(t, e.PolicyKey, false)
	resp = m.despachar(conEstadoInterfazPerezoso(withUnsafeRetryOwner(context.Background()), func() string { return interfazVerificada }), peticion{Action: "protection_recipients"})
	if resp.OK || resp.ExceptionID != e.ID || resp.Activable == nil || *resp.Activable || resp.RetryID != "" {
		t.Fatalf("con la directiva en contra el bloqueo no se puede activar: %+v", resp)
	}
}

// Una interfaz que no es la vinculada sigue sin poder activar nada.
func TestV39lDestinatariosSinAlmacenInterfazNoVerificadaNoActiva(t *testing.T) {
	home := perfil.Nuevo(t)
	cfg := perfil.DirConfig(home)
	e, _ := excepciones.Find("alto_clave_fichero_crear")
	machinepolicy.UnsetForTest(t, e.PolicyKey)
	m := &Manejador{
		Loc:           localizador.Para("es"),
		ConfigDir:     cfg,
		Destinatarios: protector.NuevoLocalStrongKeyring(cfg, filepath.Join(home, "datos"), nil),
	}
	ctx := conEstadoInterfazPerezoso(withUnsafeRetryOwner(context.Background()), func() string { return interfazNoVerificada })
	resp := m.despachar(ctx, peticion{Action: "protection_recipients"})
	if resp.OK || resp.ExceptionID != e.ID || resp.Activable == nil || *resp.Activable || resp.RetryID != "" {
		t.Fatalf("desde fuera de la interfaz vinculada no se ofrece activar: %+v", resp)
	}
}

// Las acciones que no necesitan el estado no lo calculan.
func TestV39lEstadoInterfazPerezosoNoSeCalculaSiNoHaceFalta(t *testing.T) {
	calculado := 0
	ctx := conEstadoInterfazPerezoso(context.Background(), func() string { calculado++; return interfazVerificada })
	if calculado != 0 {
		t.Fatal("calculado antes de tiempo")
	}
	// Dos lecturas seguidas: el estado se calcula una sola vez.
	primera, segunda := esInterfazVerificada(ctx), esInterfazVerificada(ctx)
	if !primera || !segunda || calculado != 1 {
		t.Fatalf("cálculos: %d", calculado)
	}
	// Una acción sin bloqueo no llega a calcularlo.
	sinUso := 0
	m := &Manejador{Loc: localizador.Para("es")}
	_ = m.despachar(conEstadoInterfazPerezoso(withUnsafeRetryOwner(context.Background()), func() string { sinUso++; return interfazVerificada }), peticion{Action: "accion_inexistente"})
	if sinUso != 0 {
		t.Fatalf("una acción sin bloqueo calculó el estado de la interfaz %d veces", sinUso)
	}
	if esInterfazVerificada(conEstadoInterfazPerezoso(context.Background(), nil)) {
		t.Fatal("sin cálculo no es la interfaz vinculada")
	}
}
