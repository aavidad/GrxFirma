//go:build linux

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package portalpaths

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// pinnedDir recorre los componentes desde / mediante descriptores. O_NOFOLLOW
// impide que un enlace sustituido entre Check y la apertura redirija la
// lectura o publicación a la configuración privada.
func pinnedDir(path string, create bool) (int, error) {
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	for _, component := range strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), "/") {
		if component == "" || component == "." {
			continue
		}
		next, err := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if create && errors.Is(err, unix.ENOENT) {
			if mkdirErr := unix.Mkdirat(fd, component, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				unix.Close(fd)
				return -1, mkdirErr
			}
			next, err = unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		}
		unix.Close(fd)
		if err != nil {
			return -1, err
		}
		fd = next
	}
	return fd, nil
}

func readFilePinned(path string, max int64) ([]byte, error) {
	if max <= 0 || max == math.MaxInt64 {
		return nil, ErrProtected
	}
	dirfd, err := pinnedDir(filepath.Dir(path), false)
	if err != nil {
		return nil, err
	}
	defer unix.Close(dirfd)
	fd, err := unix.Openat(dirfd, filepath.Base(path), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > max {
		return nil, ErrProtected
	}
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil || int64(len(data)) > max {
		return nil, ErrProtected
	}
	return data, nil
}

// writeFile publica por un descriptor de directorio fijado. Una carrera que
// reemplace un ancestro por un enlace no altera el destino abierto; sin
// confirmación se usa linkat para publicar solo si el nombre está libre.
func writeFile(path string, data []byte, force bool, check func(string) (string, error)) (string, error) {
	if len(data) == 0 {
		return "", ErrProtected
	}
	if hasSymlinkComponent(path) {
		return "", ErrProtected
	}
	physical, err := check(path)
	if err != nil {
		return "", err
	}
	dirfd, err := pinnedDir(filepath.Dir(physical), true)
	if err != nil {
		return "", err
	}
	defer unix.Close(dirfd)

	base := filepath.Base(physical)
	mode := uint32(0o600)
	var current unix.Stat_t
	if err := unix.Fstatat(dirfd, base, &current, unix.AT_SYMLINK_NOFOLLOW); err == nil {
		if current.Mode&unix.S_IFMT != unix.S_IFREG {
			return "", ErrProtected
		}
		if force {
			mode = current.Mode & 0o777
		}
	} else if !errors.Is(err, unix.ENOENT) {
		return "", err
	}

	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	temp := "." + base + ".tmp-" + hex.EncodeToString(token[:])
	fd, err := unix.Openat(dirfd, temp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, mode)
	if err != nil {
		return "", err
	}
	defer unix.Unlinkat(dirfd, temp, 0)
	f := os.NewFile(uintptr(fd), temp)
	if err := f.Chmod(os.FileMode(mode)); err != nil {
		f.Close()
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if force {
		if err := unix.Renameat(dirfd, temp, dirfd, base); err != nil {
			return "", err
		}
		return filepath.Join(filepath.Dir(physical), base), nil
	}
	for i := 0; i < 1000; i++ {
		candidate := alternativeName(base, i)
		err := unix.Linkat(dirfd, temp, dirfd, candidate, 0)
		if err == nil {
			return filepath.Join(filepath.Dir(physical), candidate), nil
		}
		if !errors.Is(err, unix.EEXIST) {
			return "", err
		}
	}
	return "", fmt.Errorf("portalpaths: sin nombre libre para %s", base)
}

func hasSymlinkComponent(path string) bool {
	current := string(filepath.Separator)
	relative := strings.TrimPrefix(filepath.Clean(path), current)
	if home, err := os.UserHomeDir(); err == nil && within(home, path) {
		if rel, err := filepath.Rel(home, path); err == nil {
			current, relative = home, rel
		}
	}
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return false
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return true
		}
	}
	return false
}

func alternativeName(name string, attempt int) string {
	if attempt == 0 {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	start := 1
	if len(base) >= 4 && base[len(base)-4] == '_' {
		if n, err := strconv.Atoi(base[len(base)-3:]); err == nil {
			base, start = base[:len(base)-4], n+1
		}
	}
	return fmt.Sprintf("%s_%03d%s", base, start+attempt-1, ext)
}
