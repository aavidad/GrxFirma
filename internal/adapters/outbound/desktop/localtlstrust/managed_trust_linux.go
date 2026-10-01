// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package localtlstrust

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"grxfirma/internal/adapters/outbound/common/securefile"
)

const managedNSSTrustInventorySuffix = ".grxfirma-nss-trust.json"
const maxManagedNSSTrustEntries = 256
const maxManagedNSSTrustInventoryBytes = 256 * 1024

type managedNSSTrustEntry struct {
	DB          string `json:"db"`
	Nickname    string `json:"nickname"`
	Fingerprint string `json:"fingerprint"`
}

type managedNSSTrustInventory struct {
	Version  int                    `json:"version"`
	Owner    string                 `json:"owner"`
	Entries  []managedNSSTrustEntry `json:"entries"`
	Revision uint64                 `json:"revision,omitempty"`
}

func ensureManagedTrustedPlatform(ctx context.Context, certFile, validatedCertFile string, cert *x509.Certificate) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("localtlstrust: localizar carpeta del usuario: %w", err)
	}
	paths, discoveryErr := linuxNSSPaths(home)
	installer := newInstaladorNSS()
	installer.rutas = paths
	installer.initMissingDB = true
	installer.inspectLegacy = true
	return errors.Join(installer.ensureManagedTrusted(ctx, certFile, validatedCertFile, cert), discoveryErr)
}

func (i *instaladorNSS) ensureManagedTrusted(ctx context.Context, certFile, validatedCertFile string, cert *x509.Certificate) (result error) {
	managedTrustMu.Lock()
	defer managedTrustMu.Unlock()
	unlock, err := acquireManagedTrustLock(certFile + ".grxfirma-trust.lock")
	if err != nil {
		return fmt.Errorf("localtlstrust: bloquear inventario NSS: %w", err)
	}
	defer unlock()
	defer func() {
		if errors.Is(result, exec.ErrNotFound) || errors.Is(result, os.ErrNotExist) {
			result = ErrHerramientaNoDisponible
		}
	}()
	path := certFile + managedNSSTrustInventorySuffix
	inventory, err := loadManagedNSSTrustInventory(path)
	if err != nil {
		return err
	}
	originalRevision := inventory.Revision
	home, _ := os.UserHomeDir()
	current := fingerprintSHA256(cert)
	nickname := managedNSSNickname(current)
	allowed := make(map[string]bool, len(i.rutas))
	for _, db := range i.rutas {
		allowed[db] = true
	}
	var added []managedNSSTrustEntry
	var operationErrors []error
	ready := make(map[string]bool, len(i.rutas))
	for _, db := range i.rutas {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !esDirectorio(db) {
			continue
		}
		if err := validateNSSStore(db); err != nil {
			operationErrors = append(operationErrors, fmt.Errorf("%s: %w", db, err))
			continue
		}
		if i.initMissingDB {
			if _, err := os.Lstat(filepath.Join(db, "cert9.db")); errors.Is(err, os.ErrNotExist) {
				if err := i.ejecutarManaged(ctx, "-N", "-d", "sql:"+db, "--empty-password"); err != nil {
					operationErrors = append(operationErrors, fmt.Errorf("%s: inicializar NSS: %w", db, err))
					continue
				}
			}
		}
		if i.inspectLegacy {
			legacy, trust, err := i.legacyNSSCertificate(ctx, db)
			if err != nil {
				operationErrors = append(operationErrors, fmt.Errorf("%s: %w", db, err))
				continue
			}
			if legacy != nil && fingerprintSHA256(legacy) == current && IsManagedLocalCA(legacy) {
				switch {
				case trust == trustNSSWebTLSRoot:
					// La misma CA ya existía: no apropiarse de su alias fijo.
					ready[db] = true
					continue
				case db == filepath.Join(home, ".pki", "nssdb") && trust == "CT,C,C":
					if err := i.ejecutarManaged(ctx, "-M", "-d", db, "-n", nicknameLocalhostRoot, "-t", trustNSSWebTLSRoot); err != nil {
						operationErrors = append(operationErrors, err)
						continue
					}
					inventory.Revision++
					ready[db] = true
					continue
				default:
					operationErrors = append(operationErrors, fmt.Errorf("localtlstrust: confianza NSS %q de CA existente no se modifica automáticamente en %s", trust, db))
					continue
				}
			}
		}
		existing, err := i.certificadoPorNickname(ctx, db, nickname)
		if err != nil {
			operationErrors = append(operationErrors, err)
			continue
		}
		if existing != nil && fingerprintSHA256(existing) != current {
			operationErrors = append(operationErrors, fmt.Errorf("localtlstrust: alias NSS gestionado ocupado en %s", db))
			continue
		}
		if existing != nil && certVigente(existing, time.Now()) {
			ready[db] = true
			continue
		}
		// El alias contiene la huella completa, así que nunca hay que borrar
		// un alias fijo o sustituir una CA que quizá añadió otro programa.
		if err := i.ejecutarManaged(ctx, "-A", "-d", db, "-n", nickname,
			"-t", trustNSSWebTLSRoot, "-a", "-i", validatedCertFile); err != nil {
			operationErrors = append(operationErrors, err)
			continue
		}
		entry := managedNSSTrustEntry{DB: db, Nickname: nickname, Fingerprint: current}
		if !containsManagedNSSEntry(inventory.Entries, entry) {
			inventory.Entries = append(inventory.Entries, entry)
		}
		added = append(added, entry)
		ready[db] = true
	}
	if len(added) != 0 || inventory.Revision != originalRevision {
		if err := writeManagedNSSTrustInventory(path, inventory); err != nil {
			// Una inserción no inventariada no debe quedarse instalada.
			for _, entry := range added {
				if ok, checkErr := i.managedNSSEntryMatches(ctx, entry); checkErr == nil && ok {
					_ = i.ejecutarManaged(ctx, "-D", "-d", entry.DB, "-n", entry.Nickname)
				}
			}
			return err
		}
	}
	remaining := make([]managedNSSTrustEntry, 0, len(inventory.Entries))
	for _, entry := range inventory.Entries {
		if !ready[entry.DB] {
			remaining = append(remaining, entry)
			continue
		}
		if !allowed[entry.DB] {
			remaining = append(remaining, entry)
			continue
		}
		if entry.Fingerprint == current {
			remaining = append(remaining, entry)
			continue
		}
		matches, err := i.managedNSSEntryMatches(ctx, entry)
		if err != nil {
			remaining = append(remaining, entry)
			operationErrors = append(operationErrors, err)
			continue
		}
		if !matches {
			continue
		}
		if err := i.ejecutarManaged(ctx, "-D", "-d", entry.DB, "-n", entry.Nickname); err != nil {
			remaining = append(remaining, entry)
			operationErrors = append(operationErrors, err)
		}
	}
	// El alias fijo anterior carecía de inventario. Se conserva incluso si
	// muestra el marcador de GrxFirma: pudo importarlo otra aplicación o el
	// usuario, y el marcador no prueba quién realizó la instalación NSS.
	inventory.Entries = remaining
	if err := writeManagedNSSTrustInventory(path, inventory); err != nil {
		return err
	}
	if len(operationErrors) != 0 {
		return fmt.Errorf("localtlstrust: no se pudo instalar la CA local en todos los navegadores; cierre Firefox, vuelva a abrir GrxFirma y después reinicie Firefox: %w", errors.Join(operationErrors...))
	}
	return nil
}

