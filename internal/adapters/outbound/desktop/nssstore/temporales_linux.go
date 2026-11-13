// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package nssstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/appdirs"
)

// prefijoTrabajoNSS nombra cada directorio de trabajo de una exportación.
const prefijoTrabajoNSS = "grxfirma-nss-"

// antiguedadRestos es la edad mínima de un resto para borrarlo: deja en paz
// una exportación que otro proceso tenga en curso.
const antiguedadRestos = 10 * time.Minute

// restosTemporalesAnteriores son los nombres que dejaban en el TMPDIR
// compartido las versiones anteriores (P12 exportado, contraseñas, clon de la
// base NSS e importaciones de certaccess) si el proceso moría.
var restosTemporalesAnteriores = []string{
	"grxfirma-nss-*.p12",
	"grxfirma-openssl-pw-*",
	"grxfirma-nssdb-*",
	"grxfirma-pw-*",
	"grxfirma-import-*.p12",
}

// directorioTrabajoNSS crea el directorio privado de una exportación dentro
// de la caché del usuario (0700, nunca el TMPDIR compartido) y antes retira
// los restos de exportaciones interrumpidas.
func directorioTrabajoNSS() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolver carpeta del usuario: %w", err)
	}
	base := filepath.Join(appdirs.Cache(home), "temporal")
	limpiarRestosTemporales(base, os.TempDir(), time.Now())
	dir, err := securefile.MkdirPrivateTemp(base, prefijoTrabajoNSS+"*")
	if err != nil {
		return "", fmt.Errorf("crear directorio privado de exportación NSS: %w", err)
	}
	return dir, nil
}

// limpiarRestosTemporales borra, sobrescribiéndolos, los directorios de
// trabajo abandonados en base y los restos de versiones anteriores en tmp.
// Solo toca entradas del usuario actual, sin enlaces, y con más de
// antiguedadRestos. Es un mejor esfuerzo: un fallo no impide exportar.
func limpiarRestosTemporales(base, tmp string, ahora time.Time) {
	candidatos, _ := filepath.Glob(filepath.Join(base, prefijoTrabajoNSS+"*"))
	for _, patron := range restosTemporalesAnteriores {
		encontrados, _ := filepath.Glob(filepath.Join(tmp, patron))
		candidatos = append(candidatos, encontrados...)
	}
	for _, ruta := range candidatos {
		if restoPropioAbandonado(ruta, ahora) {
			_ = borrarResto(ruta)
		}
	}
}

func restoPropioAbandonado(ruta string, ahora time.Time) bool {
	info, err := os.Lstat(ruta)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != os.Getuid() {
		return false
	}
	if ahora.Sub(info.ModTime()) < antiguedadRestos {
		return false
	}
	switch {
	case info.Mode().IsRegular():
		// Un enlace duro haría sobrescribir otro fichero del usuario.
		return st.Nlink == 1
	case info.IsDir():
		// Solo directorios privados como los que crea MkdirTemp.
		return info.Mode().Perm()&0o077 == 0
	default:
		return false
	}
}

func borrarResto(ruta string) error {
	info, err := os.Lstat(ruta)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return securefile.RemoveSensitiveTree(ruta)
	}
	return securefile.RemoveSensitiveFile(ruta)
}

// tuberiaSecreta devuelve el extremo de lectura de una tubería que ya
// contiene secreto y está cerrada para escritura. Se hereda en el proceso
// hijo con cmd.ExtraFiles y se nombra como /dev/fd/N o fd:N, así la
// contraseña no pasa por argv ni por un fichero.
func tuberiaSecreta(secreto string) (*os.File, error) {
	// El búfer de una tubería en Linux es de al menos 4 KiB: escribir antes de
	// lanzar el proceso no bloquea.
	if len(secreto) > 4096 {
		return nil, errors.New("contraseña NSS demasiado larga")
	}
	lectura, escritura, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("crear tubería de contraseña: %w", err)
	}
	if _, err := escritura.WriteString(secreto); err != nil {
		_ = lectura.Close()
		_ = escritura.Close()
		return nil, fmt.Errorf("escribir contraseña en tubería: %w", err)
	}
	if err := escritura.Close(); err != nil {
		_ = lectura.Close()
		return nil, fmt.Errorf("cerrar tubería de contraseña: %w", err)
	}
	return lectura, nil
}
