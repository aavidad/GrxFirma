// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package localtlstrust

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"grxfirma/internal/adapters/outbound/common/securefile"
)

// La reconciliación de inventario y almacén es una sola operación dentro del
// proceso. Evita que dos peticiones paralelas pierdan huellas propias.
var managedTrustMu sync.Mutex

const (
	managedTrustInventoryVersion  = 1
	managedTrustOwner             = "grxfirma-local-tls-ca"
	managedTrustScope             = "CurrentUser"
	managedTrustStore             = "ROOT"
	maxManagedTrustInventoryBytes = 16 * 1024
	maxManagedTrustFingerprints   = 16
)

const pendingManagedTrustRemovalSuffix = ".grxfirma-pending-removal"

// RetireManagedCAKeyWithoutPrompt deja la CA sin capacidad de emitir. Conserva
// el inventario para retirar de ROOT solo las huellas acreditadas más tarde.
func RetireManagedCAKeyWithoutPrompt(certFile string) error {
	if filepath.Base(certFile) != "websocket-localhost-root.crt.pem" {
		return errors.New("localtlstrust: ruta de CA local inesperada")
	}
	certDir := filepath.Dir(certFile)
	if _, err := os.Lstat(certDir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("localtlstrust: inspeccionar directorio TLS: %w", err)
	}
	if err := securefile.ProtectDirectory(certDir, 0o700); err != nil {
		return fmt.Errorf("localtlstrust: proteger directorio TLS: %w", err)
	}
	managedTrustMu.Lock()
	defer managedTrustMu.Unlock()
	unlock, err := acquireManagedTrustLock(certFile + ".grxfirma-trust.lock")
	if err != nil {
		return fmt.Errorf("localtlstrust: bloquear inventario de confianza: %w", err)
	}
	defer unlock()
	marker := certFile + pendingManagedTrustRemovalSuffix
	if err := securefile.WriteFileAtomic(marker, []byte("GrxFirma: retirar CA gestionada en sesión interactiva\n"), 0o600); err != nil {
		return fmt.Errorf("localtlstrust: marcar retirada pendiente: %w", err)
	}
	if err := securefile.ProtectFile(marker, 0o600); err != nil {
		return err
	}
	for _, path := range []string{
		strings.TrimSuffix(certFile, "-root.crt.pem") + "-root.key.pem",
		strings.TrimSuffix(certFile, "-root.crt.pem") + ".key.pem",
		strings.TrimSuffix(certFile, "-root.crt.pem") + ".crt.pem",
	} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("localtlstrust: inspeccionar fichero TLS propio %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("localtlstrust: fichero TLS propio no regular: %s", path)
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("localtlstrust: borrar fichero TLS propio %s: %w", path, err)
		}
	}
	return nil
}

func finishPendingManagedTrustRemoval(ctx context.Context, certFile string, store managedCertificateStore) error {
	marker := certFile + pendingManagedTrustRemovalSuffix
	info, err := os.Lstat(marker)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("localtlstrust: inspeccionar marcador de retirada: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("localtlstrust: marcador de retirada no regular")
	}
	if err := removeManagedTrust(ctx, certFile, store); err != nil {
		return err
	}
	return os.Remove(marker)
}

type managedTrustInventory struct {
	Version       int      `json:"version"`
	Owner         string   `json:"owner"`
	Scope         string   `json:"scope"`
	Store         string   `json:"store"`
	CurrentSHA256 string   `json:"current_sha256"`
	OwnedSHA256   []string `json:"owned_sha256"`
}

type managedCertificateStore interface {
	Contains(ctx context.Context, fingerprint string) (bool, error)
	Add(ctx context.Context, cert *x509.Certificate) (bool, error)
	Remove(ctx context.Context, fingerprint string) (bool, error)
}

func managedTrustInventoryPath(certFile string) string {
	return certFile + managedTrustInventorySuffix
}

