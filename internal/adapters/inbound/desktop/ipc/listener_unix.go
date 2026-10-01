// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package ipc

import (
	"errors"
	"net"
	"os"
)

func escucharIPC(socketPath string) (net.Listener, error) {
	// Eliminar solo la entrada concreta que pudo dejar una ejecucion anterior.
	if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, errors.Join(errors.New("limpiando socket IPC anterior"), err)
	}

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = ln.Close()
		_ = os.Remove(socketPath)
		return nil, errors.Join(errors.New("restringiendo permisos del socket"), err)
	}
	return ln, nil
}

func limpiarIPC(socketPath string) error {
	err := os.Remove(socketPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func socketPathPorDefecto() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return dir + "/grxfirma_ipc.sock"
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		return home + "/.local/run/grxfirma_ipc.sock"
	}
	return "/tmp/grxfirma_ipc.sock"
}
