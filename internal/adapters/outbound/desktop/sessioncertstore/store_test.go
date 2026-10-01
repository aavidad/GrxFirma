// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package sessioncertstore_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/sessioncertstore"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

func testPEMBundle(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          new(big.Int).SetInt64(42),
		Subject:               pkix.Name{CommonName: "Credencial temporal"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return append(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})...,
	)
}

func TestStoreLoadPEMNoPersisteYSePuedeRetirar(t *testing.T) {
	store := sessioncertstore.New()
	ref, err := store.Load(context.Background(), testPEMBundle(t), "")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	refs, err := store.List(context.Background())
	if err != nil || len(refs) != 1 || refs[0].ID != ref.ID {
		t.Fatalf("List() = %#v, %v", refs, err)
	}
	if _, err := store.KeyFor(context.Background(), ref); err != nil {
		t.Fatalf("KeyFor() error = %v", err)
	}
	store.Remove(ref.ID)
	refs, _ = store.List(context.Background())
	if len(refs) != 0 {
		t.Fatalf("quedaron referencias tras Remove: %#v", refs)
	}
	if _, err := store.KeyFor(context.Background(), domain.CertificateRef{ID: ref.ID}); err == nil {
		t.Fatal("KeyFor() debía fallar tras retirar la credencial")
	}
}

func TestStoreRechazaCredencialVacia(t *testing.T) {
	if _, err := sessioncertstore.New().Load(context.Background(), nil, ""); err == nil {
		t.Fatal("Load() debía rechazar datos vacíos")
	}
}

type approvingUser struct{}

func (approvingUser) Request(context.Context, string) (bool, error) {
	return true, nil
}

type recordingSigner struct {
	key ports.SigningKey
}

func (s *recordingSigner) Sign(_ context.Context, job domain.SignatureJob, key ports.SigningKey) (domain.SignatureResult, error) {
	s.key = key
	return domain.SignatureResult{
		Format: job.Format, Data: []byte("firma-temporal"), Algorithm: "test",
	}, nil
}

func TestStoreSeIntegraConOperacionDeFirmaActual(t *testing.T) {
	store := sessioncertstore.New()
	ref, err := store.Load(context.Background(), testPEMBundle(t), "")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	engine := &recordingSigner{}
	useCase := application.NuevoSignDocumentUseCase(
		store, store, engine, approvingUser{}, nil, nil,
	)
	result, err := useCase.Execute(context.Background(), application.SignCommand{
		Document:      domain.Document{Name: "documento.txt", Content: []byte("contenido")},
		Format:        domain.FormatCAdES,
		Action:        domain.ActionSign,
		CertificateID: ref.ID,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if string(result.Result.Data) != "firma-temporal" || engine.key == nil {
		t.Fatalf("la operación no usó la clave temporal: %#v", result)
	}
}
