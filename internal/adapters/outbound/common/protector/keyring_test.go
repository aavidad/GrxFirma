// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector_test

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"errors"
	"io"
	"math/big"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/pkcs12importer"
	"grxfirma/internal/adapters/outbound/common/protector"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

type certCatalogMock struct {
	refs []domain.CertificateRef
}

func (m certCatalogMock) List(context.Context) ([]domain.CertificateRef, error) {
	return append([]domain.CertificateRef(nil), m.refs...), nil
}

type signingKeyProviderMock struct {
	keys map[string]ports.SigningKey
}

func (m signingKeyProviderMock) KeyFor(_ context.Context, certificate domain.CertificateRef) (ports.SigningKey, error) {
	if key, ok := m.keys[certificate.ID]; ok {
		return key, nil
	}
	return nil, x509.CertificateInvalidError{}
}

type closableLocalSigningKey struct {
	ports.SigningKey
	local  *commonsigner.LocalSigningKey
	closed int
}

func (key *closableLocalSigningKey) ToLocalSigningKey() *commonsigner.LocalSigningKey {
	return key.local
}

func (key *closableLocalSigningKey) Close() {
	key.closed++
}

type nonExportableRSASigner struct {
	private *rsa.PrivateKey
}

func (key *nonExportableRSASigner) Public() crypto.PublicKey {
	return &key.private.PublicKey
}

func (key *nonExportableRSASigner) Sign(random io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	return key.private.Sign(random, digest, opts)
}

type sensitiveFailureKeyProvider struct {
	key ports.SigningKey
}

func (provider sensitiveFailureKeyProvider) KeyFor(_ context.Context, _ domain.CertificateRef) (ports.SigningKey, error) {
	if provider.key != nil {
		return provider.key, nil
	}
	return nil, errors.New("CAUSA-SENSIBLE-NO-DEBE-SALIR")
}

