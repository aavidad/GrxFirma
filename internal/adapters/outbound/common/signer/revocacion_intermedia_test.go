// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"testing"

	"grxfirma/internal/domain"
)

// TestCAdES_RevocacionConsultaIntermediaConCadenaIncompleta reproduce E1-09:
// la firma trae hoja e intermedia, pero no la raíz. La revocación de la
// intermedia también se comprueba.
func TestCAdES_RevocacionConsultaIntermediaConCadenaIncompleta(t *testing.T) {
	p := nuevaPKILote3(t, nil)
	contenido := []byte("pedido")
	cms := firmarBESLote3(t, p, contenido, p.intermedia)

	verificador := NewCAdESVerifierWithChecker(comprobadorLote3(p.intermedia))
	res, _, err := verificador.VerifyDetachedCMSWithAnchors(context.Background(), cms, contenido, p.anclas)
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid {
		t.Fatalf("una intermedia revocada pasó como válida: %s %v", res.Reason, res.Details)
	}
	if res.Certificate.Status != domain.VerificationStatusInvalid {
		t.Fatalf("estado del certificado=%s, se esperaba inválido por revocación", res.Certificate.Status)
	}

	// Sin la raíz en ningún sitio, la intermedia queda sin comprobar: la
	// firma no puede salir válida aunque la hoja esté vigente.
	sinRaiz := NewCAdESVerifierWithChecker(comprobadorLote3())
	res, _, err = sinRaiz.VerifyDetachedCMS(context.Background(), cms, contenido)
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid {
		t.Fatalf("la revocación de la intermedia no se comprobó y la firma salió válida: %v", res.Details)
	}
}
