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
	"grxfirma/internal/ports"
)

// strongIdentityLegacyVersion guardaba la clave privada en claro en el
// fichero. La versión 2 solo guarda la parte pública: la privada vive en el
// almacén de secretos del sistema (F-02).
const strongIdentityLegacyVersion = 1
const strongIdentityVersion = 2

// strongIdentitySecretBytes es la semilla ML-KEM-768 (64 bytes) seguida de la
// clave privada X25519 (32 bytes).
const strongIdentitySecretBytes = mlkem.SeedSize + 32

// ErrAlmacenSeguroNoDisponible indica que el almacén de secretos del sistema
// (Secret Service, Keychain o Credential Manager) no se pudo usar. Sin él no
// se crea ni se usa la identidad del perfil alto: nunca se deja en claro.
var ErrAlmacenSeguroNoDisponible = errors.New("protector: almacén seguro del sistema no disponible para la identidad del perfil alto")

const strongRecipientExportVersion = 1
const maxStrongRecipientFileBytes = 64 * 1024
const maxStrongRecipientLabelBytes = 256

// strongIdentityLegacyFile es el formato v1, con la clave privada en claro.
// Solo se lee para migrarlo.
type strongIdentityLegacyFile struct {
	Version             int    `json:"version"`
	ID                  string `json:"id"`
	Label               string `json:"label"`
	MLKEM768SeedB64     string `json:"mlkem768_seed_b64"`
	X25519PrivateKeyB64 string `json:"x25519_private_key_b64"`
}

// strongIdentityFile es el formato v2: solo material público y la etiqueta.
type strongIdentityFile struct {
	Version            int    `json:"version"`
	ID                 string `json:"id"`
	Label              string `json:"label"`
	MLKEM768PublicB64  string `json:"mlkem768_public_b64"`
	X25519PublicKeyB64 string `json:"x25519_public_key_b64"`
}

type strongRecipientFile struct {
	Version            int    `json:"version"`
	Profile            string `json:"profile"`
	ID                 string `json:"id"`
	Label              string `json:"label"`
	MLKEM768PublicB64  string `json:"mlkem768_public_b64"`
	X25519PublicKeyB64 string `json:"x25519_public_key_b64"`
}

// LocalStrongKeyring expone un keyring local para el perfil alto con
// identidades ML-KEM-768 + X25519.
//
// La parte pública de cada identidad se guarda en DataDir, que en Windows es
// %LOCALAPPDATA% y no viaja con el perfil itinerante. La clave privada se
// guarda en el almacén de secretos del sistema (Secrets) y nunca en un
// fichero. Si no existe ninguna identidad, genera una por defecto. Las
// identidades v1, con la clave en claro bajo ConfigDir, se migran al leerlas:
// la clave pasa al almacén y el fichero antiguo se sobrescribe y se borra.
// Sin almacén seguro no se crea ni se usa ninguna identidad fuerte.
type LocalStrongKeyring struct {
	// ConfigDir guarda los destinatarios fuertes importados (material
	// público) y es donde estaban las identidades v1.
	ConfigDir string
	// DataDir guarda la parte pública de las identidades propias.
	DataDir string
	// Secrets guarda la clave privada de cada identidad propia.
	Secrets ports.SecureStorage
}

