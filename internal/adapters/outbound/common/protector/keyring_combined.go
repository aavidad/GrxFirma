// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"context"
	"fmt"
	"sort"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// LocalCombinedKeyring une el keyring compat basado en certificados locales
// y el keyring fuerte local basado en identidades PQ+X25519 del usuario.
type LocalCombinedKeyring struct {
	Compat *LocalCompatKeyring
	Strong *LocalStrongKeyring
	Public *LocalPublicRecipientBook
}

func NuevoLocalCombinedKeyring(configDir string, catalog ports.CertificateCatalog, keys ports.SigningKeyProvider) *LocalCombinedKeyring {
	return &LocalCombinedKeyring{
		Compat: NuevoLocalCompatKeyring(catalog, keys),
		Strong: NuevoLocalStrongKeyring(configDir),
		Public: NuevoLocalPublicRecipientBook(configDir),
	}
}

func (k *LocalCombinedKeyring) List(ctx context.Context) ([]domain.ProtectionRecipient, error) {
	var all []domain.ProtectionRecipient
	if k != nil && k.Strong != nil {
		recipients, err := k.Strong.List(ctx)
		if err != nil {
			return nil, err
		}
		all = append(all, recipients...)
	}
	if k != nil && k.Compat != nil {
		recipients, err := k.Compat.List(ctx)
		if err != nil {
			return nil, err
		}
		all = append(all, recipients...)
	}
	if k != nil && k.Public != nil {
		recipients, err := k.Public.List(ctx)
		if err != nil {
			return nil, err
		}
		all = append(all, recipients...)
	}
	if k != nil {
		// «Otras personas» es opcional: si el almacén no existe o no se puede
		// leer, se siguen ofreciendo los demás destinatarios.
		if recipients, err := listWindowsAddressBook(ctx); err == nil {
			all = append(all, recipients...)
		} else if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
	}
	return dedupeRecipients(all), nil
}

func (k *LocalCombinedKeyring) Resolve(ctx context.Context, recipientIDs []string) ([]domain.ProtectionRecipient, error) {
	var all []domain.ProtectionRecipient
	if k != nil && k.Strong != nil {
		recipients, err := k.Strong.Resolve(ctx, recipientIDs)
		if err != nil {
			return nil, err
		}
		all = append(all, recipients...)
	}
	if k != nil && k.Compat != nil {
		recipients, err := k.Compat.Resolve(ctx, recipientIDs)
		if err != nil {
			return nil, err
		}
		all = append(all, recipients...)
	}
	if k != nil && k.Public != nil {
		recipients, err := k.Public.Resolve(ctx, recipientIDs)
		if err != nil {
			return nil, err
		}
		all = append(all, recipients...)
	}
	if k != nil {
		recipients, err := listWindowsAddressBook(ctx)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			recipients = nil
		}
		wanted := make(map[string]bool, len(recipientIDs))
		for _, id := range recipientIDs {
			wanted[id] = true
		}
		for _, recipient := range recipients {
			if wanted[recipient.ID] {
				all = append(all, recipient)
			}
		}
	}
	return dedupeRecipients(all), nil
}

func (k *LocalCombinedKeyring) ImportPublic(ctx context.Context, data []byte) (domain.ProtectionRecipient, error) {
	if k == nil || k.Public == nil {
		return domain.ProtectionRecipient{}, fmt.Errorf("libro de destinatarios públicos no configurado")
	}
	return k.Public.Import(ctx, data)
}

func (k *LocalCombinedKeyring) RemovePublic(ctx context.Context, id string) error {
	if k == nil || k.Public == nil {
		return fmt.Errorf("libro de destinatarios públicos no configurado")
	}
	return k.Public.Remove(ctx, id)
}

func (k *LocalCombinedKeyring) DecryptionKeys(ctx context.Context) ([]domain.ProtectionKeyMaterial, error) {
	var all []domain.ProtectionKeyMaterial
	if k != nil && k.Strong != nil {
		keys, err := k.Strong.DecryptionKeys(ctx)
		if err != nil {
			return nil, err
		}
		all = append(all, keys...)
	}
	if k != nil && k.Compat != nil {
		keys, err := k.Compat.DecryptionKeys(ctx)
		if err != nil {
			return nil, err
		}
		all = append(all, keys...)
	}
	return dedupeKeyMaterials(all), nil
}

func (k *LocalCombinedKeyring) Export(ctx context.Context, recipientID string) (domain.ProtectionRecipient, []byte, error) {
	if k == nil || k.Strong == nil {
		return domain.ProtectionRecipient{}, nil, fmt.Errorf("intercambio de destinatarios fuertes no configurado")
	}
	return k.Strong.Export(ctx, recipientID)
}

func (k *LocalCombinedKeyring) Import(ctx context.Context, data []byte) (domain.ProtectionRecipient, error) {
	if k == nil || k.Strong == nil {
		return domain.ProtectionRecipient{}, fmt.Errorf("intercambio de destinatarios fuertes no configurado")
	}
	return k.Strong.Import(ctx, data)
}

func dedupeRecipients(in []domain.ProtectionRecipient) []domain.ProtectionRecipient {
	out := make([]domain.ProtectionRecipient, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	certs := make(map[string]struct{}, len(in))
	for _, recipient := range in {
		if recipient.ID == "" {
			continue
		}
		if _, ok := seen[recipient.ID]; ok {
			continue
		}
		if len(recipient.CertificateDER) > 0 {
			key := string(recipient.CertificateDER)
			if _, ok := certs[key]; ok {
				continue
			}
			certs[key] = struct{}{}
		}
		seen[recipient.ID] = struct{}{}
		out = append(out, recipient)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Label == out[j].Label {
			return out[i].ID < out[j].ID
		}
		return out[i].Label < out[j].Label
	})
	return out
}

func dedupeKeyMaterials(in []domain.ProtectionKeyMaterial) []domain.ProtectionKeyMaterial {
	out := make([]domain.ProtectionKeyMaterial, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, key := range in {
		if key.RecipientID == "" {
			continue
		}
		if _, ok := seen[key.RecipientID]; ok {
			continue
		}
		seen[key.RecipientID] = struct{}{}
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].RecipientID < out[j].RecipientID
	})
	return out
}

var _ ports.ProtectionRecipientCatalog = (*LocalCombinedKeyring)(nil)
var _ ports.ProtectionKeyProvider = (*LocalCombinedKeyring)(nil)
var _ ports.ProtectionRecipientExchangeStore = (*LocalCombinedKeyring)(nil)
