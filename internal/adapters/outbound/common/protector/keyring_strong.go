// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"context"
	"crypto/ecdh"
	"crypto/mlkem"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/domain"
)

const strongIdentityVersion = 1

const strongRecipientExportVersion = 1
const maxStrongRecipientFileBytes = 64 * 1024
const maxStrongRecipientLabelBytes = 256

type strongIdentityFile struct {
	Version             int    `json:"version"`
	ID                  string `json:"id"`
	Label               string `json:"label"`
	MLKEM768SeedB64     string `json:"mlkem768_seed_b64"`
	X25519PrivateKeyB64 string `json:"x25519_private_key_b64"`
}

type strongRecipientFile struct {
	Version            int    `json:"version"`
	Profile            string `json:"profile"`
	ID                 string `json:"id"`
	Label              string `json:"label"`
	MLKEM768PublicB64  string `json:"mlkem768_public_b64"`
	X25519PublicKeyB64 string `json:"x25519_public_key_b64"`
}

// LocalStrongKeyring expone un keyring local para el perfil alto usando
// material PQ+X25519 guardado en el directorio de configuración del usuario.
//
// Si no existe ninguna identidad fuerte todavía, genera una identidad local
// por defecto de forma segura y la persiste con permisos 0600. Así el perfil
// alto queda realmente utilizable sin depender de certificados X.509.
type LocalStrongKeyring struct {
	ConfigDir string
}

func NuevoLocalStrongKeyring(configDir string) *LocalStrongKeyring {
	return &LocalStrongKeyring{ConfigDir: strings.TrimSpace(configDir)}
}

func (k *LocalStrongKeyring) List(ctx context.Context) ([]domain.ProtectionRecipient, error) {
	recipients, err := k.loadRecipients(ctx, nil)
	if err != nil {
		return nil, err
	}
	return recipients, nil
}

func (k *LocalStrongKeyring) Resolve(ctx context.Context, recipientIDs []string) ([]domain.ProtectionRecipient, error) {
	wanted := make(map[string]struct{}, len(recipientIDs))
	for _, id := range recipientIDs {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			wanted[trimmed] = struct{}{}
		}
	}
	recipients, err := k.loadRecipients(ctx, wanted)
	if err != nil {
		return nil, err
	}
	return recipients, nil
}

func (k *LocalStrongKeyring) DecryptionKeys(ctx context.Context) ([]domain.ProtectionKeyMaterial, error) {
	identities, err := k.loadIdentities(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ProtectionKeyMaterial, 0, len(identities))
	for _, identity := range identities {
		out = append(out, identity.keyMaterial())
	}
	return out, nil
}

func (k *LocalStrongKeyring) Export(ctx context.Context, recipientID string) (domain.ProtectionRecipient, []byte, error) {
	recipients, err := k.Resolve(ctx, []string{recipientID})
	if err != nil {
		return domain.ProtectionRecipient{}, nil, err
	}
	if len(recipients) == 0 {
		return domain.ProtectionRecipient{}, nil, fmt.Errorf("no se encontró el destinatario fuerte solicitado")
	}
	recipient := recipients[0]
	raw, err := json.MarshalIndent(strongRecipientFile{
		Version:            strongRecipientExportVersion,
		Profile:            "alto",
		ID:                 recipient.ID,
		Label:              recipient.Label,
		MLKEM768PublicB64:  base64.StdEncoding.EncodeToString(recipient.MLKEM768PublicKey),
		X25519PublicKeyB64: base64.StdEncoding.EncodeToString(recipient.X25519PublicKey),
	}, "", "  ")
	if err != nil {
		return domain.ProtectionRecipient{}, nil, err
	}
	return recipient, append(raw, '\n'), nil
}

func (k *LocalStrongKeyring) Import(ctx context.Context, data []byte) (domain.ProtectionRecipient, error) {
	if err := ctx.Err(); err != nil {
		return domain.ProtectionRecipient{}, err
	}
	if k == nil || strings.TrimSpace(k.ConfigDir) == "" {
		return domain.ProtectionRecipient{}, fmt.Errorf("keyring local fuerte no configurado")
	}
	recipient, err := parseStrongRecipientFile(data)
	if err != nil {
		return domain.ProtectionRecipient{}, err
	}
	if err := os.MkdirAll(k.recipientDir(), 0o700); err != nil {
		return domain.ProtectionRecipient{}, fmt.Errorf("creando directorio de destinatarios fuertes: %w", err)
	}
	filename := filepath.Join(k.recipientDir(), recipient.ID+".json")
	if current, readErr := securefile.ReadFileLimit(filename, maxStrongRecipientFileBytes); readErr == nil {
		existing, parseErr := parseStrongRecipientFile(current)
		if parseErr == nil {
			if sameStrongRecipient(existing, recipient) {
				return existing, nil
			}
			return domain.ProtectionRecipient{}, fmt.Errorf("ya existe un destinatario fuerte con el mismo identificador")
		}
	} else if !os.IsNotExist(readErr) {
		return domain.ProtectionRecipient{}, fmt.Errorf("comprobando destinatario fuerte existente: %w", readErr)
	}
	exported, err := json.MarshalIndent(strongRecipientFile{
		Version:            strongRecipientExportVersion,
		Profile:            "alto",
		ID:                 recipient.ID,
		Label:              recipient.Label,
		MLKEM768PublicB64:  base64.StdEncoding.EncodeToString(recipient.MLKEM768PublicKey),
		X25519PublicKeyB64: base64.StdEncoding.EncodeToString(recipient.X25519PublicKey),
	}, "", "  ")
	if err != nil {
		return domain.ProtectionRecipient{}, err
	}
	if err := securefile.WriteFileAtomic(filename, append(exported, '\n'), 0o600); err != nil {
		return domain.ProtectionRecipient{}, fmt.Errorf("persistiendo destinatario fuerte importado: %w", err)
	}
	return recipient, nil
}

