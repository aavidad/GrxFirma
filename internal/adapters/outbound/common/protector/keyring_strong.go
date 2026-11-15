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
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"grxfirma/internal/adapters/outbound/common/keyringstore"
	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/excepciones"
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
// (Secret Service, Keychain o Credential Manager) no se pudo usar. Sin él la
// identidad del perfil alto se guarda en el fichero v1 de siempre y se avisa
// (lote 19); el error ya solo sale si ni siquiera eso es posible.
var ErrAlmacenSeguroNoDisponible = errors.New("protector: almacén seguro del sistema no disponible para la identidad del perfil alto")

// AvisoProteccionAltoSinAlmacen es la clave de catálogo del aviso que se da
// al crear o usar una identidad del perfil alto cuya clave privada sigue en
// un fichero de este equipo porque no hay almacén de secretos (lote 19).
const AvisoProteccionAltoSinAlmacen = "proteccion.alto.sin_almacen.aviso"

// eventoClaveAltoEnFichero identifica en el registro el aviso de que la clave
// privada del perfil alto no está protegida por el almacén del sistema.
const eventoClaveAltoEnFichero = "protector_strong_key_unprotected_file"

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
// guarda en el almacén de secretos del sistema (Secrets). Si no existe ninguna
// identidad, genera una por defecto. Las identidades v1, con la clave en claro
// bajo ConfigDir, se migran al leerlas: la clave pasa al almacén y el fichero
// antiguo se sobrescribe y se borra.
//
// Sin almacén, el uso de una identidad v1 exige el ajuste de lectura y la
// creación de una nueva exige también el de creación. Cada operación admitida
// anota un aviso y conserva el registro. La migración al almacén retira el
// fichero v1 solo después de proteger y comprobar la clave.
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
	if k != nil {
		ctx = excepciones.WithConfigDir(ctx, k.ConfigDir)
	}
	recipients, _, err := k.loadRecipients(ctx, nil)
	if err != nil {
		return nil, err
	}
	return recipients, nil
}

// Resolve se usa al proteger: avisa si alguna identidad propia elegida tiene
// la clave en fichero.
func (k *LocalStrongKeyring) Resolve(ctx context.Context, recipientIDs []string) ([]domain.ProtectionRecipient, error) {
	// Los ID x509 pertenecen al catálogo público; no abrimos claves fuertes
	// privadas para resolver una protección del perfil de compatibilidad.
	if len(recipientIDs) > 0 {
		onlyPublic := true
		for _, id := range recipientIDs {
			if !strings.HasPrefix(id, "x509-") {
				onlyPublic = false
				break
			}
		}
		if onlyPublic {
			return nil, nil
		}
	}
	if k != nil {
		ctx = excepciones.WithConfigDir(ctx, k.ConfigDir)
	}
	recipients, enFichero, err := k.resolve(ctx, recipientIDs)
	if err != nil {
		return nil, err
	}
	for _, identity := range enFichero {
		if err := excepciones.Require(ctx, "alto_clave_fichero_crear"); err != nil {
			return nil, err
		}
		avisarClaveAltoEnFichero(ctx, "protect", identity.id)
	}
	return recipients, nil
}

func (k *LocalStrongKeyring) resolve(ctx context.Context, recipientIDs []string) ([]domain.ProtectionRecipient, []strongIdentity, error) {
	wanted := make(map[string]struct{}, len(recipientIDs))
	for _, id := range recipientIDs {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			wanted[trimmed] = struct{}{}
		}
	}
	return k.loadRecipients(ctx, wanted)
}