func NuevoLocalStrongKeyring(configDir, dataDir string, secrets ports.SecureStorage) *LocalStrongKeyring {
	return &LocalStrongKeyring{
		ConfigDir: strings.TrimSpace(configDir),
		DataDir:   strings.TrimSpace(dataDir),
		Secrets:   secrets,
	}
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

// DecryptionKeys devuelve solo las identidades fuertes que el sobre .afp
// nombra como destinatarias.
func (k *LocalStrongKeyring) DecryptionKeys(ctx context.Context, protected domain.Document) ([]domain.ProtectionKeyMaterial, error) {
	destinatarios := destinatariosDe(protected)
	identities, err := k.loadIdentities(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ProtectionKeyMaterial, 0, 1)
	for _, identity := range identities {
		if !destinatarios.incluyeID(identity.id) {
			continue
		}
		material, err := k.loadPrivateMaterial(ctx, identity)
		if err != nil {
			continue
		}
		out = append(out, material)
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

// strongIdentity es la parte pública de una identidad propia.
type strongIdentity struct {
	id        string
	label     string
	mlkemPub  []byte
	x25519Pub []byte
}

func (i strongIdentity) recipient() domain.ProtectionRecipient {
	return domain.ProtectionRecipient{
		ID:                i.id,
		Label:             i.label,
		Origin:            "propio",
		MLKEM768PublicKey: append([]byte(nil), i.mlkemPub...),
		X25519PublicKey:   append([]byte(nil), i.x25519Pub...),
	}
}

func strongIdentitySecretKey(id string) string {
	return "proteccion-alto-" + id
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
	if k == nil || strings.TrimSpace(k.ConfigDir) == "" || strings.TrimSpace(k.DataDir) == "" {
		return nil, fmt.Errorf("keyring local fuerte no configurado")
	}
	if err := k.prepareIdentityDir(); err != nil {
		return nil, err
	}
	migrateErr := k.migrateLegacyIdentities(ctx)
	identities, err := k.readIdentities(ctx)
	if err != nil {
		return nil, err
	}
	if len(identities) == 0 {
		if migrateErr != nil {
			// Hay identidades antiguas sin migrar: no se crea otra encima.
			return nil, migrateErr
		}
		if err := k.createDefaultIdentity(ctx); err != nil {
			return nil, err
		}
		if identities, err = k.readIdentities(ctx); err != nil {
			return nil, err
		}
	}
	return identities, nil
}

func (k *LocalStrongKeyring) prepareIdentityDir() error {
	dir := k.dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creando directorio de protección fuerte: %w", err)
	}
	// MkdirAll no corrige los permisos de un directorio que ya existía.
	if err := securefile.ProtectDirectory(dir, 0o700); err != nil {
		return fmt.Errorf("protegiendo directorio de protección fuerte: %w", err)
	}
	return nil
}

func (k *LocalStrongKeyring) readIdentities(ctx context.Context) ([]strongIdentity, error) {
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
		identity, err := readIdentityFile(filepath.Join(k.dir(), entry.Name()))
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

func (k *LocalStrongKeyring) createDefaultIdentity(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	mlkemPriv, err := mlkem.GenerateKey768()
	if err != nil {
		return fmt.Errorf("generando identidad ML-KEM-768: %w", err)
	}
	x25519Priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generando identidad X25519: %w", err)
	}
	return k.persistIdentity(ctx, defaultStrongIdentityLabel(), mlkemPriv.Bytes(), x25519Priv.Bytes())
}

// persistIdentity guarda primero la clave privada en el almacén del sistema
// y solo después escribe el fichero público. Si el almacén falla no queda
// nada en disco.
func (k *LocalStrongKeyring) persistIdentity(ctx context.Context, label string, mlkemSeed, x25519Key []byte) error {
	identity, err := strongIdentityFromPrivate(label, mlkemSeed, x25519Key)
	if err != nil {
		return err
	}
	if k.Secrets == nil {
		return ErrAlmacenSeguroNoDisponible
	}
	secret := make([]byte, 0, strongIdentitySecretBytes)
	secret = append(secret, mlkemSeed...)
	secret = append(secret, x25519Key...)
	defer zeroBytes(secret)
	if err := k.Secrets.Store(ctx, strongIdentitySecretKey(identity.id), secret); err != nil {
		return fmt.Errorf("%w: %v", ErrAlmacenSeguroNoDisponible, err)
	}
	raw, err := json.MarshalIndent(strongIdentityFile{
		Version:            strongIdentityVersion,
		ID:                 identity.id,
		Label:              identity.label,
		MLKEM768PublicB64:  base64.StdEncoding.EncodeToString(identity.mlkemPub),
		X25519PublicKeyB64: base64.StdEncoding.EncodeToString(identity.x25519Pub),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("serializando identidad fuerte local: %w", err)
	}
	if err := securefile.WriteFileAtomic(filepath.Join(k.dir(), identity.id+".json"), append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("persistiendo identidad fuerte local: %w", err)
	}
	return nil
}

// migrateLegacyIdentities pasa al almacén del sistema las identidades v1 que
// guardaban la clave privada en claro, escribe su versión pública y
// sobrescribe y borra el fichero antiguo. Si el almacén no está disponible deja
// el fichero antiguo intacto, sin usarlo, para poder migrarlo más tarde.
func (k *LocalStrongKeyring) migrateLegacyIdentities(ctx context.Context) error {
	var errs []error
	for _, dir := range k.legacyDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
			}
			continue
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			legacy, ok := readLegacyIdentityFile(path)
			if !ok {
				continue
			}
			err := k.persistIdentity(ctx, legacy.label, legacy.mlkemSeed, legacy.x25519Key)
			zeroBytes(legacy.mlkemSeed)
			zeroBytes(legacy.x25519Key)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if err := securefile.RemoveSensitiveFile(path); err != nil {
				errs = append(errs, fmt.Errorf("borrando identidad fuerte en claro: %w", err))
			}
		}
	}
	return errors.Join(errs...)
}

// legacyDirs son los directorios donde puede quedar una identidad v1: el de
// siempre bajo ConfigDir y, por si acaso, el nuevo.
func (k *LocalStrongKeyring) legacyDirs() []string {
	legacy := filepath.Join(k.ConfigDir, "protection", "strong")
	if filepath.Clean(legacy) == filepath.Clean(k.dir()) {
		return []string{legacy}
	}
	return []string{legacy, k.dir()}
}

type legacyStrongIdentity struct {
	label     string
	mlkemSeed []byte
	x25519Key []byte
}

