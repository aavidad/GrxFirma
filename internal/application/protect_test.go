// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"crypto/x509/pkix"

	"grxfirma/internal/adapters/outbound/common/protector"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type protectionRecipientCatalogMock struct {
	recipients []domain.ProtectionRecipient
	err        error
}

func (m *protectionRecipientCatalogMock) Resolve(_ context.Context, _ []string) ([]domain.ProtectionRecipient, error) {
	return m.recipients, m.err
}

type protectionKeyProviderMock struct {
	keys []domain.ProtectionKeyMaterial
	err  error
}

func (m *protectionKeyProviderMock) DecryptionKeys(_ context.Context) ([]domain.ProtectionKeyMaterial, error) {
	return m.keys, m.err
}

type protectorEngineMock struct {
	protected    domain.ProtectedPayload
	unprotected  domain.UnprotectedPayload
	protectErr   error
	unprotectErr error
	lastJob      domain.ProtectionJob
	lastKeys     []domain.ProtectionKeyMaterial
}

func (m *protectorEngineMock) Protect(_ context.Context, job domain.ProtectionJob, _ []domain.ProtectionRecipient) (domain.ProtectedPayload, error) {
	m.lastJob = job
	return m.protected, m.protectErr
}

func (m *protectorEngineMock) Unprotect(_ context.Context, _ domain.Document, keys []domain.ProtectionKeyMaterial) (domain.UnprotectedPayload, error) {
	m.lastKeys = append([]domain.ProtectionKeyMaterial(nil), keys...)
	return m.unprotected, m.unprotectErr
}

type signedProtectorEngineMock struct {
	protected  domain.ProtectedPayload
	err        error
	lastJob    domain.ProtectionJob
	lastKeys   []string
	lastSigner ports.SigningKey
}

func (m *signedProtectorEngineMock) ProtectAndSign(_ context.Context, job domain.ProtectionJob, recipients []domain.ProtectionRecipient, key ports.SigningKey) (domain.ProtectedPayload, error) {
	m.lastJob = job
	m.lastSigner = key
	m.lastKeys = m.lastKeys[:0]
	for _, recipient := range recipients {
		m.lastKeys = append(m.lastKeys, recipient.ID)
	}
	return m.protected, m.err
}

func TestProtectDocument_Exito(t *testing.T) {
	logger := &loggerMock{}
	doc, _ := domain.NewDocument("secreto.txt", []byte("hola"), "text/plain")
	protectedDoc, _ := domain.NewDocument("secreto.txt.afp", []byte(`{"ok":true}`), domain.MIMETypeProtectedEnvelope)
	uc := application.NuevoProtectDocumentUseCase(
		&protectionRecipientCatalogMock{recipients: []domain.ProtectionRecipient{{
			ID:                     "dest-1",
			RSAOAEP256PublicKeyDER: []byte("rsa-pub"),
		}}},
		&protectorEngineMock{protected: domain.ProtectedPayload{
			Document:       protectedDoc,
			Profile:        domain.ProtectionProfileCompat,
			RecipientCount: 1,
		}},
		&aprobadorMock{aprobado: true},
		application.NuevoAuditUseCase(relojMock{}, logger),
		&publicadorMock{},
	)

	res, err := uc.Ejecutar(context.Background(), application.ProtectCommand{
		Document:     doc,
		Profile:      domain.ProtectionProfileCompat,
		RecipientIDs: []string{"dest-1"},
	})
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if res.Protected.RecipientCount != 1 {
		t.Fatalf("recipientCount inesperado: %d", res.Protected.RecipientCount)
	}
	if len(logger.registros) != 1 {
		t.Fatalf("se esperaba 1 auditoria, hay %d", len(logger.registros))
	}
}

func TestProtectDocument_SinDestinatarios(t *testing.T) {
	doc, _ := domain.NewDocument("secreto.txt", []byte("hola"), "text/plain")
	uc := application.NuevoProtectDocumentUseCase(
		&protectionRecipientCatalogMock{},
		&protectorEngineMock{},
		nil, nil, nil,
	)
	if _, err := uc.Ejecutar(context.Background(), application.ProtectCommand{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
	}); err == nil {
		t.Fatal("se esperaba error por falta de destinatarios")
	}
}

