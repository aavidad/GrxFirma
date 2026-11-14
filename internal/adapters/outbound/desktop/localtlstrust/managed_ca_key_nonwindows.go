// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package localtlstrust

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"testing"

	"grxfirma/internal/adapters/outbound/common/keyringstore"
	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/ports"
)

// En Linux y macOS la clave de la CA se cifra con AES-256-GCM y la clave de
// cifrado se guarda en el almacén de secretos del sistema (Secret Service o
// Llavero), como los demás secretos de GrxFirma. En disco solo queda el texto
// cifrado.
//
// Sin almacén seguro:
//   - macOS: no se guarda la clave de la CA (fallo cerrado).
//   - Linux: se guarda sin cifrar en un fichero 0600 dentro del directorio TLS
//     0700, sin seguir enlaces, y se registra una vez el aviso
//     eventoClaveCAEnFichero. Decisión de dirección: muchos escritorios de las
//     sedes no tienen gnome-keyring ni kwallet, y sin la CA no hay canal TLS
//     local para firmar. El daño de leer esa clave queda acotado porque la CA
//     solo vale para servidor TLS en localhost, 127.0.0.0/8 y ::1 (B-01).
//     Cuando el almacén aparece, la clave se migra a él al leerla y el fichero
//     en claro se sobrescribe y se borra.
var managedCAKeyringHeader = []byte("GRXFIRMA-CA-KEYRING-1\x00")

// eventoClaveCAEnFichero identifica en el registro el aviso de que la clave de
// la CA local no está protegida por el almacén de secretos del sistema.
const eventoClaveCAEnFichero = "localtlstrust_ca_key_unprotected_file"

var (
	// managedCAKeyFileFallback permite la clave en fichero sin almacén. Solo
	// Linux; las pruebas lo cambian para comprobar el fallo cerrado de macOS.
	managedCAKeyFileFallback = runtime.GOOS == "linux"
	managedCAKeyFileWarning  sync.Once
)

const (
	managedCAKEKBytes = 32
	managedCAKEKIDLen = 16
	managedCAKEKName  = "tls-ca-local-"
)

var (
	managedCAKeySecretsMu sync.RWMutex
	managedCAKeySecrets   ports.SecureStorage
	managedCAKeyOverride  bool
)

// UseManagedCAKeyStorage sustituye el almacén de secretos usado para proteger
// la clave de la CA. Pensado para pruebas; nil vuelve al comportamiento por
// defecto.
func UseManagedCAKeyStorage(storage ports.SecureStorage) {
	managedCAKeySecretsMu.Lock()
	defer managedCAKeySecretsMu.Unlock()
	managedCAKeySecrets = storage
	managedCAKeyOverride = storage != nil
}

// managedCAKeyStorage devuelve el almacén del sistema. En los binarios de
// prueba sin almacén inyectado devuelve nil y la clave se guarda como antes
// (0600 en un directorio 0700): así las pruebas nunca escriben en el almacén
// real de quien las ejecuta.
func managedCAKeyStorage() ports.SecureStorage {
	managedCAKeySecretsMu.RLock()
	defer managedCAKeySecretsMu.RUnlock()
	if managedCAKeyOverride {
		return managedCAKeySecrets
	}
	if testing.Testing() {
		return nil
	}
	return keyringstore.New()
}

func protectManagedCAKey(key []byte) ([]byte, error) {
	storage := managedCAKeyStorage()
	if storage == nil {
		return append([]byte(nil), key...), nil
	}
	sealed, err := sealManagedCAKey(storage, key)
	if err == nil {
		return sealed, nil
	}
	if !managedCAKeyFileFallback || !errors.Is(err, errManagedCAKeyStorageUnavailable) {
		return nil, err
	}
	warnManagedCAKeyInFile(err)
	return append([]byte(nil), key...), nil
}

var errManagedCAKeyStorageUnavailable = errors.New("localtlstrust: almacén seguro del sistema no disponible para la clave de la CA")

func warnManagedCAKeyInFile(cause error) {
	managedCAKeyFileWarning.Do(func() {
		slog.Warn(eventoClaveCAEnFichero,
			"secret_storage", "unavailable",
			"protection", "file_0600_unencrypted",
			"migrates_to_secret_storage", true,
			"error", cause.Error())
	})
}