func readLegacyIdentityFile(path string) (legacyStrongIdentity, bool) {
	raw, err := securefile.ReadFileLimit(path, maxStrongRecipientFileBytes)
	if err != nil {
		return legacyStrongIdentity{}, false
	}
	defer zeroBytes(raw)
	var stored strongIdentityLegacyFile
	if err := json.Unmarshal(raw, &stored); err != nil {
		return legacyStrongIdentity{}, false
	}
	if stored.Version > strongIdentityLegacyVersion ||
		(strings.TrimSpace(stored.MLKEM768SeedB64) == "" && strings.TrimSpace(stored.X25519PrivateKeyB64) == "") {
		return legacyStrongIdentity{}, false
	}
	mlkemSeed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(stored.MLKEM768SeedB64))
	if err != nil {
		return legacyStrongIdentity{}, false
	}
	x25519Key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(stored.X25519PrivateKeyB64))
	if err != nil {
		zeroBytes(mlkemSeed)
		return legacyStrongIdentity{}, false
	}
	return legacyStrongIdentity{
		label:     strings.TrimSpace(stored.Label),
		mlkemSeed: mlkemSeed,
		x25519Key: x25519Key,
	}, true
}

func strongIdentityFromPrivate(label string, mlkemSeed, x25519Key []byte) (strongIdentity, error) {
	mlkemPriv, err := mlkem.NewDecapsulationKey768(mlkemSeed)
	if err != nil {
		return strongIdentity{}, fmt.Errorf("clave privada ML-KEM-768 no válida: %w", err)
	}
	x25519Priv, err := ecdh.X25519().NewPrivateKey(x25519Key)
	if err != nil {
		return strongIdentity{}, fmt.Errorf("clave privada X25519 no válida: %w", err)
	}
	mlkemPub := mlkemPriv.EncapsulationKey().Bytes()
	x25519Pub := x25519Priv.PublicKey().Bytes()
	id := strongIdentityID(mlkemPub, x25519Pub)
	label = strings.TrimSpace(label)
	if label == "" {
		label = id
	}
	return strongIdentity{id: id, label: label, mlkemPub: mlkemPub, x25519Pub: x25519Pub}, nil
}

func readIdentityFile(path string) (strongIdentity, error) {
	raw, err := securefile.ReadFileLimit(path, maxStrongRecipientFileBytes)
	if err != nil {
		return strongIdentity{}, err
	}
	var stored strongIdentityFile
	if err := json.Unmarshal(raw, &stored); err != nil {
		return strongIdentity{}, err
	}
	if stored.Version != strongIdentityVersion {
		return strongIdentity{}, fmt.Errorf("versión de identidad fuerte no soportada: %d", stored.Version)
	}
	mlkemPub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(stored.MLKEM768PublicB64))
	if err != nil {
		return strongIdentity{}, err
	}
	x25519Pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(stored.X25519PublicKeyB64))
	if err != nil {
		return strongIdentity{}, err
	}
	if _, err := mlkem.NewEncapsulationKey768(mlkemPub); err != nil {
		return strongIdentity{}, err
	}
	if _, err := ecdh.X25519().NewPublicKey(x25519Pub); err != nil {
		return strongIdentity{}, err
	}
	id := strongIdentityID(mlkemPub, x25519Pub)
	if strings.TrimSpace(stored.ID) != id {
		return strongIdentity{}, fmt.Errorf("el identificador de la identidad fuerte no coincide con sus claves")
	}
	label := strings.TrimSpace(stored.Label)
	if label == "" {
		label = id
	}
	return strongIdentity{id: id, label: label, mlkemPub: mlkemPub, x25519Pub: x25519Pub}, nil
}

// loadPrivateMaterial recupera del almacén del sistema la clave privada de una
// identidad y comprueba que corresponde a su parte pública.
func (k *LocalStrongKeyring) loadPrivateMaterial(ctx context.Context, identity strongIdentity) (domain.ProtectionKeyMaterial, error) {
	if k.Secrets == nil {
		return domain.ProtectionKeyMaterial{}, ErrAlmacenSeguroNoDisponible
	}
	secret, err := k.Secrets.Load(ctx, strongIdentitySecretKey(identity.id))
	if err != nil {
		return domain.ProtectionKeyMaterial{}, fmt.Errorf("%w: %v", ErrAlmacenSeguroNoDisponible, err)
	}
	defer zeroBytes(secret)
	if len(secret) != strongIdentitySecretBytes {
		return domain.ProtectionKeyMaterial{}, errors.New("clave privada fuerte con tamaño no válido")
	}
	mlkemSeed := append([]byte(nil), secret[:mlkem.SeedSize]...)
	x25519Key := append([]byte(nil), secret[mlkem.SeedSize:]...)
	derived, err := strongIdentityFromPrivate(identity.label, mlkemSeed, x25519Key)
	if err != nil || derived.id != identity.id {
		zeroBytes(mlkemSeed)
		zeroBytes(x25519Key)
		return domain.ProtectionKeyMaterial{}, errors.New("la clave privada fuerte no corresponde a la identidad")
	}
	return domain.ProtectionKeyMaterial{
		RecipientID:      identity.id,
		MLKEM768Seed:     mlkemSeed,
		X25519PrivateKey: x25519Key,
	}, nil
}

func (k *LocalStrongKeyring) dir() string {
	return filepath.Join(k.DataDir, "protection", "strong")
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
