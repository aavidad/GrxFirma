// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"sort"
	"strings"

	"grxfirma/internal/adapters/outbound/common/certutil"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// LocalCompatKeyring expone destinatarios y claves de descifrado a partir de
// identidades locales ya disponibles en el catálogo y proveedor de claves.
//
// No crea otro almacén paralelo: reutiliza exactamente las mismas fuentes que
// ya usa GrxFirma para firma local y solo extrae las identidades RSA aptas
// para cifrado/descifrado en el perfil de compatibilidad.
type LocalCompatKeyring struct {
	Catalog ports.CertificateCatalog
	Keys    ports.SigningKeyProvider
}

func NuevoLocalCompatKeyring(catalog ports.CertificateCatalog, keys ports.SigningKeyProvider) *LocalCompatKeyring {
	return &LocalCompatKeyring{
		Catalog: catalog,
		Keys:    keys,
	}
}

func (k *LocalCompatKeyring) List(ctx context.Context) ([]domain.ProtectionRecipient, error) {
	return k.listRecipients(ctx, nil)
}

func (k *LocalCompatKeyring) Resolve(ctx context.Context, recipientIDs []string) ([]domain.ProtectionRecipient, error) {
	wanted := make(map[string]struct{}, len(recipientIDs))
	for _, id := range recipientIDs {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			wanted[trimmed] = struct{}{}
		}
	}
	return k.listRecipients(ctx, wanted)
}

func (k *LocalCompatKeyring) DecryptionKeys(ctx context.Context) ([]domain.ProtectionKeyMaterial, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if k == nil || k.Catalog == nil || k.Keys == nil {
		return nil, fmt.Errorf("keyring local de compatibilidad no configurado")
	}
	refs, err := k.Catalog.List(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]domain.ProtectionKeyMaterial, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, ok := seen[ref.ID]; ok {
			continue
		}
		seen[ref.ID] = struct{}{}

		if material, ok := k.decryptionKeyFor(ctx, ref); ok {
			keys = append(keys, material)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].RecipientID < keys[j].RecipientID
	})
	return keys, nil
}

func (k *LocalCompatKeyring) listRecipients(ctx context.Context, wanted map[string]struct{}) ([]domain.ProtectionRecipient, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if k == nil || k.Catalog == nil || k.Keys == nil {
		return nil, fmt.Errorf("keyring local de compatibilidad no configurado")
	}
	refs, err := k.Catalog.List(ctx)
	if err != nil {
		return nil, err
	}
	recipients := make([]domain.ProtectionRecipient, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(wanted) > 0 {
			if _, ok := wanted[ref.ID]; !ok {
				continue
			}
		}
		if _, ok := seen[ref.ID]; ok {
			continue
		}
		seen[ref.ID] = struct{}{}

		if recipient, ok := k.recipientFor(ctx, ref); ok {
			recipients = append(recipients, recipient)
		}
	}
	sort.Slice(recipients, func(i, j int) bool {
		if recipients[i].Label == recipients[j].Label {
			return recipients[i].ID < recipients[j].ID
		}
		return recipients[i].Label < recipients[j].Label
	})
	return recipients, nil
}

func (k *LocalCompatKeyring) decryptionKeyFor(ctx context.Context, ref domain.CertificateRef) (domain.ProtectionKeyMaterial, bool) {
	key, err := k.Keys.KeyFor(ctx, ref)
	if err != nil {
		ports.CloseSigningKey(key)
		return domain.ProtectionKeyMaterial{}, false
	}
	defer ports.CloseSigningKey(key)

	local, err := extractLocalSigningKey(key)
	if err != nil || local == nil || local.Certificate == nil || local.Signer == nil {
		return domain.ProtectionKeyMaterial{}, false
	}
	privDER, ok := compatPrivateKeyPKCS8(local)
	if !ok {
		return domain.ProtectionKeyMaterial{}, false
	}
	return domain.ProtectionKeyMaterial{
		RecipientID:               ref.ID,
		RSAOAEP256PrivateKeyPKCS8: privDER,
		CertificateDER:            append([]byte(nil), local.Certificate.Raw...),
	}, true
}

