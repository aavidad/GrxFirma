// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package machinepolicy

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// PolicyFile es la política de máquina en sistemas sin registro. Debe ser
// propiedad de root y no escribible por usuarios.
const PolicyFile = "/etc/grxfirma/policy.json"

func native() bool { return false }

func readStrings(string) ([]string, bool, error) { return nil, false, nil }

func readString(string) (string, bool, error) { return "", false, nil }

func readInt64(string) (int64, bool, error) { return 0, false, nil }

func readOptIn(name string) (bool, bool, error) {
	data, err := readPolicyFile(PolicyFile, 1024*1024)
	if os.IsNotExist(err) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return false, false, err
	}
	value, ok := raw[name]
	if !ok {
		return false, false, nil
	}
	var enabled bool
	if err := json.Unmarshal(value, &enabled); err != nil {
		return false, false, err
	}
	return enabled, true, nil
}

// readPolicyFile lee la política de máquina rechazando ficheros que no sean
// regulares o que un usuario sin privilegios pueda modificar.
func readPolicyFile(path string, limit int64) ([]byte, error) {
	return readPolicyFileFrom(path, "/", 0, limit)
}

// readPolicyFileFrom recorre cada componente desde un directorio ya abierto.
// Así, una sustitución por enlace entre la comprobación y la apertura no
// puede redirigir la lectura hacia una ruta controlada por otro usuario.
func readPolicyFileFrom(path, anchor string, owner uint32, limit int64) ([]byte, error) {
	relative, err := filepath.Rel(anchor, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || filepath.IsAbs(relative) {
		return nil, errors.New("la política de máquina queda fuera del directorio de confianza")
	}
	dir, err := unix.Open(anchor, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer func() { unix.Close(dir) }()
	if err := checkPolicyNode(dir, owner, true); err != nil {
		return nil, err
	}
	parts := strings.Split(relative, string(os.PathSeparator))
	for _, component := range parts[:len(parts)-1] {
		next, err := unix.Openat(dir, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return nil, err
		}
		if err := checkPolicyNode(next, owner, true); err != nil {
			unix.Close(next)
			return nil, err
		}
		unix.Close(dir)
		dir = next
	}
	fd, err := unix.Openat(dir, parts[len(parts)-1], unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	if err := checkPolicyNode(fd, owner, false); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("la política de máquina supera el tamaño máximo")
	}
	return data, nil
}

func checkPolicyNode(fd int, owner uint32, directory bool) error {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if directory && stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return errors.New("un ancestro de la política no es un directorio")
	}
	if !directory && stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return errors.New("la política de máquina no es un fichero regular")
	}
	if stat.Uid != owner {
		return errors.New("la política de máquina o un ancestro no pertenece al propietario de confianza")
	}
	if stat.Mode&0o022 != 0 {
		return errors.New("la política de máquina o un ancestro es modificable por otros usuarios")
	}
	return nil
}