func (i *instaladorNSS) managedNSSEntryMatches(ctx context.Context, entry managedNSSTrustEntry) (bool, error) {
	if entry.Nickname != managedNSSNickname(entry.Fingerprint) {
		return false, nil
	}
	cert, err := i.certificadoPorNickname(ctx, entry.DB, entry.Nickname)
	if err != nil || cert == nil {
		return false, err
	}
	return IsManagedLocalCA(cert) && fingerprintSHA256(cert) == entry.Fingerprint, nil
}

func (i *instaladorNSS) ejecutarManaged(ctx context.Context, args ...string) error {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		last = i.ejecutar(ctx, args...)
		if last == nil || errors.Is(last, exec.ErrNotFound) {
			return last
		}
		if attempt < 2 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(250 * time.Millisecond):
			}
		}
	}
	return last
}

func (i *instaladorNSS) legacyNSSCertificate(ctx context.Context, db string) (*x509.Certificate, string, error) {
	cert, err := i.certificadoPorNickname(ctx, db, nicknameLocalhostRoot)
	if err != nil || cert == nil {
		return cert, "", err
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// #nosec G204 -- certutil es una herramienta fija; la ruta NSS ya se validó.
	out, err := exec.CommandContext(callCtx, i.certutil, "-L", "-d", db).Output()
	if err != nil {
		return nil, "", fmt.Errorf("localtlstrust: consultar confianza NSS previa: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimLeft(line, " \t")
		if !strings.HasPrefix(line, nicknameLocalhostRoot) {
			continue
		}
		rest := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, nicknameLocalhostRoot)))
		if len(rest) > 0 && strings.Count(rest[0], ",") == 2 {
			return cert, rest[0], nil
		}
	}
	return nil, "", errors.New("localtlstrust: confianza NSS previa no reconocida")
}

func managedNSSNickname(fingerprint string) string {
	return nicknameLocalhostRoot + " " + fingerprint
}

func containsManagedNSSEntry(entries []managedNSSTrustEntry, expected managedNSSTrustEntry) bool {
	for _, entry := range entries {
		if entry == expected {
			return true
		}
	}
	return false
}

