// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11worker

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

// SoftHSM's file-backed test/device store needs narrowly scoped persistent
// data, unlike PC/SC. Only its two fixed system configuration locations are
// consulted, and only when that module was explicitly selected locally. No
// HOME, SOFTHSM2_CONF or other inherited environment override is consulted.
// The sandbox builder still validates all grants and rejects broad, unsafe or
// non-private directories. Other drivers receive only explicit local grants.
func moduleSandboxResources(module string, explicit SandboxResources) (SandboxResources, error) {
	if filepath.Base(module) != "libsofthsm2.so" {
		return explicit, nil
	}
	return softHSMFileResources([]string{"/etc/softhsm2.conf", "/etc/softhsm/softhsm2.conf"}, explicit)
}

func softHSMFileResources(configPaths []string, explicit SandboxResources) (SandboxResources, error) {
	result := SandboxResources{
		ReadOnlyFiles: append([]string(nil), explicit.ReadOnlyFiles...),
		ReadOnlyDirs:  append([]string(nil), explicit.ReadOnlyDirs...),
		ReadWriteDirs: append([]string(nil), explicit.ReadWriteDirs...),
	}
	var previous []byte
	store := ""
	for _, path := range configPaths {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			continue
		}
		resolved, err := ValidateModulePath(path)
		if err != nil {
			return SandboxResources{}, ErrSandboxPolicy
		}
		fd, err := unix.Open(resolved, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if err != nil {
			return SandboxResources{}, ErrSandboxPolicy
		}
		file := os.NewFile(uintptr(fd), "local-driver-configuration") // #nosec G115 -- unix.Open returned a non-negative descriptor.
		info, statErr := file.Stat()
		if statErr != nil || !info.Mode().IsRegular() || info.Size() > 64*1024 {
			_ = file.Close()
			return SandboxResources{}, ErrSandboxPolicy
		}
		data, readErr := io.ReadAll(io.LimitReader(file, 64*1024+1))
		_ = file.Close()
		if readErr != nil || len(data) > 64*1024 || (previous != nil && !bytes.Equal(previous, data)) {
			return SandboxResources{}, ErrSandboxPolicy
		}
		store, err = parseSoftHSMStoreDirectory(data)
		if err != nil {
			return SandboxResources{}, err
		}
		previous = data
		// Bind at the configuration path the driver will open, not a newly
		// chosen override. The builder pins the corresponding filesystem object.
		result.ReadOnlyFiles = append(result.ReadOnlyFiles, path)
	}
	if store == "" {
		return SandboxResources{}, ErrSandboxPolicy
	}
	result.ReadWriteDirs = append(result.ReadWriteDirs, store)
	return result, nil
}

func parseSoftHSMStoreDirectory(data []byte) (string, error) {
	if len(data) == 0 || len(data) > 64*1024 || bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return "", ErrSandboxPolicy
	}
	store := ""
	backendSeen := false
	for _, line := range strings.Split(string(data), "\n") {
		// SoftHSM 2.6 reads into a 1024-byte fgets buffer and tokenizes '='.
		// Reject input with a different interpretation instead of granting the
		// directory parsed by one implementation but not by the actual driver.
		if len(line) > 1022 {
			return "", ErrSandboxPolicy
		}
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 || strings.Count(line, "=") != 1 {
			return "", ErrSandboxPolicy
		}
		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		switch key {
		case "directories.tokendir":
			if store != "" || !filepath.IsAbs(value) || filepath.Clean(value) != value || len(value) > 4096 || strings.ContainsAny(value, "\r\n\"'$`") {
				return "", ErrSandboxPolicy
			}
			store = value
		case "objectstore.backend":
			if backendSeen || value != "file" {
				return "", ErrSandboxPolicy
			}
			backendSeen = true
		}
	}
	if store == "" {
		return "", ErrSandboxPolicy
	}
	return store, nil
}