func TestProtectDocument_EncryptedDataPermiteSinDestinatarios(t *testing.T) {
	doc, _ := domain.NewDocument("secreto.txt", []byte("hola"), "text/plain")
	protectedDoc, _ := domain.NewDocument("secreto.txt.encrypted.p7m", []byte("cms"), domain.MIMETypeProtectedCMS)
	engine := &protectorEngineMock{protected: domain.ProtectedPayload{
		Document: protectedDoc,
		Profile:  domain.ProtectionProfileCompat,
	}}
	uc := application.NuevoProtectDocumentUseCase(
		&protectionRecipientCatalogMock{},
		engine,
		nil, nil, nil,
	)
	res, err := uc.Ejecutar(context.Background(), application.ProtectCommand{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
		Options: map[string]string{
			"container":  "cms-encrypted",
			"secret_b64": base64.StdEncoding.EncodeToString(make([]byte, 32)),
		},
	})
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if res.Protected.Document.Name != "secreto.txt.encrypted.p7m" {
		t.Fatalf("nombre inesperado: %q", res.Protected.Document.Name)
	}
}

func TestProtectDocument_EncryptedDataAliasesPermitenSinDestinatarios(t *testing.T) {
	doc, _ := domain.NewDocument("secreto.txt", []byte("hola"), "text/plain")
	cases := []string{
		"cms-encrypted-data",
		"CMS_ENCRYPTED_DATA",
		"encrypted data",
	}
	for _, container := range cases {
		t.Run(container, func(t *testing.T) {
			protectedDoc, _ := domain.NewDocument("secreto.txt.encrypted.p7m", []byte("cms"), domain.MIMETypeProtectedCMS)
			engine := &protectorEngineMock{protected: domain.ProtectedPayload{
				Document: protectedDoc,
				Profile:  domain.ProtectionProfileCompat,
			}}
			uc := application.NuevoProtectDocumentUseCase(
				&protectionRecipientCatalogMock{},
				engine,
				nil, nil, nil,
			)
			_, err := uc.Ejecutar(context.Background(), application.ProtectCommand{
				Document: doc,
				Profile:  domain.ProtectionProfileCompat,
				Options: map[string]string{
					"container":  container,
					"secret_b64": base64.StdEncoding.EncodeToString(make([]byte, 32)),
				},
			})
			if err != nil {
				t.Fatalf("Ejecutar() error = %v", err)
			}
		})
	}
}

func TestUnprotectDocument_Exito(t *testing.T) {
	logger := &loggerMock{}
	protectedDoc, _ := domain.NewDocument("secreto.txt.afp", []byte(`{"ok":true}`), domain.MIMETypeProtectedEnvelope)
	plainDoc, _ := domain.NewDocument("secreto.txt", []byte("hola"), "text/plain")
	uc := application.NuevoUnprotectDocumentUseCase(
		&protectionKeyProviderMock{keys: []domain.ProtectionKeyMaterial{{RecipientID: "dest-1", RSAOAEP256PrivateKeyPKCS8: []byte("rsa-priv")}}},
		&protectorEngineMock{unprotected: domain.UnprotectedPayload{
			Document:    plainDoc,
			Profile:     domain.ProtectionProfileCompat,
			RecipientID: "dest-1",
		}},
		application.NuevoAuditUseCase(relojMock{}, logger),
		&publicadorMock{},
	)

	res, err := uc.Ejecutar(context.Background(), application.UnprotectCommand{ProtectedDocument: protectedDoc})
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if got := string(res.Unprotected.Document.Content); got != "hola" {
		t.Fatalf("contenido inesperado: %q", got)
	}
	if len(logger.registros) != 1 {
		t.Fatalf("se esperaba 1 auditoria, hay %d", len(logger.registros))
	}
}

