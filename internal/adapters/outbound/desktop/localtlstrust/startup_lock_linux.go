// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package localtlstrust

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const (
	localTLSStartupLockName   = ".local-tls-startup.lock"
	localTLSInventoryLockName = ".local-tls-inventory.lock"
)

// WithLocalTLSStartupLock serializa entre procesos la creación de la CA local
// y su instalación en los almacenes del usuario. No sigue enlaces simbólicos
// en la ruta de configuración ni en el fichero de bloqueo.
func WithLocalTLSStartupLock(ctx context.Context, configDir string, fn func() error) error {
	return withLocalTLSFileLock(ctx, configDir, localTLSStartupLockName, fn)
}

// WithLocalTLSInventoryLock serializa cualquier modificación del inventario
// de propiedad y de los almacenes de confianza asociados a esta CA. Es
// independiente del bloqueo de arranque para incluir también WSS y retirada.
func WithLocalTLSInventoryLock(ctx context.Context, certFile string, fn func() error) error {
	if !filepath.IsAbs(certFile) || certFile != filepath.Clean(certFile) {
		return errors.New("localtlstrust: la ruta de la CA debe ser absoluta y normalizada")
	}
	return withLocalTLSFileLock(ctx, filepath.Dir(certFile), localTLSInventoryLockName, fn)
}

func withLocalTLSFileLock(ctx context.Context, dir, lockName string, fn func() error) error {
	if ctx == nil || fn == nil {
		return errors.New("localtlstrust: contexto o función de bloqueo TLS no configurados")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	dirFD, err := openLocalTLSConfigDir(dir)
	if err != nil {
		return fmt.Errorf("localtlstrust: abrir configuración para bloqueo TLS: %w", err)
	}
	defer unix.Close(dirFD)

	fd, err := unix.Openat(dirFD, lockName,
		unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600)
	if err != nil {
		return fmt.Errorf("localtlstrust: abrir bloqueo TLS: %w", err)
	}
	defer unix.Close(fd)
	var info unix.Stat_t
	if err := unix.Fstat(fd, &info); err != nil {
		return fmt.Errorf("localtlstrust: comprobar bloqueo TLS: %w", err)
	}
	if info.Mode&unix.S_IFMT != unix.S_IFREG || info.Nlink != 1 ||
		info.Uid != uint32(os.Geteuid()) || info.Mode&0o077 != 0 {
		return errors.New("localtlstrust: fichero de bloqueo TLS inseguro")
	}

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			defer unix.Flock(fd, unix.LOCK_UN)
			return fn()
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EINTR) {
			return fmt.Errorf("localtlstrust: adquirir bloqueo TLS: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func openLocalTLSConfigDir(configDir string) (int, error) {
	if !filepath.IsAbs(configDir) || configDir != filepath.Clean(configDir) {
		return -1, errors.New("la ruta de configuración debe ser absoluta y normalizada")
	}
	components := strings.Split(strings.TrimPrefix(configDir, string(filepath.Separator)), string(filepath.Separator))
	if len(components) == 0 || components[0] == "" {
		return -1, errors.New("la ruta de configuración no puede ser la raíz")
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	for _, component := range components {
		next, openErr := unix.Openat(fd, component,
			unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if errors.Is(openErr, unix.ENOENT) {
			if mkdirErr := unix.Mkdirat(fd, component, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				unix.Close(fd)
				return -1, mkdirErr
			}
			next, openErr = unix.Openat(fd, component,
				unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		unix.Close(fd)
		if openErr != nil {
			return -1, openErr
		}
		fd = next
	}
	var info unix.Stat_t
	if err := unix.Fstat(fd, &info); err != nil {
		unix.Close(fd)
		return -1, err
	}
	if info.Uid != uint32(os.Geteuid()) || info.Mode&0o022 != 0 {
		unix.Close(fd)
		return -1, errors.New("directorio de configuración TLS no privado")
	}
	return fd, nil
}
