// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package localtlstrust

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"grxfirma/internal/adapters/outbound/common/securefile"
)

const maxManagedCAKeyBytes = 16 * 1024

var ErrManagedCAKeyLegacy = errors.New("localtlstrust: clave CA anterior sin protección DPAPI")

// SaveManagedCAKey persiste la clave de la CA en el directorio privado TLS.
// En Windows la cifra DPAPI para el usuario actual antes de escribirla.
func SaveManagedCAKey(path string, keyDER []byte) error {
	if strings.TrimSpace(path) == "" || filepath.Dir(path) == "." {
		return errors.New("localtlstrust: ruta de clave CA sin directorio propio")
	}
	if len(keyDER) == 0 || len(keyDER) > maxManagedCAKeyBytes {
		return errors.New("localtlstrust: tamaño de clave CA inválido")
	}
	if err := securefile.ProtectDirectory(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("localtlstrust: proteger directorio de clave CA: %w", err)
	}
	protected, err := protectManagedCAKey(keyDER)
	if err != nil {
		return err
	}
	defer clearManagedCABytes(protected)
	if err := retireManagedCAKeyFile(path); err != nil {
		return err
	}
	if err := securefile.WriteFileAtomic(path, protected, 0o600); err != nil {
		return fmt.Errorf("localtlstrust: guardar clave CA: %w", err)
	}
	if err := securefile.ProtectFile(path, 0o600); err != nil {
		return fmt.Errorf("localtlstrust: proteger fichero de clave CA: %w", err)
	}
	return nil
}

// LoadManagedCAKey devuelve la clave protegida del usuario actual. No
// interpreta ni registra el material: el emisor comprueba su correspondencia
// con el certificado de la CA antes de firmar.
func LoadManagedCAKey(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" || filepath.Dir(path) == "." {
		return nil, errors.New("localtlstrust: ruta de clave CA sin directorio propio")
	}
	if err := securefile.ProtectDirectory(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("localtlstrust: proteger directorio de clave CA: %w", err)
	}
	if err := securefile.ProtectFile(path, os.FileMode(0o600)); err != nil {
		return nil, fmt.Errorf("localtlstrust: proteger fichero de clave CA: %w", err)
	}
	protected, err := securefile.ReadFileLimit(path, maxManagedCAKeyBytes*2)
	if err != nil {
		return nil, err
	}
	defer clearManagedCABytes(protected)
	key, err := unprotectManagedCAKey(protected)
	if err != nil {
		return nil, err
	}
	if len(key) == 0 || len(key) > maxManagedCAKeyBytes {
		clearManagedCABytes(key)
		return nil, errors.New("localtlstrust: tamaño de clave CA inválido")
	}
	migrateManagedCAKeyToStorage(path, protected, key)
	return key, nil
}

// RemoveManagedCAKey retira la clave de una CA local: olvida su clave de
// cifrado en el almacén del sistema y sobrescribe el fichero antes de
// borrarlo. No hace nada si el fichero no existe ni sigue enlaces.
func RemoveManagedCAKey(path string) error {
	if strings.TrimSpace(path) == "" || filepath.Dir(path) == "." {
		return errors.New("localtlstrust: ruta de clave CA sin directorio propio")
	}
	return retireManagedCAKeyFile(path)
}

// retireManagedCAKeyFile retira una clave de CA anterior: olvida su clave de
// cifrado en el almacén del sistema y sobrescribe el fichero antes de
// borrarlo, por si era una clave anterior guardada sin cifrar.
func retireManagedCAKeyFile(path string) error {
	previous, err := securefile.ReadFileLimit(path, maxManagedCAKeyBytes*2)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("localtlstrust: leer clave CA anterior: %w", err)
	}
	forgetManagedCAKeyProtection(previous)
	clearManagedCABytes(previous)
	if err := securefile.RemoveSensitiveFile(path); err != nil {
		return fmt.Errorf("localtlstrust: retirar clave CA anterior: %w", err)
	}
	return nil
}

func clearManagedCABytes(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
