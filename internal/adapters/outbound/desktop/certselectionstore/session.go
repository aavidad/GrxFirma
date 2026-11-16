// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certselectionstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/appdirs"
)

// ErrSessionIDNoDisponible indica que no se sabe a qué arranque o sesión
// pertenece el proceso: sin ese dato no se guardan preferencias de sesión.
var ErrSessionIDNoDisponible = errors.New("certselectionstore: identificador de arranque o de sesión no disponible")

const sessionFilePrefix = "preferred-certificates-session"

// sessionIDActual se sustituye en las pruebas para simular un reinicio.
var sessionIDActual = sessionID

// SessionPath devuelve el fichero de las preferencias que solo valen durante
// la sesión: <carpeta privada de ejecución>/grxfirma/
// preferred-certificates-session-<id>.json. El id sale del arranque (Linux,
// macOS) o de la sesión de inicio y el arranque (Windows), así que tras
// reiniciar o cerrar sesión el nombre cambia y el fichero anterior se ignora,
// aunque la carpeta de reserva (~/.local/run) sobreviva. Un fichero sin id,
// como el de versiones anteriores, tampoco se lee. Nunca está en la carpeta
// temporal compartida; ver appdirs.RuntimeUsuario.
func SessionPath() (string, error) {
	dir, err := appdirs.RuntimeUsuario()
	if err != nil {
		return "", err
	}
	id, err := sessionIDActual()
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrSessionIDNoDisponible, err)
	}
	sum := sha256.Sum256([]byte(id))
	return filepath.Join(dir, "grxfirma", sessionFilePrefix+"-"+hex.EncodeToString(sum[:8])+".json"), nil
}

// sessionDirs son la carpeta de ejecución y su subcarpeta grxfirma. Las dos
// tienen que ser del usuario y privadas: así nadie más puede cambiar una por
// otra entre la comprobación y el uso por ruta.
func sessionDirs(path string) (base, dir string) {
	dir = filepath.Dir(path)
	return filepath.Dir(dir), dir
}

// sessionDirsPrivate dice si las dos carpetas existen y son privadas. Si
// falta alguna o no es privada, no hay preferencias que leer.
func sessionDirsPrivate(path string) (bool, error) {
	base, dir := sessionDirs(path)
	for _, d := range []string{base, dir} {
		if err := securefile.CheckPrivateDir(d); err != nil {
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, securefile.ErrNotPrivateDir) {
				return false, nil
			}
			return false, err
		}
	}
	return true, nil
}

// ReadSessionFile lee las preferencias de sesión de path. Con path vacío, con
// alguna de sus carpetas ausente o no privada, o sin fichero, devuelve un mapa
// vacío: falla cerrado y la persona vuelve a elegir.
func ReadSessionFile(path string) (map[string]string, error) {
	raw := map[string]string{}
	if path == "" {
		return raw, nil
	}
	ok, err := sessionDirsPrivate(path)
	if err != nil || !ok {
		return raw, err
	}
	data, err := securefile.ReadFileLimit(path, maxPreferenceFileBytes)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return raw, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return raw, nil
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// WriteSessionFile guarda las preferencias de sesión en path, después de
// dejar privadas sus dos carpetas. Con datos vacíos borra el fichero. Borra
// también los ficheros de sesiones anteriores.
func WriteSessionFile(path string, datos map[string]string) error {
	if path == "" {
		if len(datos) == 0 {
			return nil
		}
		return ErrSessionIDNoDisponible
	}
	base, dir := sessionDirs(path)
	for _, d := range []string{base, dir} {
		if err := securefile.EnsurePrivateDir(d); err != nil {
			return err
		}
	}
	removeStaleSessionFiles(dir, filepath.Base(path))
	if len(datos) == 0 {
		if err := securefile.RemoveFile(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	data, err := json.MarshalIndent(datos, "", "  ")
	if err != nil {
		return err
	}
	return securefile.WriteFileAtomic(path, data, 0o600)
}

// RemoveSessionFile borra el fichero de esta sesión y los de sesiones
// anteriores. Dice si existía el de esta sesión. Si las carpetas no son
// privadas no borra nada: ese contenido ya se ignora.
func RemoveSessionFile(path string) (bool, error) {
	if path == "" {
		return false, nil
	}
	ok, err := sessionDirsPrivate(path)
	if err != nil || !ok {
		return false, err
	}
	_, dir := sessionDirs(path)
	removeStaleSessionFiles(dir, filepath.Base(path))
	if err := securefile.RemoveFile(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// removeStaleSessionFiles borra, sin seguir enlaces, los ficheros de
// preferencias de sesión que no son el actual. Es limpieza: un fallo no
// impide guardar.
func removeStaleSessionFiles(dir, current string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if name == current || !strings.HasPrefix(name, sessionFilePrefix) || !strings.HasSuffix(name, ".json") || !e.Type().IsRegular() {
			continue
		}
		_ = securefile.RemoveFile(filepath.Join(dir, name))
	}
}