// migrateManagedCAKeyToStorage pasa al almacén de secretos una clave guardada
// en claro en Linux cuando el almacén ya está disponible. Si no lo está, deja
// el fichero como estaba. El fichero en claro se sobrescribe antes de borrarlo.
func migrateManagedCAKeyToStorage(path string, stored, key []byte) {
	if !managedCAKeyFileFallback || bytes.HasPrefix(stored, managedCAKeyringHeader) {
		return
	}
	storage := managedCAKeyStorage()
	if storage == nil {
		return
	}
	sealed, err := sealManagedCAKey(storage, key)
	if err != nil {
		if errors.Is(err, errManagedCAKeyStorageUnavailable) {
			warnManagedCAKeyInFile(err)
		}
		return
	}
	defer clearManagedCABytes(sealed)
	if err := securefile.RemoveSensitiveFile(path); err != nil {
		forgetManagedCAKeyProtection(sealed)
		slog.Warn("localtlstrust_ca_key_migration_error", "error", err.Error())
		return
	}
	if err := securefile.WriteFileAtomic(path, sealed, 0o600); err != nil {
		// La clave en claro ya no existe: la CA se rotará en el próximo arranque.
		forgetManagedCAKeyProtection(sealed)
		slog.Warn("localtlstrust_ca_key_migration_error", "error", err.Error())
		return
	}
	slog.Info("localtlstrust_ca_key_migrated_to_secret_storage")
}

func sealManagedCAKey(storage ports.SecureStorage, key []byte) ([]byte, error) {
	kek := make([]byte, managedCAKEKBytes)
	defer clearManagedCABytes(kek)
	id := make([]byte, managedCAKEKIDLen)
	if _, err := rand.Read(kek); err != nil {
		return nil, fmt.Errorf("localtlstrust: generar clave de cifrado de la CA: %w", err)
	}
	if _, err := rand.Read(id); err != nil {
		return nil, fmt.Errorf("localtlstrust: generar identificador de la CA: %w", err)
	}
	if err := storage.Store(context.Background(), managedCAKEKName+hex.EncodeToString(id), kek); err != nil {
		return nil, fmt.Errorf("%w: %w", errManagedCAKeyStorageUnavailable, err)
	}
	name := managedCAKEKName + hex.EncodeToString(id)
	aead, err := newManagedCAKeyAEAD(kek)
	if err != nil {
		_ = storage.Delete(context.Background(), name)
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		_ = storage.Delete(context.Background(), name)
		return nil, fmt.Errorf("localtlstrust: generar nonce de la CA: %w", err)
	}
	prefix := make([]byte, 0, len(managedCAKeyringHeader)+len(id)+len(nonce))
	prefix = append(prefix, managedCAKeyringHeader...)
	prefix = append(prefix, id...)
	prefix = append(prefix, nonce...)
	return aead.Seal(prefix, nonce, key, prefix[:len(managedCAKeyringHeader)+len(id)]), nil
}

func unprotectManagedCAKey(stored []byte) ([]byte, error) {
	if !bytes.HasPrefix(stored, managedCAKeyringHeader) {
		// Sin cifrar: formato anterior (se admite para comprobar la propiedad
		// de la CA y rotarla) o clave guardada en Linux sin almacén, que
		// LoadManagedCAKey migra al almacén en cuanto está disponible.
		return append([]byte(nil), stored...), nil
	}
	id, nonce, ciphertext, err := splitManagedCAKeyring(stored)
	if err != nil {
		return nil, err
	}
	storage := managedCAKeyStorage()
	if storage == nil {
		return nil, errors.New("localtlstrust: almacén seguro del sistema no configurado para la clave de la CA")
	}
	kek, err := storage.Load(context.Background(), managedCAKEKName+hex.EncodeToString(id))
	if err != nil {
		return nil, fmt.Errorf("localtlstrust: almacén seguro del sistema no disponible para la clave de la CA: %w", err)
	}
	defer clearManagedCABytes(kek)
	aead, err := newManagedCAKeyAEAD(kek)
	if err != nil {
		return nil, err
	}
	key, err := aead.Open(nil, nonce, ciphertext, stored[:len(managedCAKeyringHeader)+managedCAKEKIDLen])
	if err != nil {
		return nil, errors.New("localtlstrust: la clave de la CA no se pudo descifrar")
	}
	return key, nil
}

// forgetManagedCAKeyProtection borra del almacén del sistema la clave de
// cifrado de una clave de CA que se va a sustituir o retirar.
func forgetManagedCAKeyProtection(stored []byte) {
	if !bytes.HasPrefix(stored, managedCAKeyringHeader) {
		return
	}
	id, _, _, err := splitManagedCAKeyring(stored)
	if err != nil {
		return
	}
	if storage := managedCAKeyStorage(); storage != nil {
		_ = storage.Delete(context.Background(), managedCAKEKName+hex.EncodeToString(id))
	}
}

func splitManagedCAKeyring(stored []byte) (id, nonce, ciphertext []byte, err error) {
	rest := stored[len(managedCAKeyringHeader):]
	const nonceLen = 12
	if len(rest) <= managedCAKEKIDLen+nonceLen {
		return nil, nil, nil, errors.New("localtlstrust: clave de CA cifrada incompleta")
	}
	return rest[:managedCAKEKIDLen], rest[managedCAKEKIDLen : managedCAKEKIDLen+nonceLen], rest[managedCAKEKIDLen+nonceLen:], nil
}

func newManagedCAKeyAEAD(kek []byte) (cipher.AEAD, error) {
	if len(kek) != managedCAKEKBytes {
		return nil, errors.New("localtlstrust: clave de cifrado de la CA no válida")
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