// DecryptionKeys devuelve solo las identidades fuertes que el sobre .afp
// nombra como destinatarias.
func (k *LocalStrongKeyring) DecryptionKeys(ctx context.Context, protected domain.Document) ([]domain.ProtectionKeyMaterial, error) {
	if k != nil {
		ctx = excepciones.WithConfigDir(ctx, k.ConfigDir)
	}
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
		material, enFichero, err := k.loadPrivateMaterial(ctx, identity)
		if err != nil {
			var block *excepciones.Block
			if errors.As(err, &block) {
				return nil, err
			}
			continue
		}
		if enFichero {
			avisarClaveAltoEnFichero(ctx, "unprotect", identity.id)
		}
		out = append(out, material)
	}
	if len(out) == 0 && k.hasLegacyRecipientFile(destinatarios.ids, publicIdentityPaths(identities)) {
		if err := excepciones.Check("alto_clave_fichero_leer", ctx); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (k *LocalStrongKeyring) Export(ctx context.Context, recipientID string) (domain.ProtectionRecipient, []byte, error) {
	if k != nil {
		ctx = excepciones.WithConfigDir(ctx, k.ConfigDir)
	}
	recipients, enFichero, err := k.resolve(ctx, []string{recipientID})
	if err != nil {
		return domain.ProtectionRecipient{}, nil, err
	}
	if len(recipients) == 0 {
		return domain.ProtectionRecipient{}, nil, fmt.Errorf("no se encontró el destinatario fuerte solicitado")
	}
	recipient := recipients[0]
	for _, identity := range enFichero {
		if identity.id == recipient.ID {
			if err := excepciones.Require(ctx, "alto_clave_fichero_crear"); err != nil {
				return domain.ProtectionRecipient{}, nil, err
			}
			avisarClaveAltoEnFichero(ctx, "export", identity.id)
		}
	}
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
	// enAlmacen dice que la clave privada está en el almacén del sistema
	// (identidad v2).
	enAlmacen bool
	// legacyPath es el fichero v1 que aún guarda la clave privada. Para una
	// identidad solo v1 es su única fuente; para una v2 que conserva su v1
	// (la migración no pudo borrarlo) solo se usa si el almacén no responde.
	legacyPath string
	sourcePath string
}

// claveEnFichero dice si la clave privada de la identidad se lee de su
// fichero v1 y no del almacén.
func (i strongIdentity) claveEnFichero() bool {
	return !i.enAlmacen && i.legacyPath != ""
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

// loadRecipients devuelve los destinatarios pedidos y, aparte, las
// identidades propias entre ellos cuya clave privada está en fichero.
func (k *LocalStrongKeyring) loadRecipients(ctx context.Context, wanted map[string]struct{}) ([]domain.ProtectionRecipient, []strongIdentity, error) {
	identities, err := k.loadIdentities(ctx)
	if err != nil {
		return nil, nil, err
	}
	recipients := make([]domain.ProtectionRecipient, 0, len(identities))
	var enFichero []strongIdentity
	for _, identity := range identities {
		recipient := identity.recipient()
		if len(wanted) > 0 {
			if _, ok := wanted[recipient.ID]; !ok {
				continue
			}
		}
		recipients = append(recipients, recipient)
		if identity.claveEnFichero() {
			enFichero = append(enFichero, identity)
		}
	}
	imported, err := k.loadImportedRecipients(ctx, wanted)
	if err != nil {
		return nil, nil, err
	}
	recipients = append(recipients, imported...)
	if len(wanted) > 0 {
		missing := make(map[string]struct{}, len(wanted))
		for id := range wanted {
			missing[id] = struct{}{}
		}
		for _, recipient := range recipients {
			delete(missing, recipient.ID)
		}
		if k.hasLegacyRecipientFile(missing, publicIdentityPaths(identities)) {
			if err := excepciones.Check("alto_clave_fichero_leer", ctx); err != nil {
				return nil, nil, err
			}
		}
	}
	return dedupeRecipients(recipients), enFichero, nil
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
	known, err := k.readIdentities(ctx)
	if err != nil {
		return nil, err
	}
	migrateErr := k.migrateLegacyIdentities(ctx, publicIdentityPaths(known))
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	identities, err := k.readAllIdentities(ctx)
	if err != nil {
		return nil, err
	}
	if len(identities) == 0 {
		if migrateErr != nil {
			// Hay identidades antiguas que no se pudieron leer ni migrar: no se
			// crea otra encima.
			return nil, migrateErr
		}
		if err := k.createDefaultIdentity(ctx); err != nil {
			return nil, err
		}
		if identities, err = k.readAllIdentities(ctx); err != nil {
			return nil, err
		}
	}
	return identities, nil
}

// readAllIdentities une las identidades v2 con las v1 que siguen en disco
// tras intentar migrarlas (almacén ausente o que falló). Si una identidad está
// en los dos formatos manda la v2 y el fichero v1 queda como respaldo.
func (k *LocalStrongKeyring) readAllIdentities(ctx context.Context) ([]strongIdentity, error) {
	identities, err := k.readIdentities(ctx)
	if err != nil {
		return nil, err
	}
	legacy, err := k.readLegacyIdentities(ctx, publicIdentityPaths(identities))
	if err != nil {
		var block *excepciones.Block
		if len(identities) > 0 && errors.As(err, &block) && block.ID == "alto_clave_fichero_leer" {
			return identities, nil
		}
		return nil, err
	}
	if len(legacy) == 0 {
		return identities, nil
	}
	index := make(map[string]int, len(identities))
	for i, identity := range identities {
		index[identity.id] = i
	}
	for _, identity := range legacy {
		if i, ok := index[identity.id]; ok {
			if identities[i].legacyPath == "" {
				identities[i].legacyPath = identity.legacyPath
			}
			continue
		}
		index[identity.id] = len(identities)
		identities = append(identities, identity)
	}
	sortStrongIdentities(identities)
	return identities, nil
}

func publicIdentityPaths(identities []strongIdentity) map[string]bool {
	paths := make(map[string]bool, len(identities))
	for _, identity := range identities {
		if identity.sourcePath != "" {
			paths[identity.sourcePath] = true
		}
	}
	return paths
}

// readLegacyIdentities lee la parte pública de las identidades v1 que quedan
// en disco, derivándola de su clave privada, que no se guarda en memoria.
func (k *LocalStrongKeyring) readLegacyIdentities(ctx context.Context, skip map[string]bool) ([]strongIdentity, error) {
	var out []strongIdentity
	seen := map[string]struct{}{}
	for _, dir := range k.legacyDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if skip[path] {
				continue
			}
			if err := excepciones.Require(ctx, "alto_clave_fichero_leer"); err != nil {
				return nil, err
			}
			identity, err := legacyIdentityPublic(path)
			if err != nil {
				continue
			}
			if _, ok := seen[identity.id]; ok {
				continue
			}
			seen[identity.id] = struct{}{}
			out = append(out, identity)
		}
	}
	return out, nil
}