type strongIdentity struct {
	id         string
	label      string
	mlkemSeed  []byte
	x25519Key  []byte
	mlkemPriv  *mlkem.DecapsulationKey768
	x25519Priv *ecdh.PrivateKey
}

func (i strongIdentity) recipient() domain.ProtectionRecipient {
	return domain.ProtectionRecipient{
		ID:                i.id,
		Label:             i.label,
		Origin:            "propio",
		MLKEM768PublicKey: i.mlkemPriv.EncapsulationKey().Bytes(),
		X25519PublicKey:   i.x25519Priv.PublicKey().Bytes(),
	}
}

func (i strongIdentity) keyMaterial() domain.ProtectionKeyMaterial {
	return domain.ProtectionKeyMaterial{
		RecipientID:      i.id,
		MLKEM768Seed:     append([]byte(nil), i.mlkemSeed...),
		X25519PrivateKey: append([]byte(nil), i.x25519Key...),
	}
}

func (k *LocalStrongKeyring) loadRecipients(ctx context.Context, wanted map[string]struct{}) ([]domain.ProtectionRecipient, error) {
	identities, err := k.loadIdentities(ctx)
	if err != nil {
		return nil, err
	}
	recipients := make([]domain.ProtectionRecipient, 0, len(identities))
	for _, identity := range identities {
		recipient := identity.recipient()
		if len(wanted) > 0 {
			if _, ok := wanted[recipient.ID]; !ok {
				continue
			}
		}
		recipients = append(recipients, recipient)
	}
	imported, err := k.loadImportedRecipients(ctx, wanted)
	if err != nil {
		return nil, err
	}
	recipients = append(recipients, imported...)
	return dedupeRecipients(recipients), nil
}

func (k *LocalStrongKeyring) loadIdentities(ctx context.Context) ([]strongIdentity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if k == nil || strings.TrimSpace(k.ConfigDir) == "" {
		return nil, fmt.Errorf("keyring local fuerte no configurado")
	}
	if err := k.ensureDefaultIdentity(ctx); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(k.dir())
	if err != nil {
		return nil, err
	}
	out := make([]strongIdentity, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		identity, err := k.readIdentity(filepath.Join(k.dir(), entry.Name()))
		if err != nil {
			continue
		}
		out = append(out, identity)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].label == out[j].label {
			return out[i].id < out[j].id
		}
		return out[i].label < out[j].label
	})
	return out, nil
}

func (k *LocalStrongKeyring) loadImportedRecipients(ctx context.Context, wanted map[string]struct{}) ([]domain.ProtectionRecipient, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir := k.recipientDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]domain.ProtectionRecipient, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		raw, err := securefile.ReadFileLimit(filepath.Join(dir, entry.Name()), maxStrongRecipientFileBytes)
		if err != nil {
			continue
		}
		recipient, err := parseStrongRecipientFile(raw)
		if err != nil {
			continue
		}
		if len(wanted) > 0 {
			if _, ok := wanted[recipient.ID]; !ok {
				continue
			}
		}
		out = append(out, recipient)
	}
	return out, nil
}

func (k *LocalStrongKeyring) ensureDefaultIdentity(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := k.dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creando directorio de protección fuerte: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			return nil
		}
	}
	mlkemPriv, err := mlkem.GenerateKey768()
	if err != nil {
		return fmt.Errorf("generando identidad ML-KEM-768: %w", err)
	}
	x25519Priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generando identidad X25519: %w", err)
	}
	id := strongIdentityID(mlkemPriv.EncapsulationKey().Bytes(), x25519Priv.PublicKey().Bytes())
	label := defaultStrongIdentityLabel()
	raw, err := json.MarshalIndent(strongIdentityFile{
		Version:             strongIdentityVersion,
		ID:                  id,
		Label:               label,
		MLKEM768SeedB64:     base64.StdEncoding.EncodeToString(mlkemPriv.Bytes()),
		X25519PrivateKeyB64: base64.StdEncoding.EncodeToString(x25519Priv.Bytes()),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("serializando identidad fuerte local: %w", err)
	}
	if err := securefile.WriteFileAtomic(filepath.Join(dir, id+".json"), append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("persistiendo identidad fuerte local: %w", err)
	}
	return nil
}