func reconcileManagedTrust(
	ctx context.Context,
	certFile string,
	cert *x509.Certificate,
	store managedCertificateStore,
) error {
	if store == nil {
		return errors.New("localtlstrust: almacén gestionado no configurado")
	}
	if cert == nil || !IsManagedLocalCA(cert) {
		return errors.New("localtlstrust: certificado gestionado inválido")
	}
	managedTrustMu.Lock()
	defer managedTrustMu.Unlock()
	unlock, err := acquireManagedTrustLock(certFile + ".grxfirma-trust.lock")
	if err != nil {
		return fmt.Errorf("localtlstrust: bloquear inventario de confianza: %w", err)
	}
	defer unlock()
	inventoryPath := managedTrustInventoryPath(certFile)
	inventory, _, err := loadManagedTrustInventory(inventoryPath)
	if err != nil {
		return err
	}

	current := fingerprintSHA256(cert)
	present, err := store.Contains(ctx, current)
	if err != nil {
		return fmt.Errorf("localtlstrust: consultar CA gestionada: %w", err)
	}
	added := false
	if !present {
		added, err = store.Add(ctx, cert)
		if err != nil {
			return fmt.Errorf("localtlstrust: instalar CA gestionada: %w", err)
		}
		if added {
			inventory.OwnedSHA256 = appendUniqueFingerprint(inventory.OwnedSHA256, current)
		}
	}

	inventory.CurrentSHA256 = current
	inventory.normalize()
	if err := writeManagedTrustInventory(inventoryPath, inventory); err != nil {
		if added {
			if _, rollbackErr := store.Remove(ctx, current); rollbackErr != nil {
				return errors.Join(
					err,
					fmt.Errorf("localtlstrust: rollback de CA sin inventario: %w", rollbackErr),
				)
			}
		}
		return err
	}

	remaining := make([]string, 0, len(inventory.OwnedSHA256))
	var cleanupErrors []error
	for _, fingerprint := range inventory.OwnedSHA256 {
		if fingerprint == current {
			remaining = append(remaining, fingerprint)
			continue
		}
		_, removeErr := store.Remove(ctx, fingerprint)
		if removeErr != nil {
			remaining = append(remaining, fingerprint)
			cleanupErrors = append(cleanupErrors,
				fmt.Errorf("localtlstrust: retirar CA rotada %s: %w", fingerprint, removeErr))
		}
	}
	inventory.OwnedSHA256 = remaining
	if err := writeManagedTrustInventory(inventoryPath, inventory); err != nil {
		cleanupErrors = append(cleanupErrors, err)
	}
	return errors.Join(cleanupErrors...)
}