func legacyIdentityPublic(path string) (strongIdentity, error) {
	legacy, ok := readLegacyIdentityFile(path)
	if !ok {
		return strongIdentity{}, errors.New("identidad fuerte v1 no válida")
	}
	defer zeroBytes(legacy.mlkemSeed)
	defer zeroBytes(legacy.x25519Key)
	identity, err := strongIdentityFromPrivate(legacy.label, legacy.mlkemSeed, legacy.x25519Key)
	if err != nil {
		return strongIdentity{}, err
	}
	identity.legacyPath = path
	return identity, nil
}

func sortStrongIdentities(out []strongIdentity) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].label == out[j].label {
			return out[i].id < out[j].id
		}
		return out[i].label < out[j].label
	})
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
		identity.enAlmacen = true
		identity.sourcePath = filepath.Join(k.dir(), entry.Name())
		out = append(out, identity)
	}
	sortStrongIdentities(out)
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
	label := defaultStrongIdentityLabel()
	mlkemSeed, x25519Key := mlkemPriv.Bytes(), x25519Priv.Bytes()
	defer zeroBytes(mlkemSeed)
	defer zeroBytes(x25519Key)
	err = k.persistIdentity(ctx, label, mlkemSeed, x25519Key)
	if err == nil || !errors.Is(err, ErrAlmacenSeguroNoDisponible) || ctx.Err() != nil {
		return err
	}
	// Sin almacén, ambos ajustes deben estar activos antes de escribir el
	// fichero. La siguiente lectura necesita el ajuste de lectura.
	if err := excepciones.Require(ctx, "alto_clave_fichero_crear"); err != nil {
		return err
	}
	if err := excepciones.Require(ctx, "alto_clave_fichero_leer"); err != nil {
		return err
	}
	id, err := k.persistLegacyIdentity(label, mlkemSeed, x25519Key)
	if err != nil {
		return err
	}
	avisarClaveAltoEnFichero(ctx, "create", id)
	return nil
}

// persistLegacyIdentity escribe la identidad en el formato v1 de siempre
// (clave privada en el fichero, 0600) en ConfigDir/protection/strong (0700).
func (k *LocalStrongKeyring) persistLegacyIdentity(label string, mlkemSeed, x25519Key []byte) (string, error) {
	identity, err := strongIdentityFromPrivate(label, mlkemSeed, x25519Key)
	if err != nil {
		return "", err
	}
	dir := k.legacyDirs()[0]
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creando directorio de protección fuerte: %w", err)
	}
	if err := securefile.ProtectDirectory(dir, 0o700); err != nil {
		return "", fmt.Errorf("protegiendo directorio de protección fuerte: %w", err)
	}
	raw, err := json.MarshalIndent(strongIdentityLegacyFile{
		Version:             strongIdentityLegacyVersion,
		ID:                  identity.id,
		Label:               identity.label,
		MLKEM768SeedB64:     base64.StdEncoding.EncodeToString(mlkemSeed),
		X25519PrivateKeyB64: base64.StdEncoding.EncodeToString(x25519Key),
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("serializando identidad fuerte local: %w", err)
	}
	raw = append(raw, '\n')
	defer zeroBytes(raw)
	if err := securefile.WriteFileAtomic(filepath.Join(dir, identity.id+".json"), raw, 0o600); err != nil {
		return "", fmt.Errorf("persistiendo identidad fuerte local: %w", err)
	}
	return identity.id, nil
}

