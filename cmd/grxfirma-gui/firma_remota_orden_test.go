// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"strings"
	"testing"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type claveOrigen struct{ origen string }

func (c claveOrigen) KeyID() string                 { return c.origen }
func (c claveOrigen) CertificateChainDER() [][]byte { return nil }

// remotaFalsa ofrece cualquier identificador, como haría un prestador que
// publicara un certificado con la huella de uno local.
type remotaFalsa struct{ refs []domain.CertificateRef }

func (r remotaFalsa) List(context.Context) ([]domain.CertificateRef, error) { return r.refs, nil }
func (r remotaFalsa) KeyFor(context.Context, domain.CertificateRef) (ports.SigningKey, error) {
	return claveOrigen{"remota"}, nil
}

func TestAgregarFirmaRemotaMismoOrdenEnCatalogoYClaves(t *testing.T) {
	huella := strings.Repeat("ab", 32)
	local := domain.CertificateRef{ID: huella, Fingerprint: huella, Subject: "CN=Local"}
	remota := domain.CertificateRef{ID: huella, Fingerprint: huella, Subject: "CN=Remota"}
	catalogo, claves := agregarFirmaRemota(
		&catalogoMemoria{certs: []domain.CertificateRef{local}},
		&proveedorClavesMemoria{claves: map[string]ports.SigningKey{huella: claveOrigen{"local"}}},
		remotaFalsa{refs: []domain.CertificateRef{remota}},
	)
	refs, err := catalogo.List(context.Background())
	if err != nil || len(refs) != 1 || refs[0].Subject != "CN=Local" {
		t.Fatalf("catálogo: %+v %v", refs, err)
	}
	k, err := claves.KeyFor(context.Background(), refs[0])
	if err != nil || k.KeyID() != "local" {
		t.Fatalf("la clave del certificado local salió de %v (%v)", k, err)
	}
	// Un certificado solo remoto sigue firmándose con el prestador.
	otra := strings.Repeat("cd", 32)
	if k, err := claves.KeyFor(context.Background(), domain.CertificateRef{ID: otra, Fingerprint: otra}); err != nil || k.KeyID() != "remota" {
		t.Fatalf("remota: %v %v", k, err)
	}
}
