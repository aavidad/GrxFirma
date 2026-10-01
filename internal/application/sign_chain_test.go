// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"context"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

func TestSignDocument_CopiaCadenaPublicaAntesDeCerrarClave(t *testing.T) {
	clave := &claveCerrableMock{claveMock: claveMock{id: "clave-cadena"}}
	caso := application.NuevoSignDocumentUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: clave},
		&motorMock{resultado: domain.SignatureResult{
			Format: domain.FormatCAdES, Data: []byte("firma"), Algorithm: "SHA256withRSA",
		}},
		&aprobadorMock{aprobado: true},
		nil,
		nil,
	)

	resultado, err := caso.Ejecutar(context.Background(), application.SignCommand{
		Document: docPrueba(), Format: domain.FormatCAdES, Action: domain.ActionSign,
	})
	if err != nil {
		t.Fatalf("firmar: %v", err)
	}
	if clave.cierres != 1 || len(resultado.CertificateChainDER) != 1 ||
		string(resultado.CertificateChainDER[0]) != "cert-der" {
		t.Fatalf("cadena o cierre inesperado: cierres=%d cadena=%q", clave.cierres, resultado.CertificateChainDER)
	}
	resultado.CertificateChainDER[0][0] = 'X'
	if string(clave.CertificateChainDER()[0]) != "cert-der" {
		t.Fatal("el resultado comparte memoria con la clave")
	}
}

func TestSignDocument_ConservaCompatibilidadConClaveSinCadena(t *testing.T) {
	caso := application.NuevoSignDocumentUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: claveSinCadena{id: "sin-cadena"}},
		&motorMock{},
		&aprobadorMock{aprobado: true},
		nil,
		nil,
	)
	resultado, err := caso.Ejecutar(context.Background(), application.SignCommand{
		Document: docPrueba(), Format: domain.FormatCAdES, Action: domain.ActionSign,
	})
	if err != nil || len(resultado.CertificateChainDER) != 0 {
		t.Fatalf("firma compatible inesperada: resultado=%+v error=%v", resultado, err)
	}
}

type claveSinCadena struct{ id string }

func (c claveSinCadena) KeyID() string               { return c.id }
func (claveSinCadena) CertificateChainDER() [][]byte { return nil }
