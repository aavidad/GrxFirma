// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package securefile provides final-component no-follow operations for
// application-managed regular files.
package securefile

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// OpenRead opens a regular file without following a symlink/reparse point in
// the final path component. The caller must close the returned file.
func OpenRead(path string) (*os.File, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("securefile: empty path")
	}
	if strings.IndexByte(path, 0) >= 0 {
		return nil, errors.New("securefile: path contains NUL")
	}
	file, err := openReadNoFollow(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("securefile: stat opened file: %w", err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("securefile: path is not a regular file: %s", path)
	}
	return file, nil
}

// OpenAppend opens or creates an application-managed regular file for
// append without following a symlink/reparse point in the final component.
// The returned handle already has private permissions. The caller must close
// it.
func OpenAppend(path string, perm os.FileMode) (*os.File, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("securefile: empty path")
	}
	if strings.IndexByte(path, 0) >= 0 {
		return nil, errors.New("securefile: path contains NUL")
	}
	return openAppendNoFollow(path, perm)
}

// OpenDir opens a real directory without following a symlink/reparse point in
// the final path component. The caller must close the returned handle.
func OpenDir(path string) (*os.File, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("securefile: empty path")
	}
	if strings.IndexByte(path, 0) >= 0 {
		return nil, errors.New("securefile: path contains NUL")
	}
	dir, err := openDirNoFollow(path)
	if err != nil {
		return nil, err
	}
	info, err := dir.Stat()
	if err != nil {
		_ = dir.Close()
		return nil, fmt.Errorf("securefile: stat opened directory: %w", err)
	}
	if !info.IsDir() {
		_ = dir.Close()
		return nil, fmt.Errorf("securefile: path is not a directory: %s", path)
	}
	return dir, nil
}

// ProtectDirectory applies private permissions through an already validated
// directory handle: POSIX mode bits on Unix and a protected owner/SYSTEM/
// Administrators DACL on Windows.
func ProtectDirectory(path string, perm os.FileMode) error {
	dir, err := OpenDir(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return protectOpenedDirectory(dir, perm)
}

// ProtectFile restringe un fichero regular ya existente: modo POSIX mediante
// descriptor abierto y DACL protegida de usuario/SYSTEM/Administradores en
// Windows.
func ProtectFile(path string, perm os.FileMode) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("securefile: empty path")
	}
	if strings.IndexByte(path, 0) >= 0 {
		return errors.New("securefile: path contains NUL")
	}
	return protectRegularFile(path, perm)
}

// ReadFile reads a complete application-managed regular file without
// following a final-component symlink.
func ReadFile(path string) ([]byte, error) {
	file, err := OpenRead(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// ReadFileLimit reads at most maxBytes from an application-managed regular
// file. It checks the already-open descriptor and also uses a limited reader,
// so a concurrent file growth cannot turn the read into an unbounded
// allocation.
func ReadFileLimit(path string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 || maxBytes == math.MaxInt64 {
		return nil, errors.New("securefile: maximum size must be positive")
	}
	file, err := OpenRead(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("securefile: stat opened file: %w", err)
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("securefile: file exceeds maximum size of %d bytes", maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("securefile: file exceeds maximum size of %d bytes", maxBytes)
	}
	return data, nil
}

// RemoveFile removes an application-managed regular file without following a
// symlink/reparse point in the final path component. The platform
// implementation validates and removes the already selected file, rather than
// deleting a possible link target.
func RemoveFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("securefile: empty path")
	}
	if strings.IndexByte(path, 0) >= 0 {
		return errors.New("securefile: path contains NUL")
	}
	return removeRegularFileNoFollow(path)
}

// WriteFileAtomic persists an application-managed regular file without ever
// opening the destination for truncation. The temporary file is created in the
// same directory and atomically replaces a regular destination; an existing
// symlink or other special file is rejected.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) (err error) {
	if strings.TrimSpace(path) == "" {
		return errors.New("securefile: empty path")
	}
	if strings.IndexByte(path, 0) >= 0 {
		return errors.New("securefile: path contains NUL")
	}
	if info, statErr := os.Lstat(path); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("securefile: destination is not a regular file: %s", path)
		}
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("securefile: create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if err := tmp.Chmod(perm.Perm()); err != nil {
		return fmt.Errorf("securefile: restrict temporary file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("securefile: write temporary file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("securefile: sync temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("securefile: close temporary file: %w", err)
	}
	if err := atomicReplace(tmpPath, path); err != nil {
		return fmt.Errorf("securefile: replace destination: %w", err)
	}
	return nil
}
