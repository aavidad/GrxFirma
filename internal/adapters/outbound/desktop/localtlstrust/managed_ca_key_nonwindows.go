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
	"sync"
	"testing"

	"grxfirma/internal/adapters/outbound/common/keyringstore"
	"grxfirma/internal/ports"
)

// En Linux y macOS la clave de la CA se cifra con AES-256-GCM y la clave de
// cifrado se guarda en el almacén de secretos del sistema (Secret Service o
// Llavero), como los demás secretos de GrxFirma. En disco solo queda el texto
// cifrado. Sin almacén seguro no se guarda la clave de la CA.
var managedCAKeyringHeader = []byte("GRXFIRMA-CA-KEYRING-1\x00")

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
		return nil, fmt.Errorf("localtlstrust: almacén seguro del sistema no disponible para la clave de la CA: %w", err)
	}
	aead, err := newManagedCAKeyAEAD(kek)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
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
		// Formato anterior sin cifrar. Se admite al leer para poder comprobar
		// la propiedad de la CA y rotarla; nunca se vuelve a escribir así
		// fuera de las pruebas.
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
