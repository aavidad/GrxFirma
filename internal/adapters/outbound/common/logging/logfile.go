// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package logging

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func prepararRutaLog(ruta string) (string, error) {
	if strings.IndexByte(ruta, 0) >= 0 {
		return "", errors.New("la ruta del log contiene un caracter nulo")
	}
	absoluta, err := filepath.Abs(ruta)
	if err != nil {
		return "", fmt.Errorf("no se pudo resolver la ruta del log: %w", err)
	}
	absoluta = filepath.Clean(absoluta)
	if err := prepararDirectorioLog(filepath.Dir(absoluta)); err != nil {
		return "", err
	}
	if err := validarDestinoLog(absoluta); err != nil {
		return "", err
	}
	return absoluta, nil
}

func prepararDirectorioLog(directorio string) error {
	directorio = canonicalizarRaizLogConocida(directorio)
	volumen := filepath.VolumeName(directorio)
	raiz := volumen + string(os.PathSeparator)
	relativo, err := filepath.Rel(raiz, directorio)
	if err != nil {
		return fmt.Errorf("no se pudo resolver el directorio del log: %w", err)
	}

	actual := raiz
	for _, componente := range strings.Split(relativo, string(os.PathSeparator)) {
		if componente == "" || componente == "." {
			continue
		}
		actual = filepath.Join(actual, componente)
		info, statErr := os.Lstat(actual)
		if errors.Is(statErr, os.ErrNotExist) {
			// actual is absolute, every parent was Lstat-checked as a real
			// directory, and only this single component is created.
			if mkdirErr := os.Mkdir(actual, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, os.ErrExist) {
				return fmt.Errorf("no se pudo crear el directorio del log: %w", mkdirErr)
			}
			info, statErr = os.Lstat(actual)
		}
		if statErr != nil {
			return fmt.Errorf("no se pudo comprobar el directorio del log: %w", statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("el directorio del log contiene un enlace simbolico: %s", actual)
		}
		if !info.IsDir() {
			return fmt.Errorf("un componente de la ruta del log no es un directorio: %s", actual)
		}
	}
	return nil
}

// canonicalizarRaizLogConocida admite los alias que forman parte de una raiz
// de escritura elegida por el proceso. El sufijo no se resuelve: el recorrido
// Lstat posterior sigue rechazando cualquier enlace introducido dentro del
// temporal o del home. En macOS /tmp es tambien un alias del sistema y forma
// parte de las rutas de depuracion documentadas.
func canonicalizarRaizLogConocida(ruta string) string {
	raices := []string{os.TempDir()}
	if home, err := os.UserHomeDir(); err == nil {
		raices = append(raices, home)
	}
	if runtime.GOOS == "darwin" {
		raices = append(raices, "/tmp")
	}

	mejorRuta := ruta
	mejorLongitud := -1
	for _, raiz := range raices {
		raizAbsoluta, err := filepath.Abs(raiz)
		if err != nil {
			continue
		}
		relativa, err := filepath.Rel(raizAbsoluta, ruta)
		if err != nil || relativaLogSaleDeRaiz(relativa) {
			continue
		}
		raizCanonica, err := filepath.EvalSymlinks(raizAbsoluta)
		if err != nil || raizCanonica == raizAbsoluta {
			continue
		}
		if len(raizAbsoluta) <= mejorLongitud {
			continue
		}
		mejorRuta = filepath.Join(raizCanonica, relativa)
		mejorLongitud = len(raizAbsoluta)
	}
	return mejorRuta
}

func relativaLogSaleDeRaiz(relativa string) bool {
	return relativa == ".." ||
		strings.HasPrefix(relativa, ".."+string(os.PathSeparator)) ||
		filepath.IsAbs(relativa)
}

func validarDestinoLog(ruta string) error {
	info, err := os.Lstat(ruta)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("no se pudo comprobar el fichero de log: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("el fichero de log no puede ser un enlace simbolico: %s", ruta)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("la ruta del log no es un fichero regular: %s", ruta)
	}
	return nil
}

func validarLogAbierto(fichero *os.File, ruta string) error {
	info, err := fichero.Stat()
	if err != nil {
		return fmt.Errorf("no se pudo comprobar el fichero de log abierto: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("la ruta del log no es un fichero regular: %s", ruta)
	}
	return nil
}