func (k *LocalCompatKeyring) recipientFor(ctx context.Context, ref domain.CertificateRef) (domain.ProtectionRecipient, bool) {
	key, err := k.Keys.KeyFor(ctx, ref)
	if err != nil {
		ports.CloseSigningKey(key)
		return domain.ProtectionRecipient{}, false
	}
	defer ports.CloseSigningKey(key)
	local, err := extractLocalSigningKey(key)
	if err != nil || local == nil {
		return domain.ProtectionRecipient{}, false
	}

	// No basta con que el certificado publique una clave RSA: el destinatario
	// solo puede anunciarse si este mismo proceso dispone de una clave privada
	// RSA exportable apta para desproteger. Esta comprobación no serializa la
	// clave al refrescar el catálogo. Los firmantes opacos de CertStore,
	// PKCS#11 o hardware pueden seguir firmando, pero no se publican como
	// destinatarios RSA mientras no exista un adaptador crypto.Decrypter.
	_, ok := compatExportableRSA(local)
	if !ok {
		return domain.ProtectionRecipient{}, false
	}
	return buildCompatRecipientFromLocal(ref, local)
}

type localSigningKeySource interface {
	ToLocalSigningKey() *commonsigner.LocalSigningKey
}

func extractLocalSigningKey(key ports.SigningKey) (*commonsigner.LocalSigningKey, error) {
	switch k := key.(type) {
	case nil:
		return nil, fmt.Errorf("clave local no disponible")
	case *commonsigner.LocalSigningKey:
		return k, nil
	case localSigningKeySource:
		out := k.ToLocalSigningKey()
		if out == nil {
			return nil, fmt.Errorf("clave local vacia")
		}
		return out, nil
	default:
		return nil, fmt.Errorf("tipo de clave no exportable para proteccion: %T", key)
	}
}

func buildCompatRecipientFromLocal(ref domain.CertificateRef, local *commonsigner.LocalSigningKey) (domain.ProtectionRecipient, bool) {
	if local == nil || local.Certificate == nil || !puedeCifrarRSA(local.Certificate) {
		return domain.ProtectionRecipient{}, false
	}
	pub, ok := local.Certificate.PublicKey.(*rsa.PublicKey)
	if !ok {
		return domain.ProtectionRecipient{}, false
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil || len(pubDER) == 0 {
		return domain.ProtectionRecipient{}, false
	}
	label := strings.TrimSpace(ref.Subject)
	if label == "" {
		label = strings.TrimSpace(local.Certificate.Subject.String())
	}
	if label == "" {
		label = ref.ID
	}
	return domain.ProtectionRecipient{
		ID:                     ref.ID,
		Label:                  label,
		Origin:                 "propio",
		RSAOAEP256PublicKeyDER: pubDER,
		CertificateDER:         append([]byte(nil), local.Certificate.Raw...),
	}, true
}

func compatPrivateKeyPKCS8(local *commonsigner.LocalSigningKey) ([]byte, bool) {
	priv, ok := compatExportableRSA(local)
	if !ok {
		return nil, false
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil || len(privDER) == 0 {
		return nil, false
	}
	parsed, err := x509.ParsePKCS8PrivateKey(privDER)
	parsedPriv, ok := parsed.(*rsa.PrivateKey)
	if err != nil || !ok || parsedPriv == nil || parsedPriv.N.Cmp(priv.N) != 0 || parsedPriv.E != priv.E {
		clear(privDER)
		return nil, false
	}
	return privDER, true
}

func compatExportableRSA(local *commonsigner.LocalSigningKey) (*rsa.PrivateKey, bool) {
	if local == nil || local.Certificate == nil || local.Signer == nil || !puedeCifrarRSA(local.Certificate) {
		return nil, false
	}
	certPublic, ok := local.Certificate.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, false
	}
	priv, ok := local.Signer.(*rsa.PrivateKey)
	if !ok || priv == nil || priv.N.Cmp(certPublic.N) != 0 || priv.E != certPublic.E {
		return nil, false
	}
	return priv, true
}

func puedeCifrarRSA(cert *x509.Certificate) bool {
	if cert == nil || !certutil.PuedeCifrar(cert) {
		return false
	}
	// RSA-OAEP transporta una clave: si el certificado declara KeyUsage, debe
	// autorizar expresamente keyEncipherment. keyAgreement por sí solo no
	// habilita RSA-OAEP.
	return cert.KeyUsage == 0 || cert.KeyUsage&x509.KeyUsageKeyEncipherment != 0
}

var _ ports.ProtectionRecipientCatalog = (*LocalCompatKeyring)(nil)
var _ ports.ProtectionKeyProvider = (*LocalCompatKeyring)(nil)
