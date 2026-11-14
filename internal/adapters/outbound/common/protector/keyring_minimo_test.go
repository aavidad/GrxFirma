// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector_test

import (
	"context"
	"crypto/x509"
	"sync"
	"testing"

	"grxfirma/internal/adapters/outbound/common/protector"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// keyProviderContador registra cada KeyFor: en NSS cada llamada exporta una
// clave privada del navegador a la memoria del proceso (F-01).
type keyProviderContador struct {
	mu      sync.Mutex
	keys    map[string]ports.SigningKey
	pedidas []string
}

func (p *keyProviderContador) KeyFor(_ context.Context, ref domain.CertificateRef) (ports.SigningKey, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pedidas = append(p.pedidas, ref.ID)
	if key, ok := p.keys[ref.ID]; ok {
		return key, nil
	}
	return nil, x509.CertificateInvalidError{}
}

func (p *keyProviderContador) llamadas() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.pedidas...)
}

func tresIdentidadesRSA(t *testing.T) ([]domain.CertificateRef, *keyProviderContador) {
	t.Helper()
	refs := make([]domain.CertificateRef, 0, 3)
	provider := &keyProviderContador{keys: map[string]ports.SigningKey{}}
	for _, cn := range []string{"Identidad Uno", "Identidad Dos", "Identidad Tres"} {
		ref, key := generarCompatRSA(t, cn, x509.KeyUsageKeyEncipherment)
		refs = append(refs, ref)
		provider.keys[ref.ID] = key
	}
	return refs, provider
}

func TestLocalCompatKeyring_ListarDestinatariosNoPideNingunaClavePrivada(t *testing.T) {
	t.Parallel()

	refs, provider := tresIdentidadesRSA(t)
	keyring := protector.NuevoLocalCompatKeyring(certCatalogMock{refs: refs}, provider)

	recipients, err := keyring.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(recipients) != 3 {
		t.Fatalf("List() = %d destinatarios; want 3", len(recipients))
	}
	if _, err := keyring.Resolve(context.Background(), []string{refs[1].ID}); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got := provider.llamadas(); len(got) != 0 {
		t.Fatalf("List/Resolve pidieron claves privadas %v; no deben pedir ninguna", got)
	}
}

func TestLocalCompatKeyring_DesprotegerSoloPideLaClaveDelDestinatario(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		nombre  string
		options map[string]string
	}{
		{nombre: "afp-compat"},
		{nombre: "cms-enveloped", options: map[string]string{"container": "cms"}},
		{nombre: "cms-auth-enveloped", options: map[string]string{"container": "cms-auth-enveloped"}},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			refs, provider := tresIdentidadesRSA(t)
			keyring := protector.NuevoLocalCompatKeyring(certCatalogMock{refs: refs}, provider)
			destino, err := keyring.Resolve(ctx, []string{refs[1].ID})
			if err != nil || len(destino) != 1 {
				t.Fatalf("Resolve() = %#v, %v", destino, err)
			}
			doc, err := domain.NewDocument("informe.txt", []byte("contenido para la identidad dos"), "text/plain")
			if err != nil {
				t.Fatal(err)
			}
			engine := protector.NuevoAdaptiveProtector()
			protected, err := engine.Protect(ctx, domain.ProtectionJob{
				Document: doc,
				Profile:  domain.ProtectionProfileCompat,
				Options:  tc.options,
			}, destino)
			if err != nil {
				t.Fatalf("Protect() error = %v", err)
			}

			keys, err := keyring.DecryptionKeys(ctx, protected.Document)
			if err != nil {
				t.Fatalf("DecryptionKeys() error = %v", err)
			}
			if got := provider.llamadas(); len(got) != 1 || got[0] != refs[1].ID {
				t.Fatalf("KeyFor pedido para %v; want solo %q", got, refs[1].ID)
			}
			if len(keys) != 1 || keys[0].RecipientID != refs[1].ID {
				t.Fatalf("DecryptionKeys() = %d claves; want solo la de la identidad dos", len(keys))
			}
			unprotected, err := engine.Unprotect(ctx, protected.Document, keys)
			if err != nil {
				t.Fatalf("Unprotect() error = %v", err)
			}
			if string(unprotected.Document.Content) != string(doc.Content) {
				t.Fatalf("contenido = %q; want %q", unprotected.Document.Content, doc.Content)
			}
		})
	}
}

func TestLocalCompatKeyring_DocumentoIlegibleNoPideClaves(t *testing.T) {
	t.Parallel()

	refs, provider := tresIdentidadesRSA(t)
	keyring := protector.NuevoLocalCompatKeyring(certCatalogMock{refs: refs}, provider)
	doc, err := domain.NewDocument("basura.p7m", []byte("no es un contenedor"), "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	keys, err := keyring.DecryptionKeys(context.Background(), doc)
	if err != nil {
		t.Fatalf("DecryptionKeys() error = %v", err)
	}
	if len(keys) != 0 || len(provider.llamadas()) != 0 {
		t.Fatalf("DecryptionKeys() = %d claves, KeyFor %v; want ninguna", len(keys), provider.llamadas())
	}
}