func TestUnprotectDocument_ErrorProveedor(t *testing.T) {
	protectedDoc, _ := domain.NewDocument("secreto.txt.afp", []byte(`{"ok":true}`), domain.MIMETypeProtectedEnvelope)
	uc := application.NuevoUnprotectDocumentUseCase(
		&protectionKeyProviderMock{err: errors.New("boom")},
		&protectorEngineMock{},
		nil, nil,
	)
	if _, err := uc.Ejecutar(context.Background(), application.UnprotectCommand{ProtectedDocument: protectedDoc}); err == nil {
		t.Fatal("se esperaba error del proveedor de claves")
	}
}

func TestUnprotectDocument_AgregaClaveSimetricaTransitoria(t *testing.T) {
	protectedDoc, _ := domain.NewDocument("secreto.txt.encrypted.p7m", []byte("cms"), domain.MIMETypeProtectedCMS)
	plainDoc, _ := domain.NewDocument("secreto.txt", []byte("hola"), "text/plain")
	engine := &protectorEngineMock{unprotected: domain.UnprotectedPayload{
		Document: plainDoc,
		Profile:  domain.ProtectionProfileCompat,
	}}
	uc := application.NuevoUnprotectDocumentUseCase(
		&protectionKeyProviderMock{},
		engine,
		nil, nil,
	)

	secret := make([]byte, 32)
	_, err := uc.Ejecutar(context.Background(), application.UnprotectCommand{
		ProtectedDocument: protectedDoc,
		Options: map[string]string{
			"secret_b64": base64.StdEncoding.EncodeToString(secret),
		},
	})
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if len(engine.lastKeys) != 1 {
		t.Fatalf("claves inesperadas: %d", len(engine.lastKeys))
	}
	if got := engine.lastKeys[0].RecipientID; got != "cms-encrypted-transient" {
		t.Fatalf("recipientID inesperado: %q", got)
	}
	if len(engine.lastKeys[0].SymmetricKey) != 32 {
		t.Fatalf("clave simetrica inesperada: %d", len(engine.lastKeys[0].SymmetricKey))
	}
	for i, v := range engine.lastKeys[0].SymmetricKey {
		if v != 0 {
			t.Fatalf("clave simetrica transitoria no zerizada en posicion %d: %d", i, v)
		}
	}
}

func TestUnprotectDocument_RechazaSecretoSimetricoQueNoSeaAES256(t *testing.T) {
	protectedDoc, _ := domain.NewDocument("secreto.txt.encrypted.p7m", []byte("cms"), domain.MIMETypeProtectedCMS)
	for _, size := range []int{0, 1, 16, 31, 33, 64} {
		t.Run(fmt.Sprintf("%d_bytes", size), func(t *testing.T) {
			engine := &protectorEngineMock{}
			uc := application.NuevoUnprotectDocumentUseCase(
				&protectionKeyProviderMock{},
				engine,
				nil, nil,
			)

			_, err := uc.Ejecutar(context.Background(), application.UnprotectCommand{
				ProtectedDocument: protectedDoc,
				Options: map[string]string{
					"secret_b64": base64.StdEncoding.EncodeToString(make([]byte, size)),
				},
			})
			if err == nil || !strings.Contains(err.Error(), "exactamente 32 bytes") {
				t.Fatalf("Ejecutar() error = %v", err)
			}
			if len(engine.lastKeys) != 0 {
				t.Fatalf("el motor no debe recibir una clave invalida: %#v", engine.lastKeys)
			}
		})
	}
}