func (k *LocalStrongKeyring) readIdentity(path string) (strongIdentity, error) {
	raw, err := securefile.ReadFileLimit(path, maxStrongRecipientFileBytes)
	if err != nil {
		return strongIdentity{}, err
	}
	var stored strongIdentityFile
	if err := json.Unmarshal(raw, &stored); err != nil {
		return strongIdentity{}, err
	}
	mlkemSeed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(stored.MLKEM768SeedB64))
	if err != nil {
		return strongIdentity{}, err
	}
	x25519Key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(stored.X25519PrivateKeyB64))
	if err != nil {
		return strongIdentity{}, err
	}
	mlkemPriv, err := mlkem.NewDecapsulationKey768(mlkemSeed)
	if err != nil {
		return strongIdentity{}, err
	}
	x25519Priv, err := ecdh.X25519().NewPrivateKey(x25519Key)
	if err != nil {
		return strongIdentity{}, err
	}
	id := strings.TrimSpace(stored.ID)
	if id == "" {
		id = strongIdentityID(mlkemPriv.EncapsulationKey().Bytes(), x25519Priv.PublicKey().Bytes())
	}
	label := strings.TrimSpace(stored.Label)
	if label == "" {
		label = id
	}
	return strongIdentity{
		id:         id,
		label:      label,
		mlkemSeed:  mlkemSeed,
		x25519Key:  x25519Key,
		mlkemPriv:  mlkemPriv,
		x25519Priv: x25519Priv,
	}, nil
}

func (k *LocalStrongKeyring) dir() string {
	return filepath.Join(k.ConfigDir, "protection", "strong")
}

func (k *LocalStrongKeyring) recipientDir() string {
	return filepath.Join(k.ConfigDir, "protection", "recipients")
}

func parseStrongRecipientFile(data []byte) (domain.ProtectionRecipient, error) {
	if len(data) == 0 || len(data) > maxStrongRecipientFileBytes {
		return domain.ProtectionRecipient{}, fmt.Errorf("tamaño de destinatario fuerte no válido")
	}
	var stored strongRecipientFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return domain.ProtectionRecipient{}, fmt.Errorf("json de destinatario fuerte no válido: %w", err)
	}
	if strings.TrimSpace(stored.Profile) != "" && strings.TrimSpace(stored.Profile) != "alto" {
		return domain.ProtectionRecipient{}, fmt.Errorf("el fichero no corresponde a un destinatario fuerte")
	}
	mlkemPub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(stored.MLKEM768PublicB64))
	if err != nil {
		return domain.ProtectionRecipient{}, err
	}
	x25519Pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(stored.X25519PublicKeyB64))
	if err != nil {
		return domain.ProtectionRecipient{}, err
	}
	if _, err := mlkem.NewEncapsulationKey768(mlkemPub); err != nil {
		return domain.ProtectionRecipient{}, fmt.Errorf("clave pública ML-KEM-768 no válida: %w", err)
	}
	if _, err := ecdh.X25519().NewPublicKey(x25519Pub); err != nil {
		return domain.ProtectionRecipient{}, fmt.Errorf("clave pública X25519 no válida: %w", err)
	}
	label := strings.TrimSpace(stored.Label)
	if len(label) > maxStrongRecipientLabelBytes {
		return domain.ProtectionRecipient{}, fmt.Errorf("etiqueta de destinatario fuerte demasiado larga")
	}
	expectedID := strongIdentityID(mlkemPub, x25519Pub)
	if strings.TrimSpace(stored.ID) != expectedID {
		return domain.ProtectionRecipient{}, fmt.Errorf("el identificador del destinatario fuerte no coincide con sus claves")
	}
	recipient := domain.ProtectionRecipient{
		ID:                expectedID,
		Label:             label,
		Origin:            "importado",
		MLKEM768PublicKey: mlkemPub,
		X25519PublicKey:   x25519Pub,
	}
	if err := recipient.Validate(domain.ProtectionProfileStrong); err != nil {
		return domain.ProtectionRecipient{}, err
	}
	if recipient.Label == "" {
		recipient.Label = recipient.ID
	}
	return recipient, nil
}

func sameStrongRecipient(a, b domain.ProtectionRecipient) bool {
	return a.ID == b.ID &&
		a.Label == b.Label &&
		base64.StdEncoding.EncodeToString(a.MLKEM768PublicKey) == base64.StdEncoding.EncodeToString(b.MLKEM768PublicKey) &&
		base64.StdEncoding.EncodeToString(a.X25519PublicKey) == base64.StdEncoding.EncodeToString(b.X25519PublicKey)
}

func strongIdentityID(mlkemPub, x25519Pub []byte) string {
	sum := sha256.Sum256(append(append([]byte(nil), mlkemPub...), x25519Pub...))
	return hex.EncodeToString(sum[:8])
}

func defaultStrongIdentityLabel() string {
	user := strings.TrimSpace(os.Getenv("USER"))
	host, _ := os.Hostname()
	switch {
	case user != "" && host != "":
		return user + "@" + host + " (alto)"
	case user != "":
		return user + " (alto)"
	case host != "":
		return host + " (alto)"
	default:
		return "Identidad local fuerte"
	}
}