func removeManagedTrust(
	ctx context.Context,
	certFile string,
	store managedCertificateStore,
) error {
	if store == nil {
		return errors.New("localtlstrust: almacén gestionado no configurado")
	}
	managedTrustMu.Lock()
	defer managedTrustMu.Unlock()
	unlock, err := acquireManagedTrustLock(certFile + ".grxfirma-trust.lock")
	if err != nil {
		return fmt.Errorf("localtlstrust: bloquear inventario de confianza: %w", err)
	}
	defer unlock()
	inventoryPath := managedTrustInventoryPath(certFile)
	inventory, exists, err := loadManagedTrustInventory(inventoryPath)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	remaining := make([]string, 0, len(inventory.OwnedSHA256))
	var cleanupErrors []error
	for _, fingerprint := range inventory.OwnedSHA256 {
		_, removeErr := store.Remove(ctx, fingerprint)
		if removeErr != nil {
			remaining = append(remaining, fingerprint)
			cleanupErrors = append(cleanupErrors,
				fmt.Errorf("localtlstrust: retirar CA propia %s: %w", fingerprint, removeErr))
		}
	}
	if len(remaining) == 0 {
		if err := removeManagedTrustInventory(inventoryPath); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	} else {
		inventory.OwnedSHA256 = remaining
		if err := writeManagedTrustInventory(inventoryPath, inventory); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	return errors.Join(cleanupErrors...)
}

func newManagedTrustInventory() managedTrustInventory {
	return managedTrustInventory{
		Version: managedTrustInventoryVersion,
		Owner:   managedTrustOwner,
		Scope:   managedTrustScope,
		Store:   managedTrustStore,
	}
}

func loadManagedTrustInventory(path string) (managedTrustInventory, bool, error) {
	raw, err := securefile.ReadFileLimit(path, maxManagedTrustInventoryBytes)
	if errors.Is(err, os.ErrNotExist) {
		return newManagedTrustInventory(), false, nil
	}
	if err != nil {
		return managedTrustInventory{}, false,
			fmt.Errorf("localtlstrust: leer inventario de confianza: %w", err)
	}
	if err := securefile.ProtectFile(path, 0o600); err != nil {
		return managedTrustInventory{}, false,
			fmt.Errorf("localtlstrust: proteger inventario de confianza antes de usarlo: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	inventory := managedTrustInventory{}
	if err := decoder.Decode(&inventory); err != nil {
		return managedTrustInventory{}, false,
			fmt.Errorf("localtlstrust: inventario de confianza inválido: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return managedTrustInventory{}, false, err
	}
	if err := inventory.validate(); err != nil {
		return managedTrustInventory{}, false, err
	}
	return inventory, true, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("localtlstrust: inventario contiene datos JSON adicionales")
		}
		return fmt.Errorf("localtlstrust: final de inventario inválido: %w", err)
	}
	return nil
}

func writeManagedTrustInventory(path string, inventory managedTrustInventory) error {
	return writeManagedTrustInventoryWithProtection(
		path,
		inventory,
		securefile.ProtectFile,
	)
}

func writeManagedTrustInventoryWithProtection(
	path string,
	inventory managedTrustInventory,
	protect func(string, os.FileMode) error,
) error {
	inventory.normalize()
	if err := inventory.validate(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return fmt.Errorf("localtlstrust: serializar inventario de confianza: %w", err)
	}
	raw = append(raw, '\n')
	if err := securefile.WriteFileAtomic(path, raw, 0o600); err != nil {
		return fmt.Errorf("localtlstrust: escribir inventario de confianza: %w", err)
	}
	if protect == nil {
		return errors.New("localtlstrust: protector de inventario no configurado")
	}
	if err := protect(path, 0o600); err != nil {
		// Se conserva el inventario para no perder la prueba de propiedad. La
		// operación falla y loadManagedTrustInventory vuelve a protegerlo antes
		// de permitir que sus huellas autoricen cualquier borrado.
		return fmt.Errorf("localtlstrust: proteger inventario de confianza: %w", err)
	}
	return nil
}

func removeManagedTrustInventory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("localtlstrust: inspeccionar inventario de confianza: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("localtlstrust: el inventario de confianza no es un fichero regular")
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("localtlstrust: retirar inventario de confianza: %w", err)
	}
	return nil
}

func (inventory *managedTrustInventory) normalize() {
	if inventory.Version == 0 {
		inventory.Version = managedTrustInventoryVersion
	}
	if inventory.Owner == "" {
		inventory.Owner = managedTrustOwner
	}
	if inventory.Scope == "" {
		inventory.Scope = managedTrustScope
	}
	if inventory.Store == "" {
		inventory.Store = managedTrustStore
	}
	unique := make(map[string]struct{}, len(inventory.OwnedSHA256))
	owned := make([]string, 0, len(inventory.OwnedSHA256))
	for _, fingerprint := range inventory.OwnedSHA256 {
		fingerprint = strings.ToLower(strings.TrimSpace(fingerprint))
		if _, exists := unique[fingerprint]; exists {
			continue
		}
		unique[fingerprint] = struct{}{}
		owned = append(owned, fingerprint)
	}
	sort.Strings(owned)
	inventory.CurrentSHA256 = strings.ToLower(strings.TrimSpace(inventory.CurrentSHA256))
	inventory.OwnedSHA256 = owned
}

func (inventory managedTrustInventory) validate() error {
	if inventory.Version != managedTrustInventoryVersion ||
		inventory.Owner != managedTrustOwner ||
		inventory.Scope != managedTrustScope ||
		inventory.Store != managedTrustStore {
		return errors.New("localtlstrust: identidad del inventario de confianza no válida")
	}
	if !validSHA256Fingerprint(inventory.CurrentSHA256) {
		return errors.New("localtlstrust: huella actual del inventario no válida")
	}
	if len(inventory.OwnedSHA256) > maxManagedTrustFingerprints {
		return errors.New("localtlstrust: demasiadas huellas propias en el inventario")
	}
	for _, fingerprint := range inventory.OwnedSHA256 {
		if !validSHA256Fingerprint(fingerprint) {
			return errors.New("localtlstrust: huella propia del inventario no válida")
		}
	}
	return nil
}

func validSHA256Fingerprint(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func appendUniqueFingerprint(values []string, fingerprint string) []string {
	for _, value := range values {
		if value == fingerprint {
			return values
		}
	}
	return append(values, fingerprint)
}