func TestProtectAndSignDocument_Exito(t *testing.T) {
	logger := &loggerMock{}
	doc, _ := domain.NewDocument("secreto.txt", []byte("hola"), "text/plain")
	protectedDoc, _ := domain.NewDocument("secreto.txt.signedenveloped.p7m", []byte("cms"), domain.MIMETypeProtectedCMS)
	engine := &signedProtectorEngineMock{protected: domain.ProtectedPayload{
		Document:       protectedDoc,
		Profile:        domain.ProtectionProfileCompat,
		RecipientCount: 1,
	}}
	uc := application.NuevoProtectAndSignDocumentUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: claveMock{id: "clave-firma"}},
		&protectionRecipientCatalogMock{recipients: []domain.ProtectionRecipient{{
			ID:                     "dest-1",
			RSAOAEP256PublicKeyDER: []byte("rsa-pub"),
			CertificateDER:         []byte("cert-der"),
		}}},
		engine,
		&aprobadorMock{aprobado: true},
		application.NuevoAuditUseCase(relojMock{}, logger),
		&publicadorMock{},
	)

	res, err := uc.Ejecutar(context.Background(), application.ProtectAndSignCommand{
		Document:      doc,
		Profile:       domain.ProtectionProfileCompat,
		RecipientIDs:  []string{"dest-1"},
		CertificateID: "cert-001",
		Options:       map[string]string{"container": "signedandenvelopeddata"},
	})
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if got := res.Protected.Document.Name; got != "secreto.txt.signedenveloped.p7m" {
		t.Fatalf("nombre inesperado: %q", got)
	}
	if got := res.CertificateUsed.ID; got != "cert-001" {
		t.Fatalf("certificateID inesperado: %q", got)
	}
	if got := engine.lastJob.Options["container"]; got != "signedandenvelopeddata" {
		t.Fatalf("container inesperado: %q", got)
	}
	if len(engine.lastKeys) != 1 || engine.lastKeys[0] != "dest-1" {
		t.Fatalf("destinatarios inesperados: %#v", engine.lastKeys)
	}
	if len(logger.registros) != 1 {
		t.Fatalf("se esperaba 1 auditoria, hay %d", len(logger.registros))
	}
}

func TestProtectAndSignDocument_CierraClaveSiFallaResolucionDeDestinatarios(t *testing.T) {
	t.Parallel()

	doc, _ := domain.NewDocument("secreto.txt", []byte("hola"), "text/plain")
	key := &claveCerrableMock{claveMock: claveMock{id: "key-close"}}
	uc := application.NuevoProtectAndSignDocumentUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: key},
		&protectionRecipientCatalogMock{err: errors.New("fallo resolviendo destinatarios")},
		&signedProtectorEngineMock{},
		nil,
		nil,
		nil,
	)

	_, err := uc.Ejecutar(context.Background(), application.ProtectAndSignCommand{
		Document:      doc,
		Profile:       domain.ProtectionProfileCompat,
		RecipientIDs:  []string{"dest-1"},
		CertificateID: "cert-001",
	})
	if err == nil {
		t.Fatal("Ejecutar() error = nil, want error")
	}
	if key.cierres != 1 {
		t.Fatalf("cierres de clave = %d, want 1", key.cierres)
	}
}

func TestProtectAndSignDocument_SinDestinatarios(t *testing.T) {
	doc, _ := domain.NewDocument("secreto.txt", []byte("hola"), "text/plain")
	uc := application.NuevoProtectAndSignDocumentUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: claveMock{id: "clave-firma"}},
		&protectionRecipientCatalogMock{},
		&signedProtectorEngineMock{},
		nil, nil, nil,
	)
	if _, err := uc.Ejecutar(context.Background(), application.ProtectAndSignCommand{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
	}); err == nil {
		t.Fatal("se esperaba error por falta de destinatarios")
	}
}

func TestProtectAndSignDocument_NormalizaAliasDeSignedAndEnvelopedData(t *testing.T) {
	doc, _ := domain.NewDocument("secreto.txt", []byte("hola"), "text/plain")
	protectedDoc, _ := domain.NewDocument("secreto.txt.signedenveloped.p7m", []byte("cms"), domain.MIMETypeProtectedCMS)
	engine := &signedProtectorEngineMock{protected: domain.ProtectedPayload{
		Document:       protectedDoc,
		Profile:        domain.ProtectionProfileCompat,
		RecipientCount: 1,
	}}
	uc := application.NuevoProtectAndSignDocumentUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: claveMock{id: "clave-firma"}},
		&protectionRecipientCatalogMock{recipients: []domain.ProtectionRecipient{{
			ID:                     "dest-1",
			RSAOAEP256PublicKeyDER: []byte("rsa-pub"),
			CertificateDER:         []byte("cert-der"),
		}}},
		engine,
		nil, nil, nil,
	)
	_, err := uc.Ejecutar(context.Background(), application.ProtectAndSignCommand{
		Document:      doc,
		Profile:       domain.ProtectionProfileCompat,
		RecipientIDs:  []string{"dest-1"},
		CertificateID: "cert-001",
		Options:       map[string]string{"container": "signed-and-enveloped-data"},
	})
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if got := engine.lastJob.Options["container"]; got != "signedandenvelopeddata" {
		t.Fatalf("container inesperado: %q", got)
	}
}