func loadManagedNSSTrustInventory(path string) (managedNSSTrustInventory, error) {
	inventory := managedNSSTrustInventory{Version: 1, Owner: managedTrustOwner}
	raw, err := securefile.ReadFileLimit(path, maxManagedNSSTrustInventoryBytes)
	if errors.Is(err, os.ErrNotExist) {
		return inventory, nil
	}
	if err != nil {
		return inventory, fmt.Errorf("localtlstrust: leer inventario NSS: %w", err)
	}
	if err := securefile.ProtectFile(path, 0o600); err != nil {
		return inventory, fmt.Errorf("localtlstrust: proteger inventario NSS: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&inventory); err != nil {
		return inventory, fmt.Errorf("localtlstrust: inventario NSS inválido: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return inventory, errors.New("localtlstrust: datos adicionales en inventario NSS")
	}
	if err := inventory.validate(); err != nil {
		return inventory, err
	}
	return inventory, nil
}

func writeManagedNSSTrustInventory(path string, inventory managedNSSTrustInventory) error {
	if err := inventory.validate(); err != nil {
		return err
	}
	sort.Slice(inventory.Entries, func(a, b int) bool {
		if inventory.Entries[a].DB == inventory.Entries[b].DB {
			return inventory.Entries[a].Nickname < inventory.Entries[b].Nickname
		}
		return inventory.Entries[a].DB < inventory.Entries[b].DB
	})
	raw, err := json.Marshal(inventory)
	if err != nil {
		return err
	}
	if len(raw) > maxManagedNSSTrustInventoryBytes {
		return errors.New("localtlstrust: inventario NSS demasiado grande")
	}
	if err := securefile.WriteFileAtomic(path, raw, 0o600); err != nil {
		return fmt.Errorf("localtlstrust: escribir inventario NSS: %w", err)
	}
	return securefile.ProtectFile(path, 0o600)
}

func (inventory managedNSSTrustInventory) validate() error {
	if inventory.Version != 1 || inventory.Owner != managedTrustOwner || len(inventory.Entries) > maxManagedNSSTrustEntries {
		return errors.New("localtlstrust: identidad o tamaño de inventario NSS inválido")
	}
	seen := make(map[string]bool, len(inventory.Entries))
	for _, entry := range inventory.Entries {
		if !filepath.IsAbs(entry.DB) || strings.ContainsRune(entry.DB, '\x00') ||
			!validSHA256Fingerprint(entry.Fingerprint) ||
			entry.Nickname != managedNSSNickname(entry.Fingerprint) ||
			seen[entry.DB+"\x00"+entry.Nickname] {
			return errors.New("localtlstrust: entrada de inventario NSS inválida")
		}
		seen[entry.DB+"\x00"+entry.Nickname] = true
	}
	return nil
}

func removeManagedTrustedPlatform(ctx context.Context, certFile string) error {
	managedTrustMu.Lock()
	defer managedTrustMu.Unlock()
	unlock, err := acquireManagedTrustLock(certFile + ".grxfirma-trust.lock")
	if err != nil {
		return fmt.Errorf("localtlstrust: bloquear inventario NSS: %w", err)
	}
	defer unlock()
	path := certFile + managedNSSTrustInventorySuffix
	inventory, err := loadManagedNSSTrustInventory(path)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	installer := newInstaladorNSS()
	remaining := make([]managedNSSTrustEntry, 0, len(inventory.Entries))
	var failures []error
	for _, entry := range inventory.Entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !allowedLinuxStorePath(home, entry.DB) {
			remaining = append(remaining, entry)
			failures = append(failures, fmt.Errorf("localtlstrust: almacén NSS fuera de la carpeta del usuario: %s", entry.DB))
			continue
		}
		if _, err := os.Lstat(entry.DB); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err := validateNSSStore(entry.DB); err != nil {
			remaining = append(remaining, entry)
			failures = append(failures, fmt.Errorf("%s: %w", entry.DB, err))
			continue
		}
		matches, err := installer.managedNSSEntryMatches(ctx, entry)
		if err != nil {
			remaining = append(remaining, entry)
			failures = append(failures, err)
			continue
		}
		if !matches {
			continue
		}
		if err := installer.ejecutarManaged(ctx, "-D", "-d", entry.DB, "-n", entry.Nickname); err != nil {
			remaining = append(remaining, entry)
			failures = append(failures, err)
		}
	}
	inventory.Entries = remaining
	if err := writeManagedNSSTrustInventory(path, inventory); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

func managedTrustLifecycleSupportedPlatform() bool { return true }

func managedTrustChangeTokenPlatform(certFile string) (string, error) {
	inventory, err := loadManagedNSSTrustInventory(certFile + managedNSSTrustInventorySuffix)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(inventory)
	return string(raw), err
}