func TestLocalCompatKeyring_ListAndResolve(t *testing.T) {
	rsaRef, rsaKey := generarCompatRSA(t, "Compat RSA", x509.KeyUsageKeyEncipherment)
	ecdsaRef, ecdsaKey := generarECDSA(t, "ECDSA Solo Firma", x509.KeyUsageDigitalSignature)

	keyring := protector.NuevoLocalCompatKeyring(
		certCatalogMock{refs: []domain.CertificateRef{rsaRef, ecdsaRef}},
		signingKeyProviderMock{keys: map[string]ports.SigningKey{
			rsaRef.ID:   rsaKey,
			ecdsaRef.ID: ecdsaKey,
		}},
	)

	recipients, err := keyring.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(recipients) != 1 {
		t.Fatalf("List() len = %d; want 1", len(recipients))
	}
	if recipients[0].ID != rsaRef.ID {
		t.Fatalf("recipient ID = %q; want %q", recipients[0].ID, rsaRef.ID)
	}

	resolved, err := keyring.Resolve(context.Background(), []string{rsaRef.ID, "no-existe"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(resolved) != 1 || resolved[0].ID != rsaRef.ID {
		t.Fatalf("Resolve() = %#v; want solo %q", resolved, rsaRef.ID)
	}
}

func TestLocalCompatKeyring_ExcluyeFirmanteRSAOpacoAunquePuedeFirmar(t *testing.T) {
	t.Parallel()

	ref, rawKey := generarCompatRSA(t, "CertStore RSA no exportable", x509.KeyUsageKeyEncipherment)
	source := rawKey.(interface {
		ToLocalSigningKey() *commonsigner.LocalSigningKey
	}).ToLocalSigningKey()
	opaque := &nonExportableRSASigner{private: source.Signer.(*rsa.PrivateKey)}
	key := desktopsigner.NuevaClaveLocal(opaque, source.Certificate)
	keyring := protector.NuevoLocalCompatKeyring(
		certCatalogMock{refs: []domain.CertificateRef{ref}},
		signingKeyProviderMock{keys: map[string]ports.SigningKey{ref.ID: key}},
	)

	digest := sha256.Sum256([]byte("la identidad opaca sigue pudiendo firmar"))
	if _, err := opaque.Sign(rand.Reader, digest[:], crypto.SHA256); err != nil {
		t.Fatalf("Sign() del firmante opaco error = %v", err)
	}
	recipients, err := keyring.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(recipients) != 0 {
		t.Fatalf("List() = %#v; no debe anunciar una clave que no puede desproteger", recipients)
	}
	resolved, err := keyring.Resolve(context.Background(), []string{ref.ID})
	if err != nil || len(resolved) != 0 {
		t.Fatalf("Resolve() = %#v, %v; want vacío sin causa interna", resolved, err)
	}
	keys, err := keyring.DecryptionKeys(context.Background())
	if err != nil || len(keys) != 0 {
		t.Fatalf("DecryptionKeys() = %#v, %v; want vacío", keys, err)
	}
}

func TestLocalCompatKeyring_ExcluyeRSAConSoloKeyAgreement(t *testing.T) {
	t.Parallel()

	ref, key := generarCompatRSA(t, "RSA KeyAgreement", x509.KeyUsageKeyAgreement)
	keyring := protector.NuevoLocalCompatKeyring(
		certCatalogMock{refs: []domain.CertificateRef{ref}},
		signingKeyProviderMock{keys: map[string]ports.SigningKey{ref.ID: key}},
	)

	recipients, err := keyring.List(context.Background())
	if err != nil || len(recipients) != 0 {
		t.Fatalf("List() = %#v, %v; RSA-OAEP requiere KeyEncipherment", recipients, err)
	}
}

func TestLocalCompatKeyring_P12RSAExportableSigueDisponibleYDesprotege(t *testing.T) {
	t.Parallel()

	_, generatedKey := generarCompatRSA(t, "P12 RSA autorizado", x509.KeyUsageKeyEncipherment)
	generated := generatedKey.(interface {
		ToLocalSigningKey() *commonsigner.LocalSigningKey
	}).ToLocalSigningKey()
	p12, err := pkcs12.Modern.Encode(
		generated.Signer.(*rsa.PrivateKey),
		generated.Certificate,
		nil,
		"grxfirma-qa",
	)
	if err != nil {
		t.Fatalf("Encode(P12) error = %v", err)
	}
	identity, err := pkcs12importer.New().ImportIdentity(
		context.Background(),
		p12,
		"grxfirma-qa",
	)
	clear(p12)
	if err != nil {
		t.Fatalf("ImportIdentity(P12) error = %v", err)
	}
	ref := identity.Reference
	key := desktopsigner.NuevaClaveLocal(identity.Signer, identity.Certificate)
	keyring := protector.NuevoLocalCompatKeyring(
		certCatalogMock{refs: []domain.CertificateRef{ref}},
		signingKeyProviderMock{keys: map[string]ports.SigningKey{ref.ID: key}},
	)
	ctx := context.Background()
	recipients, err := keyring.List(ctx)
	if err != nil || len(recipients) != 1 {
		t.Fatalf("List() = %#v, %v; want P12 RSA disponible", recipients, err)
	}
	keys, err := keyring.DecryptionKeys(ctx)
	if err != nil || len(keys) != 1 {
		t.Fatalf("DecryptionKeys() = %#v, %v; want clave P12 RSA", keys, err)
	}
	doc, err := domain.NewDocument("prueba-p12.txt", []byte("roundtrip P12 RSA compatible"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	engine := protector.NuevoEnvelopeProtector()
	protected, err := engine.Protect(ctx, domain.ProtectionJob{
		Document: doc,
		Profile:  domain.ProtectionProfileCompat,
	}, recipients)
	if err != nil {
		t.Fatalf("Protect() error = %v", err)
	}
	unprotected, err := engine.Unprotect(ctx, protected.Document, keys)
	if err != nil {
		t.Fatalf("Unprotect() error = %v", err)
	}
	if string(unprotected.Document.Content) != string(doc.Content) {
		t.Fatalf("contenido recuperado = %q; want %q", unprotected.Document.Content, doc.Content)
	}
}

func TestLocalCompatKeyring_NoExponeCausaSensibleAlOmitirIdentidad(t *testing.T) {
	t.Parallel()

	ref, _ := generarCompatRSA(t, "Identidad con fallo sensible", x509.KeyUsageKeyEncipherment)
	keyring := protector.NuevoLocalCompatKeyring(
		certCatalogMock{refs: []domain.CertificateRef{ref}},
		sensitiveFailureKeyProvider{},
	)

	recipients, err := keyring.List(context.Background())
	if len(recipients) != 0 {
		t.Fatalf("List() = %#v; want vacío", recipients)
	}
	if err != nil && strings.Contains(err.Error(), "CAUSA-SENSIBLE") {
		t.Fatalf("List() expuso causa sensible: %v", err)
	}
	if err != nil {
		t.Fatalf("List() devolvió un detalle interno no accionable: %v", err)
	}
}

func TestLocalCompatKeyring_DecryptionKeys(t *testing.T) {
	rsaRef, rsaKey := generarCompatRSA(t, "Compat RSA", x509.KeyUsageKeyEncipherment)
	signOnlyRef, signOnlyKey := generarCompatRSA(t, "RSA Solo Firma", x509.KeyUsageDigitalSignature)

	keyring := protector.NuevoLocalCompatKeyring(
		certCatalogMock{refs: []domain.CertificateRef{rsaRef, signOnlyRef}},
		signingKeyProviderMock{keys: map[string]ports.SigningKey{
			rsaRef.ID:      rsaKey,
			signOnlyRef.ID: signOnlyKey,
		}},
	)

	keys, err := keyring.DecryptionKeys(context.Background())
	if err != nil {
		t.Fatalf("DecryptionKeys() error = %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("DecryptionKeys() len = %d; want 1", len(keys))
	}
	if keys[0].RecipientID != rsaRef.ID {
		t.Fatalf("RecipientID = %q; want %q", keys[0].RecipientID, rsaRef.ID)
	}
	if len(keys[0].RSAOAEP256PrivateKeyPKCS8) == 0 {
		t.Fatal("se esperaba material PKCS#8 no vacío")
	}
}

func TestLocalCompatKeyring_CierraCadaClaveTrasExtraerElMaterial(t *testing.T) {
	t.Parallel()

	rsaRef, rawKey := generarCompatRSA(t, "Compat RSA", x509.KeyUsageKeyEncipherment)
	source, ok := rawKey.(interface {
		ToLocalSigningKey() *commonsigner.LocalSigningKey
	})
	if !ok {
		t.Fatalf("clave generada = %T, want conversor de clave local", rawKey)
	}
	key := &closableLocalSigningKey{
		SigningKey: rawKey,
		local:      source.ToLocalSigningKey(),
	}
	keyring := protector.NuevoLocalCompatKeyring(
		certCatalogMock{refs: []domain.CertificateRef{rsaRef}},
		signingKeyProviderMock{keys: map[string]ports.SigningKey{rsaRef.ID: key}},
	)

	recipients, err := keyring.List(context.Background())
	if err != nil || len(recipients) != 1 {
		t.Fatalf("List() = %#v, %v", recipients, err)
	}
	if key.closed != 1 {
		t.Fatalf("cierres tras List = %d, want 1", key.closed)
	}

	keys, err := keyring.DecryptionKeys(context.Background())
	if err != nil || len(keys) != 1 {
		t.Fatalf("DecryptionKeys() = %#v, %v", keys, err)
	}
	if key.closed != 2 {
		t.Fatalf("cierres totales = %d, want 2", key.closed)
	}
}

func generarCompatRSA(t *testing.T, cn string, usage x509.KeyUsage) (domain.CertificateRef, ports.SigningKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	cert := generarCertificado(t, cn, usage, &priv.PublicKey, priv)
	return construirRef(cert), desktopsigner.NuevaClaveLocal(priv, cert)
}

func generarECDSA(t *testing.T, cn string, usage x509.KeyUsage) (domain.CertificateRef, ports.SigningKey) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey() error = %v", err)
	}
	cert := generarCertificado(t, cn, usage, &priv.PublicKey, priv)
	return construirRef(cert), desktopsigner.NuevaClaveLocal(priv, cert)
}

func generarCertificado(t *testing.T, cn string, usage x509.KeyUsage, pub any, priv any) *x509.Certificate {
	t.Helper()
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName: cn,
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(24 * time.Hour),
		KeyUsage:  usage,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, pub, priv)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate() error = %v", err)
	}
	return cert
}

func construirRef(cert *x509.Certificate) domain.CertificateRef {
	sum := sha256.Sum256(cert.Raw)
	fp := hex.EncodeToString(sum[:])
	return domain.CertificateRef{
		ID:          fp[:16],
		Subject:     cert.Subject.String(),
		Issuer:      cert.Issuer.String(),
		NotAfter:    cert.NotAfter,
		Fingerprint: fp,
	}
}