func TestProtectAndSignDocument_RechazaContenedorCMSIncompatible(t *testing.T) {
	doc, _ := domain.NewDocument("secreto.txt", []byte("hola"), "text/plain")
	uc := application.NuevoProtectAndSignDocumentUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: claveMock{id: "clave-firma"}},
		&protectionRecipientCatalogMock{recipients: []domain.ProtectionRecipient{{
			ID:                     "dest-1",
			RSAOAEP256PublicKeyDER: []byte("rsa-pub"),
			CertificateDER:         []byte("cert-der"),
		}}},
		&signedProtectorEngineMock{},
		nil, nil, nil,
	)
	cases := []struct {
		container string
		want      string
	}{
		{container: "authenvelopeddata", want: "AuthEnvelopedData"},
		{container: "authenticateddata", want: "AuthenticatedData"},
		{container: "compresseddata", want: "CompressedData"},
		{container: "cms-encrypted", want: "EncryptedData"},
		{container: "cms", want: "EnvelopedData"},
	}
	for _, tc := range cases {
		t.Run(tc.container, func(t *testing.T) {
			_, err := uc.Ejecutar(context.Background(), application.ProtectAndSignCommand{
				Document:      doc,
				Profile:       domain.ProtectionProfileCompat,
				RecipientIDs:  []string{"dest-1"},
				CertificateID: "cert-001",
				Options:       map[string]string{"container": tc.container},
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error inesperado = %v", err)
			}
		})
	}
}

func TestProtectAndUnprotectDocument_AuthEnvelopedData(t *testing.T) {
	doc, _ := domain.NewDocument("secreto.txt", []byte("hola"), "text/plain")
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey() error = %v", err)
	}
	certDER := mustCompatRecipientCertDER(t, priv)
	uc := application.NuevoProtectDocumentUseCase(
		&protectionRecipientCatalogMock{recipients: []domain.ProtectionRecipient{{
			ID:                     "dest-1",
			RSAOAEP256PublicKeyDER: pubDER,
			CertificateDER:         certDER,
		}}},
		protector.NuevoAdaptiveProtector(),
		nil, nil, nil,
	)
	protected, err := uc.Ejecutar(context.Background(), application.ProtectCommand{
		Document:     doc,
		Profile:      domain.ProtectionProfileCompat,
		RecipientIDs: []string{"dest-1"},
		Options:      map[string]string{"container": "authenvelopeddata"},
	})
	if err != nil {
		t.Fatalf("ProtectDocument.Ejecutar(AuthEnvelopedData) error = %v", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}
	unprotectUC := application.NuevoUnprotectDocumentUseCase(
		&protectionKeyProviderMock{keys: []domain.ProtectionKeyMaterial{{
			RecipientID:               "dest-1",
			RSAOAEP256PrivateKeyPKCS8: privateDER,
			CertificateDER:            certDER,
		}}},
		protector.NuevoAdaptiveProtector(),
		nil, nil,
	)
	unprotected, err := unprotectUC.Ejecutar(context.Background(), application.UnprotectCommand{
		ProtectedDocument: protected.Protected.Document,
	})
	if err != nil {
		t.Fatalf("UnprotectDocument.Ejecutar(AuthEnvelopedData) error = %v", err)
	}
	if !bytes.Equal(unprotected.Unprotected.Document.Content, doc.Content) {
		t.Fatalf("contenido recuperado = %q; want %q", unprotected.Unprotected.Document.Content, doc.Content)
	}
}

func mustCompatRecipientCertDER(t *testing.T, priv *rsa.PrivateKey) []byte {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Compat Test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageDataEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageEmailProtection},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}
	return der
}