// avisarClaveAltoEnFichero anota el aviso para la persona en la operación en
// curso y lo deja en el registro, sin datos secretos.
func avisarClaveAltoEnFichero(ctx context.Context, operacion, id string) {
	domain.AnotarAviso(ctx, AvisoProteccionAltoSinAlmacen)
	slog.Warn(eventoClaveAltoEnFichero,
		"operation", operacion,
		"identity_id", id,
		"secret_storage", "unavailable",
		"protection", "file_0600_unencrypted",
		"migrates_to_secret_storage", true)
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
// el fichero antiguo intacto: se sigue usando con aviso y se migra más tarde.
func (k *LocalStrongKeyring) migrateLegacyIdentities(ctx context.Context, skip map[string]bool) error {
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
			if skip[path] {
				continue
			}
			if err := excepciones.Require(ctx, "alto_clave_fichero_leer"); err != nil {
				return err
			}
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

// hasLegacyRecipientFile solo mira nombres de fichero; no abre ni analiza
// ninguna clave v1 mientras la excepción esté desactivada.
func (k *LocalStrongKeyring) hasLegacyRecipientFile(ids map[string]struct{}, publicPaths map[string]bool) bool {
	for id := range ids {
		if len(id) != 16 {
			continue
		}
		if _, err := hex.DecodeString(id); err != nil {
			continue
		}
		for _, dir := range k.legacyDirs() {
			path := filepath.Join(dir, id+".json")
			if publicPaths[path] {
				continue
			}
			if _, err := os.Lstat(path); err == nil {
				return true
			}
		}
	}
	return false
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

// loadPrivateMaterial recupera la clave privada de una identidad y comprueba
// que corresponde a su parte pública. La v2 la lee del almacén del sistema;
// solo si el almacén no responde (no si responde que no la tiene) y queda su
// fichero v1, cae a ese fichero. enFichero dice si la clave salió del fichero.
func (k *LocalStrongKeyring) loadPrivateMaterial(ctx context.Context, identity strongIdentity) (domain.ProtectionKeyMaterial, bool, error) {
	if !identity.enAlmacen {
		if err := excepciones.Require(ctx, "alto_clave_fichero_leer"); err != nil {
			return domain.ProtectionKeyMaterial{}, false, err
		}
		material, err := legacyPrivateMaterial(identity)
		return material, err == nil, err
	}
	var err error
	if k.Secrets == nil {
		err = ErrAlmacenSeguroNoDisponible
	} else {
		var secret []byte
		secret, err = k.Secrets.Load(ctx, strongIdentitySecretKey(identity.id))
		if err == nil {
			material, matErr := privateMaterialFromSecret(identity, secret)
			return material, false, matErr
		}
		if errors.Is(err, keyringstore.ErrNoEncontrado) || ctx.Err() != nil {
			// v2 huérfana: el almacén responde y no la tiene. Sigue como antes.
			return domain.ProtectionKeyMaterial{}, false, err
		}
		err = fmt.Errorf("%w: %v", ErrAlmacenSeguroNoDisponible, err)
	}
	if identity.legacyPath == "" {
		return domain.ProtectionKeyMaterial{}, false, err
	}
	if err := excepciones.Require(ctx, "alto_clave_fichero_leer"); err != nil {
		return domain.ProtectionKeyMaterial{}, false, err
	}
	material, legacyErr := legacyPrivateMaterial(identity)
	if legacyErr != nil {
		return domain.ProtectionKeyMaterial{}, false, errors.Join(err, legacyErr)
	}
	return material, true, nil
}

// legacyPrivateMaterial lee la clave privada del fichero v1 de la identidad.
func legacyPrivateMaterial(identity strongIdentity) (domain.ProtectionKeyMaterial, error) {
	if identity.legacyPath == "" {
		return domain.ProtectionKeyMaterial{}, errors.New("identidad fuerte sin fichero de clave")
	}
	legacy, ok := readLegacyIdentityFile(identity.legacyPath)
	if !ok {
		return domain.ProtectionKeyMaterial{}, errors.New("identidad fuerte v1 no válida")
	}
	return privateMaterialFromKeys(identity, legacy.mlkemSeed, legacy.x25519Key)
}

func privateMaterialFromSecret(identity strongIdentity, secret []byte) (domain.ProtectionKeyMaterial, error) {
	defer zeroBytes(secret)
	if len(secret) != strongIdentitySecretBytes {
		return domain.ProtectionKeyMaterial{}, errors.New("clave privada fuerte con tamaño no válido")
	}
	mlkemSeed := append([]byte(nil), secret[:mlkem.SeedSize]...)
	x25519Key := append([]byte(nil), secret[mlkem.SeedSize:]...)
	return privateMaterialFromKeys(identity, mlkemSeed, x25519Key)
}

// privateMaterialFromKeys se queda con mlkemSeed y x25519Key: los devuelve en
// el material o los borra si no corresponden a la identidad.
func privateMaterialFromKeys(identity strongIdentity, mlkemSeed, x25519Key []byte) (domain.ProtectionKeyMaterial, error) {
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
